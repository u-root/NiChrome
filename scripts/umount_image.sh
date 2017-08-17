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
#            0           1          PMBR (Boot GUID: 09ADDEBB-0333-4B48-8E1D-CB43C88C95DE)
#            1           1          Pri GPT header
#            2          32          Pri GPT table
#      2961408    12132352       1  Label: "STATE"
#                                   Type: Linux data
#                                   UUID: CC1A2390-0A9D-9D48-883B-626E7DD188CB
#        20480       32768       2  Label: "KERN-A"
#                                   Type: ChromeOS kernel
#                                   UUID: 71B947BB-2396-314B-8C78-2A62CBFC8024
#                                   Attr: priority=15 tries=15 successful=0 
#       319488     2641920       3  Label: "ROOT-A"
#                                   Type: ChromeOS rootfs
#                                   UUID: 5D3E9883-6405-B44A-9E79-E1125B5FDC9B
#        53248       32768       4  Label: "KERN-B"
#                                   Type: ChromeOS kernel
#                                   UUID: CC947588-A4F8-B64A-99DE-81243D71A4BE
#                                   Attr: priority=0 tries=0 successful=0 
#       315392        4096       5  Label: "ROOT-B"
#                                   Type: ChromeOS rootfs
#                                   UUID: 316ADC8E-FA80-194C-9C01-F562D280239F
#        16448           1       6  Label: "KERN-C"
#                                   Type: ChromeOS kernel
#                                   UUID: 6AD6BA67-5ED6-5C43-9DE1-238C7DA83B1A
#                                   Attr: priority=0 tries=0 successful=0 
#        16449           1       7  Label: "ROOT-C"
#                                   Type: ChromeOS rootfs
#                                   UUID: AA474C7E-3368-8047-9CC4-C6459C867D53
#        86016       32768       8  Label: "OEM"
#                                   Type: Linux data
#                                   UUID: 2B5DF82C-C3D0-4443-A90A-D2A3671D5C0E
#        16450           1       9  Label: "reserved"
#                                   Type: ChromeOS reserved
#                                   UUID: 23A9BEA8-8CCA-744C-9AD4-3408624F785D
#        16451           1      10  Label: "reserved"
#                                   Type: ChromeOS reserved
#                                   UUID: 56E34019-7E1F-D244-A74F-D594B637A18F
#           64       16384      11  Label: "RWFW"
#                                   Type: ChromeOS firmware
#                                   UUID: 529423BB-45F4-434B-846B-1A15A3ED987E
#       249856       65536      12  Label: "EFI-SYSTEM"
#                                   Type: EFI System Partition
#                                   UUID: 09ADDEBB-0333-4B48-8E1D-CB43C88C95DE
#                                   Attr: legacy_boot=1 
#     15126495          32          Sec GPT table
#     15126527           1          Sec GPT header
case ${PART:-1} in
1|"STATE")
if [[ -d dir_1 ]]; then
  (
  sudo umount dir_1 || :
  rmdir dir_1
  rm -f "dir_1_STATE"
  ) &
fi
esac
case ${PART:-2} in
2|"KERN-A")
if [[ -d dir_2 ]]; then
  (
  sudo umount dir_2 || :
  rmdir dir_2
  rm -f "dir_2_KERN-A"
  ) &
fi
esac
case ${PART:-3} in
3|"ROOT-A")
if [[ -d dir_3 ]]; then
  (
  sudo umount dir_3 || :
  rmdir dir_3
  rm -f "dir_3_ROOT-A"
  ) &
fi
esac
case ${PART:-4} in
4|"KERN-B")
if [[ -d dir_4 ]]; then
  (
  sudo umount dir_4 || :
  rmdir dir_4
  rm -f "dir_4_KERN-B"
  ) &
fi
esac
case ${PART:-5} in
5|"ROOT-B")
if [[ -d dir_5 ]]; then
  (
  sudo umount dir_5 || :
  rmdir dir_5
  rm -f "dir_5_ROOT-B"
  ) &
fi
esac
case ${PART:-6} in
6|"KERN-C")
esac
case ${PART:-7} in
7|"ROOT-C")
esac
case ${PART:-8} in
8|"OEM")
if [[ -d dir_8 ]]; then
  (
  sudo umount dir_8 || :
  rmdir dir_8
  rm -f "dir_8_OEM"
  ) &
fi
esac
case ${PART:-9} in
9|"reserved")
esac
case ${PART:-10} in
10|"reserved")
esac
case ${PART:-11} in
11|"RWFW")
if [[ -d dir_11 ]]; then
  (
  sudo umount dir_11 || :
  rmdir dir_11
  rm -f "dir_11_RWFW"
  ) &
fi
esac
case ${PART:-12} in
12|"EFI-SYSTEM")
if [[ -d dir_12 ]]; then
  (
  sudo umount dir_12 || :
  rmdir dir_12
  rm -f "dir_12_EFI-SYSTEM"
  ) &
fi
esac
wait
