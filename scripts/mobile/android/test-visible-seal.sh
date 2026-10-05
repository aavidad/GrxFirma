#!/usr/bin/env bash
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/../../.." && pwd)
export GRXFIRMA_ANDROID_TEST_CLASS=io.github.aavidad.grxfirma.android.CoreVisibleSealSigningTest
export GRXFIRMA_ANDROID_PDF_ARTIFACT=firma-android-qa-sello.pdf
export GRXFIRMA_ANDROID_TEST_ONLY_PDF=1
exec "$SCRIPT_DIR/test-production-signing.sh" "${1:-$ROOT_DIR/build/android-visible-seal-smoke}"
