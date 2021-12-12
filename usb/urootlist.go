// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.11

package main

var (
	staticCmdList = []string{
		"./cmds/install",
		"./cmds/uinit",
	}
	dynamicCmdList = append(staticCmdList, []string{
		"./cmds/install",
		"./cmds/uinit",
		/*
		"../wingo",
		"github.com/nsf/godit",
		"upspin.io/cmd/upspin",
		"upspin.io/cmd/upspinfs"}...
		*/
		}...
	)
)
