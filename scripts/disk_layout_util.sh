# Copyright (c) 2012 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# BUILD_LIBRARY_DIR must be set prior to sourcing this file, since this file
# is sourced as ${BUILD_LIBRARY_DIR}/disk_layout_util.sh
. "filesystem_util.sh" || exit 1

CGPT_PY="./cgpt.py"
PARTITION_SCRIPT_PATH="usr/sbin/write_gpt.sh"
DISK_LAYOUT_PATH=

cgpt_py() {
  if [[ -n "${FLAGS_adjust_part-}" ]]; then
    set -- --adjust_part "${FLAGS_adjust_part}" "$@"
    if [[ ! -t 0 ]]; then
      warn "The --adjust_part flag was passed." \
           "This option must ONLY be used interactively. If" \
           "you need to pass a size from another script, you're" \
           "doing it wrong and should be using a disk layout type."
    fi
  fi
  "${CGPT_PY}" "$@"
}

get_disk_layout_path() {
  DISK_LAYOUT_PATH="./legacy_disk_layout.json"
}

write_partition_script() {
  local image_type=$1
  local partition_script_path=$2
  echo "get disk layout"
  get_disk_layout_path

  local temp_script_file=$(mktemp)

  sudo mkdir -p "$(dirname "${partition_script_path}")"
  
  echo "cgpty command"

  cgpt_py write "${image_type}" "${DISK_LAYOUT_PATH}" \
          "${temp_script_file}"
  sudo mv "${temp_script_file}" "${partition_script_path}"
  sudo chmod a+r "${partition_script_path}"
}

run_partition_script() {
  local outdev=$1
  local partition_script=$2

  local pmbr_img
  case ${ARCH} in
  amd64|x86)
    pmbr_img=$(readlink -f /usr/share/syslinux/gptmbr.bin)
    ;;
  *)
    pmbr_img=/dev/zero
    ;;
  esac

  . "${partition_script}"
  write_partition_table "${outdev}" "${pmbr_img}"
}

get_fs_block_size() {
  get_disk_layout_path

  cgpt_py readfsblocksize "${DISK_LAYOUT_PATH}"
}

get_block_size() {
  get_disk_layout_path

  cgpt_py readblocksize "${DISK_LAYOUT_PATH}"
}

get_image_types() {
  get_disk_layout_path

  cgpt_py readimagetypes "${DISK_LAYOUT_PATH}"
}

get_partition_size() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readpartsize "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_filesystem_format() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readfsformat "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_filesystem_options() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readfsoptions "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_format() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readformat "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_partitions() {
  local image_type=$1
  get_disk_layout_path

  cgpt_py readpartitionnums "${image_type}" "${DISK_LAYOUT_PATH}"
}

get_uuid() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readuuid "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_type() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readtype "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_filesystem_size() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readfssize "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_label() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readlabel "${image_type}" "${DISK_LAYOUT_PATH}" "${part_id}"
}

get_image_partition_number() {
  local image="$1"
  local label="$2"
  local part=$(./cgpt find -n -l "${label}" "${image}")
  echo "${part}"
}

get_layout_partition_number() {
  local image_type=$1
  local part_label=$2
  get_disk_layout_path

  cgpt_py readnumber "${image_type}" "${DISK_LAYOUT_PATH}" "${part_label}"
}

get_reserved_erase_blocks() {
  local image_type=$1
  local part_id=$2
  get_disk_layout_path

  cgpt_py readreservederaseblocks "${image_type}" "${DISK_LAYOUT_PATH}" \
    ${part_id}
}

check_valid_layout() {
  local image_type=$1
  get_disk_layout_path

  cgpt_py validate "${image_type}" "${DISK_LAYOUT_PATH}" > /dev/null
}

get_disk_layout_type() {
  DISK_LAYOUT_TYPE="base"
  if should_build_image ${CHROMEOS_FACTORY_INSTALL_SHIM_NAME}; then
    DISK_LAYOUT_TYPE="factory_install"
  fi
}

