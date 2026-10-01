#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Firefox Linux E2E for WebExtension -> Native Messaging -> Go host."""

from __future__ import annotations

import argparse
import contextlib
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import pwd
import shutil
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import time
from typing import Any, Iterator
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
import zipfile


EXTENSION_ID = "extension@dipgra.es"
EXTENSION_UUID = "8bb4947c-109d-4c05-bf91-3f1bca4de10b"
HOST_NAME = "com.dipgra.grxfirma"
W3C_ELEMENT_KEY = "element-6066-11e4-a52e-4f735466cecf"

REPO_ROOT = Path(__file__).resolve().parents[4]
EXTENSION_ROOT = REPO_ROOT / "packaging" / "browser-extensions"
FIREFOX_SOURCE = EXTENSION_ROOT / "src" / "firefox"
BUILD_MODULE_PATH = EXTENSION_ROOT / "build.py"
NATIVE_MANIFEST = (
    Path(pwd.getpwuid(os.getuid()).pw_dir)
    / ".mozilla"
    / "native-messaging-hosts"
    / f"{HOST_NAME}.json"
)


class E2EError(RuntimeError):
    pass


def run_command(
    command: list[str],
    *,
    cwd: Path | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        command,
        cwd=cwd,
        check=check,
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


def resolve_executable(value: str | None, candidates: list[str]) -> Path:
    if value:
        candidate = shutil.which(value) or value
        # Keep launcher symlinks such as /snap/bin/geckodriver intact. Resolving
        # that path to /usr/bin/snap loses the selected Snap application.
        path = Path(candidate).expanduser().absolute()
        if path.is_file() and os.access(path, os.X_OK):
            return path
        raise E2EError(f"ejecutable no disponible: {value}")

    for candidate in candidates:
        resolved = shutil.which(candidate)
        if resolved:
            return Path(resolved).absolute()
    raise E2EError(
        f"no se encontro ninguno de estos ejecutables: {', '.join(candidates)}"
    )


def resolve_go(value: str | None) -> Path:
    candidates = [str(Path.home() / "go" / "bin" / "go1.26.5"), "go1.26.5", "go"]
    return resolve_executable(value, candidates)


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


def build_firefox_xpi(output: Path) -> None:
    spec = importlib.util.spec_from_file_location(
        "grxfirma_extension_build", BUILD_MODULE_PATH
    )
    if spec is None or spec.loader is None:
        raise E2EError(f"no se pudo cargar {BUILD_MODULE_PATH}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.build_archive(FIREFOX_SOURCE, output)

    with zipfile.ZipFile(output) as archive:
        manifest = json.loads(archive.read("manifest.json"))
    packaged_id = (
        manifest.get("browser_specific_settings", {}).get("gecko", {}).get("id")
    )
    if packaged_id != EXTENSION_ID:
        raise E2EError(f"ID Gecko inesperado en el XPI: {packaged_id!r}")


def popup_locales() -> dict[str, dict[str, str]]:
    required = {
        "popupCheckingConnection",
        "popupNativeHostConnected",
        "popupLoadingCertificates",
        "popupNoCertificates",
    }
    locales: dict[str, dict[str, str]] = {}
    for catalog_path in sorted((FIREFOX_SOURCE / "_locales").glob("*/messages.json")):
        raw = json.loads(catalog_path.read_text(encoding="utf-8"))
        messages = {
            key: str(raw.get(key, {}).get("message", ""))
            for key in required
        }
        if any(not value for value in messages.values()):
            raise E2EError(f"catalogo i18n incompleto: {catalog_path}")
        locales[catalog_path.parent.name] = messages
    if not locales:
        raise E2EError("la extension Firefox no contiene catalogos i18n")
    return locales


def atomic_write(path: Path, content: bytes, mode: int) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        os.fchmod(fd, mode)
        with os.fdopen(fd, "wb") as stream:
            fd = -1
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(path)
    except BaseException:
        if fd >= 0:
            os.close(fd)
        temporary.unlink(missing_ok=True)
        raise


@contextlib.contextmanager
def manifest_registration(host: Path) -> Iterator[None]:
    try:
        previous_stat = NATIVE_MANIFEST.lstat()
    except FileNotFoundError:
        existed = False
        previous = b""
        previous_mode = 0o600
    else:
        existed = True
        if not stat.S_ISREG(previous_stat.st_mode):
            raise E2EError(
                f"el manifiesto existente no es un fichero regular: {NATIVE_MANIFEST}"
            )
        previous = NATIVE_MANIFEST.read_bytes()
        previous_mode = stat.S_IMODE(previous_stat.st_mode)
    manifest = {
        "name": HOST_NAME,
        "description": "GrxFirma Native Messaging Host E2E",
        "path": str(host),
        "type": "stdio",
        "allowed_extensions": [EXTENSION_ID],
    }
    atomic_write(
        NATIVE_MANIFEST,
        (json.dumps(manifest, ensure_ascii=True, indent=2) + "\n").encode("utf-8"),
        0o600,
    )
    try:
        yield
    finally:
        if existed:
            atomic_write(NATIVE_MANIFEST, previous, previous_mode)
        else:
            NATIVE_MANIFEST.unlink(missing_ok=True)


@contextlib.contextmanager
def exclusive_manifest_lock() -> Iterator[None]:
    lock_path = (
        Path(tempfile.gettempdir()) / f"grxfirma-firefox-e2e-{os.getuid()}.lock"
    )
    with lock_path.open("a+", encoding="utf-8") as lock:
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        yield


def is_snap_firefox(geckodriver: Path, firefox: Path | None) -> bool:
    paths = [str(geckodriver), str(firefox or "")]
    if any(path.startswith("/snap/") for path in paths):
        return True
    wrapper = Path("/usr/bin/firefox")
    if (firefox is None or firefox == wrapper) and wrapper.is_file():
        return "/snap/bin/firefox" in wrapper.read_text(
            encoding="utf-8", errors="ignore"
        )
    return False


class PortalPermissions:
    """Temporarily authorizes the Snap browser and its geckodriver sub-app."""

    def __init__(self, enabled: bool):
        self.enabled = enabled
        self.flatpak = shutil.which("flatpak") if enabled else None
        self.app_ids = ("snap.firefox", "snap.firefox_geckodriver")
        self.previous: dict[str, tuple[str, ...] | None] = {}

    def _read(self) -> dict[str, tuple[str, ...]]:
        if not self.flatpak:
            return {}
        result = run_command(
            [self.flatpak, "permissions", "webextensions"], check=False
        )
        if result.returncode != 0:
            raise E2EError(f"no se pudo leer el portal WebExtensions:\n{result.stdout}")
        entries: dict[str, tuple[str, ...]] = {}
        for line in result.stdout.splitlines():
            columns = line.split("\t")
            if len(columns) < 4 or columns[0] != "webextensions":
                continue
            if columns[1] != HOST_NAME:
                continue
            entries[columns[2]] = tuple(columns[3].split(","))
        return entries

    def grant(self) -> None:
        if not self.enabled:
            return
        if not self.flatpak:
            raise E2EError(
                "Firefox Snap requiere el portal WebExtensions y el CLI flatpak para "
                "autorizar una prueba headless"
            )
        current = self._read()
        for app_id in self.app_ids:
            self.previous[app_id] = current.get(app_id)
            run_command(
                [
                    self.flatpak,
                    "permission-set",
                    "webextensions",
                    HOST_NAME,
                    app_id,
                    "yes",
                ]
            )

    def restore(self) -> None:
        if not self.enabled or not self.flatpak:
            return
        errors: list[str] = []
        for app_id in reversed(self.app_ids):
            if app_id not in self.previous:
                continue
            previous = self.previous.get(app_id)
            if previous is None:
                command = [
                    self.flatpak,
                    "permission-remove",
                    "webextensions",
                    HOST_NAME,
                    app_id,
                ]
            else:
                command = [
                    self.flatpak,
                    "permission-set",
                    "webextensions",
                    HOST_NAME,
                    app_id,
                    *previous,
                ]
            result = run_command(command, check=False)
            if result.returncode != 0:
                errors.append(result.stdout.strip() or " ".join(command))
            else:
                del self.previous[app_id]
        if errors:
            raise E2EError(
                "no se restauraron permisos del portal: " + "; ".join(errors)
            )


def free_tcp_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def firefox_launch_arguments(headed: bool) -> list[str]:
    arguments = ["-remote-allow-system-access"]
    if not headed:
        arguments.append("-headless")
    return arguments


class WebDriverClient:
    def __init__(
        self,
        geckodriver: Path,
        log_path: Path,
        *,
        firefox: Path | None,
        headed: bool,
    ) -> None:
        self.geckodriver = geckodriver
        self.log_path = log_path
        self.firefox = firefox
        self.headed = headed
        self.port = free_tcp_port()
        self.process: subprocess.Popen[str] | None = None
        self.log_stream: Any = None
        self.session_id = ""
        self.capabilities: dict[str, Any] = {}

    def _url(self, path: str) -> str:
        return f"http://127.0.0.1:{self.port}{path}"

    def request(self, method: str, path: str, payload: Any | None = None) -> Any:
        body = None
        headers: dict[str, str] = {}
        if payload is not None:
            body = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json; charset=utf-8"
        request = Request(self._url(path), data=body, headers=headers, method=method)
        try:
            with urlopen(request, timeout=30) as response:
                decoded = json.loads(response.read().decode("utf-8"))
        except HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")
            raise E2EError(
                f"WebDriver {method} {path}: HTTP {error.code}: {detail}"
            ) from error
        except URLError as error:
            raise E2EError(f"WebDriver {method} {path}: {error}") from error
        value = decoded.get("value")
        if isinstance(value, dict) and value.get("error"):
            raise E2EError(
                f"WebDriver {value.get('error')}: {value.get('message', 'sin detalle')}"
            )
        return value

    def start(self, timeout: float) -> None:
        command = [str(self.geckodriver), "--port", str(self.port), "--log", "info"]
        if self.firefox:
            command.extend(["--binary", str(self.firefox)])
        self.log_stream = self.log_path.open("w", encoding="utf-8")
        self.process = subprocess.Popen(
            command,
            stdout=self.log_stream,
            stderr=subprocess.STDOUT,
            text=True,
        )

        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self.process.poll() is not None:
                raise E2EError(
                    f"geckodriver termino antes de iniciar (codigo {self.process.returncode})"
                )
            try:
                self.request("GET", "/status")
                break
            except E2EError:
                time.sleep(0.1)
        else:
            raise E2EError("geckodriver no abrio su endpoint dentro del plazo")

        arguments = firefox_launch_arguments(self.headed)
        preferences = {
            "browser.shell.checkDefaultBrowser": False,
            "datareporting.policy.dataSubmissionEnabled": False,
            "extensions.webextensions.uuids": json.dumps(
                {EXTENSION_ID: EXTENSION_UUID}
            ),
            "toolkit.telemetry.reportingpolicy.firstRun": False,
        }
        value = self.request(
            "POST",
            "/session",
            {
                "capabilities": {
                    "alwaysMatch": {
                        "browserName": "firefox",
                        "acceptInsecureCerts": True,
                        "moz:firefoxOptions": {
                            "args": arguments,
                            "prefs": preferences,
                        },
                    }
                }
            },
        )
        if not isinstance(value, dict) or not value.get("sessionId"):
            raise E2EError(f"respuesta de sesion WebDriver invalida: {value!r}")
        self.session_id = str(value["sessionId"])
        self.capabilities = dict(value.get("capabilities") or {})

    def session_request(
        self, method: str, suffix: str, payload: Any | None = None
    ) -> Any:
        if not self.session_id:
            raise E2EError("la sesion WebDriver no esta iniciada")
        return self.request(method, f"/session/{self.session_id}{suffix}", payload)

    def install_addon(self, xpi: Path) -> str:
        value = self.session_request(
            "POST", "/moz/addon/install", {"path": str(xpi), "temporary": True}
        )
        return str(value)

    def navigate(self, url: str) -> None:
        self.session_request("POST", "/url", {"url": url})

    def set_context(self, context: str) -> None:
        if context not in {"chrome", "content"}:
            raise ValueError(f"contexto Marionette no soportado: {context}")
        self.session_request("POST", "/moz/context", {"context": context})

    def execute_script(self, script: str, arguments: list[Any]) -> Any:
        return self.session_request(
            "POST",
            "/execute/sync",
            {"script": script, "args": arguments},
        )

    def open_extension_page(self, url: str) -> None:
        """Open a privileged WebExtension URL without WebDriver /url navigation."""
        self.set_context("chrome")
        try:
            self.execute_script(
                """
                openTrustedLinkIn(arguments[0], "current");
                """,
                [url],
            )
        finally:
            self.set_context("content")

    def find_element(self, selector: str) -> str:
        value = self.session_request(
            "POST", "/element", {"using": "css selector", "value": selector}
        )
        if not isinstance(value, dict) or W3C_ELEMENT_KEY not in value:
            raise E2EError(f"elemento no encontrado: {selector}")
        return str(value[W3C_ELEMENT_KEY])

    def find_elements(self, selector: str) -> list[str]:
        value = self.session_request(
            "POST", "/elements", {"using": "css selector", "value": selector}
        )
        if not isinstance(value, list):
            return []
        return [str(item[W3C_ELEMENT_KEY]) for item in value if W3C_ELEMENT_KEY in item]

    def element_text(self, element_id: str) -> str:
        return str(self.session_request("GET", f"/element/{element_id}/text"))

    def element_attribute(self, element_id: str, name: str) -> str:
        value = self.session_request("GET", f"/element/{element_id}/attribute/{name}")
        return "" if value is None else str(value)

    def close(self) -> None:
        error: BaseException | None = None
        if self.session_id:
            try:
                self.request("DELETE", f"/session/{self.session_id}")
            except BaseException as exc:  # Preserve cleanup even after browser failure.
                error = exc
            self.session_id = ""
        if self.process and self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait(timeout=5)
        if self.log_stream:
            self.log_stream.close()
        if error:
            raise error


def wait_until(description: str, timeout: float, operation: Any) -> Any:
    deadline = time.monotonic() + timeout
    last_error: BaseException | None = None
    while time.monotonic() < deadline:
        try:
            value = operation()
            if value:
                return value
        except E2EError as error:
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


def log_tail(path: Path, lines: int = 80) -> str:
    if not path.is_file():
        return ""
    return "\n".join(
        path.read_text(encoding="utf-8", errors="replace").splitlines()[-lines:]
    )


def install_signal_handlers() -> None:
    def interrupt(signum: int, _frame: Any) -> None:
        raise KeyboardInterrupt(f"senal {signum}")

    signal.signal(signal.SIGTERM, interrupt)
    signal.signal(signal.SIGHUP, interrupt)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--firefox", default=os.environ.get("FIREFOX_E2E_FIREFOX"))
    parser.add_argument(
        "--geckodriver", default=os.environ.get("FIREFOX_E2E_GECKODRIVER")
    )
    parser.add_argument("--go", default=os.environ.get("FIREFOX_E2E_GO"))
    parser.add_argument(
        "--native-host",
        type=Path,
        default=(
            Path(os.environ["FIREFOX_E2E_NATIVE_HOST"])
            if os.environ.get("FIREFOX_E2E_NATIVE_HOST")
            else None
        ),
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
    install_signal_handlers()
    workdir = Path(tempfile.mkdtemp(prefix="GrxFirma-firefox-e2e-", dir=Path.home()))
    workdir.chmod(0o700)
    log_path = workdir / "geckodriver.log"
    driver: WebDriverClient | None = None
    permissions: PortalPermissions | None = None
    success = False

    try:
        go_binary = resolve_go(args.go)
        geckodriver = resolve_executable(args.geckodriver, ["geckodriver"])
        firefox = resolve_executable(args.firefox, []) if args.firefox else None
        host = workdir / "grxfirma-nativehost"
        xpi = workdir / "grxfirma-firefox-e2e.xpi"
        host_source = prepare_native_host(go_binary, host, args.native_host)
        build_firefox_xpi(xpi)

        snap = is_snap_firefox(geckodriver, firefox)
        permissions = PortalPermissions(snap)
        with exclusive_manifest_lock(), manifest_registration(host):
            permissions.grant()
            try:
                driver = WebDriverClient(
                    geckodriver,
                    log_path,
                    firefox=firefox,
                    headed=args.headed,
                )
                driver.start(args.timeout)
                addon_id = driver.install_addon(xpi)
                if addon_id != EXTENSION_ID:
                    raise E2EError(f"Firefox instalo un ID distinto: {addon_id!r}")

                popup_url = f"moz-extension://{EXTENSION_UUID}/popup.html"
                driver.open_extension_page(popup_url)
                locales = popup_locales()
                checking_messages = {
                    messages["popupCheckingConnection"]
                    for messages in locales.values()
                }
                status_element = wait_until(
                    "el estado del popup",
                    args.timeout,
                    lambda: driver.find_element("#status"),
                )
                status = wait_until(
                    "la respuesta ping",
                    args.timeout,
                    lambda: (
                        text
                        if (text := driver.element_text(status_element))
                        not in checking_messages
                        else ""
                    ),
                )
                active_locale = next(
                    (
                        locale
                        for locale, messages in locales.items()
                        if status == messages["popupNativeHostConnected"]
                    ),
                    "",
                )
                if not active_locale:
                    raise E2EError(
                        f"el popup no resolvio un catalogo publicado: {status!r}"
                    )
                localized = locales[active_locale]
                status_class = driver.element_attribute(status_element, "class")
                if (
                    status != localized["popupNativeHostConnected"]
                    or "ok" not in status_class.split()
                ):
                    raise E2EError(f"ping nativo fallo en el popup: {status!r}")

                certificate_list = driver.find_element("#certList")
                certificate_state = wait_until(
                    "getCertificates",
                    args.timeout,
                    lambda: (
                        text
                        if (text := driver.element_text(certificate_list))
                        != localized["popupLoadingCertificates"]
                        else ""
                    ),
                )
                certificate_count = len(driver.find_elements("#certList .cert"))
                if (
                    certificate_count == 0
                    and certificate_state != localized["popupNoCertificates"]
                ):
                    raise E2EError("getCertificates no produjo una respuesta valida")

                processes = wait_until(
                    "el proceso del host Go",
                    args.timeout,
                    lambda: native_host_processes(host),
                )
                process = processes[0]
                arguments = process["arguments"]
                if (
                    str(NATIVE_MANIFEST) not in arguments
                    or EXTENSION_ID not in arguments
                ):
                    raise E2EError(
                        "Firefox arranco el host sin el manifiesto o ID Gecko esperados"
                    )

                evidence = {
                    "result": "pass",
                    "firefox": {
                        "version": driver.capabilities.get("browserVersion"),
                        "package": "snap" if snap else "unconfined",
                        "temporary_profile": True,
                    },
                    "extension": {
                        "id": addon_id,
                        "temporary_install": True,
                        "resolved_locale": active_locale,
                        "xpi_sha256": sha256_file(xpi),
                    },
                    "native_messaging": {
                        "host": HOST_NAME,
                        "manifest": str(NATIVE_MANIFEST),
                        "ping": True,
                        "get_certificates": True,
                        "certificate_count": certificate_count,
                        "host_pid": process["pid"],
                        "host_sha256": sha256_file(host),
                        "firefox_arguments_verified": True,
                        "host_source": host_source,
                    },
                }
            finally:
                if driver:
                    driver.close()
                    driver = None
                permissions.restore()

        evidence["cleanup"] = {
            "manifest_restored": True,
            "portal_permissions_restored": True,
        }
        print(json.dumps(evidence, ensure_ascii=True, indent=2, sort_keys=True))
        success = True
        return 0
    except (E2EError, KeyboardInterrupt, OSError, subprocess.SubprocessError) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        tail = log_tail(log_path)
        if tail:
            print("--- geckodriver.log (tail) ---", file=sys.stderr)
            print(tail, file=sys.stderr)
        return 1
    finally:
        if driver:
            with contextlib.suppress(BaseException):
                driver.close()
        if permissions:
            with contextlib.suppress(BaseException):
                permissions.restore()
        if args.keep_workdir:
            print(f"workdir: {workdir}", file=sys.stderr)
        else:
            shutil.rmtree(workdir, ignore_errors=True)
        if not success:
            print(
                "La prueba fallo; el manifiesto y los permisos se intentaron restaurar.",
                file=sys.stderr,
            )


if __name__ == "__main__":
    raise SystemExit(main())
