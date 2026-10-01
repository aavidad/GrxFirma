#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

GITHUB_ENV_FILE=
if [[ "${1:-}" == "--github-env" && $# -eq 2 ]]; then
    GITHUB_ENV_FILE=$2
elif [[ $# -ne 0 ]]; then
    printf '%s\n' \
        "Uso: install-toolchain.sh [--github-env /ruta/al/GITHUB_ENV]" >&2
    exit 2
fi

if [[ "$(uname -s)" != "Linux" || "$(uname -m)" != "x86_64" ]]; then
    printf '%s\n' "ERROR: este instalador reproducible soporta Linux x86_64" >&2
    exit 1
fi

BASE_DIR=${GRXFIRMA_ANDROID_TOOLCHAIN_DIR:-"$HOME/.cache/grxfirma-android"}
JDK_DIR="$BASE_DIR/jdk-17.0.19+10"
SDK_DIR="$BASE_DIR/android-sdk"
DOWNLOAD_DIR="$BASE_DIR/downloads"
JDK_URL=https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.19%2B10/OpenJDK17U-jdk_x64_linux_hotspot_17.0.19_10.tar.gz
JDK_SHA256=d8afc263758141a66e0e3aafc321e783f7016696f4eaea067d340a269037d331
TOOLS_URL=https://dl.google.com/android/repository/commandlinetools-linux-15859902_latest.zip
TOOLS_SHA256=4e4c464f145a7512b57d088ac6c278c03c9eea610886b35a5e0804e74eedf583

if [[ "${ACCEPT_ANDROID_SDK_LICENSES:-0}" != "1" ]]; then
    cat >&2 <<'EOF'
ERROR: la instalación del SDK requiere aceptar sus licencias.
Revise https://developer.android.com/studio/terms y repita con
ACCEPT_ANDROID_SDK_LICENSES=1 si está autorizado para aceptarlas.
EOF
    exit 1
fi

mkdir -p "$BASE_DIR" "$DOWNLOAD_DIR" "$SDK_DIR/cmdline-tools"

download_verified() {
    local url=$1
    local destination=$2
    local checksum=$3
    if [[ ! -f "$destination" ]] || ! printf '%s  %s\n' "$checksum" "$destination" | sha256sum -c - >/dev/null 2>&1; then
        curl -fL --retry 3 "$url" -o "$destination"
    fi
    printf '%s  %s\n' "$checksum" "$destination" | sha256sum -c -
}

download_verified "$JDK_URL" "$DOWNLOAD_DIR/temurin17.tar.gz" "$JDK_SHA256"
download_verified "$TOOLS_URL" "$DOWNLOAD_DIR/android-commandline-tools.zip" "$TOOLS_SHA256"

if [[ ! -x "$JDK_DIR/bin/javac" ]]; then
    mkdir -p "$JDK_DIR"
    tar -xzf "$DOWNLOAD_DIR/temurin17.tar.gz" --strip-components=1 -C "$JDK_DIR"
fi

if [[ ! -x "$SDK_DIR/cmdline-tools/latest/bin/sdkmanager" ]]; then
    TEMP_TOOLS=$(mktemp -d "${TMPDIR:-/tmp}/grxfirma-sdk.XXXXXX")
    trap 'rm -rf -- "$TEMP_TOOLS"' EXIT HUP INT TERM
    unzip -q "$DOWNLOAD_DIR/android-commandline-tools.zip" -d "$TEMP_TOOLS"
    mkdir -p "$SDK_DIR/cmdline-tools/latest"
    cp -a "$TEMP_TOOLS/cmdline-tools/." "$SDK_DIR/cmdline-tools/latest/"
fi

export JAVA_HOME="$JDK_DIR"
export ANDROID_HOME="$SDK_DIR"
export ANDROID_SDK_ROOT="$SDK_DIR"
export PATH="$JAVA_HOME/bin:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$PATH"

set +o pipefail
yes | sdkmanager --licenses >/dev/null
license_status=${PIPESTATUS[1]}
set -o pipefail
if [[ "$license_status" -ne 0 ]]; then
    printf '%s\n' "ERROR: sdkmanager no pudo registrar las licencias" >&2
    exit "$license_status"
fi
sdkmanager \
    "platform-tools" \
    "platforms;android-36" \
    "build-tools;36.0.0" \
    "ndk;28.2.13676358"

if [[ "${INSTALL_ANDROID_EMULATOR:-0}" == "1" ]]; then
    sdkmanager \
        "emulator" \
        "system-images;android-36;google_apis_playstore;x86_64"
    if ! avdmanager list avd -c | grep -Fxq grxfirma_api36; then
        printf 'no\n' | avdmanager create avd \
            --force \
            --name grxfirma_api36 \
            --package "system-images;android-36;google_apis_playstore;x86_64" \
            --device pixel_6
    fi
fi

if [[ -n "$GITHUB_ENV_FILE" ]]; then
    if [[ -L "$GITHUB_ENV_FILE" || ! -f "$GITHUB_ENV_FILE" || ! -w "$GITHUB_ENV_FILE" ]]; then
        printf '%s\n' "ERROR: GITHUB_ENV no es un fichero regular escribible" >&2
        exit 1
    fi
    for value in "$JAVA_HOME" "$ANDROID_HOME" "$PATH"; do
        if [[ "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
            printf '%s\n' "ERROR: el entorno Android contiene saltos de línea" >&2
            exit 1
        fi
    done
    {
        printf 'JAVA_HOME=%s\n' "$JAVA_HOME"
        printf 'ANDROID_HOME=%s\n' "$ANDROID_HOME"
        printf 'ANDROID_SDK_ROOT=%s\n' "$ANDROID_SDK_ROOT"
        printf 'PATH=%s\n' "$PATH"
    } >> "$GITHUB_ENV_FILE"
fi

printf 'export JAVA_HOME=%q\n' "$JAVA_HOME"
printf 'export ANDROID_HOME=%q\n' "$ANDROID_HOME"
printf 'export ANDROID_SDK_ROOT=%q\n' "$ANDROID_SDK_ROOT"
literal_dollar='$'
printf 'export PATH=%q:%s\n' \
    "$JAVA_HOME/bin:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools" \
    "${literal_dollar}PATH"
