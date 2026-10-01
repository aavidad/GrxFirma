#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Comprueba la cobertura legal de todos los ficheros versionados.

La clasificación se define en ``legal/licensing.toml``. El modo estricto exige
la cabecera canónica en los formatos que admiten comentarios. El modo
``--inventory-only`` permite validar clasificación, cobertura central, licencia
raíz y avisos de terceros antes de completar una migración masiva.
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import subprocess
import sys
import tomllib
from collections import Counter
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Any, Iterable, Mapping, Sequence


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_POLICY = ROOT / "legal" / "licensing.toml"

CENTRAL_CATEGORIES = frozenset(
    {"binary-resource", "generated", "json", "uncommentable"}
)
LICENSE_REQUIRED_MARKERS = (
    "LICENCIA PÚBLICA DE LA UNIÓN EUROPEA v. 1.2",
    "EUPL © Unión Europea 2007, 2016",
    "Licencia cedida con arreglo a la EUPL",
    "5. Obligaciones del licenciatario",
    "15. Legislación aplicable",
    "Son «licencias compatibles» con arreglo al artículo 5 de la EUPL",
)
PUBLISHED_LICENSE_PATHS = (
    "cmd/grxfirma/main.go",
    "docs/ARCHITECTURE.md",
    "packaging/linux/README_LINUX_SUITE.md",
    "packaging/linux/man/grxfirma.1",
    "packaging/macos/README_AFIRMAURI_MACOS.md",
    "packaging/macos/README_CLI_MACOS.md",
    "packaging/macos/README_DESKTOP_QML_MACOS.md",
    "packaging/macos/README_MACOS_SUITE.md",
    "packaging/macos/README_NATIVEHOST_MACOS.md",
    "packaging/windows/README_AFIRMAURI_WINDOWS.md",
    "packaging/windows/README_CLI_WINDOWS.md",
    "packaging/windows/README_DESKTOP_QML_WINDOWS.md",
    "packaging/windows/README_NATIVEHOST_WINDOWS.md",
    "packaging/windows/README_WINDOWS_SUITE.md",
)
FORBIDDEN_PROJECT_LICENSE_DECLARATIONS = (
    "GPLv3",
    "GNU GPL",
    "GPL v3",
    "GPL-3.0",
)


@dataclass(frozen=True)
class CanonicalHeader:
    copyright: str
    authorship: str
    license_notice: str
    spdx: str

    @property
    def required_lines(self) -> tuple[str, ...]:
        return (
            self.copyright,
            self.authorship,
            self.license_notice,
            self.spdx,
        )


@dataclass(frozen=True)
class ThirdPartyPolicy:
    name: str
    paths: frozenset[str]
    globs: tuple[str, ...]
    license: str
    copyright: str
    notice_files: tuple[str, ...]
    required_notice_markers: tuple[str, ...]

    def matches(self, path: str) -> bool:
        return path in self.paths or any(
            fnmatch.fnmatchcase(path, pattern) for pattern in self.globs
        )


@dataclass(frozen=True)
class ClassificationPolicy:
    inline_extensions: frozenset[str]
    inline_filenames: frozenset[str]
    json_extensions: frozenset[str]
    binary_resource_extensions: frozenset[str]
    uncommentable_extensions: frozenset[str]
    uncommentable_filenames: frozenset[str]
    license_text_filenames: frozenset[str]
    generated_globs: tuple[str, ...]
    central_fixture_paths: frozenset[str]


@dataclass(frozen=True)
class LegalPolicy:
    schema_version: int
    canonical_header: CanonicalHeader
    central_categories: frozenset[str]
    central_copyright: str
    central_authorship: str
    central_license_notice: str
    central_spdx: str
    classification: ClassificationPolicy
    third_party: tuple[ThirdPartyPolicy, ...]


@dataclass(frozen=True)
class ClassifiedPath:
    path: str
    category: str
    third_party_name: str | None = None


@dataclass(frozen=True)
class Violation:
    path: str
    code: str
    message: str

    def render(self) -> str:
        return f"{self.path}: [{self.code}] {self.message}"


@dataclass(frozen=True)
class AuditResult:
    tracked_files: tuple[str, ...]
    classifications: tuple[ClassifiedPath, ...]
    violations: tuple[Violation, ...]

    @property
    def counts(self) -> Counter[str]:
        return Counter(item.category for item in self.classifications)


def _string_set(table: Mapping[str, Any], key: str, *, context: str) -> frozenset[str]:
    value = table.get(key)
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        raise ValueError(f"{context}.{key} debe ser una lista de cadenas")
    return frozenset(value)


