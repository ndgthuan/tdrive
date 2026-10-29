#!/usr/bin/env bash
# Usage: check_android_apk_abis.sh <apk> <abi>...
#
# Fails unless the APK's lib/ directories are exactly the listed ABIs and each
# of them carries libwails.so. A lib/<abi>/ without libwails.so is what made
# x86_64 emulators pick an ABI the app cannot load (UnsatisfiedLinkError on
# WailsBridge), so every packaged ABI must be one the Go library was built for.
set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <apk> <abi>..." >&2
  exit 2
fi

apk="$1"
shift
expected=" $* "

entries="$(unzip -Z1 "$apk")"
status=0

packaged="$(sed -nE 's#^lib/([^/]+)/.*#\1#p' <<< "$entries" | sort -u)"
while IFS= read -r abi; do
  [ -n "$abi" ] || continue
  if [[ "$expected" != *" $abi "* ]]; then
    echo "APK packages unexpected ABI lib/$abi/" >&2
    status=1
  fi
  if ! grep -Fxq "lib/$abi/libwails.so" <<< "$entries"; then
    echo "APK has lib/$abi/ but no lib/$abi/libwails.so" >&2
    status=1
  fi
done <<< "$packaged"

for abi in "$@"; do
  if ! grep -Fxq "lib/$abi/libwails.so" <<< "$entries"; then
    echo "APK is missing lib/$abi/libwails.so" >&2
    status=1
  fi
done

if [ "$status" -ne 0 ]; then
  echo "Packaged ABIs: $(tr '\n' ' ' <<< "$packaged")" >&2
  exit "$status"
fi
echo "APK ABIs are $* and each ships libwails.so"
