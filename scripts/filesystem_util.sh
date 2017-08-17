# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# . "${BUILD_LIBRARY_DIR}/common.sh" || exit 1

# Usage: fs_parse_option <mount_options> <option_key> [default_value]
#
# Print the value associated with the option_key in the passed mount_options,
# or the optional default_value if it wasn't specified.
#
# Args:
#   mount_options: Options that could be passed to the "mount" command, for
#       example "loop,ro,offset=1234".
#   option_key: The key you are looking for.
#   default_value: An optional default value used if the option key is not
#       found.
fs_parse_option() {
  # If we are mounting a partition inside a device by passing an offset
  # in the mount options, we need to tell unsquashfs to read from there.
  local mount_options="$1"
  local option_key="$2"
  local default_value="${3:-}"

  local option_value
  if option_value=$(echo "${mount_options}" | tr , '\n' | \
      grep -E "^${option_key}"'(=|$)'); then
    echo "${option_value}" | cut --fields=2 --delimiter== --only-delimited
  else
    echo "${default_value}"
  fi
}

# Usage: fs_mount <part_dev> <mount_point> <fs_format> [ro_rw] [mount_options]
#
# Mount the passed partition device in the mount point. The partition is mounted
# as the fs_format filesystem (if fs_format is not empty). If the filesystem
# doesn't support to be mounted as read-write, like for example squashfs or
# ubifs, and "rw" mount is requested the contents are copied instead. When
# unmounted, the contents will be copied back to the partition, but you need to
# unmount the filesystem calling fs_umount.
#
# Args:
#   part_dev: A block device with the partition to mount.
#   mount_point: A directory where to mount the filesystem.
#   fs_format: The filesystem format, such as for example "ext2" or "squashfs".
#   ro_rw: The ro_rw parameter should be "ro" or "rw" (the default if empty).
#   mount_options: Extra mount options passed to the command "mount" when used.
fs_mount() {
  local part_dev="$1"
  local mount_point="$2"
  local fs_format="$3"
  local ro_rw="${4:-rw}"
  local mount_options="${5:-}"

  if [[ "${ro_rw}" != "ro" && "${ro_rw}" != "rw" ]]; then
    die "ro_rw must be \"ro\" or \"rw\", not \"${ro_rw}\"."
  fi

  local all_options="${ro_rw}"
  [[ -n "${mount_options}" ]] && all_options="${ro_rw},${mount_options}"

  # TODO: move this to layout file.(crbug.com/710929)
  case ${fs_format} in
  btrfs) all_options+=",compress=zlib";;
  esac

  case ${fs_format} in
  ext[234]|fat12|fat16|fat32|fat|vfat|btrfs|"")
    local extra_flags=()
    [[ -n "${fs_format}" ]] && extra_flags=( -t "${fs_format}" )
    sudo mount "${part_dev}" "${mount_point}" -o "${all_options}" \
        "${extra_flags[@]}"
    ;;
  squashfs)
    if [[ "${ro_rw}" == "ro" ]]; then
      sudo mount "${part_dev}" "${mount_point}" -o "${all_options}" \
          -t "${fs_format}"
    else
      local offset=$(fs_parse_option "${mount_options}" offset 0)
      local sizelimit=$(fs_parse_option "${mount_options}" sizelimit)
      local sizelimit_arg=""
      if [[ "${offset}" != "0" || -n "${sizelimit}" ]]; then
        local losetup_opts=( --show --read-only --offset "${offset}" )
        if [[ -n "${sizelimit}" ]]; then
          losetup_opts+=( --sizelimit "${sizelimit}" )
        fi
        part_dev=$(sudo losetup "${losetup_opts[@]}" -f "${part_dev}")
      fi

      sudo unsquashfs -dest "${mount_point}" -no-progress -force "${part_dev}"

      if [[ "${offset}" != "0" ]]; then
        # Cleanup the loop device used to unsquash the filesystem.
        sudo losetup -d "${part_dev}"
      fi
    fi
    ;;
  *)
    die "Unknown fs format '${fs_format}'";;
  esac
}

# Usage: fs_umount <part_dev> <mount_point> <fs_format> <fs_options> \
#   [mount_options]
#
# Unmount the partition mounted with fs_mount.
#
# Args:
#   part_dev: The block device with the partition that was mounted.
#   mount_point: The directory where the partition was mounted.
#   fs_format: The filesystem format, such as for example "ext2" or "squashfs".
#   fs_options: The options used when creating the filesystem. These options are
#       used when we need to recreate the fs.
#   mount_options: Extra mount options passed to the command "mount" when used.
#       Only the "offset=" options is considering while unmounting.
fs_umount() {
  local part_dev="$1"
  local mount_point="$2"
  local fs_format="$3"
  local fs_options="$4"
  local mount_options="${5:-}"

  if mountpoint -q "${mount_point}"; then
    # First use safe_umount_tree for the general case. This also unmounts
    # mount points created with "mount --bind" in the filesystem.
    safe_umount_tree "${mount_point}"
    return
  fi

  case ${fs_format} in
  ext[234]|fat12|fat16|fat32|fat|vfat|"")
    # Nothing else to do for these filesystems.
    ;;
  squashfs)
    # Unmount anything else that could be mounted in the filesystem before
    # re-squashing.
    safe_umount_tree "${mount_point}"

    # Re-squash the filesystem to a temporary file.
    local squash_file="$(mktemp --suffix=.squashfs)"
    local fs_options_arr=(${fs_options})
    # If there are errors in mkquashfs they are sent to stderr, but in the
    # normal case a lot of useless information is sent to stdout.
    sudo mksquashfs "${mount_point}" "${squash_file}" -noappend \
        -no-progress -no-recovery "${fs_options_arr[@]}" >/dev/null

    local sizelimit=$(fs_parse_option "${mount_options}" sizelimit)
    local squashed_size=$(stat -c%s "${squash_file}")

    if [[ -n "${sizelimit}" && "${sizelimit}" -lt "${squashed_size}" ]]; then
      sudo rm -f "${squash_file}"
      die "The squashfs filesystem mounted at ${mount_point} is "\
"${squashed_size} bytes but the sizelimit is ${sizelimit} bytes, about "\
"$(( (squashed_size - sizelimit) / 1024 )) KiB smaller. Please increase the "\
"size of your filesystem or remove some files from it."
    fi

    local offset=$(fs_parse_option "${mount_options}" offset 0)
    # mksquashfs pads the filesystem up to 4kB, but we can use a bigger block
    # size to improve speed.
    sudo dd if="${squash_file}" of="${part_dev}" bs=8M seek="${offset}" \
        oflag=seek_bytes conv=notrunc status=none
    sudo rm -f "${squash_file}"
    ;;
  *)
    die "Unknown fs format '${fs_format}'";;
  esac
}

# Usage: fs_remove_mountpoint <mount_point>
#
# fs_umount will unmount the filesystem but will keep the mount point
# directory. When using squashfs in rw mode, the contents of the filesystem
# will remain in the mount point directory.
# This function removes the mountpoint directory as long as it is not mounted.
# Returns whether it was successfully removed.
fs_remove_mountpoint() {
  local mount_point="$1"
  safe_umount_tree "${mount_point}" || return
  if ! mountpoint -q "${mount_point}"; then
    sudo rm -rf "${mount_point}"
  fi
}
