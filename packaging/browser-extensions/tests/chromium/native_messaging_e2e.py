#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Chromium-family Linux E2E for WebExtension -> Native Messaging -> Go."""

from __future__ import annotations

import argparse
import contextlib
from dataclasses import dataclass
import fcntl
import hashlib
import importlib.metadata
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
from typing import Any, Iterator
import zipfile


HOST_NAME = "com.dipgra.grxfirma"
BACKGROUND_PATH = "background.js"
MAX_PACKAGE_BYTES = 20 * 1024 * 1024

REPO_ROOT = Path(__file__).resolve().parents[4]
DEFAULT_PACKAGE = (
    REPO_ROOT / "packaging" / "browser-extensions" / "dipgra-extension-chromium.zip"
)

NEGATIVE_ROUNDTRIP = r"""
host => new Promise(resolve => {
    const port = chrome.runtime.connectNative(host);
    let complete = false;
    const finish = result => {
        if (complete) return;
        complete = true;
        clearTimeout(timer);
        resolve(result);
    };
    const timer = setTimeout(() => finish({
        disconnected: false,
        error: "timeout"
    }), 10000);
    port.onMessage.addListener(() => finish({
        disconnected: false,
        error: "unexpected native response"
    }));
    port.onDisconnect.addListener(() => finish({
        disconnected: true,
        error: (chrome.runtime.lastError && chrome.runtime.lastError.message) ||
            "native host disconnected"
    }));
    port.postMessage({ requestId: "e2e-negative", action: "ping" });
})
"""

POSITIVE_ROUNDTRIP = r"""
host => new Promise(resolve => {
    const port = chrome.runtime.connectNative(host);
    globalThis.__grxfirmaChromiumE2EPort = port;
    const result = {
        ping: false,
        getCertificates: false,
        certificateCount: null
    };
    let complete = false;
    const finish = error => {
        if (complete) return;
        complete = true;
        clearTimeout(timer);
        resolve({ ...result, error: error || "" });
    };
    const timer = setTimeout(() => finish("timeout"), 30000);

    port.onDisconnect.addListener(() => finish(
        (chrome.runtime.lastError && chrome.runtime.lastError.message) ||
        "native host disconnected"
    ));
    port.onMessage.addListener(message => {
        if (message.requestId === "e2e-ping" && message.success) {
            result.ping = true;
            port.postMessage({
                requestId: "e2e-certificates",
                action: "getCertificates"
            });
            return;
        }
        if (message.requestId === "e2e-certificates" && message.success) {
            result.getCertificates = true;
            result.certificateCount = Array.isArray(message.certificates)
                ? message.certificates.length
                : 0;
            finish("");
        }
    });
    port.postMessage({ requestId: "e2e-ping", action: "ping" });
})
"""

DISCONNECT_E2E_PORT = r"""
() => {
    const port = globalThis.__grxfirmaChromiumE2EPort;
    if (!port) return false;
    port.disconnect();
    delete globalThis.__grxfirmaChromiumE2EPort;
    return true;
}
"""


class E2EError(RuntimeError):
    pass


@dataclass(frozen=True)
class BrowserSpec:
    key: str
    label: str
    executable: Path
    config_subdir: Path
    package_type: str
    sideload_block_is_limit: bool = False


@dataclass(frozen=True)
class FileSnapshot:
    content: bytes | None
    mode: int | None


