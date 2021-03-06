// Package edgeos provides methods and structures to retrieve, parse and render EdgeOS configuration data and files.
package edgeos

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/britannic/blacklist/internal/regx"
)

// bNodes is a map of leaf nodes
type tree map[string]*object

// ConfLoader interface defines configuration load method
type ConfLoader interface {
	read() io.Reader
}

// CFile holds an array of file names
type CFile struct {
	*Parms
	names []string
	nType ntype
}

// Config is a struct of configuration fields
type Config struct {
	*Parms
	tree
}

const (
	agent     = `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_6) AppleWebKit/601.7.7 (KHTML, like Gecko) Version/9.1.2 Safari/601.7.7`
	all       = "all"
	blackhole = "dns-redirect-ip"
	dbg       = false
	disabled  = "disabled"
	domains   = "domains"
	files     = "file"
	hosts     = "hosts"
	notknown  = "unknown"
	preNoun   = "pre-configured"
	rootNode  = "blacklist"
	src       = "source"
	urls      = "url"
	zones     = "zones"

	// ExcDomns labels domain exclusions
	ExcDomns = "domn-excludes"
	// ExcHosts labels host exclusions
	ExcHosts = "host-excludes"
	// ExcRoots labels global domain exclusions
	ExcRoots = "root-excludes"
	// PreDomns designates string label for preconfigured blacklisted domains
	PreDomns = preNoun + "-domain"
	// PreHosts designates string label for preconfigured blacklisted hosts
	PreHosts = preNoun + "-host"
	// False is a string constant
	False = "false"
	// True is a string constant
	True = "true"
)

func (c *Config) addExc(node string) *Objects {
	var (
		ltype string
		o     = &Objects{Parms: c.Parms}
	)

	switch node {
	case domains:
		ltype = ExcDomns
	case hosts:
		ltype = ExcHosts
	case rootNode:
		ltype = ExcRoots
	}

	o.x = append(o.x, &object{
		desc:  ltype + " exclusions",
		exc:   c.tree[node].exc,
		ip:    c.tree.getIP(node),
		ltype: ltype,
		name:  ltype,
		nType: getType(ltype).(ntype),
		Parms: c.Parms,
	})
	return o
}

func (c *Config) addInc(node string) *object {
	var (
		inc   = c.tree[node].inc
		ltype string
		n     ntype
	)

	if len(inc) > 0 {
		switch node {
		case domains:
			ltype = getType(preDomn).(string)
			n = getType(ltype).(ntype)
		case hosts:
			ltype = getType(preHost).(string)
			n = getType(ltype).(ntype)
		}

		return &object{
			desc:  ltype + " blacklist content",
			inc:   inc,
			ip:    c.tree.getIP(node),
			ltype: ltype,
			name:  fmt.Sprintf("includes.[%v]", len(inc)),
			nType: n,
			Parms: c.Parms,
		}
	}
	return nil
}

// NewContent returns an interface of the requested IFace type
func (c *Config) NewContent(iface IFace) (Contenter, error) {
	var (
		err   error
		ltype = iface.String()
		o     *Objects
	)

	switch ltype {
	case ExcDomns:
		o = c.addExc(domains)
	case ExcHosts:
		o = c.addExc(hosts)
	case ExcRoots:
		o = c.addExc(rootNode)
	case urls:
		switch iface {
		case URLdObj:
			o = c.Get(domains).Filter(urls)
			return &URLDomnObjects{Objects: o}, nil
		case URLhObj:
			o = c.Get(hosts).Filter(urls)
			return &URLHostObjects{Objects: o}, nil
		}
	case "unknown":
		err = errors.New("Invalid interface requested")
	default:
		o = c.GetAll(ltype)
	}

	switch iface {
	case ExDmObj:
		return &ExcDomnObjects{Objects: o}, nil
	case ExHtObj:
		return &ExcHostObjects{Objects: o}, nil
	case ExRtObj:
		return &ExcRootObjects{Objects: o}, nil
	case FileObj:
		return &FIODataObjects{Objects: o}, nil
	case PreDObj:
		return &PreDomnObjects{Objects: o}, nil
	case PreHObj:
		return &PreHostObjects{Objects: o}, nil
	}

	return nil, err
}

