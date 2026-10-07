#!/bin/sh
# Builds gbe.app, for Intel and Apple Silicon, and the disk image that
# holds it, on macOS:
#
#	packaging/macos/build.sh v0.1.11 dist
#
# writes dist/gbe.app and dist/gbe-v0.1.11-macos-universal.dmg. The app is
# signed ad hoc only: without an Apple Developer ID, macOS asks to confirm
# the first launch (see the README).
set -eu

version=${1:?usage: build.sh version outdir}
out=${2:?usage: build.sh version outdir}
# The bundle version is numbers only: 0.0.0 for a build that is not a
# release (e.g. a commit hash).
bundle_version=$(echo "$version" | sed -n 's/^v\{0,1\}\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\).*/\1/p')
bundle_version=${bundle_version:-0.0.0}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
for arch in amd64 arm64; do
	CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$work/gbe-$arch" ./cmd/gbe
done

mkdir -p "$out"
app="$out/gbe.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create -output "$app/Contents/MacOS/gbe" "$work/gbe-amd64" "$work/gbe-arm64"
cp assets/icon/gbe.icns "$app/Contents/Resources/"
sed "s/@VERSION@/$bundle_version/g" packaging/macos/Info.plist > "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist" >/dev/null
codesign --force --sign - "$app"
codesign --verify --strict "$app"

# The disk image shows the app next to a link to Applications, to drag it
# there.
mkdir "$work/dmg"
cp -R "$app" "$work/dmg/"
ln -s /Applications "$work/dmg/Applications"
dmg="$out/gbe-$version-macos-universal.dmg"
rm -f "$dmg"
hdiutil create -quiet -volname gbe -srcfolder "$work/dmg" -format UDZO "$dmg"
echo "$dmg"
