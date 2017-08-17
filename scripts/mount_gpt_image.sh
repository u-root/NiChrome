#!/bin/bash

# Copyright (c) 2012 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Helper script that mounts chromium os image from a device or directory
# and creates mount points for /var and /usr/local (if in dev_mode).

# Helper scripts should be run from the same location as this script.
SCRIPT_ROOT=$(dirname "$(readlink -f "$0")")
. "common.sh" || exit 1
. "filesystem_util.sh" || exit 1
. "disk_layout_util.sh" || exit 1

# Load functions and constants for chromeos-install
. "chromeos-common.sh" || exit 1

locate_gpt

# Default value for FLAGS_image.
DEFAULT_IMAGE="chromiumos_image.bin"

# Flags.
DEFINE_string board "$DEFAULT_BOARD" \
  "The board for which the image was built." b
DEFINE_boolean read_only ${FLAGS_FALSE} \
  "Mount in read only mode -- skips stateful items."
DEFINE_boolean safe ${FLAGS_FALSE} \
  "Mount rootfs in read only mode."
DEFINE_boolean unmount ${FLAGS_FALSE} \
  "Unmount previously mounted image. Can't be used with --remount." u
DEFINE_boolean remount ${FLAGS_FALSE} \
  "Remount a previously mounted image. This is useful when changing the "\
"--read_only and --safe settings. Can't be used with --unmount."
DEFINE_string from "" \
  "Directory, image, or device with image on it" f
DEFINE_string image "${DEFAULT_IMAGE}" \
  "Name of the bin file if a directory is specified in the from flag" i
DEFINE_string partition_script "partition_script.sh" \
  "Name of the script with the partition layout if a directory is specified"
DEFINE_string rootfs_mountpt "/tmp/m" "Mount point for rootfs" r
DEFINE_string stateful_mountpt "/tmp/s" \
  "Mount point for stateful partition" s
DEFINE_string esp_mountpt "" \
  "Mount point for esp partition" e
DEFINE_boolean delete_mountpts ${FLAGS_FALSE} \
  "Delete the mountpoint directories when unmounting."
DEFINE_boolean most_recent ${FLAGS_FALSE} "Use the most recent image dir" m

# Parse flags
FLAGS "$@" || exit 1
eval set -- "${FLAGS_ARGV}"

# Die on error
switch_to_strict_mode

# Find the last image built on the board.
if [[ ${FLAGS_most_recent} -eq ${FLAGS_TRUE} ]] ; then
  FLAGS_from="$(${SCRIPT_ROOT}/get_latest_image.sh --board="${FLAGS_board}")"
fi

# Check for conflicting args.
if [[ ${FLAGS_unmount} -eq ${FLAGS_TRUE} &&
      ${FLAGS_remount} -eq ${FLAGS_TRUE} ]]; then
  die_notrace "Can't use --unmount with --remount."
fi

# If --from is a block device, --image can't also be specified.
if [[ -b "${FLAGS_from}" ]]; then
  if [[ "${FLAGS_image}" != "${DEFAULT_IMAGE}" ]]; then
    die_notrace "-i ${FLAGS_image} can't be used with block device ${FLAGS_from}"
  fi
fi

# Allow --from /foo/file.bin
if [[ -f "${FLAGS_from}" ]]; then
  # If --from is specified as a file, --image cannot be also specified.
  if [[ "${FLAGS_image}" != "${DEFAULT_IMAGE}" ]]; then
    die_notrace "-i ${FLAGS_image} can't be used with --from file ${FLAGS_from}"
  fi
  # The order is important here. We want to override FLAGS_image before
  # destroying FLAGS_from.
  FLAGS_image="$(basename "${FLAGS_from}")"
  FLAGS_from="$(dirname "${FLAGS_from}")"
fi

# Usage: get_partition_size <filename> <part_num>
#
# Print the partition size for the given partition number by looking either
# at the partition_script values (if loaded) or the GPT in the passed image.
#
# This function fails (returns 1) if the partition is empty, not present in the
# GPT image or failed to get the size for any other reason.
get_partition_size() {
  local filename="$1"
  local part_num="$2"
  if [[ -z "${part_num}" ]]; then
      error "Skipping blank partition number."
      return 1
  fi
  local part_size var_name

  var_name="PARTITION_SIZE_${part_num}"
  part_size="${!var_name}"
  if [[ -z "${part_size}" ]]; then
    part_size=$(partsize "${filename}" ${part_num}) || true
    if [[ -z "${part_size}" ]]; then
      info "Skipping unknown partition ${part_num}."
      return 1
    fi
  fi
  if [[ ${part_size} -eq 0 ]]; then
    info "Skipping empty partition ${part_num}."
    return 1
  fi
  echo "${part_size}"
  return 0
}

