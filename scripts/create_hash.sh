#!/bin/bash

# Copyright (c) 2010 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Helper script that generates the signed kernel image

SCRIPT_ROOT=$(dirname $(readlink -f "$0"))
. "${SCRIPT_ROOT}/common.sh" || exit 1

# Flags.
DEFINE_string arch "x86" \
  "The boot architecture: arm, x86, or amd64. (Default: x86)"
DEFINE_string to "/tmp/vmlinuz.image" \
  "The path to the kernel image to be created. (Default: /tmp/vmlinuz.image)"
DEFINE_string hd_vblock "" \
  "The path to the installed kernel's vblock"
DEFINE_string vmlinuz "vmlinuz" \
  "The path to the kernel (Default: vmlinuz)"
DEFINE_string working_dir "/tmp/vmlinuz.working" \
  "Working directory for in-progress files. (Default: /tmp/vmlinuz.working)"
DEFINE_boolean keep_work ${FLAGS_FALSE} \
  "Keep temporary files (*.keyblock, *.vbpubk). (Default: false)"
DEFINE_string keys_dir "vboot/devkeys/" \
  "Directory with the RSA signing keys. (Defaults to test keys)"
DEFINE_string keyblock "kernel.keyblock" \
  "The keyblock to use. (Defaults to kernel.keyblock)"
DEFINE_string private "kernel_data_key.vbprivk" \
  "The private key to sign the kernel (Defaults to kernel_data_key.vbprivk)"
DEFINE_string public "kernel_subkey.vbpubk" \
  "The public key to verify the kernel (Defaults to kernel_subkey.vbpubk)"
# Note, to enable verified boot, the caller would manually pass:
# --boot_args='dm="... %U+1 %U+1 ..." \
# --root=/dev/dm-0
DEFINE_string boot_args "noinitrd" \
  "Additional boot arguments to pass to the commandline (Default: noinitrd)"
# If provided, will automatically add verified boot arguments.
DEFINE_string rootfs_image "" \
  "Optional path to the rootfs device or image.(Default: \"\")"
DEFINE_string rootfs_image_size "" \
  "Optional size in bytes of the rootfs_image file. Must be a multiple of 4 \
KiB. If omitted, the filesystem size detected from rootfs_image is used."
DEFINE_string rootfs_hash "" \
  "Optional path to output the rootfs hash to. (Default: \"\")"
DEFINE_integer verity_error_behavior 3 \
  "Verified boot error behavior [0: I/O errors, 1: reboot, 2: nothing] \
(Default: 3)"
DEFINE_integer verity_max_ios -1 \
  "Optional number of outstanding I/O operations. (Default: -1)"
DEFINE_string verity_hash_alg "sha1" \
  "Cryptographic hash algorithm used for dm-verity. (Default: sha1)"
DEFINE_string verity_salt "" \
  "Salt to use for rootfs hash (Default: \"\")"
DEFINE_boolean enable_rootfs_verification ${FLAGS_TRUE} \
  "Enable kernel-based root fs integrity checking. (Default: true)"
DEFINE_boolean enable_bootcache ${FLAGS_FALSE} \
  "Enable boot cache to accelerate booting. (Default: false)"
DEFINE_string enable_serial "" \
  "Enable serial port for printks. Example values: ttyS0"
DEFINE_integer loglevel 7 \
  "The loglevel to add to the kernel command line."

# Parse flags
FLAGS "$@" || exit 1
eval set -- "${FLAGS_ARGV}"

# Die on error
switch_to_strict_mode

# N.B.  Ordering matters for some of the libraries below, because
# some of the files contain initialization used by later files.
. "./disk_layout_util.sh" || exit 1


