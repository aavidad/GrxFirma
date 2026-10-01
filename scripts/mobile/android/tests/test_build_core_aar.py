# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import unittest


SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "build-core-aar.sh"


class BuildCoreAarTest(unittest.TestCase):
    def test_disables_vcs_stamping_inside_git_archive(self) -> None:
        source = SCRIPT.read_text(encoding="utf-8")
        archive = source.index("git -C \"$ROOT_DIR\" archive")
        disable = source.index("-buildvcs=false")
        gomobile = source.index("\"$WORK_DIR/bin/gomobile\" bind")

        self.assertLess(archive, disable)
        self.assertLess(disable, gomobile)
        self.assertIn(
            'export GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false -trimpath"',
            source,
        )

    def test_uses_source_stable_work_path_for_reproducible_build_info(self) -> None:
        source = SCRIPT.read_text(encoding="utf-8")

        self.assertIn(
            'SOURCE_FINGERPRINT=$(',
            source,
        )
        self.assertIn('git -C "$ROOT_DIR" ls-tree -r HEAD --', source)
        for source_path in ("go.mod", "go.sum", "mobilebind", "internal", "third_party"):
            self.assertIn(source_path, source)
        self.assertIn(
            'WORK_DIR="${TMPDIR:-/tmp}/grxfirma-gomobile-$SOURCE_FINGERPRINT"',
            source,
        )
        self.assertNotIn("grxfirma-gomobile.XXXXXX", source)


if __name__ == "__main__":
    unittest.main()
