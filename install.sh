#!/bin/sh
# Installs the ine CLI from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/gverdugo-dev/ine/main/install.sh | sh
#
# Environment:
#   INE_VERSION      tag to install (default: latest release)
#   INE_INSTALL_DIR  target directory (default: /usr/local/bin if writable,
#                    otherwise ~/.local/bin)
set -eu

repo="gverdugo-dev/ine"

fail() {
	echo "ine install: $*" >&2
	exit 1
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "unsupported system $(uname -s); on Windows download the zip from https://github.com/$repo/releases" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported architecture $(uname -m)" ;;
esac

if [ -n "${INE_VERSION:-}" ]; then
	base="https://github.com/$repo/releases/download/$INE_VERSION"
else
	base="https://github.com/$repo/releases/latest/download"
fi

if [ -n "${INE_INSTALL_DIR:-}" ]; then
	dir="$INE_INSTALL_DIR"
elif [ -w /usr/local/bin ]; then
	dir=/usr/local/bin
else
	dir="$HOME/.local/bin"
fi

archive="ine_${os}_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $archive"
curl -fsSL "$base/$archive" -o "$tmp/$archive" || fail "download failed: $base/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "download failed: $base/checksums.txt"

expected="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d ' ' -f 1)"
[ -n "$expected" ] || fail "no checksum for $archive"
if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)"
else
	actual="$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" ine
mkdir -p "$dir"
mv "$tmp/ine" "$dir/ine"
chmod 755 "$dir/ine"

echo "Installed $("$dir/ine" version) in $dir/ine"
case ":$PATH:" in
*":$dir:"*) ;;
*) echo "Add $dir to your PATH to run ine from anywhere." ;;
esac
