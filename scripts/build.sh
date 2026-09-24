#!/bin/bash
# build.sh — build the aiul binary for both Mac architectures.
#
#   ./scripts/build.sh              # version from git
#   AIUL_VERSION=1.2.3 ./scripts/build.sh
#
# Output: dist/aiul, a universal binary that runs on Apple Silicon and Intel.
#
# A "universal binary" is one file containing both builds, joined by `lipo`, a
# tool that ships with Xcode's command line tools. macOS picks the right half at
# launch, so one package works on every Mac.
#
# Signing is NOT done here: see docs/PACKAGING.md. If AIUL_SIGN_IDENTITY is set,
# the binary is signed with it; otherwise the build is left unsigned and says so.

set -euo pipefail

cd "$(dirname "$0")/.."

AGENT_DIR="agent"
OUT_DIR="dist"
BINARY="$OUT_DIR/aiul"

# Version: whatever was asked for, else the git description, else a placeholder.
# With no tags in the repo, `git describe --always` is the short commit, which is
# identity enough; once there are tags it is the tag.
VERSION="${AIUL_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-unknown)}"

mkdir -p "$OUT_DIR"

echo "Building aiul $VERSION"

# -s -w drop the symbol table and DWARF debug info: a smaller binary, and nothing
# a customer's machine needs.
LDFLAGS="-s -w -X main.version=$VERSION"

for arch in arm64 amd64; do
  echo "   darwin/$arch"
  (cd "$AGENT_DIR" && CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "../$OUT_DIR/aiul-$arch" ./cmd/aiul)
done

lipo -create -output "$BINARY" "$OUT_DIR/aiul-arm64" "$OUT_DIR/aiul-amd64"
rm -f "$OUT_DIR/aiul-arm64" "$OUT_DIR/aiul-amd64"
chmod 755 "$BINARY"

# Windows x64: copy dist/aiul.exe to the Windows PC. Unsigned, like the darwin
# binary without AIUL_SIGN_IDENTITY; SmartScreen may warn on first run.
echo "   windows/amd64"
(cd "$AGENT_DIR" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -ldflags "$LDFLAGS" -o "../$OUT_DIR/aiul.exe" ./cmd/aiul)

# Linux, both architectures: dist/aiul-linux-amd64 and dist/aiul-linux-arm64.
# enroll-device.sh on the Linux machine picks the one for its CPU.
for arch in amd64 arm64; do
  echo "   linux/$arch"
  (cd "$AGENT_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "../$OUT_DIR/aiul-linux-$arch" ./cmd/aiul)
done

if [ -n "${AIUL_SIGN_IDENTITY:-}" ]; then
  # --options runtime turns on the hardened runtime, which notarization requires.
  echo "Signing with $AIUL_SIGN_IDENTITY"
  codesign --force --options runtime --timestamp \
    --sign "$AIUL_SIGN_IDENTITY" "$BINARY"
  codesign --verify --strict --verbose=2 "$BINARY"
else
  echo "NOT SIGNED (AIUL_SIGN_IDENTITY is not set). Fine for development;"
  echo "a real deployment needs a Developer ID — see docs/PACKAGING.md."
fi

echo
lipo -archs "$BINARY" | sed 's/^/architectures: /'
"$BINARY" version | sed 's/^/reports:       /'
echo "built:         $BINARY"
