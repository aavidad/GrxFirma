#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

GOMOBILE_VERSION=${GOMOBILE_VERSION:-v0.0.0-20260217195705-b56b3793a9c4}
XCODEPROJ_VERSION=${XCODEPROJ_VERSION:-1.28.1}

if [[ $(uname -s) != Darwin ]]; then
    printf '%s\n' "ERROR: el toolchain iOS completo solo existe en macOS" >&2
    exit 1
fi
for command in xcodebuild xcrun go ruby gem python3; do
    command -v "$command" >/dev/null 2>&1 || { printf 'ERROR: falta %s\n' "$command" >&2; exit 1; }
done
xcodebuild -checkFirstLaunchStatus
xcrun --sdk iphoneos --show-sdk-path >/dev/null
XCODE_MAJOR=$(xcodebuild -version | awk 'NR == 1 { split($2, version, "."); print version[1] }')
if [[ ! "$XCODE_MAJOR" =~ ^[0-9]+$ || "$XCODE_MAJOR" -lt 16 ]]; then
    printf '%s\n' "ERROR: se requiere Xcode 16 o posterior" >&2
    exit 1
fi

gem install --user-install xcodeproj -v "$XCODEPROJ_VERSION" --no-document
GOBIN=${GOBIN:-"$(go env GOPATH)/bin"}
mkdir -p "$GOBIN"
GOBIN="$GOBIN" go install "golang.org/x/mobile/cmd/gomobile@$GOMOBILE_VERSION"
GOBIN="$GOBIN" go install "golang.org/x/mobile/cmd/gobind@$GOMOBILE_VERSION"
"$GOBIN/gomobile" init

printf 'Xcode: %s\n' "$(xcodebuild -version | tr '\n' ' ')"
printf 'Go: %s\n' "$(go version)"
printf 'xcodeproj: %s\n' "$(ruby -rxcodeproj -e 'print Xcodeproj::VERSION')"
printf '%s\n' "Toolchain iOS instalado para el usuario actual"
