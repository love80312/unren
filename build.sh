#!/usr/bin/env bash
# Build UnRen-Go for all supported platforms.
# The GitHub Actions workflow uploads the generated binaries as an artifact.

set -Eeuo pipefail

OUTPUT_DIR="dist"
BINARY_NAME="unren-go"

# GOOS GOARCH output suffix
TARGETS=(
    "windows amd64 windows-x64.exe"
    "windows 386 windows-x86.exe"
    "windows arm64 windows-arm64.exe"
    "linux amd64 linux-x64"
    "linux 386 linux-x86"
    "linux arm64 linux-arm64"
    "linux arm linux-arm"
    "darwin amd64 macos-x64"
    "darwin arm64 macos-arm64"
)

mkdir -p "$OUTPUT_DIR"

# Remove previous outputs so stale binaries cannot be uploaded after a partial build.
rm -f "$OUTPUT_DIR"/"$BINARY_NAME"-*

echo "Building UnRen-Go for all supported platforms..."
echo

for target in "${TARGETS[@]}"; do
    read -r goos goarch suffix <<< "$target"
    output="$OUTPUT_DIR/$BINARY_NAME-$suffix"

    echo "Building $output ($goos/$goarch)..."

    if ! env CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
        go build -trimpath -ldflags="-s -w" -o "$output" .; then
        rm -f "$output"
        echo "ERROR: Build failed for $goos/$goarch." >&2
        exit 1
    fi

    size=$(wc -c < "$output" | tr -d '[:space:]')
    echo "OK: $output ($size bytes)"
    echo
done

echo "Build completed successfully."
echo "Generated binaries:"
ls -lh "$OUTPUT_DIR"