// excludes returns a string array of excludes
func (c *Config) excludes(nodes ...string) list {
	var exc []string
	switch nodes {
	case nil:
		for _, k := range c.Nodes() {
			if len(c.tree[k].exc) != 0 {
				exc = append(exc, c.tree[k].exc...)
			}
		}
	default:
		for _, node := range nodes {
			exc = append(exc, c.tree[node].exc...)
		}
	}
	return updateEntry(exc)
}

// Get returns an *Object for a given node
func (c *Config) Get(node string) *Objects {
	o := &Objects{Parms: c.Parms, x: []*object{}}

	switch node {
	case all:
		for _, node := range c.Parms.Nodes {
			o.addObj(c, node)
		}
	default:
		o.addObj(c, node)
	}
	return o
}

// GetAll returns an array of Objects
func (c *Config) GetAll(ltypes ...string) *Objects {
	var (
		newDomns = true
		newHosts = true
		o        = &Objects{Parms: c.Parms}
	)

	for _, node := range o.Nodes {
		switch ltypes {
		case nil:
			o.addObj(c, node)
		default:
			for _, ltype := range ltypes {
				switch ltype {
				case PreDomns:
					if newDomns && node == domains {
						o.x = append(o.x, c.addInc(node))
						newDomns = false
					}
				case PreHosts:
					if newHosts && node == hosts {
						o.x = append(o.x, c.addInc(node))
						newHosts = false
					}
				default:
					obj := c.validate(node).x
					for i := range obj {
						if obj[i].ltype == ltype {
							o.x = append(o.x, obj[i])
						}
					}
				}
			}
		}
	}
	return o
}

// InSession returns true if VyOS/EdgeOS configuration is in session
func (c *Config) InSession() bool {
	return os.ExpandEnv("$_OFR_CONFIGURE") == "ok"
}

// load reads the config using the EdgeOS/VyOS cli-shell-api
func (c *Config) load(act, lvl string) ([]byte, error) {
	cmd := exec.Command(c.Bash)
	cmd.Stdin = strings.NewReader(fmt.Sprintf("%v %v %v", c.API, apiCMD(act, c.InSession()), lvl))

	return cmd.Output()
}

// Nodes returns an array of configured nodes
func (c *Config) Nodes() (nodes []string) {
	for k := range c.tree {
		nodes = append(nodes, k)
	}
	sort.Strings(nodes)

	return nodes
}

// ReadCfg extracts nodes from a EdgeOS/VyOS configuration structure
func (c *Config) ReadCfg(r ConfLoader) error {
	var (
		tnode string
		b     = bufio.NewScanner(r.read())
		leaf  string
		nodes []string
		rx    = regx.Obj
		o     *object
	)

LINE:
	for b.Scan() {
		line := bytes.TrimSpace(b.Bytes())

		switch {
		case rx.MLTI.Match(line):
			incExc := regx.Get([]byte("mlti"), line)
			switch string(incExc[1]) {
			case "exclude":
				c.tree[tnode].exc = append(c.tree[tnode].exc, string(incExc[2]))

			case "include":
				c.tree[tnode].inc = append(c.tree[tnode].inc, string(incExc[2]))
			}

		case rx.NODE.Match(line):
			node := regx.Get([]byte("node"), line)
			tnode = string(node[1])
			nodes = append(nodes, tnode)
			c.tree[tnode] = newObject()

		case rx.LEAF.Match(line):
			srcName := regx.Get([]byte("leaf"), line)
			leaf = string(srcName[2])
			nodes = append(nodes, string(srcName[1]))

			if bytes.Equal(srcName[1], []byte(src)) {
				o = newObject()
				o.name = leaf
				o.nType = getType(tnode).(ntype)
			}

		case rx.DSBL.Match(line):
			c.tree[tnode].disabled = strToBool(string(regx.Get([]byte("dsbl"), line)[1]))

		case rx.IPBH.Match(line) && nodes[len(nodes)-1] != src:
			c.tree[tnode].ip = string(regx.Get([]byte("ipbh"), line)[1])

		case rx.NAME.Match(line):
			name := regx.Get([]byte("name"), line)
			switch string(name[1]) {
			case "description":
				o.desc = string(name[2])

			case blackhole:
				o.ip = string(name[2])

			case files:
				o.file = string(name[2])
				o.ltype = string(name[1])
				c.tree[tnode].Objects.x = append(c.tree[tnode].Objects.x, o)

			case "prefix":
				o.prefix = string(name[2])

			case urls:
				o.ltype = string(name[1])
				o.url = string(name[2])
				c.tree[tnode].Objects.x = append(c.tree[tnode].Objects.x, o)
			}

		case rx.DESC.Match(line) || rx.CMNT.Match(line) || rx.MISC.Match(line):
			continue LINE

		case rx.RBRC.Match(line):
			if len(nodes) > 1 {
				nodes = nodes[:len(nodes)-1] // pop last node
				tnode = nodes[len(nodes)-1]
			}
		}
	}

	if len(c.tree) < 1 {
		return errors.New("Configuration data is empty, cannot continue")
	}

	return nil
}

