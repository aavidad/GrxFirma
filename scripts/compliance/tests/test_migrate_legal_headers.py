# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import unittest
from pathlib import Path

from scripts.compliance.check_legal_headers import (
    ROOT,
    audit_repository,
    classify_path,
    load_policy,
)
from scripts.compliance.migrate_legal_headers import (
    UTF8_BOM,
    comment_style_for_path,
    migrate_content,
)


class LegalHeaderMigrationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.policy = load_policy()
        cls.header = cls.policy.canonical_header

    def migrate(self, path: str, text: str) -> str:
        return migrate_content(path, text.encode("utf-8"), self.header).decode(
            "utf-8"
        )

    def test_replaces_the_legacy_project_header_and_is_idempotent(self) -> None:
        legacy = """\
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Diputacion de Granada
// Autor: Alberto Avidad Fernandez (Oficina de Software Libre de la Diputacion de Granada)

package ejemplo
"""
        migrated = self.migrate("ejemplo.go", legacy)
        self.assertNotIn("GPL-3.0", migrated)
        self.assertNotIn("Copyright (C) 2026 Diputacion", migrated)
        for line in self.header.required_lines:
            self.assertIn(f"// {line}\n", migrated)
        self.assertEqual(
            migrate_content("ejemplo.go", migrated.encode(), self.header),
            migrated.encode(),
        )

    def test_preserves_shebang_encoding_bom_and_crlf(self) -> None:
        original = (
            UTF8_BOM
            + b"#!/usr/bin/env python3\r\n"
            + b"# -*- coding: utf-8 -*-\r\n"
            + b'print("correcto")\r\n'
        )
        migrated = migrate_content("ejemplo.py", original, self.header)
        self.assertTrue(
            migrated.startswith(
                UTF8_BOM
                + b"#!/usr/bin/env python3\r\n"
                + b"# -*- coding: utf-8 -*-\r\n"
            )
        )
        self.assertNotIn(b"\n", migrated.replace(b"\r\n", b""))
        self.assertTrue(migrated.endswith(b'print("correcto")\r\n'))

    def test_preserves_ruby_magic_comment_immediately_after_shebang(self) -> None:
        source = """\
#!/usr/bin/env ruby
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

# frozen_string_literal: true
# SPDX-License-Identifier: GPL-3.0-or-later

puts "correcto"
"""
        migrated = self.migrate("script.rb", source)
        self.assertTrue(
            migrated.startswith(
                "#!/usr/bin/env ruby\n# frozen_string_literal: true\n"
            )
        )
        self.assertEqual(migrated.count("SPDX-License-Identifier:"), 1)
        self.assertNotIn("GPL-3.0", migrated)

    def test_preserves_xml_declaration_and_html_doctype(self) -> None:
        xml = self.migrate(
            "ejemplo.xml",
            '<?xml version="1.0" encoding="UTF-8"?>\n<raiz />\n',
        )
        self.assertTrue(xml.startswith('<?xml version="1.0" encoding="UTF-8"?>\n'))
        self.assertIn("<!-- SPDX-License-Identifier: EUPL-1.2 -->", xml)

        html = self.migrate(
            "ejemplo.html",
            "<!doctype html>\n<html></html>\n",
        )
        self.assertTrue(html.startswith("<!doctype html>\n"))
        self.assertIn("<!-- SPDX-License-Identifier: EUPL-1.2 -->", html)

    def test_msbuild_props_are_inline_xml_and_migration_is_idempotent(self) -> None:
        name = "cmd/gui-winui/Directory.Build.props"
        self.assertEqual(classify_path(name, self.policy).category, "inline")
        migrated = self.migrate(name, "<Project />\n")
        self.assertIn("<!-- SPDX-License-Identifier: EUPL-1.2 -->", migrated)
        self.assertEqual(self.migrate(name, migrated), migrated)

    def test_preserves_markdown_frontmatter_before_the_header(self) -> None:
        markdown = """\
---
id: T001
status: HECHA
---

# Tarea
"""
        migrated = self.migrate("tasks/T001.md", markdown)
        self.assertTrue(migrated.startswith("---\nid: T001\nstatus: HECHA\n---\n"))
        closing = migrated.index("---\n", 4) + len("---\n")
        header = migrated.index("<!-- Derechos de autor", closing)
        title = migrated.index("# Tarea")
        self.assertEqual(closing, header)
        self.assertLess(header, title)

    def test_preserves_unrelated_prior_notices(self) -> None:
        source = """\
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Diputacion de Granada

// Copyright (C) 2024 Otra persona
// Aviso que debe conservarse.
package ejemplo
"""
        migrated = self.migrate("ejemplo.go", source)
        self.assertIn("// Copyright (C) 2024 Otra persona", migrated)
        self.assertIn("// Aviso que debe conservarse.", migrated)

    def test_preserves_a_leading_go_build_constraint(self) -> None:
        source = """\
//go:build regression_v1

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Diputacion de Granada

package regression_test
"""
        migrated = self.migrate("regression_test.go", source)
        self.assertTrue(migrated.startswith("//go:build regression_v1\n\n"))
        self.assertNotIn("GPL-3.0", migrated)
        self.assertIn("// SPDX-License-Identifier: EUPL-1.2", migrated)

    def test_preserves_the_roff_title_as_the_first_line(self) -> None:
        source = """\
.\\\" Derechos de autor (C) 2026 Alberto Avidad Fernández.
.\\\" Autoría: Alberto Avidad Fernández
.\\\" Licencia: EUPL 1.2 o posterior
.\\\" SPDX-License-Identifier: EUPL-1.2

.TH GRXFIRMA 1
.SH NOMBRE
"""
        migrated = self.migrate("grxfirma.1", source)
        self.assertTrue(migrated.startswith(".TH GRXFIRMA 1\n"))
        self.assertEqual(migrated.count("SPDX-License-Identifier:"), 1)

    def test_comment_syntax_is_defined_for_every_inline_tracked_file(self) -> None:
        inventory = audit_repository(ROOT, enforce_headers=False)
        inline_paths = (
            item.path
            for item in inventory.classifications
            if item.category == "inline"
        )
        for path in inline_paths:
            with self.subTest(path=path):
                comment_style_for_path(path)

    def test_known_comment_styles_remain_syntactically_valid(self) -> None:
        expected = {
            "main.go": "// ",
            "script.ps1": "# ",
            "installer.nsi": "; ",
            "manual.7": '.\\" ',
            "styles.css": "/* ",
            "vista.xaml": "<!-- ",
            "AppxManifest.xml.in": "<!-- ",
            "SafariNativeOnly.js.in": "// ",
        }
        for path, prefix in expected.items():
            with self.subTest(path=path):
                self.assertEqual(comment_style_for_path(path).prefix, prefix)


if __name__ == "__main__":
    unittest.main()
