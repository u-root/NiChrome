#!/bin/bash
if [ -z "${GOPATH}" ]; then
	export GOPATH=/home/travis/gopath
fi
set -e


cpio -ivt < /tmp/initramfs.linux_amd64.cpio

