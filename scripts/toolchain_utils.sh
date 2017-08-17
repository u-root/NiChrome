#!/bin/bash

# Copyright (c) 2012 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# TODO: Convert this to python.

get_all_board_toolchains()
{
  cros_setup_toolchains --show-board-cfg="$1" | sed 's:,: :g'
}

get_ctarget_from_board()
{
  local all_toolchains=( $(get_all_board_toolchains "$@") )
  echo "${all_toolchains[0]}"
}

get_board_arch()
{
  local ctarget=$(get_ctarget_from_board "$@")

  # Ask crossdev what the magical portage arch is!
  local arch=$(eval $(crossdev --show-target-cfg "${ctarget}"); echo ${arch})
  if [[ -z ${arch} ]] ; then
    error "Unable to determine ARCH from toolchain: ${ctarget}"
    return 1
  fi

  echo "${arch}"
  return 0
}

is_number()
{
  echo "$1" | grep -q "^[0-9]\+$"
}

is_config_installed()
{
  gcc-config -l | cut -d" " -f3 | grep -q "$1$"
}

get_installed_atom_from_config()
{
  local gcc_path="$(gcc-config -B "$1")"
  equery b "$gcc_path" | head -n1
}

get_atom_from_config()
{
  echo "$1" | sed -E "s|(.*)-(.*)|cross-\1/gcc-\2|g"
}

get_ctarget_from_atom()
{
  local atom="$1"
  echo "$atom" | sed -E 's|cross-([^/]+)/.*|\1|g'
}

get_current_binutils_config()
{
  local ctarget="$1"
  binutils-config -l | grep "$ctarget" | grep "*" | awk '{print $NF}'
}

get_bfd_config()
{
  local ctarget="$1"
  binutils-config -l | grep "$ctarget" | grep -v "gold" | head -n1 | \
    awk '{print $NF}'
}

emerge_gcc()
{
  local atom="$1"
  local ctarget="$(get_ctarget_from_atom $atom)"
  mask_file="/etc/portage/package.mask/cross-$ctarget"
  moved_mask_file=0

  # Move the package mask file elsewhere.
  if [[ -f "$mask_file" ]]
  then
    temp_file="$(mktemp)"
    sudo mv "$mask_file" "$temp_file"
    moved_mask_file=1
  fi

  USE+=" multislot"
  if echo "$atom" | grep -q "gcc-4.6.0$"
  then
    old_binutils_config="$(get_current_binutils_config $ctarget)"
    bfd_binutils_config="$(get_bfd_config $ctarget)"
    if [[ "$old_binutils_config" != "$bfd_binutils_config" ]]
    then
      sudo binutils-config "$bfd_binutils_config"
    fi
  fi
  sudo ACCEPT_KEYWORDS="*" USE="$USE" emerge ="$atom"
  emerge_retval=$?

  # Move the package mask file back.
  if [[ $moved_mask_file -eq 1 ]]
  then
    sudo mv "$temp_file" "$mask_file"
  fi

  if [[ ! -z "$old_binutils_config" &&
        "$old_binutils_config" != "$(get_current_binutils_config $ctarget)" ]]
  then
    sudo binutils-config "$old_binutils_config"
  fi

  return $emerge_retval
}

# This function should only be called when testing experimental toolchain
# compilers. Please don't call this from any other script.
cros_gcc_config()
{
  # Return if we're not switching profiles.
  if [[ "$1" == -* ]]
  then
    sudo gcc-config "$1"
    return $?
  fi

  # cros_gcc_config can be called like:
  # cros_gcc_config <number> to switch config to that
  # number. In that case we should just try to switch to
  # that config and not try to install a missing one.
  if ! is_number "$1" && ! is_config_installed "$1"
  then
    info "Configuration $1 not found."
    info "Trying to emerge it..."
    local atom="$(get_atom_from_config "$1")"
    emerge_gcc "$atom" || die "Could not install $atom"
  fi

  sudo gcc-config "$1" || die "Could not switch to $1"

  local boards=$(get_boards_from_config "$1")
  local board
  for board in $boards
  do
    emerge-"$board" --oneshot sys-devel/libtool
  done
}

get_boards_from_config()
{
  local atom=$(get_installed_atom_from_config "$1")
  if [[ $atom != cross* ]]
  then
    warn "$atom is not a cross-compiler."
    warn "Therefore not adding its libs to the board roots."
    return 0
  fi

  # Now copy the lib files into all possible boards.
  local ctarget="$(get_ctarget_from_atom "$atom")"
  for board_root in /build/*
  do
    local board="$(basename $board_root)"
    local board_tc="$(get_ctarget_from_board $board)"
    if [[ "${board_tc}" == "${ctarget}" ]]
    then
      echo "$board"
    fi
  done
}