def run_command(
    command: list[str],
    *,
    cwd: Path | None = None,
    check: bool = True,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        command,
        cwd=cwd,
        check=check,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def resolve_executable(value: str | None, candidates: list[str]) -> Path | None:
    if value:
        candidate = shutil.which(value) or value
        path = Path(candidate).expanduser().absolute()
        if path.is_file() and os.access(path, os.X_OK):
            return path
        raise E2EError(f"ejecutable no disponible: {value}")

    for candidate in candidates:
        resolved = shutil.which(candidate)
        if resolved:
            return Path(resolved).absolute()
    return None


def resolve_go(value: str | None) -> Path:
    candidates = [str(Path.home() / "go" / "bin" / "go1.26.5"), "go1.26.5", "go"]
    result = resolve_executable(value, candidates)
    if result is None:
        raise E2EError("no se encontro Go")
    return result


def version_key(path: Path) -> tuple[int, ...]:
    version = path.parent.parent.name.removeprefix("linux-")
    try:
        return tuple(int(part) for part in version.split("."))
    except ValueError:
        return (0,)


def discover_chrome_for_testing(value: str | None) -> Path | None:
    explicit = resolve_executable(value, []) if value else None
    if explicit:
        return explicit

    cache = Path.home() / ".cache" / "grxfirma-browsers" / "chrome"
    matches = [
        path
        for path in cache.glob("linux-*/chrome-linux64/chrome")
        if path.is_file() and os.access(path, os.X_OK)
    ]
    return max(matches, key=version_key) if matches else None


def browser_version(executable: Path) -> str:
    result = run_command([str(executable), "--version"], check=False)
    return result.stdout.strip().splitlines()[0] if result.stdout.strip() else "unknown"


def discover_browsers(args: argparse.Namespace) -> dict[str, BrowserSpec | None]:
    chrome_testing = discover_chrome_for_testing(args.chrome)
    chrome_stable = resolve_executable(
        args.chrome_stable, ["google-chrome", "google-chrome-stable"]
    )
    chromium = resolve_executable(args.chromium, ["chromium", "chromium-browser"])
    brave = resolve_executable(args.brave, ["brave-browser", "brave"])

    return {
        "chrome-stable": (
            BrowserSpec(
                "chrome-stable",
                "Google Chrome Stable",
                chrome_stable,
                Path("google-chrome"),
                "system",
                sideload_block_is_limit=True,
            )
            if chrome_stable
            else None
        ),
        "chrome": (
            BrowserSpec(
                "chrome",
                "Google Chrome for Testing",
                chrome_testing,
                Path("google-chrome-for-testing"),
                "user-cache",
            )
            if chrome_testing
            else None
        ),
        "chromium": (
            BrowserSpec(
                "chromium",
                "Chromium",
                chromium,
                Path("chromium"),
                "snap" if str(chromium).startswith("/snap/") else "system",
            )
            if chromium
            else None
        ),
        "brave": (
            BrowserSpec(
                "brave",
                "Brave",
                brave,
                Path("BraveSoftware") / "Brave-Browser",
                "system",
            )
            if brave
            else None
        ),
    }


def selected_browser_keys(args: argparse.Namespace) -> list[str]:
    if args.browser == "all":
        return ["chrome-stable", "chrome", "chromium", "brave"]
    if args.browser != "auto":
        return [args.browser]
    return ["chrome", "chromium", "brave"]


def build_native_host(go_binary: Path, output: Path) -> None:
    result = run_command(
        [
            str(go_binary),
            "build",
            "-trimpath",
            "-tags=production",
            "-o",
            str(output),
            "./cmd/nativehost",
        ],
        cwd=REPO_ROOT,
        check=False,
    )
    if result.returncode != 0:
        raise E2EError(f"fallo al compilar el host Go:\n{result.stdout}")
    output.chmod(0o700)


def prepare_native_host(
    go_binary: Path,
    output: Path,
    prebuilt: Path | None = None,
) -> str:
    source = "production-build"
    if prebuilt is None:
        build_native_host(go_binary, output)
    else:
        candidate = prebuilt.expanduser().resolve()
        if not candidate.is_file() or not os.access(candidate, os.X_OK):
            raise E2EError(f"host nativo precompilado no ejecutable: {prebuilt}")
        shutil.copyfile(candidate, output)
        output.chmod(0o700)
        source = "prebuilt"

    metadata = run_command(
        [str(go_binary), "version", "-m", str(output)],
        check=False,
    )
    if (
        metadata.returncode != 0
        or "\tbuild\t-tags=production" not in metadata.stdout
    ):
        raise E2EError("el host nativo E2E no fue compilado con la etiqueta production")
    return source


def validate_package(package: Path) -> dict[str, Any]:
    if not package.is_file():
        raise E2EError(f"paquete Chromium inexistente: {package}")
    if package.stat().st_size > MAX_PACKAGE_BYTES:
        raise E2EError("el paquete Chromium supera el limite de la prueba")

    with zipfile.ZipFile(package) as archive:
        try:
            manifest = json.loads(archive.read("manifest.json"))
        except (KeyError, json.JSONDecodeError) as error:
            raise E2EError("el ZIP no contiene un manifest.json valido") from error
        names = archive.namelist()

    if len(names) != len(set(names)):
        raise E2EError("el ZIP contiene entradas duplicadas")
    if manifest.get("manifest_version") != 3:
        raise E2EError("la extension empaquetada no usa Manifest V3")
    if "nativeMessaging" not in manifest.get("permissions", []):
        raise E2EError("la extension empaquetada no declara nativeMessaging")
    if manifest.get("background", {}).get("service_worker") != BACKGROUND_PATH:
        raise E2EError("service worker Chromium inesperado")
    return manifest


def extract_package(package: Path, destination: Path) -> None:
    destination.mkdir(mode=0o700, parents=True)
    total = 0
    with zipfile.ZipFile(package) as archive:
        for info in archive.infolist():
            relative = PurePosixPath(info.filename)
            if relative.is_absolute() or ".." in relative.parts:
                raise E2EError(f"ruta insegura dentro del ZIP: {info.filename!r}")
            file_type = (info.external_attr >> 16) & 0o170000
            if file_type == stat.S_IFLNK:
                raise E2EError(f"enlace simbolico no permitido en ZIP: {info.filename!r}")
            total += info.file_size
            if total > MAX_PACKAGE_BYTES:
                raise E2EError("contenido descomprimido superior al limite")

            target = destination.joinpath(*relative.parts)
            if info.is_dir():
                target.mkdir(mode=0o700, parents=True, exist_ok=True)
                continue
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            with archive.open(info) as source, target.open("xb") as output:
                shutil.copyfileobj(source, output)
            target.chmod(0o600)


def atomic_write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    path.parent.chmod(0o700)
    content = (json.dumps(payload, ensure_ascii=True, indent=2) + "\n").encode("utf-8")
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "wb") as stream:
            descriptor = -1
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(path)
        if stat.S_IMODE(path.stat().st_mode) != 0o600:
            raise E2EError(f"permisos inseguros en manifiesto temporal: {path}")
    except BaseException:
        if descriptor >= 0:
            os.close(descriptor)
        temporary.unlink(missing_ok=True)
        raise


