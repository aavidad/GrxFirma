#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Extrae propuestas de licencia de Syft y cachés locales; nunca acepta riesgos."""

import argparse
import json
import xml.etree.ElementTree as ET
from pathlib import Path

from dependency_dossier import ROOT, collect, digest


def pom_licenses(cache, group, name, version, seen=None):
    seen = set() if seen is None else seen
    key = (group, name, version)
    if key in seen:
        return [], []
    seen.add(key)
    for path in sorted((cache / group / name / version).glob("*/*.pom")):
        xml = ET.parse(path).getroot()
        for node in xml.iter():
            node.tag = node.tag.rsplit("}", 1)[-1]
        licenses = [{"name": n.findtext("name"), "url": n.findtext("url")}
                    for n in xml.findall("./licenses/license")]
        evidence = [{"source": "$GRADLE_CACHE/" + str(path.relative_to(cache)), "sha256": digest(path)}]
        if licenses:
            return [json.dumps(x, ensure_ascii=False, sort_keys=True) for x in licenses], evidence
        parent = xml.find("parent")
        if parent is not None:
            values, inherited = pom_licenses(cache, parent.findtext("groupId"), parent.findtext("artifactId"), parent.findtext("version"), seen)
            return values, evidence + inherited
        return [], evidence
    return [], []


def candidates(root, syft, gradle_cache, nuget_cache, go_root):
    rows, manifests = collect(root)
    artifacts = json.loads(syft.read_text())["artifacts"]
    legal = __import__("tomllib").loads((root / "legal/licensing.toml").read_text())["third_party"]
    entries = {}
    for row in rows:
        eco, name, version = row["ecosystem"], row["name"], row["version"]
        licenses, evidence = [], []
        if eco in {"go", "npm"}:
            for artifact in artifacts:
                if artifact["type"] == {"go": "go-module", "npm": "npm"}[eco] and artifact["name"] == name and artifact["version"] == version:
                    for value in artifact["licenses"]:
                        license_value = value.get("spdxExpression") or value["value"]
                        if license_value not in licenses:
                            licenses.append(license_value)
                    evidence.append({"source": "syft-local.json", "sha256": digest(syft), "licenses": artifact["licenses"]})
        elif eco == "maven":
            group, artifact = name.split(":")
            licenses, evidence = pom_licenses(gradle_cache, group, artifact, version)
        elif eco == "nuget":
            for path in sorted((nuget_cache / name.lower() / version).glob("*.nuspec")):
                xml = ET.parse(path).getroot()
                for node in xml.iter():
                    node.tag = node.tag.rsplit("}", 1)[-1]
                for node in xml.findall("./metadata/license"):
                    licenses.append(f"{node.attrib.get('type', '')}: {node.text}")
                evidence.append({"source": "$NUGET_CACHE/" + str(path.relative_to(nuget_cache)), "sha256": digest(path)})
        elif eco in {"legal", "vendored"}:
            base_name = name.rsplit("/", 1)[-1]
            entry = next(x for x in legal if x["name"] == base_name)
            licenses = [entry["license"]]
            for notice in entry.get("notice_files", [entry.get("notice_file")]):
                evidence.append({"source": notice, "sha256": digest(root / notice)})
        elif eco == "toolchain" and name == "Gradle":
            licenses = ["Apache-2.0"]
            evidence = [{"source": "mobile/android/gradlew (wrapper; licencia de distribución pendiente de cotejo)", "sha256": digest(root / "mobile/android/gradlew")}]
        elif eco == "toolchain" and name == "Go":
            if (go_root / "VERSION").is_file() and (go_root / "VERSION").read_text().splitlines()[0] == "go" + version:
                licenses = ["BSD-3-Clause"]
                evidence = [{"source": "$GOROOT/LICENSE", "sha256": digest(go_root / "LICENSE"), "note": "Toolchain local de la versión declarada; cotejar cada plataforma distribuida."}]
        if eco == "go" and name in {"github.com/digitorus/pdf", "github.com/digitorus/pdfsign"}:
            base_name = name.rsplit("/", 1)[-1]
            notice = f"third_party/{base_name}/LICENSE"
            declaration = next(x for x in legal if x["name"] == base_name)
            licenses = [declaration["license"]]
            evidence = [{"source": notice, "sha256": digest(root / notice), "note": "Se distribuye la copia local indicada por replace, con sus parches."}]
        entry = {"uses": sorted({o["use"] for o in row["occurrences"]}), "licenses": licenses, "license_evidence": evidence}
        if not licenses:
            entry["baseline_pending"] = "Deuda técnica inicial 2026-11-14: falta licencia exacta acreditada localmente para esta identidad y versión; no supone aceptación. Alberto debe asignar responsable y decidir distribución."
            if not evidence:
                entry["license_evidence"] = [{"source": row["occurrences"][0]["source"], "note": "El manifiesto no acredita licencia; no se ha consultado la red."}]
        entries[row["id"]] = entry
    return {"schema": 1, "status": "propuesta técnica; aceptación contractual pendiente de Alberto", "declarations": {m["path"]: m["sha256"] for m in manifests if Path(m["path"]).suffix in {".kts", ".csproj", ".pro"}}, "dependencies": entries}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--syft-json", required=True, type=Path)
    parser.add_argument("--gradle-cache", required=True, type=Path)
    parser.add_argument("--nuget-cache", required=True, type=Path)
    parser.add_argument("--go-root", type=Path, required=True)
    parser.add_argument("--output", required=True, type=Path, help="Propuesta separada; revisar antes de incorporar a legal/dependency-review.json")
    args = parser.parse_args()
    result = candidates(ROOT, args.syft_json, args.gradle_cache, args.nuget_cache, args.go_root)
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
