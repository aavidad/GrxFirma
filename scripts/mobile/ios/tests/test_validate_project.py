# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import plistlib
import shutil
import sys
import tempfile
import unittest


SCRIPT_DIR = pathlib.Path(__file__).resolve().parents[1]
REPO_ROOT = SCRIPT_DIR.parents[2]
IOS_ROOT = REPO_ROOT / "mobile/ios"
sys.path.insert(0, str(SCRIPT_DIR))
import validate_project as validator  # noqa: E402


class ValidateProjectTests(unittest.TestCase):
    def test_current_project_is_structurally_valid(self) -> None:
        validator.validate(IOS_ROOT)

    def test_rejects_relaxed_ats(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            copied = pathlib.Path(temp) / "ios"
            shutil.copytree(IOS_ROOT, copied)
            info_path = copied / "GrxFirma/Resources/Info.plist"
            with info_path.open("rb") as handle:
                info = plistlib.load(handle)
            info["NSAppTransportSecurity"]["NSAllowsArbitraryLoads"] = True
            with info_path.open("wb") as handle:
                plistlib.dump(info, handle)
            with self.assertRaisesRegex(validator.ValidationError, "ATS"):
                validator.validate(copied)

    def test_rejects_release_without_production_core(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            copied = pathlib.Path(temp) / "ios"
            shutil.copytree(IOS_ROOT, copied)
            release = copied / "Config/Release.xcconfig"
            text = release.read_text(encoding="utf-8").replace(
                "GRXFIRMA_CORE_MODE = production",
                "GRXFIRMA_CORE_MODE = verification",
            )
            release.write_text(text, encoding="utf-8")
            with self.assertRaisesRegex(validator.ValidationError, "Release"):
                validator.validate(copied)

    def test_rejects_incomplete_file_metadata_reasons(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            copied = pathlib.Path(temp) / "ios"
            shutil.copytree(IOS_ROOT, copied)
            privacy_path = copied / "GrxFirma/Resources/PrivacyInfo.xcprivacy"
            with privacy_path.open("rb") as handle:
                privacy = plistlib.load(handle)
            privacy["NSPrivacyAccessedAPITypes"][0][
                "NSPrivacyAccessedAPITypeReasons"
            ] = ["C617.1"]
            with privacy_path.open("wb") as handle:
                plistlib.dump(privacy, handle)
            with self.assertRaisesRegex(validator.ValidationError, "motivos"):
                validator.validate(copied)


if __name__ == "__main__":
    unittest.main()
