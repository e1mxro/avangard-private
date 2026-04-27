#!/usr/bin/env bash
# Builds avmobile.aar from the Go source via gomobile.
# Output: mobile/android/app/libs/avmobile.aar
#
# Requirements (set via env or auto-detected from GitHub Actions):
#   ANDROID_HOME    — Android SDK location
#   ANDROID_NDK_HOME — Android NDK location
#   JAVA_HOME       — JDK 17+
#
# Usage:
#   ./mobile/scripts/build-aar.sh
set -euo pipefail

cd "$(dirname "$0")/../.."

OUT_DIR="mobile/android/app/libs"
OUT_AAR="$OUT_DIR/avmobile.aar"
mkdir -p "$OUT_DIR"

if ! command -v gomobile >/dev/null 2>&1; then
    echo "Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@latest
    go install golang.org/x/mobile/cmd/gobind@latest
fi

# Make sure `golang.org/x/mobile` is in the module graph. The /bind package
# only compiles for specific GOOS/GOARCH so we keep an importer file in
# mobile/avmobile/tools.go behind a `tools` build tag.
if ! grep -q 'golang.org/x/mobile' go.mod; then
    go get golang.org/x/mobile@latest
fi
go mod tidy

# gomobile init prepares the NDK-aware Go workspace. Idempotent.
gomobile init || true

echo "Building avmobile.aar..."
# Without -javapkg, gomobile produces classes under `go.<pkgname>`. We use the
# default so that the Kotlin import is `go.avmobile.Avmobile`.
gomobile bind \
    -target=android \
    -androidapi 24 \
    -o "$OUT_AAR" \
    ./mobile/avmobile

echo "Built: $OUT_AAR"
ls -la "$OUT_AAR"
