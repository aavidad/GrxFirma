#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Static security and release-gate checks for the Android adapter."""

from __future__ import annotations

import pathlib
import re
import hashlib
import sys
import xml.etree.ElementTree as ET


ANDROID = "{http://schemas.android.com/apk/res/android}"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def validate(root: pathlib.Path) -> None:
    project = root / "mobile/android"
    wrapper = project / "gradle/wrapper/gradle-wrapper.jar"
    wrapper_sha = hashlib.sha256(wrapper.read_bytes()).hexdigest()
    require(
        wrapper_sha
        == "55243ef57851f12b070ad14f7f5bb8302daceeebc5bce5ece5fa6edb23e1145c",
        "El JAR de Gradle Wrapper no coincide con Gradle 9.4.1",
    )
    wrapper_properties = (
        project / "gradle/wrapper/gradle-wrapper.properties"
    ).read_text(encoding="utf-8")
    require(
        "distributionSha256Sum=2ab2958f2a1e51120c326cad6f385153bb11ee93b3c216c5fccebfdfbb7ec6cb"
        in wrapper_properties,
        "Falta el SHA-256 de la distribución Gradle",
    )
    require(
        (project / "gradle/verification-metadata.xml").is_file(),
        "Falta verificación de dependencias",
    )
    require(
        (project / "app/gradle.lockfile").is_file(), "Falta bloqueo de dependencias"
    )
    manifest_path = project / "app/src/main/AndroidManifest.xml"
    manifest = ET.parse(manifest_path).getroot()
    permissions = {
        node.get(f"{ANDROID}name") for node in manifest.findall("uses-permission")
    }
    require(
        permissions == {"android.permission.NFC", "android.permission.INTERNET"},
        "Solo se permiten NFC (DNIe) e INTERNET (TSA y evidencias LT/LTA); no almacenamiento amplio",
    )
    application = manifest.find("application")
    require(application is not None, "Falta application en el manifiesto")
    require(
        application.get(f"{ANDROID}allowBackup") == "false",
        "allowBackup debe ser false",
    )
    require(
        application.get(f"{ANDROID}usesCleartextTraffic") == "false",
        "usesCleartextTraffic debe ser false",
    )

    network = ET.parse(
        project / "app/src/main/res/xml/network_security_config.xml"
    ).getroot()
    base = network.find("base-config")
    require(base is not None, "Falta base-config de red")
    require(
        base.get("cleartextTrafficPermitted") == "false",
        "La política de red permite tráfico en claro",
    )

    build = (project / "app/build.gradle.kts").read_text(encoding="utf-8")
    for token in (
        "verifyProductionCore",
        "verifyProductionSigning",
        "GRXFIRMA_ANDROID_CORE_SHA256",
        "GRXFIRMA_ANDROID_KEYSTORE",
        "GRXFIRMA_ANDROID_SOURCE_COMMIT",
        "GRXFIRMA_ANDROID_VERSION_NAME",
        "VERSION.txt",
        "enableV1Signing = false",
        "enableV2Signing = true",
        "enableV3Signing = true",
        "enableV4Signing = false",
        "includeInApk = false",
        "includeInBundle = true",
        'CORE_MODE", quoted("verification")',
        'SOURCE_COMMIT", quoted(sourceCommit.lowercase())',
    ):
        require(token in build, f"Falta gate Android: {token}")
    manifest_text = manifest_path.read_text(encoding="utf-8")
    for token in (
        "es.dipgra.grxfirma.SOURCE_COMMIT",
        "es.dipgra.grxfirma.CORE_SHA256",
    ):
        require(token in manifest_text, f"Falta trazabilidad Android: {token}")
    release_verifier = (
        project.parent.parent
        / "scripts/mobile/android/verify_release_artifacts.py"
    ).read_text(encoding="utf-8")
    for token in ("28.2.13676358", "--strip-unneeded"):
        require(token in release_verifier, f"Falta normalización nativa Android: {token}")

    kotlin_files = {
        path: path.read_text(encoding="utf-8")
        for path in (project / "app/src/main/java").rglob("*.kt")
    }
    source = "\n".join(kotlin_files.values())
    require("Math.random" not in source, "No se admite aleatoriedad insegura")
    # Solo el sello y las preferencias generales (formato, perfil, TSA, nombre
    # de salida y tema) se persisten; nunca secretos.
    preference_files = {"SealSettings.kt", "AppPreferences.kt"}
    for path, contents in kotlin_files.items():
        if "SharedPreferences" in contents or "getSharedPreferences" in contents:
            require(path.name in preference_files, "Solo se pueden persistir preferencias del sello y generales")
            keys = re.findall(r'(?:put|get)(?:String|Boolean|Int|Float|Long)\("([^"]+)"', contents)
            for key in keys:
                require(not re.search(r"pass|pin|secret|key|clave|p12|pkcs|cert", key, re.IGNORECASE),
                        f"Las preferencias no pueden guardar secretos: {path.name} ({key})")
        # AppLinks solo contiene destinos que abre el navegador del sistema.
        if "https://" in contents:
            require(path.name in {"SealSettings.kt", "AppLinks.kt"}, "La app base no debe abrir red")
    # La red (TSA, OCSP/CRL, AEAT, GitHub) la abre el núcleo Go, no Kotlin.
    for token in ("HttpURLConnection", "openConnection(", "OkHttpClient", "java.net.Socket"):
        require(token not in source, f"La app Kotlin no debe abrir conexiones: {token}")
    require(
        "takePersistableUriPermission" not in source,
        "La app no debe conservar permisos SAF entre sesiones",
    )
    require(
        "http://" not in source,
        "La app base no debe abrir red",
    )
    require("FLAG_SECURE" in source, "La variante de producción debe proteger capturas")
    for token in (
        "clearSession",
        "releasePersistableUriPermission",
        "canAcceptIncomingDocument",
        "UiText.Verification",
    ):
        require(token in source, f"Falta control Android de cierre: {token}")


def main() -> int:
    root = pathlib.Path(__file__).resolve().parents[3]
    try:
        validate(root)
    except (RuntimeError, ET.ParseError) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print("Proyecto Android: controles estáticos correctos")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