def native_manifest_paths(
    spec: BrowserSpec, profile: Path, config_home: Path
) -> list[Path]:
    return [
        profile / "NativeMessagingHosts" / f"{HOST_NAME}.json",
        config_home
        / spec.config_subdir
        / "NativeMessagingHosts"
        / f"{HOST_NAME}.json",
    ]


def write_native_manifests(
    paths: list[Path], *, host: Path, extension_id: str
) -> None:
    payload = {
        "name": HOST_NAME,
        "description": "GrxFirma Native Messaging Host E2E",
        "path": str(host),
        "type": "stdio",
        "allowed_origins": [f"chrome-extension://{extension_id}/"],
    }
    for path in paths:
        atomic_write_json(path, payload)


def is_extension_service_worker(worker: Any) -> bool:
    return worker.url.startswith("chrome-extension://") and worker.url.endswith(
        f"/{BACKGROUND_PATH}"
    )


def wait_for_service_worker(context: Any, timeout: float) -> Any | None:
    for worker in context.service_workers:
        if is_extension_service_worker(worker):
            return worker

    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        remaining_ms = max(1, int((deadline - time.monotonic()) * 1000))
        try:
            worker = context.wait_for_event(
                "serviceworker", timeout=min(remaining_ms, 1000)
            )
        except Exception as error:
            if error.__class__.__name__ != "TimeoutError":
                raise
            continue
        if is_extension_service_worker(worker):
            return worker
    return None


