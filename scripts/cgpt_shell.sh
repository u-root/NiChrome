#!/bin/sh
# Copyright (c) 2012 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This script is automatically generated by @SCRIPT_GENERATOR@.
# Do not edit!

if ! type numsectors >/dev/null 2>&1; then
  . "./chromeos-common.sh" || exit 1
fi
locate_gpt

# Usage: create_image <device> <min_disk_size> <block_size>
# If <device> is a block device, wipes out the GPT
# If it's not, it creates a new file of the requested size
create_image() {
  local dev="$1"
  local min_disk_size="$2"
  local block_size="$3"
  if [ -b "${dev}" ]; then
    # Zap any old partitions (otherwise gpt complains).
    dd if=/dev/zero of="${dev}" conv=notrunc bs=512 count=32
    dd if=/dev/zero of="${dev}" conv=notrunc bs=512 \
      seek=$(( min_disk_size - 1 - 33 )) count=33
  else
    if [ ! -e "${dev}" ]; then
      dd if=/dev/zero of="${dev}" bs="${block_size}" count=1 \
        seek=$(( min_disk_size - 1 ))
    fi
  fi
}

