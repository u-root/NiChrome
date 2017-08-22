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

#        start        size    part  contents
#            0           1          PMBR (Boot GUID: 85B23BBF-452E-7648-BF1B-338B5366FE32)
#            1           1          Pri GPT header
#            2          32          Pri GPT table
#      2961408    12132352       1  Label: "STATE"
#                                   Type: Linux data
#                                   UUID: C64389E7-762F-6741-8726-0E0BF46D0211
#        20480       32768       2  Label: "KERN-A"
#                                   Type: ChromeOS kernel
#                                   UUID: E3BB7159-156C-6145-B9BD-AE802ABCD369
#                                   Attr: priority=15 tries=15 successful=0 
#       319488     2641920       3  Label: "ROOT-A"
#                                   Type: ChromeOS rootfs
#                                   UUID: F6DFD34E-05B2-5040-9CA7-F4915BE29DE5
#        53248       32768       4  Label: "KERN-B"
#                                   Type: ChromeOS kernel
#                                   UUID: 2F598542-05F7-A942-88B5-5F54A3BDAFA0
#                                   Attr: priority=0 tries=0 successful=0 
#       315392        4096       5  Label: "ROOT-B"
#                                   Type: ChromeOS rootfs
#                                   UUID: 6CFD9C03-70DD-8640-9B4A-AC06DFBC4BDC
#        16448           1       6  Label: "KERN-C"
#                                   Type: ChromeOS kernel
#                                   UUID: 079555E1-F98E-2A4F-8E75-0F2E49503108
#                                   Attr: priority=0 tries=0 successful=0 
#        16449           1       7  Label: "ROOT-C"
#                                   Type: ChromeOS rootfs
#                                   UUID: 2011FAC4-EB65-AD46-8D67-25BB0049A5C4
#        86016       32768       8  Label: "OEM"
#                                   Type: Linux data
#                                   UUID: F42F11A2-29E8-9F42-B078-336C2693A945
#        16450           1       9  Label: "reserved"
#                                   Type: ChromeOS reserved
#                                   UUID: 7B1D007B-13EA-5442-A148-06EFA9995904
#        16451           1      10  Label: "reserved"
#                                   Type: ChromeOS reserved
#                                   UUID: 11898E9B-F997-8547-B415-E81E7853D2EE
#           64       16384      11  Label: "RWFW"
#                                   Type: ChromeOS firmware
#                                   UUID: FBD045B9-32FF-6843-A9BC-527EE7D17B53
#       249856       65536      12  Label: "EFI-SYSTEM"
#                                   Type: EFI System Partition
#                                   UUID: 85B23BBF-452E-7648-BF1B-338B5366FE32
#                                   Attr: legacy_boot=1 
#     15126495          32          Sec GPT table
#     15126527           1          Sec GPT header
case ${PART:-1} in
1|"STATE")
(
mkdir -p dir_1
m=( sudo mount -o loop,offset=1516240896,sizelimit=6211764224 "${TARGET}" dir_1 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_1
    exit 0
  fi
fi
ln -sfT dir_1 "dir_1_STATE"
) &
esac
case ${PART:-2} in
2|"KERN-A")
(
mkdir -p dir_2
m=( sudo mount -o loop,offset=10485760,sizelimit=16777216 "${TARGET}" dir_2 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_2
    exit 0
  fi
fi
ln -sfT dir_2 "dir_2_KERN-A"
) &
esac
case ${PART:-3} in
3|"ROOT-A")
(
mkdir -p dir_3
m=( sudo mount -o loop,offset=163577856,sizelimit=1352663040 "${TARGET}" dir_3 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_3
    exit 0
  fi
fi
ln -sfT dir_3 "dir_3_ROOT-A"
) &
esac
case ${PART:-4} in
4|"KERN-B")
(
mkdir -p dir_4
m=( sudo mount -o loop,offset=27262976,sizelimit=16777216 "${TARGET}" dir_4 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_4
    exit 0
  fi
fi
ln -sfT dir_4 "dir_4_KERN-B"
) &
esac
case ${PART:-5} in
5|"ROOT-B")
(
mkdir -p dir_5
m=( sudo mount -o loop,offset=161480704,sizelimit=2097152 "${TARGET}" dir_5 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_5
    exit 0
  fi
fi
ln -sfT dir_5 "dir_5_ROOT-B"
) &
esac
case ${PART:-6} in
6|"KERN-C")
esac
case ${PART:-7} in
7|"ROOT-C")
esac
case ${PART:-8} in
8|"OEM")
(
mkdir -p dir_8
m=( sudo mount -o loop,offset=44040192,sizelimit=16777216 "${TARGET}" dir_8 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_8
    exit 0
  fi
fi
ln -sfT dir_8 "dir_8_OEM"
) &
esac
case ${PART:-9} in
9|"reserved")
esac
case ${PART:-10} in
10|"reserved")
esac
case ${PART:-11} in
11|"RWFW")
(
mkdir -p dir_11
m=( sudo mount -o loop,offset=32768,sizelimit=8388608 "${TARGET}" dir_11 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_11
    exit 0
  fi
fi
ln -sfT dir_11 "dir_11_RWFW"
) &
esac
case ${PART:-12} in
12|"EFI-SYSTEM")
(
mkdir -p dir_12
m=( sudo mount -o loop,offset=127926272,sizelimit=33554432 "${TARGET}" dir_12 )
if ! "${m[@]}"; then
  if ! "${m[@]}" -o ro; then
    rmdir dir_12
    exit 0
  fi
fi
ln -sfT dir_12 "dir_12_EFI-SYSTEM"
) &
esac
wait
