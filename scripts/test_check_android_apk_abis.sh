#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root/scripts/check_android_apk_abis.sh"
work="$(mktemp -d /tmp/tdrive-apk-abis.XXXXXX)"
trap 'rm -rf "$work"' EXIT

# make_apk <name> <entry>...: zip the listed (empty) entries into <name>.apk
make_apk() {
  local name="$1" entry
  shift
  rm -rf "$work/stage"
  mkdir -p "$work/stage"
  for entry in "$@"; do
    mkdir -p "$work/stage/$(dirname "$entry")"
    : > "$work/stage/$entry"
  done
  (cd "$work/stage" && zip -qD "$work/$name.apk" "$@")
}

expect_pass() {
  local apk="$1"
  shift
  if ! bash "$script" "$work/$apk.apk" "$@" > /dev/null 2>&1; then
    echo "expected $apk to pass for ABIs: $*" >&2
    exit 1
  fi
}

expect_fail() {
  local apk="$1"
  shift
  if bash "$script" "$work/$apk.apk" "$@" > /dev/null 2>&1; then
    echo "expected $apk to fail for ABIs: $*" >&2
    exit 1
  fi
}

make_apk arm_only \
  lib/arm64-v8a/libwails.so lib/arm64-v8a/libpython.so lib/arm64-v8a/libffmpeg.so \
  classes.dex
make_apk arm_with_x86_deps_no_wails \
  lib/arm64-v8a/libwails.so lib/arm64-v8a/libpython.so \
  lib/x86_64/libpython.so lib/x86_64/libffmpeg.so classes.dex
make_apk arm_with_x86_wails \
  lib/arm64-v8a/libwails.so lib/x86_64/libwails.so classes.dex
make_apk fat_both_wails \
  lib/arm64-v8a/libwails.so lib/arm64-v8a/libpython.so \
  lib/x86_64/libwails.so lib/x86_64/libpython.so classes.dex
make_apk no_wails \
  lib/arm64-v8a/libpython.so classes.dex

expect_pass arm_only arm64-v8a
expect_fail arm_with_x86_deps_no_wails arm64-v8a
expect_fail arm_with_x86_wails arm64-v8a
expect_pass fat_both_wails arm64-v8a x86_64
expect_fail fat_both_wails arm64-v8a
expect_fail no_wails arm64-v8a
expect_fail arm_only arm64-v8a x86_64

echo "Android APK ABI guard tests passed"
