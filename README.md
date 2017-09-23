NiChrome
=======

[![Build Status](https://travis-ci.org/u-root/NiChrome.svg?branch=master)](https://travis-ci.org/u-root/NiChrome) [![Go Report Card](https://goreportcard.com/badge/github.com/u-root/NiChrome)](https://goreportcard.com/report/github.com/u-root/NiChrome) [![GoDoc](https://godoc.org/github.com/u-root/NiChrome?status.svg)](https://godoc.org/github.com/u-root/NiChrome) [![License: GPL v2](https://img.shields.io/badge/License-GPL%20v2-blue.svg)](https://github.com/u-root/NiChrome/blob/master/LICENSE)


# Description
Things we need for NiChrome.

To get an image, for both KERN-[AB] and ROOT-[AB], 

Build the usb tool: (cd usb && go build .)

Plug in a chromeos-formatted USB stick (TODO: how do we correctly do this formatting)

./usb/usb -fetch=true -dev=/dev/your-usb-stick

e.g.
./usb/usb -fetch=true -dev=/dev/sdb

usb will default to /dev/null, which makes it easy to test it. You can also run travis.sh to test.

You can skip the -fetch=true on second or later runs of usb.

This defaults to writing the A image (partitions 2 and 3). To use the B image, invoke usb with -useB=true
