#!/bin/bash
# package.sh — build the installer package a managed Mac can be given.
#
#   ./scripts/build.sh && ./scripts/package.sh
#
# Output: dist/aiul-<version>.pkg
#
# `pkgbuild` is Apple's own tool and needs no developer account. The package it
# produces installs the binary and then runs packaging/scripts/postinstall, which
# calls `aiul install --apply --yes`.
#
# UNSIGNED unless AIUL_INSTALLER_IDENTITY is set. An unsigned package installs
# fine with `sudo installer`, but Gatekeeper warns on a double-click and most MDMs
# refuse to push it — see docs/PACKAGING.md.

set -euo pipefail

cd "$(dirname "$0")/.."

OUT_DIR="dist"
BINARY="$OUT_DIR/aiul"
IDENTIFIER="com.aiul.agent"
VERSION="${AIUL_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-unknown)}"
PKG="$OUT_DIR/aiul-$VERSION.pkg"

if [ ! -x "$BINARY" ]; then
  echo "No $BINARY. Run ./scripts/build.sh first." >&2
  exit 1
fi

# The payload is assembled in a temporary root that mirrors the destination
# filesystem, which is how pkgbuild decides where files land.
ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT

install -d -m 755 "$ROOT/usr/local/bin"
install -m 755 "$BINARY" "$ROOT/usr/local/bin/aiul"

# Scripts have to be executable and owned by root to be run by the installer.
SCRIPTS="$(mktemp -d)"
trap 'rm -rf "$ROOT" "$SCRIPTS"' EXIT
install -m 755 packaging/scripts/postinstall "$SCRIPTS/postinstall"

echo "Packaging $IDENTIFIER $VERSION"

# Clear any quarantine or download attributes picked up along the way. Note that
# com.apple.provenance cannot be removed — macOS adds it to every executable — so
# `pkgutil --payload-files` will still list AppleDouble "._aiul" entries. Those are
# how the payload encodes extended attributes; the installer restores them as
# attributes on the real file. `pkgutil --expand-full` confirms the payload holds
# exactly one file. Nothing to chase.
xattr -cr "$ROOT" "$SCRIPTS" >/dev/null 2>&1 || true

# pkgbuild writes "write: Permission denied" to stderr once per file while copying
# extended attributes it cannot copy — com.apple.provenance, which macOS adds to
# every executable and will not let anyone remove. The package it produces is
# correct (verified with `pkgutil --expand-full`: one file, right mode), so those
# lines are filtered out rather than left looking like a failure. Anything else
# pkgbuild says still comes through, and its exit status is still what decides.
PKGBUILD_ERR="$(mktemp)"
set +e
pkgbuild \
  --root "$ROOT" \
  --scripts "$SCRIPTS" \
  --identifier "$IDENTIFIER" \
  --version "$VERSION" \
  --install-location / \
  --ownership recommended \
  "$PKG" 2>"$PKGBUILD_ERR"
PKGBUILD_STATUS=$?
set -e
grep -v '^write: Permission denied$' "$PKGBUILD_ERR" >&2 || true
rm -f "$PKGBUILD_ERR"

if [ "$PKGBUILD_STATUS" -ne 0 ]; then
  echo "pkgbuild failed with status $PKGBUILD_STATUS" >&2
  exit "$PKGBUILD_STATUS"
fi

if [ -n "${AIUL_INSTALLER_IDENTITY:-}" ]; then
  echo "Signing with $AIUL_INSTALLER_IDENTITY"
  SIGNED="$OUT_DIR/aiul-$VERSION-signed.pkg"
  productsign --sign "$AIUL_INSTALLER_IDENTITY" "$PKG" "$SIGNED"
  mv "$SIGNED" "$PKG"
  pkgutil --check-signature "$PKG"
else
  echo "NOT SIGNED (AIUL_INSTALLER_IDENTITY is not set)."
  echo "Installs with 'sudo installer'; Gatekeeper warns on a double-click and"
  echo "most MDMs will refuse it. See docs/PACKAGING.md."
fi

echo
echo "built:    $PKG"
echo "contents:"
# The ._ entries are extended-attribute encoding, not files. See the note above.
pkgutil --payload-files "$PKG" | grep -v '/\._' | sed 's/^/          /'
echo
echo "Install with:   sudo installer -pkg $PKG -target /"
echo "Then check:     aiul status        (log: /var/log/aiul-install.log)"
echo "Remove with:    sudo aiul uninstall"
