#!/usr/bin/env bash
set -euo pipefail

die() { printf 'package-mediadl-unix: %s\n' "$*" >&2; exit 1; }
[ "$#" -eq 1 ] || die 'usage: package_mediadl_unix.sh <output-directory>'
output_dir="$1"
[ -n "$output_dir" ] || die 'output directory is empty'

case "$(uname -s)" in
  Darwin)
    platform=macos
    ytdlp_asset=yt-dlp_macos
    ffmpeg_repo=vanloctech/ffmpeg-macos
    case "$(uname -m)" in
      arm64) ffmpeg_asset=ffmpeg-macos-arm64.tar.gz ;;
      x86_64) ffmpeg_asset=ffmpeg-macos-x64.tar.gz ;;
      *) die "unsupported macOS architecture: $(uname -m)" ;;
    esac
    ;;
  Linux)
    platform=linux
    case "$(uname -m)" in
      x86_64|amd64) ytdlp_asset=yt-dlp_linux; ffmpeg_asset=ffmpeg-master-latest-linux64-gpl.tar.xz ;;
      aarch64|arm64) ytdlp_asset=yt-dlp_linux_aarch64; ffmpeg_asset=ffmpeg-master-latest-linuxarm64-gpl.tar.xz ;;
      *) die "unsupported Linux architecture: $(uname -m)" ;;
    esac
    ffmpeg_repo=yt-dlp/FFmpeg-Builds
    ;;
  *) die "this script only runs on macOS or Linux" ;;
esac

command -v curl >/dev/null 2>&1 || die 'curl is required'
command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 || die 'sha256sum or shasum is required'
command -v tar >/dev/null 2>&1 || die 'tar is required'

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "$output_dir"
for owned in yt-dlp yt-dlp_macos yt-dlp_linux yt-dlp_linux_aarch64 ffmpeg SOURCE.txt THIRD_PARTY_NOTICES.txt; do
  rm -f "$output_dir/$owned" "$output_dir/$owned.exe"
done

work="$(mktemp -d "${TMPDIR:-/tmp}/tdrive-mediadl.XXXXXX")"
trap 'rm -rf "$work"' EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print tolower($1)}'
  else
    shasum -a 256 "$1" | awk '{print tolower($1)}'
  fi
}

download() {
  curl --fail --location --retry 3 --silent --show-error "$1" -o "$2"
}

checksum_for() {
  local sums_file="$1" asset="$2" value
  value="$(awk -v wanted="$asset" '
    NF >= 2 {
      name=$NF; gsub(/^\*/, "", name); sub(/^.*\//, "", name)
      if (name == wanted && $1 ~ /^[0-9A-Fa-f]{64}$/) { print tolower($1); exit }
    }' "$sums_file")"
  [[ "$value" =~ ^[0-9a-f]{64}$ ]] || die "checksum manifest has no valid entry for $asset"
  printf '%s\n' "$value"
}

download_verified() {
  local url="$1" asset="$2" sums_url="$3" destination="$4" sums expected actual
  sums="$work/$asset.sums"
  download "$sums_url" "$sums"
  expected="$(checksum_for "$sums" "$asset")"
  download "$url" "$destination"
  actual="$(sha256_file "$destination")"
  [ "$actual" = "$expected" ] || die "$asset failed SHA-256 verification (expected $expected, got $actual)"
  printf '%s\n' "$expected"
}

ytdlp_url="https://github.com/yt-dlp/yt-dlp/releases/latest/download/$ytdlp_asset"
ytdlp_sums_url="https://github.com/yt-dlp/yt-dlp/releases/latest/download/SHA2-256SUMS"
ytdlp_archive="$work/$ytdlp_asset"
ytdlp_sha="$(download_verified "$ytdlp_url" "$ytdlp_asset" "$ytdlp_sums_url" "$ytdlp_archive")"

if [ "$platform" = macos ]; then
  ffmpeg_base="https://github.com/$ffmpeg_repo/releases/latest/download"
  ffmpeg_sums_url="$ffmpeg_base/$ffmpeg_asset.sha256"
else
  ffmpeg_base="https://github.com/$ffmpeg_repo/releases/latest/download"
  ffmpeg_sums_url="$ffmpeg_base/checksums.sha256"
fi
ffmpeg_archive="$work/$ffmpeg_asset"
ffmpeg_sha="$(download_verified "$ffmpeg_base/$ffmpeg_asset" "$ffmpeg_asset" "$ffmpeg_sums_url" "$ffmpeg_archive")"

ffmpeg_extract="$work/ffmpeg"
mkdir -p "$ffmpeg_extract"
tar -xf "$ffmpeg_archive" -C "$ffmpeg_extract"
ffmpeg_candidate=""
ffmpeg_count=0
while IFS= read -r candidate; do
  ffmpeg_candidate="$candidate"
  ffmpeg_count=$((ffmpeg_count + 1))
done < <(find "$ffmpeg_extract" -type f -name ffmpeg -print)
[ "$ffmpeg_count" -eq 1 ] || die "archive must contain exactly one executable named ffmpeg (found $ffmpeg_count)"

cp "$ytdlp_archive" "$output_dir/yt-dlp"
cp "$ffmpeg_candidate" "$output_dir/ffmpeg"
chmod 755 "$output_dir/yt-dlp" "$output_dir/ffmpeg"

ytdlp_version="$("$output_dir/yt-dlp" --version 2>&1 | head -n 1)" || die 'packaged yt-dlp failed to run'
ffmpeg_version="$("$output_dir/ffmpeg" -version 2>&1 | head -n 1)" || die 'packaged ffmpeg failed to run'

cat > "$output_dir/SOURCE.txt" <<EOF
TDrive $platform media downloader runtime

yt-dlp: $ytdlp_version
yt-dlp asset: $ytdlp_url
yt-dlp SHA-256: $ytdlp_sha

FFmpeg: $ffmpeg_version
FFmpeg asset: $ffmpeg_base/$ffmpeg_asset
FFmpeg archive SHA-256: $ffmpeg_sha

Each downloaded archive was verified against its upstream SHA-256 manifest before packaging.
EOF
cat > "$output_dir/THIRD_PARTY_NOTICES.txt" <<EOF
TDrive bundles the following third-party executables for $platform:

yt-dlp - Unlicense. The executable is downloaded from the official yt-dlp GitHub release.
Source and license: https://github.com/yt-dlp/yt-dlp

FFmpeg - GNU General Public License version 3 or later (GPL build).
Source and license: https://github.com/$ffmpeg_repo
FFmpeg project: https://ffmpeg.org/

This package does not alter either executable. Refer to the upstream projects for complete license texts and source-code offers.
EOF

printf 'package-mediadl-unix: verified %s and %s\n' "$ytdlp_version" "$ffmpeg_version"
printf 'package-mediadl-unix: bundled runtime into %s\n' "$output_dir"