def wait_until(description: str, timeout: float, operation: Any) -> Any:
    deadline = time.monotonic() + timeout
    last_error: BaseException | None = None
    while time.monotonic() < deadline:
        try:
            value = operation()
            if value:
                return value
        except BaseException as error:
            last_error = error
        time.sleep(0.2)
    suffix = f": {last_error}" if last_error else ""
    raise E2EError(f"timeout esperando {description}{suffix}")


def native_host_processes(host: Path) -> list[dict[str, Any]]:
    expected = host.resolve()
    matches: list[dict[str, Any]] = []
    for proc_dir in Path("/proc").glob("[0-9]*"):
        try:
            executable = (proc_dir / "exe").resolve(strict=True)
            if executable != expected:
                continue
            arguments = [
                item.decode("utf-8", errors="replace")
                for item in (proc_dir / "cmdline").read_bytes().split(b"\0")
                if item
            ]
            matches.append({"pid": int(proc_dir.name), "arguments": arguments})
        except (FileNotFoundError, PermissionError, ProcessLookupError):
            continue
    return matches


def verified_host_process(host: Path, extension_id: str, timeout: float) -> dict[str, Any]:
    origin = f"chrome-extension://{extension_id}/"

    def find_process() -> dict[str, Any] | None:
        for process in native_host_processes(host):
            if origin in process["arguments"]:
                return process
        return None

    return wait_until("el proceso Go y su origen Chromium", timeout, find_process)


