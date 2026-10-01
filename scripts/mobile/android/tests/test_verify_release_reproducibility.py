# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import unittest


SCRIPT = (
    pathlib.Path(__file__).resolve().parents[1]
    / "verify-release-reproducibility.sh"
)


class VerifyReleaseReproducibilityTest(unittest.TestCase):
    def test_builds_apk_and_aab_twice_with_fixed_inputs(self) -> None:
        source = SCRIPT.read_text(encoding="utf-8")
        self.assertEqual(2, source.count('build_and_copy "$WORK_DIR/'))
        self.assertIn("assembleProductionRelease", source)
        self.assertIn("bundleProductionRelease", source)
        self.assertIn("GRXFIRMA_ANDROID_SIGNING_CERT_SHA256", source)
        self.assertIn("GRXFIRMA_ANDROID_SOURCE_COMMIT", source)
        self.assertIn("cmp -s", source)
        self.assertIn("SOURCE_DATE_EPOCH", source)


if __name__ == "__main__":
    unittest.main()
