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
        "io.github.aavidad.grxfirma.SOURCE_COMMIT",
        "io.github.aavidad.grxfirma.CORE_SHA256",
    ):
        require(token in manifest_text, f"Falta trazabilidad Android: {token}")
    release_verifier = (
        project.parent.parent
        / "scripts/mobile/android/verify_release_artifacts.py"
    ).read_text(encoding="utf-8")
    for token in ("28.2.13676358", "--strip-unneeded"):
        require(token in release_verifier, f"Falta normalización nativa Android: {token}")

    check_manifest_components(manifest)
    kotlin_files = {
        path: path.read_text(encoding="utf-8")
        for path in (project / "app/src/main/java").rglob("*.kt")
    }
    check_kotlin_sources(kotlin_files)


# Solo el sello y las preferencias generales (formato, perfil, TSA, nombre de
# salida y tema) se persisten; nunca secretos.
PREFERENCE_FILES = {"SealSettings.kt", "AppPreferences.kt"}
SENSITIVE_KEY = re.compile(r"pass|pin|secret|key|clave|p12|pkcs|cert|token", re.IGNORECASE)
PREFERENCE_CALL = re.compile(
    r"\b(?:put|get)(?:StringSet|String|Boolean|Int|Float|Long)\(\s*([A-Za-z_][\w.]*|\"[^\"]*\")"
)
CONSTANT = re.compile(r"\bval\s+([A-Za-z_]\w*)\s*(?::\s*String\s*)?=\s*\"([^\"]*)\"")
# Almacenes y salidas que podrían dejar secretos en disco fuera de lo revisado.
FORBIDDEN_STORAGE = (
    ("androidx.datastore", "No se admite DataStore: solo SharedPreferences revisadas"),
    ("preferencesDataStore", "No se admite DataStore: solo SharedPreferences revisadas"),
    ("DataStoreFactory", "No se admite DataStore: solo SharedPreferences revisadas"),
    ("openFileOutput(", "No se admite openFileOutput: los datos temporales van a noBackupFilesDir"),
)
# La red (TSA, OCSP/CRL, AEAT, GitHub) la abre el núcleo Go, no Kotlin.
FORBIDDEN_NETWORK = (
    (re.compile(r"HttpURLConnection"), "HttpURLConnection"),
    (re.compile(r"\bURLConnection\b"), "URLConnection"),
    (re.compile(r"openConnection\("), "openConnection("),
    (re.compile(r"\bopenStream\("), "openStream("),
    (re.compile(r"\bjava\.net\.URL\b"), "java.net.URL"),
    (re.compile(r"(?<![\w.])URL\("), "URL("),
    (re.compile(r"OkHttpClient"), "OkHttpClient"),
    (re.compile(r"java\.net\.Socket"), "java.net.Socket"),
)
SECURE_IN_PRODUCTION = re.compile(
    r'if\s*\(\s*BuildConfig\.CORE_MODE\s*==\s*"production"\s*\)\s*\{?\s*'
    r"window\.addFlags\(\s*WindowManager\.LayoutParams\.FLAG_SECURE\s*\)"
)


def _preference_keys(contents: str) -> list[str]:
    """Claves usadas con SharedPreferences, resolviendo constantes del fichero."""
    constants = dict(CONSTANT.findall(contents))
    keys = []
    for argument in PREFERENCE_CALL.findall(contents):
        if argument.startswith('"'):
            keys.append(argument.strip('"'))
            continue
        name = argument.rsplit(".", 1)[-1]
        require(name in constants, f"Clave de preferencias sin valor literal comprobable: {argument}")
        # Se revisa tanto el nombre de la constante como su valor.
        keys.extend((name, constants[name]))
    return keys


def check_kotlin_sources(kotlin_files: dict[pathlib.Path, str]) -> None:
    source = "\n".join(kotlin_files.values())
    require("Math.random" not in source, "No se admite aleatoriedad insegura")
    for token, message in FORBIDDEN_STORAGE:
        require(token not in source, message)
    for path, contents in kotlin_files.items():
        if "SharedPreferences" in contents or "getSharedPreferences" in contents:
            require(path.name in PREFERENCE_FILES, "Solo se pueden persistir preferencias del sello y generales")
            for key in _preference_keys(contents):
                require(not SENSITIVE_KEY.search(key),
                        f"Las preferencias no pueden guardar secretos: {path.name} ({key})")
        # AppLinks solo contiene destinos que abre el navegador del sistema.
        if "https://" in contents:
            require(path.name in {"SealSettings.kt", "AppLinks.kt"}, "La app base no debe abrir red")
    for pattern, token in FORBIDDEN_NETWORK:
        require(not pattern.search(source), f"La app Kotlin no debe abrir conexiones: {token}")
    require(
        "takePersistableUriPermission" not in source,
        "La app no debe conservar permisos SAF entre sesiones",
    )
    require(
        "http://" not in source,
        "La app base no debe abrir red",
    )
    activity = next((text for path, text in kotlin_files.items() if path.name == "MainActivity.kt"), "")
    require(
        SECURE_IN_PRODUCTION.search(activity) is not None,
        "La variante de producción debe aplicar FLAG_SECURE en MainActivity",
    )
    for line in source.splitlines():
        if "clearFlags" in line and "FLAG_SECURE" in line:
            require('CORE_MODE != "production"' in line,
                    "FLAG_SECURE solo puede retirarse fuera de la variante de producción")
    for token in (
        "clearSession",
        "releasePersistableUriPermission",
        "canAcceptIncomingDocument",
        "UiText.Verification",
    ):
        require(token in source, f"Falta control Android de cierre: {token}")


# Únicos componentes que otras apps pueden invocar.
EXPORTED_ALLOWED = {".MainActivity"}


def check_manifest_components(manifest: ET.Element) -> None:
    application = manifest.find("application")
    require(application is not None, "Falta application en el manifiesto")
    for kind in ("activity", "activity-alias", "service", "receiver", "provider"):
        for node in application.findall(kind):
            name = node.get(f"{ANDROID}name", "")
            exported = node.get(f"{ANDROID}exported")
            has_filter = node.find("intent-filter") is not None
            if has_filter:
                require(exported is not None, f"Declare android:exported en {name}")
            if exported == "true" or (exported is None and has_filter):
                require(kind == "activity" and name in EXPORTED_ALLOWED,
                        f"Componente exportado no permitido en el manifiesto: {name}")
            if kind == "provider":
                require(exported == "false", f"Los proveedores no pueden exportarse: {name}")


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