emit_gpt_scripts() {
  local image="$1"
  local dir="$2"

  local pack="pack_partitions.sh"
  local unpack="unpack_partitions.sh"
  local mount="mount_image.sh"
  local umount="umount_image.sh"

  local start size part x

  local default
  # Write out the header for the script.
  local cgpt_output=$(./cgpt show "${image}")
  local gpt_layout=$(echo "${cgpt_output}" | sed -e 's/^/# /')
 for x in "${unpack}" "${pack}" "${mount}" "${umount}"; do
    cat >"${x}" <<\EOF
#!/bin/bash -eu
# File automatically generated. Do not edit.

usage() {
  local ret=0
  if [[ $# -gt 0 ]]; then
    # Write to stderr on errors.
    exec 1>&2
    echo "ERROR: $*"
    echo
    ret=1
  fi
  echo "Usage: $0 [image] [part]"
  echo "Example: $0 chromiumos_image.bin"
  exit ${ret}
}

TARGET=${1:-}
PART=${2:-}
case ${TARGET} in
-h|--help)
  usage
  ;;
"")
  for TARGET in chromiumos_{,base_}image.bin ""; do
    if [[ -e ${TARGET} ]]; then
      echo "autodetected image: ${TARGET}"
      break
    fi
  done
  if [[ -z ${TARGET} ]]; then
    usage "could not autodetect an image"
  fi
  ;;
*)
  if [[ ! -e ${TARGET} ]]; then
    usage "image does not exist: ${TARGET}"
  fi
esac

EOF
    echo "${gpt_layout}" >> "${x}"
  done

  # Read each partition and generate code for it.
  while read start size part x; do
    local file="part_${part}"
    local dir="dir_${part}"
    local target='"${TARGET}"'
    local dd_args="bs=512 count=${size}"
    local start_b=$(( start * 512 ))
    local size_b=$(( size * 512 ))
    local label=$(./cgpt show "${image}" -i ${part} -l)

    for x in "${unpack}" "${pack}" "${mount}" "${umount}"; do
      cat <<EOF >> "${x}"
case \${PART:-${part}} in
${part}|"${label}")
EOF
    done

    cat <<EOF >> "${unpack}"
dd if=${target} of=${file} ${dd_args} skip=${start}
ln -sfT ${file} "${file}_${label}"
EOF
    cat <<EOF >> "${pack}"
dd if=${file} of=${target} ${dd_args} seek=${start} conv=notrunc
EOF

    if [[ ${size} -gt 1 ]]; then
      cat <<-EOF >>"${mount}"
(
mkdir -p ${dir}
m=( sudo mount -o loop,offset=${start_b},sizelimit=${size_b} ${target} ${dir} )
if ! "\${m[@]}"; then
  if ! "\${m[@]}" -o ro; then
    rmdir ${dir}
    exit 0
  fi
fi
ln -sfT ${dir} "${dir}_${label}"
) &
EOF
      cat <<-EOF >>"${umount}"
if [[ -d ${dir} ]]; then
  (
  sudo umount ${dir} || :
  rmdir ${dir}
  rm -f "${dir}_${label}"
  ) &
fi
EOF
    fi

    for x in "${unpack}" "${pack}" "${mount}" "${umount}"; do
      echo "esac" >> "${x}"
    done
  done < <(./cgpt show -q "${image}")

  echo "wait" >> "${mount}"
  echo "wait" >> "${umount}"

  chmod +x "${unpack}" "${pack}" "${mount}" "${umount}"
}

