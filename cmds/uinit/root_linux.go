// Copyright 2014-2017 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package util contains various u-root utility functions.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"syscall"

	"github.com/u-root/u-root/pkg/ulog"
	"golang.org/x/sys/unix"
)

const (
	// Not all these paths may be populated or even exist but OTOH they might.
	PATHHEAD = "/ubin"
	PATHMID  = "/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/bin:/usr/local/sbin"
	PATHTAIL = "/buildbin:/bbin"
)

type Creator interface {
	Create() error
	fmt.Stringer
}

type Dir struct {
	Name string
	Mode os.FileMode
}

func (d Dir) Create() error {
	return os.MkdirAll(d.Name, d.Mode)
}

func (d Dir) String() string {
	return fmt.Sprintf("dir %q (mode %#o)", d.Name, d.Mode)
}

type File struct {
	Name     string
	Contents string
	Mode     os.FileMode
}

func (f File) Create() error {
	return ioutil.WriteFile(f.Name, []byte(f.Contents), f.Mode)
}

func (f File) String() string {
	return fmt.Sprintf("file %q (mode %#o)", f.Name, f.Mode)
}

type Symlink struct {
	Target  string
	NewPath string
}

func (s Symlink) Create() error {
	os.Remove(s.NewPath)
	return os.Symlink(s.Target, s.NewPath)
}

func (s Symlink) String() string {
	return fmt.Sprintf("symlink %q -> %q", s.NewPath, s.Target)
}

type Link struct {
	OldPath string
	NewPath string
}

func (s Link) Create() error {
	os.Remove(s.NewPath)
	return os.Link(s.OldPath, s.NewPath)
}

func (s Link) String() string {
	return fmt.Sprintf("link %q -> %q", s.NewPath, s.OldPath)
}

type Dev struct {
	Name string
	Mode uint32
	Dev  int
}

func (d Dev) Create() error {
	os.Remove(d.Name)
	return syscall.Mknod(d.Name, d.Mode, d.Dev)
}

func (d Dev) String() string {
	return fmt.Sprintf("dev %q (mode %#o; magic %d)", d.Name, d.Mode, d.Dev)
}

type Mount struct {
	Source string
	Target string
	FSType string
	Flags  uintptr
	Opts   string
}

func (m Mount) Create() error {
	return syscall.Mount(m.Source, m.Target, m.FSType, m.Flags, m.Opts)
}

func (m Mount) String() string {
	return fmt.Sprintf("mount -t %q -o %s %q %q flags %#x", m.FSType, m.Opts, m.Source, m.Target, m.Flags)
}

func GoBin() string {
	return fmt.Sprintf("/go/bin/%s_%s:/go/bin:/go/pkg/tool/%s_%s", runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH)
}

func create(namespace []Creator) {
	// Clear umask bits so that we get stuff like ptmx right.
	m := unix.Umask(0)
	defer unix.Umask(m)
	for _, c := range namespace {
		if err := c.Create(); err != nil {
			ulog.KernelLog.Printf("u-root init: error creating %s: %v", c, err)
		}
	}
}