load_image_partition_numbers() {
  local partition_script="${FLAGS_from}/${FLAGS_partition_script}"
  # Attempt to load the partition script from the rootfs when not found in the
  # FLAGS_from directory.
  if [[ ! -f "${partition_script}" ]]; then
    partition_script="${FLAGS_rootfs_mountpt}/${PARTITION_SCRIPT_PATH}"
  fi
  if [[ -f "${partition_script}" ]]; then
    . "${partition_script}"
    load_partition_vars
    return
  fi

  # Without a partition script, infer numbers from the payload image.
  local image
  if [[ -b "${FLAGS_from}" ]]; then
    image="${FLAGS_from}"
  else
    image="${FLAGS_from}/${FLAGS_image}"
    if [[ ! -f "${image}" ]]; then
      die "Image ${image} does not exist."
    fi
  fi
  PARTITION_NUM_STATE="$(get_image_partition_number "${image}" "STATE")"
  PARTITION_NUM_ROOT_A="$(get_image_partition_number "${image}" "ROOT-A")"
  PARTITION_NUM_OEM="$(get_image_partition_number "${image}" "OEM")"
  PARTITION_NUM_EFI_SYSTEM="$(get_image_partition_number "${image}" \
    "EFI-SYSTEM")"
}

# Common unmounts for either a device or directory
unmount_image() {
  info "Unmounting image from ${FLAGS_stateful_mountpt}" \
      "and ${FLAGS_rootfs_mountpt}"
  # Don't die on error to force cleanup
  set +e
  # Reset symlinks in /usr/local.
  if mount | egrep -q ".* ${FLAGS_stateful_mountpt} .*\(rw,"; then
    setup_symlinks_on_root "." "/var" "${FLAGS_stateful_mountpt}"
    fix_broken_symlinks "${FLAGS_rootfs_mountpt}"
  fi

  local filename
  if [[ -b "${FLAGS_from}" ]]; then
    filename="${FLAGS_from}"
  else
    filename="${FLAGS_from}/${FLAGS_image}"
    if [[ ! -f "${filename}" ]]; then
      if [[ "${FLAGS_image}" == "${DEFAULT_IMAGE}" ]]; then
        warn "Umount called without passing the image. Some filesystems can't" \
          "be unmounted in this way."
        filename=""
      else
        die "Image ${filename} does not exist."
      fi
    fi
  fi

  # Unmount in reverse order: EFI, OEM, stateful and rootfs.
  local var_name mountpoint fs_format fs_options
  local part_label part_num part_offset part_size data_size
  for part_label in EFI_SYSTEM OEM STATE ROOT_A; do
    var_name="${part_label}_MOUNTPOINT"
    mountpoint="${!var_name}"
    [[ -n "${mountpoint}" ]] || continue
    var_name="PARTITION_NUM_${part_label}"
    part_num="${!var_name}"
    [[ -n "${part_num}" ]] || continue

    part_size=$(get_partition_size "${filename}" "${part_num}") || continue

    if [[ -z "${filename}" ]]; then
      # TODO(deymo): Remove this legacy umount.
      if ! mountpoint -q "${mountpoint}"; then
        die "You must pass --image or --from when using --unmount to unmount" \
          "this image."
      fi
      safe_umount_tree "${mountpoint}"
      continue
    fi

    part_offset=$(partoffset "${filename}" ${part_num}) || \
      die "Failed to get partition offset for partition ${part_num}"
    # Get the variables loaded with load_partition_vars during mount_*.
    var_name="FS_FORMAT_${part_num}"
    fs_format="${!var_name}"
    var_name="FS_OPTIONS_${part_num}"
    fs_options="${!var_name}"
    var_name="DATA_SIZE_${part_num}"
    data_size="${!var_name}"

    mount_options="offset=$(( part_offset * 512 ))"
    if [[ -n "${data_size}" ]]; then
      mount_options+=",sizelimit=${data_size}"
    fi
    fs_umount "${filename}" "${mountpoint}" "${fs_format}" "${fs_options}" \
      "${mount_options}"
  done

  # We need to remove the mountpoints after we unmount all the partitions since
  # there could be nested mounts.
  if [[ ${FLAGS_delete_mountpts} -eq ${FLAGS_TRUE} ]]; then
    for part_label in EFI_SYSTEM OEM STATE ROOT_A; do
      var_name="${part_label}_MOUNTPOINT"
      mountpoint="${!var_name}"
      # Check this is a directory.
      [[ -n "${mountpoint}" && -d "${mountpoint}" ]] || continue
      fs_remove_mountpoint "${mountpoint}"
    done
  fi

  switch_to_strict_mode
}

