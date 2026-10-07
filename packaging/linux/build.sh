#!/bin/sh
# Builds gbe as an AppImage for Linux: one file that runs on most
# distributions, with its icon and its menu entry.
#
#	packaging/linux/build.sh v0.1.11 amd64 dist
#
# writes dist/gbe-v0.1.11-linux-amd64.AppImage. It needs appimagetool and
# the AppImage runtime of the architecture, given by APPIMAGETOOL and
# RUNTIME. gbe loads X11, OpenGL and ALSA from the system at run time, so
# the AppImage holds nothing else.
set -eu

version=${1:?usage: build.sh version amd64|arm64 outdir}
arch=${2:?usage: build.sh version amd64|arm64 outdir}
out=${3:?usage: build.sh version amd64|arm64 outdir}
case $arch in
amd64) appimage_arch=x86_64 ;;
arm64) appimage_arch=aarch64 ;;
*) echo "unknown architecture $arch" >&2; exit 2 ;;
esac

app=$(mktemp -d)/gbe.AppDir
trap 'rm -rf "$(dirname "$app")"' EXIT
mkdir -p "$app/usr/bin" "$app/usr/share/applications"
CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
	-ldflags "-s -w -X main.version=$version" -o "$app/usr/bin/gbe" ./cmd/gbe
cp packaging/linux/gbe.desktop "$app/"
cp packaging/linux/gbe.desktop "$app/usr/share/applications/"
for n in 256 512; do
	mkdir -p "$app/usr/share/icons/hicolor/${n}x$n/apps"
	cp "assets/icon/icon-$n.png" "$app/usr/share/icons/hicolor/${n}x$n/apps/gbe.png"
done
cp assets/icon/icon-256.png "$app/gbe.png"
ln -s gbe.png "$app/.DirIcon"
ln -s usr/bin/gbe "$app/AppRun"

mkdir -p "$out"
image="$out/gbe-$version-linux-$arch.AppImage"
ARCH=$appimage_arch "$APPIMAGETOOL" --appimage-extract-and-run --no-appstream \
	--runtime-file "$RUNTIME" "$app" "$image" >&2
echo "$image"