rootdigest() {
  local digest=${table#*root_hexdigest=}
  echo ${digest% salt*}
}

salt() {
  local salt=${table#*salt=}
  echo ${salt%}
}

hashstart() {
  local hash=${table#*hashstart=}
  echo ${hash% alg*}
}

# Estimate of sectors used by verity
# (num blocks) * 32 (bytes per hash) * 2 (overhead) / 512 (bytes per sector)
veritysize() {
  echo $((root_fs_blocks * 32 * 2 / 512))
}

get_base_root() {
  echo 'PARTUUID=%U/PARTNROFF=1'
}

base_root=$(get_base_root)

device_mapper_args=
# Even with a rootfs_image, root= is not changed unless specified.
if [[ -n "${FLAGS_rootfs_image}" && -n "${FLAGS_rootfs_hash}" ]]; then
  # Gets the number of blocks. 4096 byte blocks _are_ expected.
  if [[ -n "${FLAGS_rootfs_image_size}" ]]; then
    root_fs_size=${FLAGS_rootfs_image_size}
  else
    # We try to autodetect the rootfs_image filesystem size.
    if [[ -f "${FLAGS_rootfs_image}" ]]; then
      root_fs_size=$(stat -c '%s' ${FLAGS_rootfs_image})
    elif [[ -b "${FLAGS_rootfs_image}" ]]; then
      root_fs_type="$(awk -v rootdev="${FLAGS_rootfs_image}" \
                     '$1 == rootdev { print $3 }' /proc/mounts | head -n 1)"
      case "${root_fs_type}" in
        squashfs)
          # unsquashfs returns the size in KiB as a float value with two
          # decimals, rounded as printf() would do. To avoid corner cases like
          # when the fractional part of the size in KiB is less than 0.005,
          # instead of rounding up the value to the nearest 4KiB, we round it
          # down and add 1 extra 4 KiB block.
          root_fs_size_kib=$(sudo unsquashfs -stat "${FLAGS_rootfs_image}" |
                             grep -E -o 'Filesystem size [0-9\.]+ Kbytes' |
                             cut -f 3 -d ' ' | cut -f 1 -d '.')
          root_fs_size=$(( (root_fs_size_kib / 4 + 1) * 4096 ))
        ;;
        ext[234])
          root_fs_blocks=$(sudo dumpe2fs "${FLAGS_rootfs_image}" 2>/dev/null |
                         grep "Block count" |
                         tr -d ' ' |
                         cut -f2 -d:)
          root_fs_block_sz=$(sudo dumpe2fs "${FLAGS_rootfs_image}" 2>/dev/null |
                           grep "Block size" |
                           tr -d ' ' |
                           cut -f2 -d:)
          root_fs_size=$(( root_fs_blocks * root_fs_block_sz ))
        ;;
        *)
          die "Unknown root filesystem type ${root_fs_type}."
        ;;
      esac
    else
      die "Couldn't determine the size of ${FLAGS_rootfs_image}, pass the size \
with --rootfs_image_size."
    fi
  fi
  # Verity assumes a 4 KiB block size.
  if [[ ! $(( root_fs_size % 4096 )) -eq 0 ]]; then
    die "The root filesystem size (${root_fs_size}) must be a multiple of \
4 KiB."
  fi
  root_fs_blocks=$((root_fs_size / 4096))
  info "rootfs is ${root_fs_blocks} blocks of 4096 bytes."

  info "Generating root fs hash tree (salt '${FLAGS_verity_salt}')."
  # Runs as sudo in case the image is a block device.
  # First argument to verity is reserved/unused and MUST be 0
  table=$(sudo ./verity mode=create \
                      alg=${FLAGS_verity_hash_alg} \
                      payload=${FLAGS_rootfs_image} \
                      payload_blocks=${root_fs_blocks} \
                      hashtree=${FLAGS_rootfs_hash} \
                      salt=${FLAGS_verity_salt})
  if [[ -f "${FLAGS_rootfs_hash}" ]]; then
    sudo chmod a+r "${FLAGS_rootfs_hash}"
  fi
  # Don't claim the root device unless verity is enabled.
  # Doing so will claim /dev/sdDP out from under the system.
  if [[ ${FLAGS_enable_rootfs_verification} -eq ${FLAGS_TRUE} ]]; then
    if [[ ${FLAGS_enable_bootcache} -eq ${FLAGS_TRUE} ]]; then
      base_root='254:0'  # major:minor numbers for /dev/dm-0
    fi
    table=${table//HASH_DEV/${base_root}}
    table=${table//ROOT_DEV/${base_root}}
  fi
  verity_dev="vroot none ro 1,${table}"
  if [[ ${FLAGS_enable_bootcache} -eq ${FLAGS_TRUE} ]]; then
    signature=$(rootdigest)
    cachestart=$(($(hashstart) + $(veritysize)))
    size_limit=512
    max_trace=20000
    max_pages=100000
    bootcache_args="PARTUUID=%U/PARTNROFF=1"
    bootcache_args+=" ${cachestart} ${signature} ${size_limit}"
    bootcache_args+=" ${max_trace} ${max_pages}"
    bootcache_dev="vboot none ro 1,0 ${cachestart} bootcache ${bootcache_args}"
    device_mapper_args="dm=\"2 ${bootcache_dev}, ${verity_dev}\""
  else
    device_mapper_args="dm=\"1 ${verity_dev}\""
  fi
  info "device mapper configuration: ${device_mapper_args}"
fi

mkdir -p "${FLAGS_working_dir}"

# Only let dm-verity block if rootfs verification is configured.
# By default, we use a firmware enumerated value, but it isn't reliable for
# production use.  If +%d can be added upstream, then we can use:
#   root_dev=PARTUID=uuid+1
dev_wait=0
root_dev=${base_root}
if [[ ${FLAGS_enable_rootfs_verification} -eq ${FLAGS_TRUE} ]]; then
  dev_wait=1
  if [[ ${FLAGS_enable_bootcache} -eq ${FLAGS_TRUE} ]]; then
    root_dev=/dev/dm-1
  else
    root_dev=/dev/dm-0
  fi
else
  if [[ ${FLAGS_enable_bootcache} -eq ${FLAGS_TRUE} ]]; then
    die "Having bootcache without verity is not supported"
  fi
fi

# kern_guid should eventually be changed to use PARTUUID
cat <<EOF > "${FLAGS_working_dir}/boot.config"
ro
dm_verity.error_behavior=${FLAGS_verity_error_behavior}
dm_verity.max_bios=${FLAGS_verity_max_ios}
dm_verity.dev_wait=${dev_wait}
${device_mapper_args}
${FLAGS_boot_args}
vt.global_cursor_default=0
kern_guid=%U
EOF
## kern_guid should eventually be changed to use PARTUUID
#cat <<EOF > "${FLAGS_working_dir}/boot.config"
#root=${root_dev}
#ro
#dm_verity.error_behavior=${FLAGS_verity_error_behavior}
#dm_verity.max_bios=${FLAGS_verity_max_ios}
#dm_verity.dev_wait=${dev_wait}
#${device_mapper_args}
#${FLAGS_boot_args}
#vt.global_cursor_default=0
#kern_guid=%U
#EOF

WORK="${WORK} ${FLAGS_working_dir}/boot.config"
info "Emitted cross-platform boot params to ${FLAGS_working_dir}/boot.config"

# Add common boot options first.
config="${FLAGS_working_dir}/config.txt"
if [[ -n ${FLAGS_enable_serial} ]]; then
  console=${FLAGS_enable_serial}
  if [[ ${console} != *,* ]]; then
    console+=",115200n8"
  fi
  cat <<EOF > "${config}"
earlyprintk=${console}
console=tty1
console=${console}
EOF
else
  cat <<EOF > "${config}"
earlyprintk=ttyS0,115200,keep 
rootwait  
EOF
fi

cat <<EOF - "${FLAGS_working_dir}/boot.config" >> "${config}"
loglevel=${FLAGS_loglevel}
cros_secure
oops=panic
panic=-1
EOF

if [[ "${FLAGS_arch}" = "x86" || "${FLAGS_arch}" = "amd64" ]]; then
  # Legacy BIOS will use the kernel in the rootfs (via syslinux), as will
  # standard EFI BIOS (via grub, from the EFI System Partition). Chrome OS
  # BIOS will use a separate signed kernel partition, which we'll create now.
  cat <<EOF >> "${FLAGS_working_dir}/config.txt"
add_efi_memmap
boot=local
noresume
noswap
i915.modeset=1
tpm_tis.force=1
tpm_tis.interrupts=0
nmi_watchdog=panic,lapic
EOF
  WORK="${WORK} ${FLAGS_working_dir}/config.txt"

  bootloader_path="lib64/bootstub/bootstub.efi"
  kernel_image="${FLAGS_vmlinuz}"
elif [[ "${FLAGS_arch}" = "arm" || "${FLAGS_arch}" = "mips" ]]; then
  WORK="${WORK} ${FLAGS_working_dir}/config.txt"

  # arm does not need/have a bootloader in kernel partition
  dd if="/dev/zero" of="${FLAGS_working_dir}/bootloader.bin" bs=512 count=1
  WORK="${WORK} ${FLAGS_working_dir}/bootloader.bin"

  bootloader_path="${FLAGS_working_dir}/bootloader.bin"
  kernel_image="${FLAGS_vmlinuz/vmlinuz/vmlinux.uimg}"
else
  error "Unknown arch: ${FLAGS_arch}"
fi

# Save the kernel as a .bin to allow it to be automatically extracted as
# an artifact by cbuildbot.  Non .bin's need to be explicitly specified
# and would require the entire set of artifacts to be specified.
info "Saving ${kernel_image} as ${FLAGS_working_dir}/vmlinuz.bin"
cp ${kernel_image} ${FLAGS_working_dir}/vmlinuz.bin

for image_type in $(get_image_types); do
  already_seen_rootfs=0
  for partition in $(get_partitions ${image_type}); do
    format=$(get_format ${image_type} "${partition}")
    if [[ "${format}" == "ubi" ]]; then
      type=$(get_type ${image_type} "${partition}")
      # cgpt.py ensures that the rootfs partitions are compatible, in that if
      # one is ubi then both are, and they have the same number of reserved
      # blocks. We only want to attach one of them in boot to save time, so
      # attach %P and get the information for whichever rootfs comes first.
      if [[ "${type}" == "rootfs" ]]; then
        if [[ "${already_seen_rootfs}" -ne 0 ]]; then
          continue
        fi
        already_seen_rootfs=1
        partname='%P'
      else
        partname="${partition}"
      fi
      reserved=$(get_reserved_erase_blocks ${image_type} "${partition}")
      echo "ubi.mtd=${partname},0,${reserved},${partname}" \
          >> "${FLAGS_working_dir}/config.txt"
      fs_format=$(get_filesystem_format ${image_type} "${partition}")
      if [[ "${fs_format}" != "ubifs" ]]; then
        echo "ubi.block=${partname},0" >> "${FLAGS_working_dir}/config.txt"
      fi
    fi
  done
done

config_file="${FLAGS_working_dir}/config.txt"
# Create and sign the kernel blob
./vbutil_kernel \
  --pack "${FLAGS_to}" \
  --keyblock "${FLAGS_keys_dir}/${FLAGS_keyblock}" \
  --signprivate "${FLAGS_keys_dir}/${FLAGS_private}" \
  --version 1 \
  --config "${config_file}" \
  --bootloader "bootstub.efi" \
  --vmlinuz "kernels/bzImage" \
  --arch "${FLAGS_arch}"

# And verify it.
./vbutil_kernel \
  --verify "${FLAGS_to}" \
  --signpubkey "${FLAGS_keys_dir}/${FLAGS_public}"

if [[ -n "${FLAGS_hd_vblock}" ]]; then
  dd if="${FLAGS_to}" bs=65536 count=1 of="${FLAGS_hd_vblock}"
fi

set +e  # cleanup failure is a-ok

if [[ ${FLAGS_keep_work} -eq ${FLAGS_FALSE} ]]; then
  info "Cleaning up temporary files: ${WORK}"
  rm ${WORK}
  rmdir ${FLAGS_working_dir}
fi

info "Kernel partition image emitted: ${FLAGS_to}"

if [[ -f ${FLAGS_rootfs_hash} ]]; then
  info "Root filesystem hash emitted: ${FLAGS_rootfs_hash}"
fi
