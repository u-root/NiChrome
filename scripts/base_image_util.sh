# Copyright (c) 2012 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

. "toolchain_utils.sh" || exit 1
. "common.sh" || exit 1
CHROMEOS_MASTER_CONFIG_FILE="${BOARD_ROOT}/usr/share/chromeos-config/config.dtb"
BUILD_DIR="build"
BUILD_LIBRARY_DIR="."
SCRIPTS_DIR="."
echo "HERE"
. "${BUILD_LIBRARY_DIR}/disk_layout_util.sh" || exit 1
echo "HERE"
. "${BUILD_LIBRARY_DIR}/mount_gpt_util.sh" || exit 1
echo "HERE"
. "${BUILD_LIBRARY_DIR}/build_image_util.sh" || exit 1
echo "HERE"


check_full_disk() {
  local prev_ret=$?

  # Disable die on error.
  set +e

  # See if we ran out of space.  Only show if we errored out via a trap.
  if [[ ${prev_ret} -ne 0 ]]; then
    local df=$(df -B 1M "${root_fs_dir}")
    if [[ ${df} == *100%* ]]; then
      error "Here are the biggest [partially-]extracted files (by disk usage):"
      # Send final output to stderr to match `error` behavior.
      sudo find "${root_fs_dir}" -xdev -type f -printf '%b %P\n' | \
        awk '$1 > 16 { $1 = $1 * 512; print }' | sort -n | tail -100 1>&2
      error "Target image has run out of space:"
      error "${df}"
    fi
  fi

   # Turn die on error back on.
  set -e
}

zero_free_space() {
  local fs_mount_point=$1

  if ! mountpoint -q "${fs_mount_point}"; then
    info "Not zeroing freespace in ${fs_mount_point} since it isn't a mounted" \
        "filesystem. This is normal for squashfs and ubifs partitions."
    return 0
  fi

  info "Zeroing freespace in ${fs_mount_point}"
  # dd is a silly thing and will produce a "No space left on device" message
  # that cannot be turned off and is confusing to unsuspecting victims.
  info "${fs_mount_point}/filler"
  ( sudo dd if=/dev/zero of="${fs_mount_point}/filler" bs=4096 conv=fdatasync \
      status=noxfer || true ) 2>&1 | grep -v "No space left on device"
  sudo rm "${fs_mount_point}/filler"
}