// readDir returns a listing of dnsmasq blacklist configuration files
func (c *CFile) readDir(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// ReloadDNS reloads the dnsmasq configuration
func (c *Config) ReloadDNS() ([]byte, error) {
	cmd := exec.Command(c.Bash)
	cmd.Stdin = strings.NewReader(c.DNSsvc)

	return cmd.CombinedOutput()
}

// Remove deletes a CFile array of file names
func (c *CFile) Remove() error {
	d, err := c.readDir(fmt.Sprintf(c.FnFmt, c.Dir, c.Wildcard.Node, c.Wildcard.Name, c.Parms.Ext))
	if err != nil {
		return err
	}

	return purgeFiles(diffArray(c.names, d))
}

// sortKeys returns a slice of keys in lexicographical sorted order.
func (c *Config) sortKeys() (pkeys sort.StringSlice) {
	pkeys = make(sort.StringSlice, len(c.tree))
	i := 0
	for k := range c.tree {
		pkeys[i] = k
		i++
	}
	pkeys.Sort()

	return pkeys
}

// String returns pretty print for the Blacklist struct
func (c *Config) String() (s string) {
	indent := 1
	cmma := comma
	cnt := len(c.sortKeys())
	s += fmt.Sprintf("{\n%v%q: [{\n", tabs(indent), "nodes")

	for i, pkey := range c.sortKeys() {
		if i == cnt-1 {
			cmma = null
		}

		indent++
		s += fmt.Sprintf("%v%q: {\n", tabs(indent), pkey)
		indent++

		s += fmt.Sprintf("%v%q: %q,\n", tabs(indent), disabled,
			booltoStr(c.tree[pkey].disabled))
		s = is(indent, s, "ip", c.tree[pkey].ip)
		s += getJSONArray(&cfgJSON{array: c.tree[pkey].exc, pk: pkey, leaf: "excludes", indent: indent})

		if pkey != rootNode {
			s += getJSONArray(&cfgJSON{array: c.tree[pkey].inc, pk: pkey, leaf: "includes", indent: indent})
			s += getJSONsrcArray(&cfgJSON{Config: c, pk: pkey, indent: indent})
		}

		indent--
		s += fmt.Sprintf("%v}%v\n", tabs(indent), cmma)
		indent--
	}

	s += tabs(indent) + "}]\n}"

	return s
}

// String implements string method
func (c *CFile) String() string {
	return strings.Join(c.names, "\n")
}

// Strings returns a sorted array of strings.
func (c *CFile) Strings() []string {
	sort.Strings(c.names)
	return c.names
}

// LTypes returns an array of configured nodes
func (c *Config) LTypes() []string {
	return c.Parms.Ltypes
}

func (b tree) getIP(node string) (ip string) {
	switch b[node].ip {
	case "":
		ip = b[rootNode].ip
	default:
		ip = b[node].ip
	}
	return ip
}

func (b tree) validate(node string) *Objects {
	for _, obj := range b[node].Objects.x {
		if obj.ip == "" {
			obj.ip = b.getIP(node)
		}
	}
	return &b[node].Objects
}