# Usage: mk_fs  <image_file> <image_type> <partition_num>
# Args:
#   image_file: The image file.
#   image_type: The layout name used to look up partition info in disk layout.
#   partition_num: The partition number to look up in the disk layout.
#
# Note: After we mount the fs, we will attempt to reset the root dir ownership
#       to 0:0 to workaround a bug in mke2fs (fixed in upstream git now).
mk_fs() {
  local image_file=$1
  local image_type=$2
  local part_num=$3

  echo "CALLED WITH"
  echo $1 $2 $3

  # These are often not in non-root $PATH, but they contain tools that
  # we can run just fine w/non-root users when we work on plain files.
  local p
  for p in /sbin /usr/sbin; do
    if [[ ":${PATH}:" != *:${p}:* ]]; then
      PATH+=":${p}"
    fi
  done

  # Keep `local` decl split from assignment so return code is checked.
  local fs_bytes fs_label fs_format fs_options fs_block_size offset fs_type
  fs_format=$(get_filesystem_format ${image_type} ${part_num})
  echo ${fs_format}
  fs_options="$(get_filesystem_options ${image_type} ${part_num})"
  # Split the fs_options into an array.
  local fs_options_arr=(${fs_options})
  if [ -z "${fs_format}" ]; then
    echo "HERE"
    # We only make fs for partitions that specify a format.
    return 0
  fi

  fs_bytes=$(get_filesystem_size ${image_type} ${part_num})
  fs_block_size=$(get_fs_block_size)
    echo "HERE"
  if [ "${fs_bytes}" -le ${fs_block_size} ]; then
    # Skip partitions that are too small.
    echo "Skipping partition ${part_num} as the blocksize is too small."
    return 0
  fi

  echo "Creating FS for partition ${part_num} with format ${fs_format}."

  fs_label=$(get_label ${image_type} ${part_num})
  fs_uuid=$(get_uuid ${image_type} ${part_num})
  fs_type=$(get_type ${image_type} ${part_num})
  echo "MADE ABBUNCH OF VARIABLES"
  # Mount at the correct place in the file.
  offset=$(( $(partoffset "${image_file}" "${part_num}") * 512 ))
    echo "HERE"
  # Root is needed to mount on loopback device.
  # sizelimit is used to denote the FS size for mkfs if not specified.
  local part_dev=$(sudo losetup -f --show --offset=${offset} \
      --sizelimit=${fs_bytes} "${image_file}")
    echo "HERE"
  if [ ! -e "${part_dev}" ]; then
    die "No free loopback device to create partition."
  fi

  echo "HERE"
  case ${fs_format} in
  ext[234])
    # When mke2fs supports the same values for -U as tune2fs does, the
    # following conditionals can be removed and ${fs_uuid} can be used
    # as the value of the -U option as-is.
    echo "HERE"
    local uuid_option=()
    if [[ "${fs_uuid}" == "clear" ]]; then
      fs_uuid="00000000-0000-0000-0000-000000000000"
    fi
    echo "HERE"
    if [[ "${fs_uuid}" != "random" ]]; then
      uuid_option=( -U "${fs_uuid}" )
    fi
    echo "HERE"
    sudo mkfs.${fs_format} -F -q -O ext_attr \
        "${uuid_option[@]}" \
        -E lazy_itable_init=0 \
        -b ${fs_block_size} "${part_dev}" "$((fs_bytes / fs_block_size))"
    echo "HERE"
    # We need to redirect from stdin and clear the prompt variable to make
    # sure tune2fs doesn't throw up random prompts on us.  We know that the
    # command below is what we want and is safe (it's a new FS).
    unset TUNE2FS_FORCE_PROMPT
    echo "HERE"
    sudo tune2fs -L "${fs_label}" \
        -c 0 \
        -i 0 \
        -T 20091119110000 \
        -m 0 \
        -r 0 \
        -e remount-ro \
        "${part_dev}" \
        "${fs_options_arr[@]}" </dev/null
    echo "FINISHED ${fs_format}"
    ;;
  fat12|fat16|fat32)
    sudo mkfs.vfat -F ${fs_format#fat} -n "${fs_label}" "${part_dev}" \
        "${fs_options_arr[@]}"
    ;;
  fat|vfat)
    sudo mkfs.vfat -n "${fs_label}" "${part_dev}" "${fs_options_arr[@]}"
    ;;
  squashfs)
    # Creates an empty squashfs filesystem so unsquashfs works.
    local squash_dir="$(mktemp -d --suffix=.squashfs)"
    local squash_file="$(mktemp --suffix=.squashfs)"
    # Make sure / has the right permission. "-all-root" will change the uid/gid.
    chmod 0755 "${squash_dir}"
    # If there are errors in mkquashfs they are sent to stderr, but in the
    # normal case a lot of useless information is sent to stdout.
    mksquashfs "${squash_dir}" "${squash_file}" -noappend -all-root \
        -no-progress -no-recovery "${fs_options_arr[@]}" >/dev/null
    rmdir "${squash_dir}"
    sudo dd if="${squash_file}" of="${part_dev}" bs=4096 status=none
    rm "${squash_file}"
    ;;
  btrfs)
    sudo mkfs.${fs_format} -b "$((fs_bytes))" -d single -m single -M \
      -L "${fs_label}" -O "${fs_options_arr[@]}" "${part_dev}"
    ;;
  *)
    die "Unknown fs format '${fs_format}' for part ${part_num}";;
  esac

  local mount_dir="$(mktemp -d)"
  local cmds=(
    # mke2fs is funky and sets the root dir owner to current uid:gid.
    "chown 0:0 '${mount_dir}' 2>/dev/null || :"
  )

  # Prepare partitions with well-known mount points.
  if [ "${fs_label}" = "STATE" ]; then
    # These directories are used to mount data from stateful onto the rootfs.
    cmds+=("sudo mkdir '${mount_dir}/dev_image'"
           "sudo mkdir '${mount_dir}/var_overlay'"
    )
  elif [ "${fs_type}" = "rootfs" ]; then
    # These rootfs mount points are necessary to mount data from other
    # partitions onto the rootfs. These are used by both build and run times.
    cmds+=("sudo mkdir -p '${mount_dir}/mnt/stateful_partition'"
           "sudo mkdir -p '${mount_dir}/usr/local'"
           "sudo mkdir -p '${mount_dir}/usr/share/oem'"
           "sudo mkdir '${mount_dir}/var'"
    )
  fi
  fs_mount "${part_dev}" "${mount_dir}" "${fs_format}" "rw"
  sudo_multi "${cmds[@]}"
  fs_umount "${part_dev}" "${mount_dir}" "${fs_format}" "${fs_options}"
  fs_remove_mountpoint "${mount_dir}"
}

