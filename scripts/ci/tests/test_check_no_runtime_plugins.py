# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from scripts.ci.check_no_runtime_plugins import check_repository


class NoRuntimePluginsGateTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        self.root = Path(self.temporary_directory.name)

    def write(self, relative_path: str, content: str) -> None:
        path = self.root / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def reasons(self) -> list[str]:
        return [finding.reason for finding in check_repository(self.root)]

    def test_accepts_qt_and_pkcs11_plugins(self) -> None:
        self.write(
            "internal/platform/native.go",
            '''package platform
import (
    "github.com/miekg/pkcs11"
    "example.invalid/grxfirma/internal/qmlplugin"
)
var _ = pkcs11.CKF_SIGN
''',
        )
        self.write(
            "cmd/gui-qml/grxfirma_qt.pro",
            "QT += core gui qml quick\nLIBS += -lpkcs11\n",
        )

        self.assertEqual(check_repository(self.root), [])

    def test_rejects_stdlib_plugin_with_alias(self) -> None:
        self.write(
            "internal/runtime/loader.go",
            '''package runtime
import runtimeplugin "plugin"
func load(path string) { _, _ = runtimeplugin.Open(path) }
''',
        )

        self.assertIn('stdlib "plugin"', " ".join(self.reasons()))

    def test_rejects_runtime_frameworks_in_source_and_go_mod(self) -> None:
        self.write(
            "internal/runtime/loader.go",
            '''package runtime
import (
    "github.com/hashicorp/go-plugin"
    "github.com/traefik/yaegi/interp"
    "github.com/tetratelabs/wazero"
)
''',
        )
        self.write(
            "go.mod",
            """module example.invalid/grxfirma
require github.com/yuin/gopher-lua v1.1.1
""",
        )

        reasons = " ".join(self.reasons())
        self.assertIn("framework de plugins RPC", reasons)
        self.assertIn("intérprete Go embebido", reasons)
        self.assertIn("runtime WebAssembly embebido", reasons)
        self.assertIn("intérprete Lua embebido", reasons)

    def test_rejects_native_lua_linkage(self) -> None:
        self.write(
            "native/CMakeLists.txt",
            "find_package(Lua 5.4 REQUIRED)\ntarget_link_libraries(app PRIVATE -llua5.4)\n",
        )

        reasons = " ".join(self.reasons())
        self.assertIn("CMake carga Lua", reasons)
        self.assertIn("enlace nativo con Lua", reasons)

    def test_does_not_scan_docs_or_comments_as_dependencies(self) -> None:
        self.write(
            "docs/decision.md",
            "No se cargará github.com/hashicorp/go-plugin ni Lua.\n",
        )
        self.write(
            "go.mod",
            """module example.invalid/grxfirma
// github.com/hashicorp/go-plugin figura solo como ejemplo prohibido.
require github.com/miekg/pkcs11 v1.1.2
""",
        )

        self.assertEqual(check_repository(self.root), [])


if __name__ == "__main__":
    unittest.main()