def _string_tuple(
    table: Mapping[str, Any], key: str, *, context: str
) -> tuple[str, ...]:
    return tuple(sorted(_string_set(table, key, context=context)))


def _required_string(table: Mapping[str, Any], key: str, *, context: str) -> str:
    value = table.get(key)
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{context}.{key} debe ser una cadena no vacía")
    return value


def load_policy(path: Path = DEFAULT_POLICY) -> LegalPolicy:
    """Carga y valida la política legal central."""
    with path.open("rb") as policy_file:
        raw = tomllib.load(policy_file)

    schema_version = raw.get("schema_version")
    if schema_version != 1:
        raise ValueError(f"schema_version no soportada en {path}: {schema_version!r}")

    header_table = raw.get("canonical_header")
    central_table = raw.get("central_coverage")
    classification_table = raw.get("classification")
    third_party_tables = raw.get("third_party")
    if not isinstance(header_table, dict):
        raise ValueError("falta la tabla [canonical_header]")
    if not isinstance(central_table, dict):
        raise ValueError("falta la tabla [central_coverage]")
    if not isinstance(classification_table, dict):
        raise ValueError("falta la tabla [classification]")
    if not isinstance(third_party_tables, list):
        raise ValueError("falta al menos una tabla [[third_party]]")

    canonical_header = CanonicalHeader(
        copyright=_required_string(
            header_table, "copyright", context="canonical_header"
        ),
        authorship=_required_string(
            header_table, "authorship", context="canonical_header"
        ),
        license_notice=_required_string(
            header_table, "license_notice", context="canonical_header"
        ),
        spdx=_required_string(header_table, "spdx", context="canonical_header"),
    )
    if canonical_header.spdx != "SPDX-License-Identifier: EUPL-1.2":
        raise ValueError("la licencia canónica debe ser EUPL-1.2")
    if canonical_header.copyright != (
        "Derechos de autor (C) 2026 Alberto Avidad Fernández."
    ):
        raise ValueError("el copyright canónico no coincide con el acordado")
    if canonical_header.authorship != "Autoría: Alberto Avidad Fernández":
        raise ValueError("la autoría canónica no coincide con la acordada")

    central_categories = _string_set(
        central_table, "categories", context="central_coverage"
    )
    if central_categories != CENTRAL_CATEGORIES:
        raise ValueError(
            "central_coverage.categories debe cubrir exactamente "
            f"{sorted(CENTRAL_CATEGORIES)}"
        )

    classification = ClassificationPolicy(
        inline_extensions=_string_set(
            classification_table,
            "inline_extensions",
            context="classification",
        ),
        inline_filenames=_string_set(
            classification_table,
            "inline_filenames",
            context="classification",
        ),
        json_extensions=_string_set(
            classification_table,
            "json_extensions",
            context="classification",
        ),
        binary_resource_extensions=_string_set(
            classification_table,
            "binary_resource_extensions",
            context="classification",
        ),
        uncommentable_extensions=_string_set(
            classification_table,
            "uncommentable_extensions",
            context="classification",
        ),
        uncommentable_filenames=_string_set(
            classification_table,
            "uncommentable_filenames",
            context="classification",
        ),
        license_text_filenames=_string_set(
            classification_table,
            "license_text_filenames",
            context="classification",
        ),
        generated_globs=_string_tuple(
            classification_table,
            "generated_globs",
            context="classification",
        ),
        central_fixture_paths=_string_set(
            classification_table,
            "central_fixture_paths",
            context="classification",
        ),
    )

    extension_groups = (
        classification.inline_extensions,
        classification.json_extensions,
        classification.binary_resource_extensions,
        classification.uncommentable_extensions,
    )
    seen_extensions: set[str] = set()
    for group in extension_groups:
        overlap = seen_extensions.intersection(group)
        if overlap:
            raise ValueError(
                "extensiones duplicadas entre categorías: " + ", ".join(sorted(overlap))
            )
        seen_extensions.update(group)

    third_party: list[ThirdPartyPolicy] = []
    for index, table in enumerate(third_party_tables):
        context = f"third_party[{index}]"
        if not isinstance(table, dict):
            raise ValueError(f"{context} debe ser una tabla")
        paths = table.get("paths", [])
        globs = table.get("globs", [])
        notice_files = table.get("notice_files")
        notice_file = table.get("notice_file")
        if notice_files is None:
            notice_files = [] if notice_file is None else [notice_file]
        for key, value in (
            ("paths", paths),
            ("globs", globs),
            ("notice_files", notice_files),
        ):
            if not isinstance(value, list) or not all(
                isinstance(item, str) for item in value
            ):
                raise ValueError(f"{context}.{key} debe ser una lista")
        markers = table.get("required_notice_markers")
        if not isinstance(markers, list) or not all(
            isinstance(item, str) for item in markers
        ):
            raise ValueError(f"{context}.required_notice_markers debe ser una lista")
        third_party.append(
            ThirdPartyPolicy(
                name=_required_string(table, "name", context=context),
                paths=frozenset(paths),
                globs=tuple(globs),
                license=_required_string(table, "license", context=context),
                copyright=_required_string(table, "copyright", context=context),
                notice_files=tuple(notice_files),
                required_notice_markers=tuple(markers),
            )
        )

    central_copyright = _required_string(
        central_table, "copyright", context="central_coverage"
    )
    central_authorship = _required_string(
        central_table, "authorship", context="central_coverage"
    )
    central_license_notice = _required_string(
        central_table, "license_notice", context="central_coverage"
    )
    central_spdx = _required_string(central_table, "spdx", context="central_coverage")
    central_header = (
        central_copyright,
        central_authorship,
        central_license_notice,
        central_spdx,
    )
    if central_header != canonical_header.required_lines:
        raise ValueError(
            "la cobertura central debe reproducir exactamente la cabecera canónica"
        )

    return LegalPolicy(
        schema_version=schema_version,
        canonical_header=canonical_header,
        central_categories=central_categories,
        central_copyright=central_copyright,
        central_authorship=central_authorship,
        central_license_notice=central_license_notice,
        central_spdx=central_spdx,
        classification=classification,
        third_party=tuple(third_party),
    )


