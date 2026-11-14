#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Relaciona manifiestos, avisos legales y revisión por versión sin usar la red."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
import tomllib
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from scripts.ci.prepare_runtime_sbom_scope import (  # noqa: E402
    ANDROID_RUNTIME_CONFIGURATIONS,
)
from scripts.ci.filter_gradle_lockfile import filter_lockfile  # noqa: E402
from scripts.ci.third_party_bases import read_bases  # noqa: E402

USES = {"ejecucion", "pruebas", "construccion", "opcional"}
REGISTRY = "legal/dependency-review.json"
BASELINE_PENDING = "legal/dependency-license-pending-0.0.127.json"


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def discover(root: Path) -> list[Path]:
    """Descubre nuevas fuentes; una fuente desconocida hace fallar el control."""
    names = {"go.mod", "go.sum", "package-lock.json", "gradle.lockfile",
             "settings-gradle.lockfile", "packages.lock.json", "licensing.toml"}
    return sorted(p for p in root.rglob("*") if p.is_file()
                  and not {".git", "node_modules", ".gradle", "bin", "obj"}.intersection(p.relative_to(root).parts)
                  and (p.name in names or p.suffix in {".csproj", ".pro", ".kts"}
                       or p.as_posix().endswith("gradle-wrapper.properties")))


def collect(root: Path) -> tuple[list[dict], list[dict]]:
    rows: dict[str, dict] = {}
    manifests = []

    def add(eco, name, version, source, use, provenance=None, integrity=None):
        key = f"{eco}:{name}@{version}"
        row = rows.setdefault(key, {"id": key, "ecosystem": eco, "name": name,
                                    "version": version, "occurrences": []})
        occurrence = {"source": source, "use": use}
        if provenance:
            occurrence["provenance"] = provenance
        if integrity:
            occurrence["integrity"] = integrity
        if occurrence not in row["occurrences"]:
            row["occurrences"].append(occurrence)

    paths = discover(root)
    # go.sum aporta integridad; una entrada histórica no prueba selección.
    sums = {}
    for path in paths:
        if path.name == "go.sum":
            for line in path.read_text().splitlines():
                name, version, value = line.split()
                sums[(name, version)] = value
    for path in paths:
        source = path.relative_to(root).as_posix()
        manifests.append({"path": source, "sha256": digest(path)})
        text = path.read_text()
        if path.name == "go.mod":
            for name, version in re.findall(r"^\s*(?:require\s+)?([\w./-]+)\s+(v[^\s]+)", text, re.M):
                use = "pruebas" if name in {"github.com/stretchr/testify", "github.com/davecgh/go-spew", "github.com/pmezard/go-difflib", "github.com/kr/text"} else "ejecucion"
                if name == "github.com/miekg/pkcs11":
                    use = "opcional"
                replacement = re.search(r"^replace\s+" + re.escape(name) + r"\s+=>\s+(\S+)", text, re.M)
                add("go", name, version, source, use,
                    replacement[1] if replacement else "módulo Go; origen autenticado por go.sum",
                    sums.get((name, version), sums.get((name, version + "/go.mod"))))
            version = re.search(r"^go\s+(\S+)", text, re.M)[1]
            add("toolchain", "Go", version, source, "construccion", "go.mod")
        elif path.name == "package-lock.json":
            if source != "packaging/browser-extensions/tools/package-lock.json":
                raise ValueError(f"clasificar uso del nuevo lockfile npm: {source}")
            for location, value in json.loads(text)["packages"].items():
                if not location:
                    continue
                name = value.get("name") or location.rsplit("node_modules/", 1)[-1]
                add("npm", name, value["version"], source, "construccion", value.get("resolved"), value.get("integrity"))
        elif path.name.endswith("gradle.lockfile"):
            if source not in {"mobile/android/app/gradle.lockfile", "mobile/android/settings-gradle.lockfile"}:
                raise ValueError(f"clasificar nuevo lockfile Gradle: {source}")
            runtime = set()
            if path.name == "gradle.lockfile":
                runtime = {line.split("=")[0] for line in filter_lockfile(path, list(ANDROID_RUNTIME_CONFIGURATIONS)).splitlines() if line and not line.startswith(("#", "empty="))}
            for line in text.splitlines():
                if not line or line.startswith(("#", "empty=")):
                    continue
                coordinate, configurations = line.split("=", 1)
                group, name, version = coordinate.split(":")
                use = "ejecucion" if coordinate in runtime else ("pruebas" if all("Test" in x for x in configurations.split(",")) else "construccion")
                add("maven", f"{group}:{name}", version, source, use, {"configurations": configurations.split(",")})
        elif path.name == "packages.lock.json":
            if source.startswith("cmd/gui-winui/src/"):
                use = "ejecucion"
            elif source.startswith(("cmd/gui-winui/tests/", "scripts/windows-qa/")):
                use = "pruebas"
            else:
                raise ValueError(f"clasificar nuevo lockfile NuGet: {source}")
            for framework, dependencies in json.loads(text)["dependencies"].items():
                for name, value in dependencies.items():
                    if value["type"] == "Project":
                        continue
                    add("nuget", name, value["resolved"], source, use, {"framework": framework}, value.get("contentHash"))
        elif path.suffix == ".pro" and path.name != "proguard-rules.pro":
            if not source.startswith("cmd/gui-qml/"):
                raise ValueError(f"clasificar nuevo proyecto Qt: {source}")
            for modules in re.findall(r"QT\s*\+=\s*([^\n#]+)", text):
                for module in modules.split():
                    add("qt", module, "no-fijada-en-proyecto", source,
                        "ejecucion" if path.name == "grxfirma_qt.pro" else "pruebas", "kit Qt del constructor; falta atestación de release")
        elif path.name == "gradle-wrapper.properties":
            match = re.search(r"gradle-([\d.]+)-(?:bin|all)\.zip", text)
            if not match:
                raise ValueError(f"no se reconoce el wrapper: {source}")
            add("toolchain", "Gradle", match[1], source, "construccion", "distributionUrl", re.search(r"distributionSha256Sum=(\S+)", text)[1])
        elif path.suffix == ".csproj":
            for ref in ET.fromstring(text).iter("PackageReference"):
                # El lockfile debe contener la versión realmente resuelta.
                if not (path.parent / "packages.lock.json").is_file():
                    raise ValueError(f"falta lockfile NuGet para {source}: {ref.attrib}")
        elif path.suffix == ".kts":
            # Se conserva el hash de la declaración; Gradle fija el grafo en el lockfile.
            if not source.startswith("mobile/android/"):
                raise ValueError(f"clasificar nuevo proyecto Gradle: {source}")
    bases, errors = read_bases(root)
    if errors:
        raise ValueError("; ".join(errors))
    for name, version in bases:
        add("vendored", name, version, "third_party/", "ejecucion", "Base: de README.md; parches locales, no equivale al upstream")
    legal = tomllib.loads((root / "legal/licensing.toml").read_text())
    for entry in legal["third_party"]:
        add("legal", entry["name"], "copia-local", "legal/licensing.toml", "construccion" if entry["name"] == "Gradle Wrapper" else "ejecucion", entry)
    return sorted(rows.values(), key=lambda r: r["id"]), manifests