mount_usb_partitions() {
  local ro_rw="rw"
  local rootfs_ro_rw="rw"
  if [[ ${FLAGS_read_only} -eq ${FLAGS_TRUE} ]]; then
    ro_rw="ro"
  fi
  if [[ ${FLAGS_read_only} -eq ${FLAGS_TRUE} ||
        ${FLAGS_safe} -eq ${FLAGS_TRUE} ]]; then
    rootfs_ro_rw="ro"
  fi

  if [[ -f "${FLAGS_from}/${FLAGS_partition_script}" ]]; then
    . "${FLAGS_from}/${FLAGS_partition_script}"
    load_partition_vars
  fi

  fs_mount "${FLAGS_from}${PARTITION_NUM_ROOT_A}" "${ROOT_A_MOUNTPOINT}" \
    "${FS_FORMAT_ROOT_A}" "${rootfs_ro_rw}"
  fs_mount "${FLAGS_from}${PARTITION_NUM_STATE}" "${STATE_MOUNTPOINT}" \
    "${FS_FORMAT_STATE}" "${ro_rw}"
  fs_mount "${FLAGS_from}${PARTITION_NUM_OEM}" "${OEM_MOUNTPOINT}" \
    "${FS_FORMAT_OEM}" "${ro_rw}"

  if [[ -n "${FLAGS_esp_mountpt}" && \
        -e ${FLAGS_from}${PARTITION_NUM_EFI_SYSTEM} ]]; then
    fs_mount "${FLAGS_from}${PARTITION_NUM_EFI_SYSTEM}" \
      "${EFI_SYSTEM_MOUNTPOINT}" "${FS_FORMAT_EFI_SYSTEM}" "${ro_rw}"
  fi
}

mount_gpt_partitions() {
  local filename="${FLAGS_from}/${FLAGS_image}"

  local ro_rw="rw"
  if [[ ${FLAGS_read_only} -eq ${FLAGS_TRUE} ]]; then
    ro_rw="ro"
  fi

  if [[ ! -f "${filename}" ]]; then
    die "Image ${filename} does not exist."
  fi

  if [[ -f "${FLAGS_from}/${FLAGS_partition_script}" ]]; then
    . "${FLAGS_from}/${FLAGS_partition_script}"
    load_partition_vars
  fi

  if [[ ${FLAGS_read_only} -eq ${FLAGS_FALSE} && \
        ${FLAGS_safe} -eq ${FLAGS_FALSE} ]]; then
    local rootfs_offset=$(partoffset "${filename}" ${PARTITION_NUM_ROOT_A})
    # Make sure any callers can actually mount and modify the fs
    # if desired.
    # cros_make_image_bootable should restore the bit if needed.
    enable_rw_mount "${filename}" "$(( rootfs_offset * 512 ))"
  fi

  # Mount in order: rootfs, stateful, OEM and EFI.
  local var_name mountpoint fs_format
  local part_label part_num part_offset part_size part_ro_rw
  for part_label in ROOT_A STATE OEM EFI_SYSTEM; do
    var_name="${part_label}_MOUNTPOINT"
    mountpoint="${!var_name}"
    [[ -n "${mountpoint}" ]] || continue

    var_name="PARTITION_NUM_${part_label}"
    part_num="${!var_name}"
    [[ -n "${part_num}" ]] || continue

    part_size=$(get_partition_size "${filename}" ${part_num}) || continue
    part_offset=$(partoffset "${filename}" ${part_num}) ||
        die "Failed to get partition offset for partition ${part_num}"
    var_name="FS_FORMAT_${part_num}"
    fs_format="${!var_name}"

    # The "safe" flags tells if the rootfs should be mounted as read-only,
    # otherwise we use ${ro_rw}.
    part_ro_rw="${ro_rw}"
    if [[ ${part_num} -eq ${PARTITION_NUM_ROOT_A} && \
          ${FLAGS_safe} -eq ${FLAGS_TRUE} ]]; then
      part_ro_rw="ro"
    fi
    mount_options="offset=$(( part_offset * 512 ))"

    if ! fs_mount "${filename}" "${mountpoint}" "${fs_format}" \
        "${part_ro_rw}" "${mount_options}"; then
      error "mount failed: device=${filename}" \
        "target=${mountpoint} format=${fs_format} ro/rw=${part_ro_rw}" \
        "options=${mount_options}"
      return 1
    fi
  done
}