def list_tracked_files(root: Path) -> tuple[str, ...]:
    """Enumera exclusivamente el índice Git, con rutas NUL-safe."""
    completed = subprocess.run(
        ["git", "-C", str(root), "ls-files", "-z"],
        check=True,
        stdout=subprocess.PIPE,
    )
    return tuple(
        sorted(
            item.decode("utf-8", errors="strict")
            for item in completed.stdout.split(b"\0")
            if item
        )
    )


def classify_path(path: str, policy: LegalPolicy) -> ClassifiedPath:
    """Clasifica una ruta o devuelve ``unknown`` sin aplicar fallback."""
    for third_party in policy.third_party:
        if third_party.matches(path):
            return ClassifiedPath(
                path=path,
                category="third-party",
                third_party_name=third_party.name,
            )

    classification = policy.classification
    if any(
        fnmatch.fnmatchcase(path, pattern) for pattern in classification.generated_globs
    ):
        return ClassifiedPath(path=path, category="generated")
    if path in classification.central_fixture_paths:
        return ClassifiedPath(path=path, category="uncommentable")

    pure_path = PurePosixPath(path)
    suffix = pure_path.suffix.lower()
    filename = pure_path.name
    if suffix in classification.json_extensions:
        return ClassifiedPath(path=path, category="json")
    if suffix in classification.binary_resource_extensions:
        return ClassifiedPath(path=path, category="binary-resource")
    if (
        suffix in classification.uncommentable_extensions
        or filename in classification.uncommentable_filenames
    ):
        return ClassifiedPath(path=path, category="uncommentable")
    if filename in classification.license_text_filenames:
        return ClassifiedPath(path=path, category="license-text")
    if (
        suffix in classification.inline_extensions
        or filename in classification.inline_filenames
    ):
        return ClassifiedPath(path=path, category="inline")
    return ClassifiedPath(path=path, category="unknown")


def header_region(text: str) -> str:
    """Devuelve la zona donde debe estar la cabecera sin romper front matter."""
    lines = text.splitlines()
    start = 0
    if lines and lines[0].strip() == "---":
        for index, line in enumerate(lines[1:80], start=1):
            if line.strip() == "---":
                start = index + 1
                break
    return "\n".join(lines[start : start + 24])


def check_inline_header(
    path: str, text: str, canonical: CanonicalHeader
) -> list[Violation]:
    """Exige todos los elementos de la cabecera canónica."""
    region = header_region(text)
    violations: list[Violation] = []
    missing = [line for line in canonical.required_lines if line not in region]
    if missing:
        violations.append(
            Violation(
                path=path,
                code="missing-canonical-header",
                message="faltan en la cabecera: " + " | ".join(missing),
            )
        )

    spdx_lines = [
        line.strip()
        for line in region.splitlines()
        if "SPDX-License-Identifier:" in line
    ]
    if spdx_lines and not all(canonical.spdx in line for line in spdx_lines):
        violations.append(
            Violation(
                path=path,
                code="wrong-license",
                message=(
                    "la cabecera contiene una licencia distinta de EUPL-1.2: "
                    + " | ".join(spdx_lines)
                ),
            )
        )
    return violations