def ensure_no_host_process(host: Path, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if not native_host_processes(host):
            return
        time.sleep(0.2)
    processes = native_host_processes(host)
    for process in processes:
        with contextlib.suppress(ProcessLookupError, PermissionError):
            os.kill(process["pid"], signal.SIGTERM)
    raise E2EError(f"quedaron procesos del host Go: {[p['pid'] for p in processes]}")


def snapshot_file(path: Path) -> FileSnapshot:
    if not path.exists():
        return FileSnapshot(None, None)
    return FileSnapshot(path.read_bytes(), stat.S_IMODE(path.stat().st_mode))


def canonical_manifest_paths() -> list[Path]:
    home = Path.home()
    return [
        home
        / ".config"
        / "google-chrome"
        / "NativeMessagingHosts"
        / f"{HOST_NAME}.json",
        home
        / ".config"
        / "google-chrome-for-testing"
        / "NativeMessagingHosts"
        / f"{HOST_NAME}.json",
        home
        / ".config"
        / "chromium"
        / "NativeMessagingHosts"
        / f"{HOST_NAME}.json",
        home
        / ".config"
        / "BraveSoftware"
        / "Brave-Browser"
        / "NativeMessagingHosts"
        / f"{HOST_NAME}.json",
    ]


def assert_snapshots_unchanged(snapshots: dict[Path, FileSnapshot]) -> None:
    changed = [path for path, snapshot in snapshots.items() if snapshot_file(path) != snapshot]
    if changed:
        raise E2EError(
            "se modificaron manifiestos reales del usuario: "
            + ", ".join(str(path) for path in changed)
        )


def validate_runtime_identity(worker: Any, expected_manifest: dict[str, Any]) -> dict[str, Any]:
    identity = wait_until(
        "la API runtime de la extension",
        5.0,
        lambda: worker.evaluate(
            """() => {
                if (!globalThis.chrome || !chrome.runtime || !chrome.runtime.id) {
                    return null;
                }
                const manifest = chrome.runtime.getManifest();
                return {
                    id: chrome.runtime.id,
                    name: manifest.name,
                    version: manifest.version,
                    manifestVersion: manifest.manifest_version,
                    uiLanguage: chrome.i18n.getUILanguage(),
                    localized: {
                        extensionName: chrome.i18n.getMessage("extensionName"),
                        checkingConnection: chrome.i18n.getMessage("popupCheckingConnection"),
                        nativeHostConnected: chrome.i18n.getMessage("popupNativeHostConnected"),
                        loadingCertificates: chrome.i18n.getMessage("popupLoadingCertificates"),
                        noCertificates: chrome.i18n.getMessage("popupNoCertificates")
                    }
                };
            }"""
        ),
    )
    if identity.get("manifestVersion") != 3:
        raise E2EError("el navegador no cargo la extension Manifest V3")
    expected_name = expected_manifest.get("name")
    if expected_name == "__MSG_extensionName__":
        expected_name = identity.get("localized", {}).get("extensionName")
    if identity.get("name") != expected_name:
        raise E2EError("el navegador cargo una extension distinta")
    if identity.get("version") != expected_manifest.get("version"):
        raise E2EError("la version cargada no coincide con el ZIP")
    extension_id = str(identity.get("id") or "")
    if len(extension_id) != 32 or any(character < "a" or character > "p" for character in extension_id):
        raise E2EError(f"ID Chromium invalido: {extension_id!r}")
    return identity


def validate_negative_path(worker: Any, host: Path) -> str:
    result = worker.evaluate(NEGATIVE_ROUNDTRIP, HOST_NAME)
    error = str(result.get("error") or "")
    if not result.get("disconnected") or not error or error == "timeout":
        raise E2EError(f"el camino de error Native Messaging no fallo bien: {result!r}")
    if native_host_processes(host):
        raise E2EError("el camino negativo ejecuto el host valido")
    return error


def validate_positive_path(worker: Any) -> dict[str, Any]:
    result = worker.evaluate(POSITIVE_ROUNDTRIP, HOST_NAME)
    if result.get("error"):
        raise E2EError(f"Native Messaging devolvio error: {result['error']}")
    if not result.get("ping") or not result.get("getCertificates"):
        raise E2EError(f"respuesta Native Messaging incompleta: {result!r}")
    count = result.get("certificateCount")
    if not isinstance(count, int) or count < 0:
        raise E2EError("getCertificates no devolvio un recuento valido")
    return result


def validate_popup(
    context: Any,
    extension_id: str,
    certificate_count: int,
    timeout: float,
    localized: dict[str, str],
) -> dict[str, Any]:
    page = context.new_page()
    page.goto(f"chrome-extension://{extension_id}/popup.html")
    status = page.locator("#status")
    status.wait_for(timeout=timeout * 1000)

    status_text = wait_until(
        "el ping en el popup",
        timeout,
        lambda: (
            text
            if (text := status.inner_text()) != localized["checkingConnection"]
            and ("ok" in (status.get_attribute("class") or "").split())
            else ""
        ),
    )
    if status_text != localized["nativeHostConnected"]:
        raise E2EError(f"el popup no uso Native Messaging: {status_text!r}")

    certificate_list = page.locator("#certList")
    certificate_text = wait_until(
        "getCertificates en el popup",
        timeout,
        lambda: (
            text
            if (text := certificate_list.inner_text()) != localized["loadingCertificates"]
            else ""
        ),
    )
    rendered_count = page.locator("#certList .cert").count()
    if rendered_count != certificate_count:
        raise E2EError(
            f"el popup mostro {rendered_count} certificados y el host {certificate_count}"
        )
    if certificate_count == 0 and certificate_text != localized["noCertificates"]:
        raise E2EError("el popup no represento correctamente el almacen vacio")
    return {"status": status_text, "certificate_count": rendered_count}


def run_browser(
    playwright: Any,
    spec: BrowserSpec,
    root_workdir: Path,
    host: Path,
    package: Path,
    package_manifest: dict[str, Any],
    package_sha256: str,
    *,
    headed: bool,
    keep_workdir: bool,
    timeout: float,
) -> dict[str, Any]:
    browser_workdir = root_workdir / spec.key
    browser_workdir.mkdir(mode=0o700)
    extension = browser_workdir / "extension"
    profile = browser_workdir / "profile"
    config_home = browser_workdir / "xdg-config"
    extract_package(package, extension)

    environment = dict(os.environ)
    environment["XDG_CONFIG_HOME"] = str(config_home)
    context: Any | None = None
    command_version = browser_version(spec.executable)
    try:
        context = playwright.chromium.launch_persistent_context(
            str(profile),
            executable_path=str(spec.executable),
            headless=not headed,
            env=environment,
            args=[
                f"--disable-extensions-except={extension}",
                f"--load-extension={extension}",
                "--no-first-run",
                "--no-default-browser-check",
            ],
        )
        page = context.pages[0] if context.pages else context.new_page()
        cdp = context.new_cdp_session(page)
        protocol_version = cdp.send("Browser.getVersion")
        worker = wait_for_service_worker(context, min(timeout, 12.0))
        if worker is None:
            if spec.sideload_block_is_limit:
                return {
                    "browser": {
                        "key": spec.key,
                        "name": spec.label,
                        "version": command_version,
                        "product": protocol_version.get("product"),
                        "package": spec.package_type,
                    },
                    "status": "limited",
                    "reason": "branded_chrome_blocks_command_line_sideload",
                }
            if spec.package_type == "snap":
                return {
                    "browser": {
                        "key": spec.key,
                        "name": spec.label,
                        "version": command_version,
                        "product": protocol_version.get("product"),
                        "package": spec.package_type,
                    },
                    "status": "limited",
                    "reason": "snap_confinement_blocks_unpacked_extension",
                }
            raise E2EError(f"{spec.label} no cargo el service worker empaquetado")

        identity = validate_runtime_identity(worker, package_manifest)
        extension_id = identity["id"]
        manifest_paths = native_manifest_paths(spec, profile, config_home)

        write_native_manifests(
            manifest_paths,
            host=browser_workdir / "missing-native-host",
            extension_id=extension_id,
        )
        negative_error = validate_negative_path(worker, host)

        write_native_manifests(manifest_paths, host=host, extension_id=extension_id)
        native_result = validate_positive_path(worker)
        direct_process = verified_host_process(host, extension_id, timeout)

        if not worker.evaluate(DISCONNECT_E2E_PORT):
            raise E2EError("no se pudo cerrar el puerto Native Messaging directo")
        ensure_no_host_process(host, min(timeout, 10.0))

        popup_result = validate_popup(
            context,
            extension_id,
            native_result["certificateCount"],
            timeout,
            identity["localized"],
        )
        popup_process = verified_host_process(host, extension_id, timeout)

        return {
            "browser": {
                "key": spec.key,
                "name": spec.label,
                "version": command_version,
                "product": protocol_version.get("product"),
                "package": spec.package_type,
                "headless": not headed,
                "temporary_profile": True,
            },
            "status": "pass",
            "extension": {
                "id": extension_id,
                "name": identity["name"],
                "version": identity["version"],
                "manifest_version": identity["manifestVersion"],
                "ui_language": identity["uiLanguage"],
                "package_sha256": package_sha256,
                "packaged_zip_loaded": True,
            },
            "native_messaging": {
                "host": HOST_NAME,
                "connect_native": True,
                "ping": True,
                "get_certificates": True,
                "certificate_count": native_result["certificateCount"],
                "direct_host_pid": direct_process["pid"],
                "popup_host_pid": popup_process["pid"],
                "host_sha256": sha256_file(host),
                "executable_verified": True,
                "origin_argument_verified": True,
            },
            "popup": popup_result,
            "error_path": {
                "invalid_host_rejected": True,
                "browser_error_present": bool(negative_error),
            },
        }
    finally:
        if context is not None:
            with contextlib.suppress(BaseException):
                context.close()
        ensure_no_host_process(host, min(timeout, 10.0))
        if not keep_workdir:
            shutil.rmtree(browser_workdir, ignore_errors=True)


@contextlib.contextmanager
def exclusive_lock() -> Iterator[None]:
    lock_path = Path(tempfile.gettempdir()) / f"grxfirma-chromium-e2e-{os.getuid()}.lock"
    with lock_path.open("a+", encoding="utf-8") as lock:
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        yield


def install_signal_handlers() -> None:
    def interrupt(signum: int, _frame: Any) -> None:
        raise KeyboardInterrupt(f"senal {signum}")

    signal.signal(signal.SIGTERM, interrupt)
    signal.signal(signal.SIGHUP, interrupt)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--browser",
        choices=("auto", "all", "chrome", "chrome-stable", "chromium", "brave"),
        default="auto",
    )
    parser.add_argument("--chrome", default=os.environ.get("CHROMIUM_E2E_CHROME"))
    parser.add_argument(
        "--chrome-stable", default=os.environ.get("CHROMIUM_E2E_CHROME_STABLE")
    )
    parser.add_argument("--chromium", default=os.environ.get("CHROMIUM_E2E_CHROMIUM"))
    parser.add_argument("--brave", default=os.environ.get("CHROMIUM_E2E_BRAVE"))
    parser.add_argument("--go", default=os.environ.get("CHROMIUM_E2E_GO"))
    parser.add_argument(
        "--native-host",
        type=Path,
        default=(
            Path(os.environ["CHROMIUM_E2E_NATIVE_HOST"])
            if os.environ.get("CHROMIUM_E2E_NATIVE_HOST")
            else None
        ),
    )
    parser.add_argument(
        "--package",
        type=Path,
        default=Path(os.environ.get("CHROMIUM_E2E_PACKAGE", DEFAULT_PACKAGE)),
    )
    parser.add_argument("--headed", action="store_true")
    parser.add_argument("--keep-workdir", action="store_true")
    parser.add_argument("--timeout", type=float, default=60.0)
    return parser.parse_args()


