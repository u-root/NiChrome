# Copyright (c) 2012 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# Usage: run_lddtree <root> [args to lddtree] <files to process>
run_lddtree() {
  # Keep `local` decl split from assignment so return code is checked.
  local lddtree='/mnt/host/source/chromite/bin/lddtree'
  local root="$1"
  shift

  # Since we'll feed files via xargs, we need to extract the options
  # so we can pass it to the lddtree tool.
  local flags=()
  while [[ $# -gt 0 ]]; do
    case $1 in
    --) break ;;
    -*) flags+=( "$1" ) ;;
    *) break ;;
    esac
    shift
  done

  # In case the file list is too big, send it through xargs.
  # http://crbug.com/369314
  printf '%s\0' "$@" | xargs -0 \
    sudo "${lddtree}" -R "${root}" --skip-non-elfs "${flags[@]}"
}

# Usage: test_elf_deps <root> <files to check>
test_elf_deps() {
  # Keep `local` decl split from assignment so return code is checked.
  local f deps
  local root="$1"
  shift

  # We first check everything in one go.  We assume that it'll usually be OK,
  # so we make this the fast path.  If it does fail, we'll fall back to one at
  # a time so the error output is human readable.
  deps=$(run_lddtree "${root}" -l "$@") || return 1
  if echo "${deps}" | grep -q '^[^/]'; then
    error "test_elf_deps: Failed dependency check"
    for f in "$@"; do
      deps=$(run_lddtree "${root}" -l "${f}")
      if echo "${deps}" | grep -q '^[^/]'; then
        error "Package: $(qfile --root "${root}" -qCRv "${root}${f}")"
        error "$(run_lddtree "${root}" "${f}")"
      fi
    done
    return 1
  fi

  return 0
}

test_image_content() {
  local root="$1"
  local returncode=0

  # Keep `local` decl split from assignment so return code is checked.
  local libs

  # Check that all .so files, plus the binaries, have the appropriate
  # dependencies.  Need to use sudo as some files are set*id.
  # Exclude Chrome components, which are the only files in the lib directory.
  local components="${root}/opt/google/chrome/lib/*"
  libs=( $(sudo \
      find "${root}" -type f -name '*.so*' -not -name '*.so.debug' \
        -not -path "${components}" -printf '/%P\n') )
  if ! test_elf_deps "${root}" "${binaries[@]}" "${libs[@]}"; then
    returncode=1
  fi

  local blacklist_dirs=(
    "$root/usr/share/locale"
  )
  for dir in "${blacklist_dirs[@]}"; do
    if [ -d "$dir" ]; then
      error "test_image_content: Blacklisted directory found: $dir"
      returncode=1
    fi
  done

  # Check that /etc/localtime is a symbolic link pointing at
  # /var/lib/timezone/localtime.
  local localtime="$root/etc/localtime"
  if [ ! -L "$localtime" ]; then
    error "test_image_content: /etc/localtime is not a symbolic link"
    returncode=1
  else
    local dest=$(readlink "$localtime")
    if [ "$dest" != "/var/lib/timezone/localtime" ]; then
      error "test_image_content: /etc/localtime points at $dest"
      returncode=1
    fi
  fi

  return $returncode
}
