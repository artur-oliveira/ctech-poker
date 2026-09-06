#!/bin/sh
# Installs the CTech Poker CLI. Downloads the latest `vX.Y.Z` release
# asset for this OS/arch and drops the `poker` binary into $PREFIX/bin
# (default ~/.local/bin).
#
#   curl -fsSL https://raw.githubusercontent.com/artur-oliveira/ctech-poker/main/cli/install.sh | sh
#
set -eu

REPO="artur-oliveira/ctech-poker"
PREFIX="${PREFIX:-$HOME/.local}"
API="https://api.github.com/repos/$REPO/releases"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux | darwin) ext=tar.gz ;;
  mingw* | msys* | cygwin*) os=windows; ext=zip ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

# Newest stable release whose tag is a semantic CLI version.
release_tag=$(curl -fsSL "$API?per_page=30" \
  | grep -o '"tag_name": *"v[0-9][0-9.]*"' \
  | head -n1 | sed 's/.*"\([^"]*\)"/\1/')
if [ -z "$release_tag" ]; then
  echo "no vX.Y.Z release found for $REPO" >&2
  exit 1
fi

version=${release_tag#v}
asset="poker_${version}_${os}_${arch}.${ext}"
url="https://github.com/$REPO/releases/download/$release_tag/$asset"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "downloading $asset ..."
curl -fsSL "$url" -o "$tmp/$asset"

case "$ext" in
  tar.gz) tar -xzf "$tmp/$asset" -C "$tmp" ;;
  zip) unzip -q "$tmp/$asset" -d "$tmp" ;;
esac

mkdir -p "$PREFIX/bin"
install -m 0755 "$tmp/poker" "$PREFIX/bin/poker"
echo "installed poker $version to $PREFIX/bin/poker"

case ":$PATH:" in
  *":$PREFIX/bin:"*) ;;
  *) echo "note: add $PREFIX/bin to your PATH" ;;
esac
