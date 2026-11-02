#!/usr/bin/env bash
# Build release archives for every supported platform.
#
#   scripts/release-build.sh VERSION [OUTDIR]
#
# Produces OUTDIR/modhak_VERSION_OS_ARCH.tar.gz (zip on Windows) and
# OUTDIR/SHA256SUMS. Used by .github/workflows/release.yml and `make dist`.
# Needs: go, tar, zip, sha256sum. Binaries are static (CGO off).
set -euo pipefail

version="${1:?usage: release-build.sh VERSION [OUTDIR]}"
out="${2:-dist}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

platforms="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"

rm -rf "$out"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

cd "$root"
for platform in $platforms; do
  os="${platform%/*}"
  arch="${platform#*/}"
  name="modhak_${version}_${os}_${arch}"
  dir="$stage/$name"
  bin="modhak"
  [ "$os" = "windows" ] && bin="modhak.exe"

  mkdir -p "$dir/configs"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=${version}" -o "$dir/$bin" ./cmd/modhak
  cp LICENSE README.md "$dir/"
  cp configs/modhak.example.yaml "$dir/configs/"

  if [ "$os" = "windows" ]; then
    (cd "$stage" && zip -q -r "$out/$name.zip" "$name")
  else
    tar -C "$stage" -czf "$out/$name.tar.gz" "$name"
  fi
  echo "built $name"
done

(cd "$out" && sha256sum modhak_* > SHA256SUMS)
echo "wrote $(ls "$out" | wc -l) files to $out"