# Mount a gpt based image.
mount_image() {
  mkdir -p "${FLAGS_rootfs_mountpt}"
  mkdir -p "${FLAGS_stateful_mountpt}"
  if [[ -n "${FLAGS_esp_mountpt}" ]]; then
    mkdir -p "${FLAGS_esp_mountpt}"
  fi
  # Get the partitions for the image / device.
  if [[ -b "${FLAGS_from}" ]]; then
    mount_usb_partitions
  elif ! mount_gpt_partitions; then
    echo "Current loopback device status:"
    sudo losetup --all | sed 's/^/    /'
    die "Failed to mount all partitions in ${FLAGS_from}/${FLAGS_image}"
  fi

  # Mount directories and setup symlinks.
  sudo mount --bind "${FLAGS_stateful_mountpt}" \
    "${FLAGS_rootfs_mountpt}/mnt/stateful_partition"
  sudo mount --bind "${FLAGS_stateful_mountpt}/var_overlay" \
    "${FLAGS_rootfs_mountpt}/var"
  sudo mount --bind "${FLAGS_stateful_mountpt}/dev_image" \
    "${FLAGS_rootfs_mountpt}/usr/local"

  # Setup symlinks in /usr/local so you can emerge packages into /usr/local.
  if [[ ${FLAGS_read_only} -eq ${FLAGS_FALSE} ]]; then
    setup_symlinks_on_root "." \
      "${FLAGS_stateful_mountpt}/var_overlay" "${FLAGS_stateful_mountpt}"
  fi
  info "Image specified by ${FLAGS_from} mounted at"\
    "${FLAGS_rootfs_mountpt} successfully."
}

# Remount a previously mounted gpt based image.
remount_image() {
  local ro_rw="rw"
  if [[ ${FLAGS_read_only} -eq ${FLAGS_TRUE} ]]; then
    ro_rw="ro"
  fi

  # If all the filesystems are mounted via the kernel, just issue the remount
  # command with the proper flags.
  local var_name mountpoint
  local part_label part_num part_ro_rw part_size
  for part_label in ROOT_A STATE OEM EFI_SYSTEM; do
    var_name="${part_label}_MOUNTPOINT"
    mountpoint="${!var_name}"
    [[ -n "${mountpoint}" ]] || continue

    var_name="PARTITION_NUM_${part_label}"
    part_num="${!var_name}"

    part_size=$(get_partition_size "${filename}" ${part_num}) || continue

    if ! mountpoint -q "${mountpoint}"; then
      # Fallback to unmount everything and re-mount everything.
      info "Fallback --remount to --unmount and then --remount."
      unmount_image
      mount_image
      return
    fi

    # The "safe" flags tells if the rootfs should be mounted as read-only,
    # otherwise we use ${ro_rw}.
    part_ro_rw="${ro_rw}"
    if [[ ${part_num} -eq ${PARTITION_NUM_ROOT_A} && \
          ${FLAGS_safe} -eq ${FLAGS_TRUE} ]]; then
      part_ro_rw="ro"
    fi

    sudo mount "${mountpoint}" -o "remount,${part_ro_rw}"
  done
}

# Turn paths into absolute paths.
[[ -n "${FLAGS_from}" ]] && FLAGS_from="$(readlink -f "${FLAGS_from}")"
FLAGS_rootfs_mountpt="$(readlink -f "${FLAGS_rootfs_mountpt}")"
FLAGS_stateful_mountpt="$(readlink -f "${FLAGS_stateful_mountpt}")"

# Partition mountpoints based on the flags.
ROOT_A_MOUNTPOINT="${FLAGS_rootfs_mountpt}"
STATE_MOUNTPOINT="${FLAGS_stateful_mountpt}"
OEM_MOUNTPOINT="${FLAGS_rootfs_mountpt}/usr/share/oem"
EFI_SYSTEM_MOUNTPOINT="${FLAGS_esp_mountpt}"

whereis fs_mount
# Read the image partition numbers from the GPT.
load_image_partition_numbers

# Perform desired operation.
if [[ ${FLAGS_unmount} -eq ${FLAGS_TRUE} ]]; then
  unmount_image
elif [[ ${FLAGS_remount} -eq ${FLAGS_TRUE} ]]; then
  remount_image
else
  mount_image
  
fi
