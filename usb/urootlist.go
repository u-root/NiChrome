// +build go1.9

package main

var (
	staticCmdList = []string{
		"github.com/u-root/NiChrome/cmds/install",
		"github.com/u-root/NiChrome/cmds/uinit",
	}
	dynamicCmdList = append(staticCmdList, []string{
		"github.com/u-root/NiChrome/cmds/install",
		"github.com/u-root/NiChrome/cmds/uinit",
		"github.com/u-root/wingo",
		"github.com/nsf/godit",
		"upspin.io/cmd/upspin",
		"upspin.io/cmd/upspinfs"}...)
)
