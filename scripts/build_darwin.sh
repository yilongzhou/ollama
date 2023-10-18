#!/bin/sh

set -eu

export VERSION=${VERSION:-0.0.0}
export GOFLAGS="'-ldflags=-w -s \"-X=github.com/jmorganca/ollama/version.Version=$VERSION\" \"-X=github.com/jmorganca/ollama/server.mode=release\"'"

mkdir -p dist

for TARGETARCH in arm64 amd64; do
    CGO_ENABLED=1 GOOS=darwin GOARCH=$TARGETARCH go generate ./...
    CGO_ENABLED=1 GOOS=darwin GOARCH=$TARGETARCH go build -o dist/ollama-darwin-$TARGETARCH
done

# create a universal binary
lipo -create -output dist/ollama-darwin dist/ollama-darwin-*
rm -f dist/ollama-darwin-*
chmod +x dist/ollama-darwin

# create the mac app
rm -rf dist/Ollama.app
cp -R app/Ollama.app dist/
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" dist/Ollama.app/Contents/Info.plist
cp dist/ollama-darwin dist/Ollama.app/Contents/MacOS/Ollama

# sign and notarize the app
codesign -f --timestamp --deep --options=runtime --sign "$APPLE_IDENTITY" --identifier ai.ollama.ollama dist/Ollama.app
ditto -c -k --keepParent dist/Ollama.app dist/Ollama-darwin.zip
rm -rf dist/Ollama.app
xcrun notarytool submit dist/Ollama-darwin.zip --wait --timeout 10m --apple-id $APPLE_ID --password $APPLE_PASSWORD --team-id $APPLE_TEAM_ID
unzip dist/Ollama-darwin.zip -d dist
rm -f dist/Ollama-darwin.zip
xcrun stapler staple "dist/Ollama.app"
ditto -c -k --keepParent dist/Ollama.app dist/Ollama-darwin.zip
rm -rf dist/Ollama.app

# sign and notarize the binary
codesign -f --timestamp --sign "$APPLE_IDENTITY" --identifier ai.ollama.ollama --options=runtime dist/ollama-darwin
ditto -c -k dist/ollama-darwin dist/binary.zip
rm -rf dist/ollama-darwin
xcrun notarytool submit dist/binary.zip --wait --timeout 10m --apple-id $APPLE_ID --password $APPLE_PASSWORD --team-id $APPLE_TEAM_ID
unzip dist/binary.zip -d dist
rm -f dist/binary.zip
