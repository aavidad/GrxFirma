#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Static guard against weakening the tag-triggered official release workflow."""

from __future__ import annotations

import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github" / "workflows" / "release.yml"
LOCAL_OFFICIAL = ROOT / "scripts" / "release" / "build-official-release.sh"
ACTION_RE = re.compile(r"(?m)^\s*uses:\s*([^\s#]+)")


def job_block(text: str, name: str) -> str:
    match = re.search(
        rf"(?ms)^  {re.escape(name)}:\s*\n(.*?)(?=^  [A-Za-z0-9_-]+:\s*(?:#.*)?$|\Z)",
        text,
    )
    if not match:
        raise ValueError(f"missing required release job: {name}")
    return match.group(0)


def require(block: str, needle: str, context: str) -> None:
    if needle not in block:
        raise ValueError(f"{context} does not contain required control: {needle}")


def main() -> int:
    text = WORKFLOW.read_text(encoding="utf-8")
    local_official = LOCAL_OFFICIAL.read_text(encoding="utf-8")
    require(
        local_official,
        "publicacion oficial local esta deshabilitada",
        "local release guard",
    )
    if not re.search(r"(?m)^exit 1$", local_official):
        raise SystemExit("local official release guard must fail closed")
    if "continue-on-error:" in text:
        raise SystemExit("official release workflow must not use continue-on-error")
    require(text, 'tags:\n      - "v*"', "release trigger")

    policy = job_block(text, "release-policy")
    require(policy, "environment: official-release", "release-policy")
    require(policy, "check-official-release-env.sh", "release-policy")
    require(policy, "validate-official-tag.py", "release-policy")
    require(policy, "check-official-release-policy.py", "release-policy")
    require(policy, "fetch-depth: 0", "release-policy")
    require(policy, "merge-base --is-ancestor", "release-policy")

    for job_name in (
        "build-firefox-extension",
        "build-linux",
        "build-sbom",
        "build-windows",
        "build-macos",
        "verify-android-reproducibility",
        "build-android",
    ):
        block = job_block(text, job_name)
        require(block, "release-policy", job_name)

    firefox = job_block(text, "build-firefox-extension")
    require(firefox, "environment: official-release", "build-firefox-extension")
    require(
        firefox,
        "check-firefox-signing-env.sh",
        "build-firefox-extension",
    )
    require(firefox, "web-ext@10.5.0", "build-firefox-extension")
    require(
        firefox,
        'GRXFIRMA_REQUIRE_SIGNED_FIREFOX_XPI: "1"',
        "build-firefox-extension",
    )
    require(firefox, "secrets.WEB_EXT_API_KEY", "build-firefox-extension")
    require(firefox, "secrets.WEB_EXT_API_SECRET", "build-firefox-extension")
    require(firefox, "firefox-extension-signed", "build-firefox-extension")

    windows = job_block(text, "build-windows")
    require(windows, "environment: official-release", "build-windows")
    require(windows, "finalize-windows-release.ps1", "build-windows")
    require(windows, "WINDOWS_SIGNING_PFX_BASE64", "build-windows")
    require(windows, "windows-official", "build-windows")
    require(windows, "runs-on: windows-2022", "build-windows")
    require(windows, 'dotnet-version: "10.0.302"', "build-windows")
    require(windows, "build-desktop-winui.ps1", "build-windows")
    require(windows, "--with-winui --with-qt --nsis", "build-windows")
    require(
        windows,
        "choco install nsis --version 3.12.0",
        "build-windows",
    )

    for job_name in ("build-linux", "build-windows", "build-macos"):
        platform = job_block(text, job_name)
        require(platform, "build-firefox-extension", job_name)
        require(platform, "firefox-extension-signed", job_name)
        require(platform, "GRXFIRMA_FIREFOX_SIGNED_XPI", job_name)
        require(
            platform,
            'GRXFIRMA_REQUIRE_SIGNED_FIREFOX_XPI: "1"',
            job_name,
        )

    windows_verify = job_block(text, "verify-windows")
    require(windows_verify, "needs: build-windows", "verify-windows")
    require(windows_verify, "verify-windows-release.ps1", "verify-windows")
    require(windows_verify, "runs-on: windows-2022", "verify-windows")
    require(windows_verify, "-RequireDualGui", "verify-windows")
    if (
        "secrets." in windows_verify
        or "environment: official-release" in windows_verify
    ):
        raise SystemExit(
            "verify-windows must remain independent from signing credentials"
        )

    macos = job_block(text, "build-macos")
    require(macos, "environment: official-release", "build-macos")
    require(macos, "configure-macos-signing.sh", "build-macos")
    require(macos, "notarize-macos-release.sh", "build-macos")
    require(macos, "verify-macos-release.sh", "build-macos")
    require(macos, "cleanup-macos-signing.sh", "build-macos")
    require(macos, "if: ${{ always() }}", "build-macos cleanup")
    if "*.tar.gz" in macos:
        raise SystemExit("official macOS job must upload only the notarized PKG")

    macos_verify = job_block(text, "verify-macos")
    require(macos_verify, "needs: build-macos", "verify-macos")
    require(macos_verify, "verify-macos-release.sh", "verify-macos")
    if "secrets." in macos_verify or "environment: official-release" in macos_verify:
        raise SystemExit(
            "verify-macos must remain independent from signing credentials"
        )

    android_reproducibility = job_block(text, "verify-android-reproducibility")
    require(
        android_reproducibility,
        "verify-release-reproducibility.sh",
        "verify-android-reproducibility",
    )
    require(
        android_reproducibility,
        "ANDROID_QA_SIGNING_CERT_SHA256",
        "verify-android-reproducibility",
    )
    if text.count('install-toolchain.sh --github-env "$GITHUB_ENV"') != 3:
        raise SystemExit(
            "los tres jobs Android deben persistir el toolchain fijado en GITHUB_ENV"
        )

    android = job_block(text, "build-android")
    require(android, "environment: official-release", "build-android")
    require(android, "build-core-aar.sh", "build-android")
    require(android, "assembleProductionRelease", "build-android")
    require(android, "bundleProductionRelease", "build-android")
    require(android, "verify_release_artifacts.py", "build-android")
    require(android, "ANDROID-SIGNATURES.json", "build-android")
    require(android, "GRXFIRMA_ANDROID_SIGNING_CERT_SHA256", "build-android")

    android_verify = job_block(text, "verify-android")
    require(android_verify, "needs: build-android", "verify-android")
    require(android_verify, "verify_release_artifacts.py", "verify-android")
    if "secrets." in android_verify or "environment: official-release" in android_verify:
        raise SystemExit(
            "verify-android must remain independent from signing credentials"
        )

    publish = job_block(text, "publish")
    require(publish, "environment: official-release", "publish")
    for dependency in (
        "build-linux",
        "build-sbom",
        "verify-android",
        "verify-macos",
        "verify-windows",
    ):
        require(publish, f"- {dependency}", "publish dependencies")
    require(publish, "release_artifacts.py copy", "publish")
    require(publish, "no tenga una release previa", "publish immutable release")
    require(publish, "SHA256SUMS-linux.txt", "publish Linux signature")
    require(publish, "sign-checksums.sh", "publish")
    require(publish, "verify-signed-checksums.sh", "publish")
    require(publish, "RELEASE-SIGNING-KEY.asc", "publish")
    require(publish, "softprops/action-gh-release@", "publish")
    if "release/installers/macos/*.tar.gz" in publish:
        raise SystemExit(
            "official publication must not expose a non-notarized macOS tarball"
        )

    required_context = {
        "secrets.WINDOWS_SIGNING_PFX_BASE64",
        "secrets.WINDOWS_SIGNING_PFX_PASSWORD",
        "vars.WINDOWS_SIGNING_CERT_THUMBPRINT",
        "secrets.ANDROID_SIGNING_KEYSTORE_BASE64",
        "secrets.GRXFIRMA_ANDROID_KEYSTORE_PASSWORD",
        "vars.GRXFIRMA_ANDROID_KEY_ALIAS",
        "secrets.GRXFIRMA_ANDROID_KEY_PASSWORD",
        "vars.GRXFIRMA_ANDROID_SIGNING_CERT_SHA256",
        "secrets.ANDROID_QA_KEYSTORE_BASE64",
        "secrets.ANDROID_QA_KEYSTORE_PASSWORD",
        "vars.ANDROID_QA_KEY_ALIAS",
        "secrets.ANDROID_QA_KEY_PASSWORD",
        "vars.ANDROID_QA_SIGNING_CERT_SHA256",
        "secrets.WEB_EXT_API_KEY",
        "secrets.WEB_EXT_API_SECRET",
        "secrets.MACOS_APPLICATION_CERT_P12_BASE64",
        "secrets.MACOS_APPLICATION_CERT_PASSWORD",
        "secrets.MACOS_INSTALLER_CERT_P12_BASE64",
        "secrets.MACOS_INSTALLER_CERT_PASSWORD",
        "secrets.MACOS_NOTARY_KEY_P8_BASE64",
        "vars.MACOS_CODESIGN_IDENTITY",
        "vars.MACOS_INSTALLER_IDENTITY",
        "vars.MACOS_TEAM_ID",
        "vars.MACOS_NOTARY_KEY_ID",
        "vars.MACOS_NOTARY_ISSUER_ID",
        "secrets.RELEASE_GPG_PRIVATE_KEY_BASE64",
        "secrets.RELEASE_GPG_PASSPHRASE",
        "vars.RELEASE_GPG_FINGERPRINT",
    }
    missing_context = sorted(item for item in required_context if item not in text)
    if missing_context:
        raise SystemExit(
            f"release workflow is missing credential bindings: {missing_context}"
        )

    for reference in ACTION_RE.findall(text):
        if reference.startswith("./") or reference.startswith("docker://"):
            continue
        revision = reference.rsplit("@", 1)[-1]
        if not re.fullmatch(r"[0-9a-fA-F]{40}", revision):
            raise SystemExit(f"release action is not pinned by full SHA: {reference}")

    print("Official release policy is fail-closed and all actions are pinned.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
