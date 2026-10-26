#!/usr/bin/env bash
set -euo pipefail

# Install from the exact module already pinned by the application. Using
# `go install package@version` also queries the module's latest deprecation
# metadata; an invalid upstream latest tag must not break a pinned build.
module=github.com/wailsapp/wails/v3
version="$(go list -m -f '{{.Version}}' "$module")"
go mod download "$module@$version"
module_dir="$(go list -m -f '{{.Dir}}' "$module")"
test -n "$module_dir"
go -C "$module_dir" install ./cmd/wails3
