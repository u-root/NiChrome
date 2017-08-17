# Copyright (c) 2011 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This global array variable is used to remember options from
# mount_image so that unmount_image can do its job.
MOUNT_GPT_OPTIONS=( )

# Usage: _check_mount_image_flags [mode_flag ...]
#
# Check that all the passed flags only modify the read or write access of the
# mounted image when passed to mount_gpt_image.sh.
# Args:
#   mode_flag ...: 0 or more flags.
_check_mount_image_flags() {
  local flag
  for flag; do
    case "${flag}" in
      --read_only|--safe) ;;
      *)
        die "Invalid flag '${flag}'."
    esac
  done
}

# Usage: mount_image image rootfs_mountpt stateful_mountpt esp_mountpt \
#   [esp_mount] [mode_flag]
#
# Mount the root, stateful, and optionally ESP partitions in a Chromium OS
# image. Create the mountpoints when needed.
# Args:
#  image: path to image to be mounted.
#  rootfs_mountpt: path to root fs mount point.
#  stateful_mountpt: path to stateful fs mount point.
#  esp_mount: optional path to ESP fs mount point; if empty the ESP will not be
#    mounted.
#  mode_flag: Optional flag to control how the image is mounted:
#    --read-only mounts all partitions as read-only.
#    --safe, mounts only the rootfs as read-only and the rest as read/write.
#    If omitted, the image will be mounted read-write. See mount_gpt_image.sh
#    for details on these flags, and remount_image to modify this flag after it
#    is mounted.
mount_image() {
  MOUNT_GPT_OPTIONS=(
    --from "$1"
    --rootfs_mountpt "$2"
    --stateful_mountpt "$3"
  )

  if [[ -n "${4:-}" ]]; then
    MOUNT_GPT_OPTIONS+=( --esp_mountpt "$4" )
  fi
  # Bash returns an empty array for mode_flag if $@ has less than 5 elements.
  local mode_flag=( "${@:5:1}" )
  _check_mount_image_flags "${mode_flag[@]:+${mode_flag[@]}}"

  "./mount_gpt_image.sh" "${MOUNT_GPT_OPTIONS[@]}" \
    "${mode_flag[@]:+${mode_flag[@]}}"
}

# Usage: remount_image [mode_flag]
#
# Remount the file systems mounted in the previous call to mount_image with the
# passed extra flags.
# Args:
#   mode_flag: Optional flag to control how the image is mounted. See
#     mount_image for details.
remount_image() {
  _check_mount_image_flags "$@"
  "./mount_gpt_image.sh" --remount "${MOUNT_GPT_OPTIONS[@]}" "$@"
}

# Usage: unmount_image
# Unmount the file systems mounted in the previous call to mount_image. The
# mountpoint directories will be removed, regardless of whether or not they
# existed when mount_image was called.
# No arguments.
unmount_image() {
  if [[ ${#MOUNT_GPT_OPTIONS[@]} -eq 0 ]]; then
    warn "Image already unmounted."
    return 1
  fi
  "./mount_gpt_image.sh" --unmount "${MOUNT_GPT_OPTIONS[@]}" \
    --delete_mountpts

  MOUNT_GPT_OPTIONS=( )
}

# Usage: get_board_from_image [image]
#
# Echoes the board name specified in the image.
get_board_from_image() {
  local image=$1
  local dir="$(mktemp -d)"
  local rootfs="${dir}/rootfs"
  local stateful="${dir}/stateful"
  (
    trap "unmount_image > /dev/null; rm -rf ${dir}" EXIT
    mount_image "${image}" "${rootfs}" "${stateful}" > /dev/null
    echo "$(sed -n '/^CHROMEOS_RELEASE_BOARD=/s:^[^=]*=::p' \
      "${rootfs}/etc/lsb-release")"
  )
}
