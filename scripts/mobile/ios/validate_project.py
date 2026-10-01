#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Static, cross-platform validation for the iOS shell and Xcode project."""

from __future__ import annotations

import argparse
import json
import pathlib
import plistlib
import re
import struct
import sys


class ValidationError(RuntimeError):
    pass


def load_plist(path: pathlib.Path) -> dict:
    try:
        with path.open("rb") as handle:
            value = plistlib.load(handle)
    except (OSError, plistlib.InvalidFileException) as error:
        raise ValidationError(f"Plist inválido: {path}") from error
    if not isinstance(value, dict):
        raise ValidationError(f"Plist sin diccionario raíz: {path}")
    return value


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValidationError(message)


def validate_info_plist(root: pathlib.Path) -> None:
    info = load_plist(root / "GrxFirma/Resources/Info.plist")
    ats = info.get("NSAppTransportSecurity")
    require(isinstance(ats, dict), "Falta NSAppTransportSecurity")
    for key in (
        "NSAllowsArbitraryLoads",
        "NSAllowsArbitraryLoadsForMedia",
        "NSAllowsArbitraryLoadsInWebContent",
        "NSAllowsLocalNetworking",
    ):
        require(ats.get(key) is False, f"ATS no falla cerrado: {key}")
    require("NSExceptionDomains" not in ats, "ATS contiene dominios excepcionados")

    schemes = []
    for entry in info.get("CFBundleURLTypes", []):
        schemes.extend(entry.get("CFBundleURLSchemes", []))
    require(schemes == ["afirma"], "El único URL scheme debe ser afirma")
    require(
        info.get("LSSupportsOpeningDocumentsInPlace") is True,
        "Falta apertura documental in-place",
    )
    require(
        info.get("GrxFirmaCoreMode") == "$(GRXFIRMA_CORE_MODE)",
        "CoreMode no procede de xcconfig",
    )
    require(
        info.get("GrxFirmaAppGroup") == "$(GRXFIRMA_APP_GROUP)",
        "App Group no procede de xcconfig",
    )
    require(
        bool(info.get("CFBundleDocumentTypes")), "No hay tipos documentales declarados"
    )

    share = load_plist(root / "ShareExtension/Info.plist")
    extension = share.get("NSExtension", {})
    require(
        extension.get("NSExtensionPointIdentifier") == "com.apple.share-services",
        "Share point inválido",
    )
    rule = extension.get("NSExtensionAttributes", {}).get(
        "NSExtensionActivationRule", {}
    )
    require(
        rule.get("NSExtensionActivationSupportsFileWithMaxCount") == 1,
        "Share debe limitar cada operación a un documento",
    )
    require(
        share.get("GrxFirmaAppGroup") == "$(GRXFIRMA_APP_GROUP)",
        "Share App Group incoherente",
    )


def validate_entitlements(root: pathlib.Path) -> None:
    app = load_plist(root / "GrxFirma/GrxFirma.entitlements")
    share = load_plist(root / "ShareExtension/ShareExtension.entitlements")
    require(
        app.get("com.apple.developer.default-data-protection")
        == "NSFileProtectionComplete",
        "La app no exige NSFileProtectionComplete",
    )
    require(
        share.get("com.apple.developer.default-data-protection")
        == "NSFileProtectionComplete",
        "Share no exige NSFileProtectionComplete",
    )
    app_groups = app.get("com.apple.security.application-groups")
    share_groups = share.get("com.apple.security.application-groups")
    require(app_groups == ["$(GRXFIRMA_APP_GROUP)"], "App Group de app inválido")
    require(share_groups == app_groups, "App y Share no comparten el mismo grupo")
    require(
        app.get("keychain-access-groups")
        == ["$(AppIdentifierPrefix)$(GRXFIRMA_KEYCHAIN_GROUP)"],
        "Keychain access group inválido",
    )
    forbidden = {"get-task-allow", "com.apple.developer.networking.networkextension"}
    require(not forbidden.intersection(app), "Entitlements de producción peligrosos")


def validate_privacy(root: pathlib.Path) -> None:
    privacy = load_plist(root / "GrxFirma/Resources/PrivacyInfo.xcprivacy")
    require(
        privacy.get("NSPrivacyTracking") is False, "Privacy manifest permite tracking"
    )
    require(
        privacy.get("NSPrivacyTrackingDomains") == [],
        "Privacy manifest declara tracking domains",
    )
    require(
        privacy.get("NSPrivacyCollectedDataTypes") == [],
        "Privacy manifest declara recogida de datos",
    )
    api_types = privacy.get("NSPrivacyAccessedAPITypes")
    require(
        isinstance(api_types, list) and api_types, "Faltan motivos de APIs requeridas"
    )
    file_metadata = next(
        (
            entry
            for entry in api_types
            if entry.get("NSPrivacyAccessedAPIType")
            == "NSPrivacyAccessedAPICategoryFileTimestamp"
        ),
        None,
    )
    require(file_metadata is not None, "Falta la categoría de metadatos de fichero")
    reasons = set(file_metadata.get("NSPrivacyAccessedAPITypeReasons", []))
    require(
        {"C617.1", "3B52.1"}.issubset(reasons),
        "Faltan motivos para contenedor y documentos elegidos por el usuario",
    )