def validate(rows: list[dict], registry: dict, strict: bool = False, baseline_pending: set[str] | None = None) -> list[str]:
    if baseline_pending is None:
        baseline_pending = set(json.loads((ROOT / BASELINE_PENDING).read_text())["ids"])
    pending = []
    for row in rows:
        review = registry.get(row["id"])
        if not review:
            raise ValueError(f"dependencia nueva sin revisión de licencia y uso: {row['id']}")
        uses = {o["use"] for o in row["occurrences"]}
        if not uses or not uses <= USES or uses != set(review.get("uses", [])):
            raise ValueError(f"uso sin clasificar o cambiado: {row['id']}")
        if not review.get("license_evidence"):
            raise ValueError(f"falta procedencia de licencia: {row['id']}")
        if not review.get("licenses"):
            if not review.get("baseline_pending") or row["id"] not in baseline_pending:
                raise ValueError(f"licencia ausente: {row['id']}")
            pending.append(row["id"])
            if strict:
                raise ValueError(f"licencia pendiente de acreditar: {row['id']}")
    return pending


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--strict", action="store_true", help="Exige cerrar también la deuda de licencia inicial")
    args = parser.parse_args()
    try:
        rows, manifests = collect(args.root)
        review_document = json.loads((args.root / REGISTRY).read_text())
        registry = review_document["dependencies"]
        for manifest in manifests:
            if Path(manifest["path"]).suffix in {".kts", ".csproj", ".pro"}:
                if review_document.get("declarations", {}).get(manifest["path"]) != manifest["sha256"]:
                    raise ValueError(f"declaración cambiada: revisar licencia y uso de {manifest['path']}")
        pending = validate(rows, registry, args.strict, set(json.loads((args.root / BASELINE_PENDING).read_text())["ids"]))
        for row in rows:
            row["review"] = registry[row["id"]]
        result = {"schema": 1, "version": args.version, "scope": "manifiestos fuente; complemento de SBOM SPDX de CI, no atestación de binarios publicados",
                  "manifests": manifests, "review_registry_sha256": digest(args.root / REGISTRY),
                  "pending_licenses": pending, "dependencies": rows}
        if args.output:
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
        print(f"{len(rows)} dependencias clasificadas; {len(pending)} licencias pendientes de la línea base; {len(manifests)} fuentes con SHA-256.")
    except (OSError, ValueError, KeyError) as error:
        print(f"Expediente incompleto: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
