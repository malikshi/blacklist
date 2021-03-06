package edgeos

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"
	"time"

	logging "github.com/op/go-logging"
	. "github.com/smartystreets/goconvey/convey"
)

func TestParmsLog(t *testing.T) {
	Convey("Testing log()", t, func() {
		tests := []struct {
			dbug bool
			name string
			str  string
			verb bool
		}{
			{name: "Info", str: "This is a log.Info test", dbug: false, verb: true},
			{name: "Debug", str: "This is a log.Debug test", dbug: true, verb: false},
			{name: "Debug & Info", str: "This is both a log.Debug and log.Info test ", dbug: true, verb: true},
			{name: "Both Debug or Info", str: "This is both a log.Debug and log.Info test and there should be output", dbug: true, verb: true},
			{name: "Neither Debug or Info", str: "This is both a log.Debug and log.Info test and there shouldn't be any output", dbug: false, verb: false},
		}

		var (
			scrFmt = logging.MustStringFormatter(`%{level:.4s}[%{id:03x}] ▶ %{message}`)

			act      = &bytes.Buffer{}
			p        = &Parms{Logger: logging.MustGetLogger("TestParmsLog"), Verb: true}
			scr      = logging.NewLogBackend(act, "", 0)
			scrFmttr = logging.NewBackendFormatter(scr, scrFmt)
		)

		logging.SetBackend(scrFmttr)

		for i, tt := range tests {
			Convey("Testing "+tt.name, func() {
				p.Dbug = tt.dbug
				p.Verb = tt.verb

				switch {
				case tt.dbug:
					p.debug(tt.str)
					So(act.String(), ShouldEqual, fmt.Sprintf("DEBU[00%d] ▶ %v\n", i+1, tt.str))
				case tt.verb:
					p.log(tt.str)
					So(act.String(), ShouldEqual, fmt.Sprintf("INFO[00%d] ▶ %v\n", i+1, tt.str))
				case tt.dbug && tt.verb:
					exp := fmt.Sprintf("DEBU[00%d] ▶ %v\n", i+1, tt.str)
					exp += fmt.Sprintf("INFO[00%d] ▶ %v\n", i+1, tt.str)
					p.debug(tt.str)
					p.log(tt.str)
					So(act.String(), ShouldEqual, exp)
				default:
					p.debug(tt.str)
					p.log(tt.str)
					So(act.String(), ShouldEqual, "")
				}
				act.Reset()
			})
		}
	})
}

func TestOption(t *testing.T) {
	Convey("Testing Option()", t, func() {
		vanilla := Parms{}

		exp := `{
	"API": "/bin/cli-shell-api",
	"Arch": "amd64",
	"Bash": "/bin/bash",
	"Cores": 2,
	"Dbug": true,
	"Dex": {
		"entry": {}
	},
	"Dir": "/tmp",
	"dnsmasq service": "service dnsmasq restart",
	"Exc": {
		"entry": {}
	},
	"dnsmasq fileExt.": "blacklist.conf",
	"File": "/config/config.boot",
	"File name fmt": "%v/%v.%v.%v",
	"CLI Path": "service dns forwarding",
	"Leaf nodes": [
		"file",
		"pre-configured-domain",
		"pre-configured-host",
		"url"
	],
	"HTTP method": "GET",
	"Nodes": [
		"domains",
		"hosts"
	],
	"Prefix": "address=",
	"Poll": 10,
	"Test": true,
	"Timeout": 30000000000,
	"Verbosity": false,
	"Wildcard": {}
}`

		expRaw := Parms{
			API:      "/bin/cli-shell-api",
			Arch:     "amd64",
			Bash:     "/bin/bash",
			Cores:    2,
			Dbug:     true,
			Dex:      list{entry: entry{}},
			Dir:      "/tmp",
			DNSsvc:   "service dnsmasq restart",
			Exc:      list{entry: entry{}},
			Ext:      "blacklist.conf",
			File:     "/config/config.boot",
			FnFmt:    "%v/%v.%v.%v",
			InCLI:    "inSession",
			Level:    "service dns forwarding",
			Ltypes:   []string{files, PreDomns, PreHosts, urls},
			Method:   "GET",
			Nodes:    []string{domains, hosts},
			Pfx:      "address=",
			Poll:     10,
			Test:     true,
			Timeout:  30000000000,
			Wildcard: Wildcard{Node: "*s", Name: "*"},
		}

		c := NewConfig()
		vanilla.Dex = c.Dex
		vanilla.Exc = c.Exc
		So(c.Parms, ShouldResemble, &vanilla)

		c = NewConfig(
			Arch(runtime.GOARCH),
			API("/bin/cli-shell-api"),
			Bash("/bin/bash"),
			Cores(2),
			Dbug(true),
			Dir("/tmp"),
			DNSsvc("service dnsmasq restart"),
			Ext("blacklist.conf"),
			File("/config/config.boot"),
			FileNameFmt("%v/%v.%v.%v"),
			InCLI("inSession"),
			Logger(nil),
			Method("GET"),
			Nodes([]string{domains, hosts}),
			Poll(10),
			Prefix("address="),
			Level("service dns forwarding"),
			LTypes([]string{"file", PreDomns, PreHosts, urls}),
			Test(true),
			Timeout(30*time.Second),
			Verb(false),
			WCard(Wildcard{Node: "*s", Name: "*"}),
			Writer(nil),
		)

		expRaw.Dex.RWMutex = c.Dex.RWMutex
		expRaw.Exc.RWMutex = c.Exc.RWMutex

		So(*c.Parms, ShouldResemble, expRaw)
		So(c.Parms.String(), ShouldEqual, exp)
	})
}