def validate_icon(root: pathlib.Path) -> None:
    icon = (
        root
        / "GrxFirma/Resources/Assets.xcassets/AppIcon.appiconset/AppIcon-1024.png"
    )
    try:
        data = icon.read_bytes()
    except OSError as error:
        raise ValidationError("Falta el App Icon 1024") from error
    require(data[:8] == b"\x89PNG\r\n\x1a\n", "App Icon no es PNG")
    width, height, bit_depth, color_type = struct.unpack(">IIBB", data[16:26])
    require((width, height) == (1024, 1024), "App Icon debe medir 1024x1024")
    require(
        bit_depth == 8 and color_type == 2, "App Icon debe ser RGB 8-bit sin canal alfa"
    )
    contents = json.loads((icon.parent / "Contents.json").read_text(encoding="utf-8"))
    require(
        contents["images"][0].get("filename") == icon.name,
        "Contents.json no referencia el App Icon",
    )


def xcconfig_value(text: str, key: str) -> str | None:
    match = re.search(rf"(?m)^\s*{re.escape(key)}\s*=\s*(.*?)\s*$", text)
    return match.group(1) if match else None


def validate_configs(root: pathlib.Path) -> None:
    base = (root / "Config/Base.xcconfig").read_text(encoding="utf-8")
    debug = (root / "Config/Debug.xcconfig").read_text(encoding="utf-8")
    release = (root / "Config/Release.xcconfig").read_text(encoding="utf-8")
    require(
        xcconfig_value(base, "IPHONEOS_DEPLOYMENT_TARGET") == "16.0",
        "Deployment target inesperado",
    )
    require(
        xcconfig_value(debug, "GRXFIRMA_CORE_MODE") == "verification",
        "Debug no es verification",
    )
    require("PRODUCTION_CORE" not in debug, "Debug activa el núcleo de producción")
    require(
        xcconfig_value(release, "GRXFIRMA_CORE_MODE") == "production",
        "Release no es production",
    )
    require(
        "PRODUCTION_CORE" in release and "GRXFIRMA_PRODUCTION_CORE=1" in release,
        "Release no enlaza el núcleo",
    )
    require(
        xcconfig_value(release, "CODE_SIGN_STYLE") == "Manual",
        "Release no exige firma manual",
    )
    require(
        xcconfig_value(release, "CODE_SIGN_IDENTITY") == "Apple Distribution",
        "Identidad Release incorrecta",
    )
    require("__APPLE_TEAM_ID_REQUIRED__" in base, "Baseline no falla sin Team ID")
    require(
        "__APP_PROVISIONING_PROFILE_REQUIRED__" in base,
        "Baseline no falla sin perfil app",
    )
    require(
        "Signing.local.xcconfig" in debug and "Signing.local.xcconfig" in release,
        "Falta include privado opcional",
    )


def validate_sources(root: pathlib.Path) -> None:
    source_root = root / "GrxFirma"
    sources = sorted(
        [
            *source_root.rglob("*.swift"),
            *source_root.rglob("*.m"),
            *source_root.rglob("*.h"),
        ]
    )
    text = "\n".join(path.read_text(encoding="utf-8") for path in sources)
    required_tokens = (
        "FileProtectionType.complete",
        "kSecAttrAccessibleWhenUnlockedThisDeviceOnly",
        "kSecAttrSynchronizable",
        "assertFreshAndRecord",
        "requestSignatureApproval",
        "executeApprovedSignature",
        "https",
        "startAccessingSecurityScopedResource",
        "MobilebindNewIOSFacade",
    )
    missing = [token for token in required_tokens if token not in text]
    require(not missing, f"Faltan controles nativos: {', '.join(missing)}")
    require(
        "URLSession" not in text,
        "La shell evita transporte directo fuera del núcleo/trust policy",
    )
    require(
        "try!" not in text and "fatalError(" not in text and " as! " not in text,
        "Código Swift con fallo no controlado",
    )
    require(
        len(list((root / "Tests").glob("*.swift"))) >= 3, "Cobertura Swift insuficiente"
    )


def validate_project(root: pathlib.Path) -> None:
    pbx = (root / "GrxFirma.xcodeproj/project.pbxproj").read_text(encoding="utf-8")
    scheme = (
        root / "GrxFirma.xcodeproj/xcshareddata/xcschemes/GrxFirma.xcscheme"
    ).read_text(encoding="utf-8")
    for target in ("GrxFirma", "GrxFirmaShare", "GrxFirmaTests"):
        require(target in pbx and target in scheme, f"Target/esquema ausente: {target}")
    require("Embed App Extensions" in pbx, "Share Extension no está embebida")
    require("Validate Release Inputs" in pbx, "Falta build phase fail-closed")
    require(
        "OTHER_LDFLAGS" in pbx and "-framework" in pbx and "Mobilebind" in pbx,
        "Release no enlaza Mobilebind",
    )
    require("SWIFT_OBJC_BRIDGING_HEADER" in pbx, "Falta bridging header")
    require(
        pbx.count("PrivacyInfo.xcprivacy in Resources") >= 4,
        "El privacy manifest no está en app y Share Extension",
    )
    for source in sorted([*root.rglob("*.swift"), *root.rglob("*.m")]):
        if "GrxFirma.xcodeproj" not in source.parts:
            require(
                source.name in pbx,
                f"Fuente fuera del proyecto Xcode: {source.relative_to(root)}",
            )


def validate(root: pathlib.Path) -> None:
    validate_info_plist(root)
    validate_entitlements(root)
    validate_privacy(root)
    validate_icon(root)
    validate_configs(root)
    validate_sources(root)
    validate_project(root)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ios-root", required=True, type=pathlib.Path)
    args = parser.parse_args()
    try:
        validate(args.ios_root.resolve())
    except (
        ValidationError,
        OSError,
        ValueError,
        KeyError,
        json.JSONDecodeError,
    ) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print(f"Proyecto iOS estructuralmente válido: {args.ios_root}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