# Creates the gpt image for the given disk layout. In addition to creating
# the partition layout it creates all the initial filesystems. After this file
# is created, mount_gpt_image.sh can be used to mount all the filesystems onto
# directories.
build_gpt_image() {
  local outdev="$1"
  local disk_layout="$2"

  # Build the partition table and partition script.
  local partition_script_path="$(dirname "${outdev}")/partition_script.sh"
  echo "writing partition"
  write_partition_script "${disk_layout}" "${partition_script_path}"
  run_partition_script "${outdev}" "${partition_script_path}"

  # Emit the gpt scripts so we can use them from here on out.
  emit_gpt_scripts "${outdev}" "$(dirname "${outdev}")"

  echo "MADE IT HERE"
  # Create the filesystem on each partition defined in the layout file.
  local p
  for p in $(get_partitions "${disk_layout}"); do
    mk_fs "${outdev}" "${disk_layout}" "${p}"
  done

  echo "MADE IT RIGHT HERE"
  # Pre-set "sucessful" bit in gpt, so we will never mark-for-death
  # a partition on an SDCard/USB stick.
  ./cgpt add -i $(get_layout_partition_number "${disk_layout}" KERN-A) -S 1 \
    "${outdev}"
}

round_up_4096() {
  local blocks=$1
  local round_up=$(( blocks % 4096 ))
  if [ $round_up -ne 0 ]; then
    blocks=$(( blocks + 4096 - round_up ))
  fi
  echo $blocks
}

