# Copyright (c) 2011 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Shell function library for functions specific to creating test
# images from dev images.  This file also contains additional
# functions and initialization shared between build_image and
# mod_image_for_test.sh.
#

# Emerges chromeos-test onto the image.
emerge_chromeos_test() {
  # Determine the root dir for test packages.
  local root_dev_dir="${root_fs_dir}/usr/local"

  emerge_to_image --root="${root_fs_dir}" chromeos-test-root
  emerge_to_image --root="${root_dev_dir}" virtual/target-os-test
}

# Converts a dev image into a test or factory test image
# Takes as an arg the name of the image to be created.
mod_image_for_test () {
  local image_name="$1"

  trap "check_full_disk ; unmount_image ; delete_prompt" EXIT
  mount_image "${BUILD_DIR}/${image_name}" "${root_fs_dir}" "${stateful_fs_dir}"

  emerge_chromeos_test

  local mod_test_script="${SCRIPTS_DIR}/mod_for_test_scripts/test_setup.sh"
  # Run test setup script to modify the image
  sudo -E GCLIENT_ROOT="${GCLIENT_ROOT}" ROOT_FS_DIR="${root_fs_dir}" \
    STATEFUL_DIR="${stateful_fs_dir}" ARCH="${ARCH}" \
    BOARD_ROOT="${BOARD_ROOT}" BUILD_DIR="${BUILD_DIR}" \
    "${mod_test_script}"

  # Run depmod to recalculate the kernel module dependencies.
  run_depmod "${BOARD_ROOT}" "${root_fs_dir}"

  # Re-run ldconfig to fix /etc/ld.so.cache.
  run_ldconfig "${root_fs_dir}"

  unmount_image
  trap - EXIT

  if [[ ${skip_kernelblock_install} -ne 1 ]]; then
    # Now make it bootable with the flags from build_image.
    if should_build_image ${image_name}; then
      "${SCRIPTS_DIR}/bin/cros_make_image_bootable" "${BUILD_DIR}" \
         ${image_name} --force_developer_mode
    fi
  fi
}
