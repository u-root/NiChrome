#!/bin/bash
if [ -z "${GOPATH}" ]; then
	export GOPATH=/home/travis/gopath
fi
set -e

echo "Check vendored dependencies"
(dep status)

(cd usb && go build .)
./usb/usb -fetch=true -dev=/dev/null

cpio -ivt < /tmp/initramfs.linux_amd64.cpio

