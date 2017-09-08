#!/bin/bash
export GOPATH=/home/travis/gopath
set -e

go run newscript.go -device=/dev/null

cpio -ivt < /tmp/initramfs.linux_amd64.cpio