# Rebuild an image's partition table with new stateful size.
#  $1: source image filename
#  $2: source stateful partition image filename
#  $3: number of sectors to allocate to the new stateful partition
#  $4: destination image filename
# Used by dev/host/tests/mod_recovery_for_decryption.sh and
# mod_image_for_recovery.sh.
update_partition_table() {
  local src_img=$1              # source image
  local src_state=$2            # stateful partition image
  local dst_stateful_blocks=$3  # number of blocks in resized stateful partition
  local dst_img=$4

  rm -f "${dst_img}"

  # Find partition number of STATE.
  local part=0
  local label=""
  while [ "${label}" != "STATE" ]; do
    part=$(( part + 1 ))
    local label=$(./cgpt show -i ${part} -l ${src_img})
    local src_start=$(./cgpt show -i ${part} -b ${src_img})
    if [ ${src_start} -eq 0 ]; then
      echo "Could not find 'STATE' partition" >&2
      return 1
    fi
  done

  # Make sure new stateful's size is a multiple of 4096 blocks so that
  # relocated partitions following it are not misaligned.
  dst_stateful_blocks=$(round_up_4096 $dst_stateful_blocks)
  # Calculate change in image size.
  local src_stateful_blocks=$(./cgpt show -i ${part} -s ${src_img})
  local delta_blocks=$(( dst_stateful_blocks - src_stateful_blocks ))
  local dst_stateful_bytes=$(( dst_stateful_blocks * 512 ))
  local src_stateful_bytes=$(( src_stateful_blocks * 512 ))
  local src_size=$(stat -c %s ${src_img})
  local dst_size=$(( src_size - src_stateful_bytes + dst_stateful_bytes ))
  truncate -s ${dst_size} ${dst_img}

  # Copy MBR, initialize GPT.
  dd if="${src_img}" of="${dst_img}" conv=notrunc bs=512 count=1 status=none
  ./cgpt create ${dst_img}

  local src_state_start=$(./cgpt show -i ${part} -b ${src_img})

  # Duplicate each partition entry.
  part=0
  while :; do
    part=$(( part + 1 ))
    local src_start=$(./cgpt show -i ${part} -b ${src_img})
    if [ ${src_start} -eq 0 ]; then
      # No more partitions to copy.
      break
    fi
    local dst_start=${src_start}
    # Load source partition details.
    local size=$(./cgpt show -i ${part} -s ${src_img})
    local label=$(./cgpt show -i ${part} -l ${src_img})
    local attr=$(./cgpt show -i ${part} -A ${src_img})
    local tguid=$(./cgpt show -i ${part} -t ${src_img})
    local uguid=$(./cgpt show -i ${part} -u ${src_img})
    if [[ ${size} -eq 0 ]]; then
      continue
    fi
    # Change size of stateful.
    if [ "${label}" = "STATE" ]; then
      size=${dst_stateful_blocks}
    fi
    # Partitions located after STATE need to have their start moved.
    if [ ${src_start} -gt ${src_state_start} ]; then
      dst_start=$(( dst_start + delta_blocks ))
    fi
    # Add this partition to the destination.
    ./cgpt add -i ${part} -b ${dst_start} -s ${size} -l "${label}" -A ${attr} \
             -t ${tguid} -u ${uguid} ${dst_img}
    if [ "${label}" != "STATE" ]; then
      # Copy source partition as-is.
      dd if="${src_img}" of="${dst_img}" conv=notrunc bs=512 \
        skip=${src_start} seek=${dst_start} count=${size} status=none
    else
      # Copy new stateful partition into place.
      dd if="${src_state}" of="${dst_img}" conv=notrunc bs=512 \
        seek=${dst_start} status=none
    fi
  done
  return 0
}