# create_dev_install_lists updates package lists used by
# chromeos-base/dev-install
create_dev_install_lists() {
  local root_fs_dir=$1

  info "Building dev-install package lists"

  local pkgs=(
    portage
    virtual/target-os
    virtual/target-os-dev
    virtual/target-os-test
  )

  local pkgs_out=$(mktemp -d)

  for pkg in "${pkgs[@]}" ; do
    emerge-${BOARD} --color n --pretend --quiet --emptytree \
      --root-deps=rdeps ${pkg} | \
      egrep -o ' [[:alnum:]-]+/[^[:space:]/]+\b' | \
      tr -d ' ' | \
      sort > "${pkgs_out}/${pkg##*/}.packages"
    local _pipestatus=${PIPESTATUS[*]}
    [[ ${_pipestatus// } -eq 0 ]] || error "\`emerge-${BOARD} ${pkg}\` failed"
  done

  # bootstrap = portage - target-os
  comm -13 "${pkgs_out}/target-os.packages" \
    "${pkgs_out}/portage.packages" > "${pkgs_out}/bootstrap.packages"

  # chromeos-base = target-os + portage - virtuals
  sort -u "${pkgs_out}/target-os.packages" "${pkgs_out}/portage.packages" \
    | grep -v "virtual/" \
     > "${pkgs_out}/chromeos-base.packages"

  # package.installable = target-os-dev + target-os-test - target-os + virtuals
  comm -23 <(cat "${pkgs_out}/target-os-dev.packages" \
                 "${pkgs_out}/target-os-test.packages" | sort) \
    "${pkgs_out}/target-os.packages" \
    > "${pkgs_out}/package.installable"
  grep "virtual/" "${pkgs_out}/target-os.packages" \
    >> "${pkgs_out}/package.installable"

  # Add dhcp to the list of packages installed since its installation will not
  # complete (can not add dhcp group since /etc is not writeable). Bootstrap it
  # instead.
  grep "net-misc/dhcp-" "${pkgs_out}/target-os-dev.packages" \
    >> "${pkgs_out}/chromeos-base.packages" || true
  grep "net-misc/dhcp-" "${pkgs_out}/target-os-dev.packages" \
    >> "${pkgs_out}/bootstrap.packages" || true

  sudo mkdir -p \
    "${root_fs_dir}/usr/share/dev-install/portage/make.profile/package.provided"
  sudo cp "${pkgs_out}/bootstrap.packages" \
    "${root_fs_dir}/usr/share/dev-install/portage"
  sudo cp "${pkgs_out}/package.installable" \
    "${root_fs_dir}/usr/share/dev-install/portage/make.profile"
  sudo cp "${pkgs_out}/chromeos-base.packages" \
    "${root_fs_dir}/usr/share/dev-install/portage/make.profile/package.provided"

  rm -r "${pkgs_out}"
}

install_libc() {
  root_fs_dir="$1"
  # We need to install libc manually from the cross toolchain.
  # TODO: Improve this? It would be ideal to use emerge to do this.
  libc_version="$(get_variable "${BOARD_ROOT}/${SYSROOT_SETTINGS_FILE}" \
    "LIBC_VERSION")"
  PKGDIR="/var/lib/portage/pkgs"
  local libc_atom="cross-${CHOST}/glibc-${libc_version}"
  LIBC_PATH="${PKGDIR}/${libc_atom}.tbz2"

  if [[ ! -e ${LIBC_PATH} ]]; then
    sudo emerge --nodeps -gf "=${libc_atom}"
  fi

  # Strip out files we don't need in the final image at runtime.
  local libc_excludes=(
    # Compile-time headers.
    'usr/include' 'sys-include'
    # Link-time objects.
    '*.[ao]'
    # Debug commands not used by normal runtime code.
    'usr/bin/'{getent,ldd}
    # LD_PRELOAD objects for debugging.
    'lib*/lib'{memusage,pcprofile,SegFault}.so 'usr/lib*/audit'
    # We only use files & dns with nsswitch, so throw away the others.
    'lib*/libnss_'{compat,db,hesiod,nis,nisplus}'*.so*'
    # This is only for very old packages which we don't have.
    'lib*/libBrokenLocale*.so*'
  )
  pbzip2 -dc --ignore-trailing-garbage=1 "${LIBC_PATH}" | \
    sudo tar xpf - -C "${root_fs_dir}" ./usr/${CHOST} \
      --strip-components=3 "${libc_excludes[@]/#/--exclude=}"
}

create_base_image() {
  local image_name=$1
  local rootfs_verification_enabled=$2
  local bootcache_enabled=$3
  local output_dev=$4
  local image_type="usb"

  BUILD_DIR="build"
  check_valid_layout "base"
  check_valid_layout "${image_type}"

  echo "Using image type ${image_type}"
  get_disk_layout_path
  echo "Using disk layout ${DISK_LAYOUT_PATH}"
  
  mkdir -p "$BUILD_DIR"
  
  root_fs_dir="${BUILD_DIR}/rootfs"
  stateful_fs_dir="${BUILD_DIR}/stateful"
  esp_fs_dir="${BUILD_DIR}/esp"

  mkdir "${root_fs_dir}" "${stateful_fs_dir}" "${esp_fs_dir}"
  echo "Building GPT IMAGE"
  build_gpt_image "${output_dev}" "${image_type}"

  echo "Mounting GPT IMAGE"
  mount_image "${output_dev}" "${root_fs_dir}" \
    "${stateful_fs_dir}" "${esp_fs_dir}"

  echo "Df- h command"
  df -h "${root_fs_dir}"

  # Create symlinks so that /usr/local/usr based directories are symlinked to
  # /usr/local/ directories e.g. /usr/local/usr/bin -> /usr/local/bin, etc.

  # INSTALL KERNEL ON PARTITION 2
  
  local kernel_partition="2"
  sudo dd if=kernels/linux.bin of=${output_dev}${kernel_partition}

  "${VBOOT_SIGNING_DIR}"/insert_container_publickey.sh \
    "${root_fs_dir}" \
    "${VBOOT_DEVKEYS_DIR}"/cros-oci-container-pub.pem


  "${GCLIENT_ROOT}/chromite/bin/cros_set_lsb_release" \
    --sysroot="${root_fs_dir}" \
    --board="${BOARD}" \
    "${model_flags[@]}" \
    ${builder_path} \
    --version_string="${CHROMEOS_VERSION_STRING}" \
    --auserver="${CHROMEOS_VERSION_AUSERVER}" \
    --devserver="${CHROMEOS_VERSION_DEVSERVER}" \
    ${official_flag} \
    --buildbot_build="${BUILDBOT_BUILD:-"N/A"}" \
    --track="${CHROMEOS_VERSION_TRACK:-"developer-build"}" \
    --branch_number="${CHROMEOS_BRANCH}" \
    --build_number="${CHROMEOS_BUILD}" \
    --chrome_milestone="${CHROME_BRANCH}" \
    --patch_number="${CHROMEOS_PATCH}" \
    "${arc_flags[@]}"

  # Set /etc/os-release on the image.
  # Note: fields in /etc/os-release can come from different places:
  # * /etc/os-release itself with docrashid
  # * /etc/os-release.d for fields created with do_osrelease_field
  sudo "${GCLIENT_ROOT}/chromite/bin/cros_generate_os_release" \
    --root="${root_fs_dir}" \
    --version="${CHROME_BRANCH}" \
    --build_id="${CHROMEOS_VERSION_STRING}"

  # Create the boot.desc file which stores the build-time configuration
  # information needed for making the image bootable after creation with
  # cros_make_image_bootable.
  create_boot_desc "${image_type}"

  # Write out the GPT creation script.
  # This MUST be done before writing bootloader templates else we'll break
  # the hash on the root FS.
  write_partition_script "${image_type}" \
    "${root_fs_dir}/${PARTITION_SCRIPT_PATH}"
  sudo chown root:root "${root_fs_dir}/${PARTITION_SCRIPT_PATH}"

  # Populates the root filesystem with legacy bootloader templates
  # appropriate for the platform.  The autoupdater and installer will
  # use those templates to update the legacy boot partition (12/ESP)
  # on update.
  # (This script does not populate vmlinuz.A and .B needed by syslinux.)
  # Factory install shims may be booted from USB by legacy EFI BIOS, which does
  # not support verified boot yet (see create_legacy_bootloader_templates.sh)
  # so rootfs verification is disabled if we are building with --factory_install
  local enable_rootfs_verification=
  if [[ ${rootfs_verification_enabled} -eq ${FLAGS_TRUE} ]]; then
    enable_rootfs_verification="--enable_rootfs_verification"
  fi
  local enable_bootcache=
  if [[ ${bootcache_enabled} -eq ${FLAGS_TRUE} ]]; then
    enable_bootcache="--enable_bootcache"
  fi

  create_legacy_bootloader_templates.sh \
    --arch=${ARCH} \
    --board=${BOARD} \
    --image_type="${image_type}" \
    --to="${root_fs_dir}"/boot \
    --boot_args="${FLAGS_boot_args}" \
    --enable_serial="${FLAGS_enable_serial}" \
    --loglevel="${FLAGS_loglevel}" \
      ${enable_rootfs_verification} \
      ${enable_bootcache}



  # Zero rootfs free space to make it more compressible so auto-update
  # payloads become smaller
  zero_free_space "${root_fs_dir}"

  unmount_image
  trap - EXIT

  USE_DEV_KEYS="--use_dev_keys"

  if [[ ${skip_kernelblock_install} -ne 1 ]]; then
    # Place flags before positional args.
    ${SCRIPTS_DIR}/bin/cros_make_image_bootable "${BUILD_DIR}" \
      ${output_dev} ${USE_DEV_KEYS} --adjust_part="${FLAGS_adjust_part}"
  fi
}
create_base_image $1 $2 $3 $4
