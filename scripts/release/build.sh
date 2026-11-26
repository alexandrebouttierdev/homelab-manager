#!/usr/bin/env bash
# Builds the release archives for Linux amd64 and arm64, and their checksums.
# Usage: scripts/release/build.sh <version> <output directory>
set -euo pipefail

version="$1"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version must be MAJOR.MINOR.PATCH, got '$version'" >&2
  exit 1
fi
root=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$2"
out=$(cd "$2" && pwd)

# The interface does not depend on the architecture: build it once.
VITE_APP_VERSION="$version" npm --prefix "$root/frontend" run build

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
for arch in amd64 arm64; do
  name="monoops_${version}_linux_${arch}"
  dir="$stage/$name"
  mkdir -p "$dir/bin" "$dir/frontend"
  for command in manager docker-proxy backup-runner; do
    # The Manager is the product itself: its binary is plain "monoops".
    binary="monoops-$command"
    [[ "$command" == manager ]] && binary=monoops
    # Pure Go (modernc SQLite): no C toolchain, static binaries.
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go -C "$root/backend" build -trimpath \
      -ldflags "-s -w -X main.version=$version" \
      -o "$dir/bin/$binary" "./cmd/$command"
  done
  cp -R "$root/frontend/dist" "$dir/frontend/dist"
  cp -R "$root/deployments" "$dir/deployments"
  cp "$root/README.md" "$dir/"
  tar -C "$stage" --owner=0 --group=0 --numeric-owner -czf "$out/$name.tar.gz" "$name"
done
(cd "$out" && sha256sum monoops_*.tar.gz > SHA256SUMS)