def main() -> int:
    if sys.platform != "linux":
        print("SKIP: esta prueba E2E requiere Linux", file=sys.stderr)
        return 77

    args = parse_args()
    if args.timeout < 10:
        print("FAIL: --timeout debe ser al menos 10 segundos", file=sys.stderr)
        return 2

    install_signal_handlers()
    snapshots = {path: snapshot_file(path) for path in canonical_manifest_paths()}
    # Chromium sandboxes may drop access to 0700 ancestors in a custom HOME.
    # mkdtemp still creates the leaf as 0700 in the OS temporary directory,
    # which remains reachable by the browser sandbox without exposing files.
    workdir = Path(tempfile.mkdtemp(prefix="GrxFirma-chromium-e2e-"))
    workdir.chmod(0o700)
    host = workdir / "grxfirma-nativehost"
    success = False
    evidence: dict[str, Any] = {
        "result": "fail",
        "browsers": [],
    }
    exit_code = 1

    try:
        try:
            import playwright.sync_api
        except ImportError as error:
            raise E2EError(
                "falta Playwright: python3 -m pip install --user playwright"
            ) from error

        go_binary = resolve_go(args.go)
        package = args.package.expanduser().resolve()
        package_manifest = validate_package(package)
        package_sha256 = sha256_file(package)
        browsers = discover_browsers(args)
        keys = selected_browser_keys(args)
        available_tests = 0
        failed_tests = 0
        limited_tests = 0

        host_source = prepare_native_host(go_binary, host, args.native_host)
        with exclusive_lock(), playwright.sync_api.sync_playwright() as playwright:
            for key in keys:
                spec = browsers.get(key)
                if spec is None:
                    if args.browser not in ("all", "auto"):
                        raise E2EError(f"navegador solicitado no disponible: {key}")
                    evidence["browsers"].append({"key": key, "status": "skipped"})
                    continue
                try:
                    result = run_browser(
                        playwright,
                        spec,
                        workdir,
                        host,
                        package,
                        package_manifest,
                        package_sha256,
                        headed=args.headed,
                        keep_workdir=args.keep_workdir,
                        timeout=args.timeout,
                    )
                except E2EError:
                    raise
                except Exception as error:
                    raise E2EError(
                        f"{spec.label}: {error.__class__.__name__}: {error}"
                    ) from error
                evidence["browsers"].append(result)
                if result["status"] == "pass":
                    available_tests += 1
                    if args.browser == "auto":
                        break
                elif result["status"] == "limited":
                    limited_tests += 1
                else:
                    failed_tests += 1

        if available_tests == 0:
            if failed_tests == 0 and limited_tests > 0:
                assert_snapshots_unchanged(snapshots)
                evidence["result"] = "limited"
                evidence["tooling"] = {
                    "playwright": importlib.metadata.version("playwright"),
                    "go": run_command([str(go_binary), "version"]).stdout.strip(),
                    "native_host_source": host_source,
                }
                exit_code = 77
            else:
                raise E2EError("ningun navegador completo la prueba E2E")
        else:
            assert_snapshots_unchanged(snapshots)
            evidence["result"] = "pass"
            evidence["tooling"] = {
                "playwright": importlib.metadata.version("playwright"),
                "go": run_command([str(go_binary), "version"]).stdout.strip(),
                "native_host_source": host_source,
            }
            success = True
            exit_code = 0
    except (E2EError, KeyboardInterrupt, OSError, subprocess.SubprocessError) as error:
        evidence["error"] = str(error)
        print(f"FAIL: {error}", file=sys.stderr)
    finally:
        host_cleanup_error = ""
        try:
            ensure_no_host_process(host, 5.0)
        except BaseException as error:
            host_cleanup_error = str(error)
        canonical_manifests_unchanged = True
        try:
            assert_snapshots_unchanged(snapshots)
        except BaseException as error:
            canonical_manifests_unchanged = False
            evidence["cleanup_error"] = str(error)
        if args.keep_workdir:
            evidence["cleanup"] = {
                "workdir_removed": False,
                "canonical_manifests_unchanged": canonical_manifests_unchanged,
                "host_processes_remaining": len(native_host_processes(host)),
            }
            print(f"workdir: {workdir}", file=sys.stderr)
        else:
            host_process_count = len(native_host_processes(host))
            if host_process_count == 0:
                shutil.rmtree(workdir, ignore_errors=True)
            evidence["cleanup"] = {
                "workdir_removed": not workdir.exists(),
                "canonical_manifests_unchanged": canonical_manifests_unchanged,
                "host_processes_remaining": host_process_count,
            }
        if host_cleanup_error:
            evidence["cleanup_error"] = host_cleanup_error
        cleanup = evidence["cleanup"]
        cleanup_complete = (
            not host_cleanup_error
            and canonical_manifests_unchanged
            and cleanup.get("host_processes_remaining", 0) == 0
            and (args.keep_workdir or cleanup["workdir_removed"])
        )
        if evidence["result"] in ("pass", "limited") and not cleanup_complete:
            success = False
            exit_code = 1
            evidence["result"] = "fail"
            evidence.setdefault("error", "limpieza o restauracion incompleta")
        print(json.dumps(evidence, ensure_ascii=True, indent=2, sort_keys=True))
        if evidence["result"] == "fail":
            print("La restauracion y limpieza se intentaron tras el fallo.", file=sys.stderr)
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
