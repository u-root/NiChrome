#!/bin/bash
if [ -z "${GOPATH}" ]; then
	export GOPATH=/home/travis/gopath
fi
set -e

#echo "Check vendored dependencies"
#(dep status)

(cd usb && go build .)
./usb/usb --apt=true --fetch=true --dev=/dev/null

# in case of emergency break glass
# cpio -ivt < initramfs.linux_amd64.cpio
# cpio -ivt < linux-stabld/initramfs.linux_amd64.cpio
