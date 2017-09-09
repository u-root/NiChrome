#!/bin/bash
if [ -z "${GOPATH}" ]; then
	export GOPATH=/home/travis/gopath
fi
set -e

(cd usb && go build .)
./usb/usb -device=/dev/null

cpio -ivt < /tmp/initramfs.linux_amd64.cpio