def _read_utf8(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def check_root_license(root: Path) -> list[Violation]:
    license_path = root / "LICENSE"
    if not license_path.is_file():
        return [
            Violation(
                path="LICENSE",
                code="missing-root-license",
                message="falta el texto oficial español de EUPL 1.2",
            )
        ]
    try:
        text = _read_utf8(license_path)
    except (OSError, UnicodeError) as error:
        return [
            Violation(
                path="LICENSE",
                code="invalid-root-license",
                message=str(error),
            )
        ]
    missing = [marker for marker in LICENSE_REQUIRED_MARKERS if marker not in text]
    if not missing:
        return []
    return [
        Violation(
            path="LICENSE",
            code="incomplete-root-license",
            message="faltan marcadores oficiales: " + " | ".join(missing),
        )
    ]


def check_readme_license(root: Path) -> list[Violation]:
    """Exige que la licencia publicada en el README coincida con la canónica."""
    readme_path = root / "README.md"
    if not readme_path.is_file():
        return [
            Violation(
                path="README.md",
                code="missing-readme-license",
                message="falta el README que publica la licencia EUPL-1.2",
            )
        ]
    try:
        text = _read_utf8(readme_path)
    except (OSError, UnicodeError) as error:
        return [
            Violation(
                path="README.md",
                code="invalid-readme-license",
                message=str(error),
            )
        ]

    heading = "## Licencia"
    start = text.find(heading)
    if start < 0:
        return [
            Violation(
                path="README.md",
                code="missing-readme-license",
                message="falta la sección «Licencia»",
            )
        ]
    end = text.find("\n## ", start + len(heading))
    section = text[start:] if end < 0 else text[start:end]
    if "EUPL 1.2 o posterior" in section and "GPLv3." not in section:
        return []
    return [
        Violation(
            path="README.md",
            code="inconsistent-readme-license",
            message=(
                "la sección «Licencia» debe declarar EUPL 1.2 o posterior "
                "y no puede declarar GPLv3 como licencia del código propio"
            ),
        )
    ]


def check_published_license_consistency(
    root: Path,
    paths: Sequence[str] = PUBLISHED_LICENSE_PATHS,
) -> list[Violation]:
    """Evita que ayudas y manuales propios contradigan la licencia canónica."""
    violations: list[Violation] = []
    for relative_path in paths:
        path = root / relative_path
        if not path.is_file():
            violations.append(
                Violation(
                    path=relative_path,
                    code="missing-published-license-surface",
                    message="falta una ayuda o manual sujeto al gate de licencia",
                )
            )
            continue
        try:
            text = _read_utf8(path)
        except (OSError, UnicodeError) as error:
            violations.append(
                Violation(
                    path=relative_path,
                    code="invalid-published-license-surface",
                    message=str(error),
                )
            )
            continue
        forbidden = tuple(
            marker
            for marker in FORBIDDEN_PROJECT_LICENSE_DECLARATIONS
            if marker in text
        )
        if forbidden:
            violations.append(
                Violation(
                    path=relative_path,
                    code="inconsistent-published-license",
                    message="declaración obsoleta del código propio: "
                    + ", ".join(forbidden),
                )
            )
    return violations


def check_third_party_notices(
    root: Path,
    tracked_files: frozenset[str],
    policies: Sequence[ThirdPartyPolicy],
) -> list[Violation]:
    violations: list[Violation] = []
    for policy in policies:
        matched = tuple(path for path in tracked_files if policy.matches(path))
        if not matched:
            violations.append(
                Violation(
                    path="legal/licensing.toml",
                    code="unused-third-party-policy",
                    message=f"la política {policy.name!r} no cubre ningún fichero",
                )
            )
        for notice_file in policy.notice_files:
            if notice_file not in tracked_files:
                violations.append(
                    Violation(
                        path=notice_file,
                        code="untracked-third-party-notice",
                        message=f"aviso exigido para {policy.name}",
                    )
                )
                continue
            try:
                notice = _read_utf8(root / notice_file)
            except (OSError, UnicodeError) as error:
                violations.append(
                    Violation(
                        path=notice_file,
                        code="invalid-third-party-notice",
                        message=str(error),
                    )
                )
                continue
            for marker in policy.required_notice_markers:
                if marker not in notice:
                    violations.append(
                        Violation(
                            path=notice_file,
                            code="third-party-notice-changed",
                            message=(f"falta el aviso de {policy.name}: {marker}"),
                        )
                    )
    return violations


def audit_repository(
    root: Path = ROOT,
    *,
    policy_path: Path | None = None,
    enforce_headers: bool = True,
    tracked_files: Iterable[str] | None = None,
) -> AuditResult:
    """Audita clasificación, cobertura, avisos y opcionalmente cabeceras."""
    policy = load_policy(policy_path or root / "legal" / "licensing.toml")
    tracked = tuple(
        sorted(tracked_files if tracked_files is not None else list_tracked_files(root))
    )
    tracked_set = frozenset(tracked)
    classifications: list[ClassifiedPath] = []
    violations: list[Violation] = []

    for relative_path in tracked:
        classified = classify_path(relative_path, policy)
        classifications.append(classified)
        if classified.category == "unknown":
            violations.append(
                Violation(
                    path=relative_path,
                    code="unclassified",
                    message="la ruta no tiene una categoría legal explícita",
                )
            )
            continue
        if classified.category in CENTRAL_CATEGORIES:
            if classified.category not in policy.central_categories:
                violations.append(
                    Violation(
                        path=relative_path,
                        code="missing-central-coverage",
                        message=(
                            f"la categoría {classified.category} no está "
                            "cubierta por la política central"
                        ),
                    )
                )
            continue
        if classified.category != "inline" or not enforce_headers:
            continue

        disk_path = root / relative_path
        try:
            text = _read_utf8(disk_path)
        except (OSError, UnicodeError) as error:
            violations.append(
                Violation(
                    path=relative_path,
                    code="unreadable-inline-file",
                    message=str(error),
                )
            )
            continue
        violations.extend(
            check_inline_header(relative_path, text, policy.canonical_header)
        )

    violations.extend(check_root_license(root))
    violations.extend(check_readme_license(root))
    violations.extend(check_published_license_consistency(root))
    violations.extend(check_third_party_notices(root, tracked_set, policy.third_party))
    return AuditResult(
        tracked_files=tracked,
        classifications=tuple(classifications),
        violations=tuple(
            sorted(violations, key=lambda item: (item.path, item.code, item.message))
        ),
    )


def _json_report(result: AuditResult) -> str:
    return json.dumps(
        {
            "tracked_files": len(result.tracked_files),
            "categories": dict(sorted(result.counts.items())),
            "violations": [
                {
                    "path": item.path,
                    "code": item.code,
                    "message": item.message,
                }
                for item in result.violations
            ],
        },
        ensure_ascii=False,
        indent=2,
        sort_keys=True,
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Audita autoría, licencia, terceros y cobertura central de todos "
            "los ficheros devueltos por git ls-files."
        )
    )
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--policy", type=Path)
    parser.add_argument(
        "--inventory-only",
        action="store_true",
        help=(
            "valida clasificación/cobertura/terceros, pero no exige todavía "
            "las cabeceras inline"
        ),
    )
    parser.add_argument("--json", action="store_true", dest="as_json")
    parser.add_argument(
        "--max-errors",
        type=int,
        default=50,
        help="máximo de errores mostrados en texto; 0 muestra todos",
    )
    args = parser.parse_args(argv)

    root = args.root.resolve()
    policy_path = args.policy.resolve() if args.policy else None
    try:
        result = audit_repository(
            root,
            policy_path=policy_path,
            enforce_headers=not args.inventory_only,
        )
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"No se pudo ejecutar la auditoría legal: {error}", file=sys.stderr)
        return 2

    if args.as_json:
        print(_json_report(result))
    else:
        counts = ", ".join(
            f"{category}={count}" for category, count in sorted(result.counts.items())
        )
        print(
            f"Ficheros versionados: {len(result.tracked_files)}. "
            f"Clasificación: {counts}."
        )
        if result.violations:
            limit = len(result.violations) if args.max_errors == 0 else args.max_errors
            for violation in result.violations[:limit]:
                print(violation.render(), file=sys.stderr)
            omitted = len(result.violations) - limit
            if omitted > 0:
                print(
                    f"... {omitted} incumplimientos adicionales omitidos",
                    file=sys.stderr,
                )
        else:
            mode = "inventario" if args.inventory_only else "estricto"
            print(f"Auditoría legal correcta en modo {mode}.")

    return 1 if result.violations else 0


if __name__ == "__main__":
    raise SystemExit(main())
