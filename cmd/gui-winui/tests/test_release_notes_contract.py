# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[3]
VERSION = ROOT / "VERSION.txt"
NOTES = ROOT / "docs/NOVEDADES.md"
CHECK = ROOT / "scripts/comprobar-novedades.py"
NEXT = ROOT / "scripts/nueva-version.sh"
PROJECT = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/GrxFirma.WinUI.csproj"
ABOUT = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/ViewModels/AboutPageViewModel.cs"
ABOUT_VIEW = ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Views/AboutPage.xaml"


class ReleaseNotesContractTests(unittest.TestCase):
    def test_current_version_has_one_dated_section(self):
        version = VERSION.read_text(encoding="utf-8").strip()
        notes = NOTES.read_text(encoding="utf-8")
        headings = re.findall(
            rf"^## {re.escape(version)} — \d{{4}}-\d{{2}}-\d{{2}}$",
            notes,
            re.MULTILINE,
        )
        self.assertEqual(len(headings), 1)
        self.assertEqual(
            subprocess.run(["python3", str(CHECK)], capture_output=True).returncode,
            0,
        )

    def test_version_script_increments_and_rejects_reuse(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "scripts").mkdir()
            (root / "docs").mkdir()
            shutil.copy2(NEXT, root / "scripts/nueva-version.sh")
            shutil.copy2(CHECK, root / "scripts/comprobar-novedades.py")
            (root / "VERSION.txt").write_text("2.0.2\n", encoding="utf-8")
            (root / "docs/NOVEDADES.md").write_text(
                "# Novedades\n\n## 2.0.2 — 2026-09-26\n\n- Cambio.\n",
                encoding="utf-8",
            )
            script = root / "scripts/nueva-version.sh"
            self.assertEqual(subprocess.run(["bash", str(script)], capture_output=True).returncode, 0)
            self.assertEqual((root / "VERSION.txt").read_text().strip(), "2.0.3")
            text = (root / "docs/NOVEDADES.md").read_text()
            self.assertLess(text.index("## 2.0.3"), text.index("## 2.0.2"))
            self.assertIn("Pendiente:", text)
            before = text
            for candidate in ("2.0.3", "2.0.2", "2.0.3+otra", "02.0.4", "2.0.4-01"):
                result = subprocess.run(["bash", str(script), candidate], capture_output=True)
                self.assertNotEqual(result.returncode, 0, candidate)
                self.assertEqual((root / "VERSION.txt").read_text().strip(), "2.0.3")
                self.assertEqual((root / "docs/NOVEDADES.md").read_text(), before)
            self.assertEqual(
                subprocess.run(["bash", str(script), "2.1.0"], capture_output=True).returncode,
                0,
            )
            self.assertEqual((root / "VERSION.txt").read_text().strip(), "2.1.0")
            self.assertEqual(
                subprocess.run(["bash", str(script), "2.1.1-rc.1"], capture_output=True).returncode,
                0,
            )
            self.assertEqual(
                subprocess.run(["python3", str(root / "scripts/comprobar-novedades.py")], capture_output=True).returncode,
                0,
            )

    def test_build_guard_rejects_missing_or_duplicate_current_section(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "scripts").mkdir()
            (root / "docs").mkdir()
            shutil.copy2(CHECK, root / "scripts/comprobar-novedades.py")
            (root / "VERSION.txt").write_text("2.0.2\n", encoding="utf-8")
            notes_path = root / "docs/NOVEDADES.md"
            check = ["python3", str(root / "scripts/comprobar-novedades.py")]
            notes_path.write_text("## 2.0.1 — 2026-09-26\n", encoding="utf-8")
            self.assertNotEqual(subprocess.run(check, capture_output=True).returncode, 0)
            section = "## 2.0.2 — 2026-09-26\n\n- Cambio.\n"
            notes_path.write_text(section, encoding="utf-8")
            self.assertEqual(subprocess.run(check, capture_output=True).returncode, 0)
            notes_path.write_text(section + section, encoding="utf-8")
            self.assertNotEqual(subprocess.run(check, capture_output=True).returncode, 0)

    def test_renumerar_requires_explicit_distinct_version(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "scripts").mkdir()
            (root / "docs").mkdir()
            shutil.copy2(NEXT, root / "scripts/nueva-version.sh")
            (root / "VERSION.txt").write_text("2.0.3\n", encoding="utf-8")
            notes = root / "docs/NOVEDADES.md"
            notes.write_text("## 2.0.3 — 2026-09-26\n\n- Cambio.\n", encoding="utf-8")
            script = root / "scripts/nueva-version.sh"
            for arguments in (("0.0.90",), ("--renumerar",), ("--renumerar", "2.0.3")):
                with self.subTest(arguments=arguments):
                    self.assertNotEqual(subprocess.run(["bash", str(script), *arguments], capture_output=True).returncode, 0)
                    self.assertEqual((root / "VERSION.txt").read_text().strip(), "2.0.3")
            self.assertEqual(subprocess.run(["bash", str(script), "--renumerar", "0.0.90"], capture_output=True).returncode, 0)
            self.assertEqual((root / "VERSION.txt").read_text().strip(), "0.0.90")
            self.assertEqual(notes.read_text().count("## 0.0.90 — "), 1)

    def test_packagers_check_notes_before_build(self):
        for relative in (
            "packaging/windows/build-suite.ps1",
            "packaging/windows/build-desktop-winui.ps1",
            "packaging/linux/build-suite.sh",
        ):
            source = (ROOT / relative).read_text(encoding="utf-8")
            self.assertIn("comprobar-novedades.py", source, relative)
        for relative in (
            "packaging/windows/build-suite.ps1",
            "packaging/linux/build-suite.sh",
        ):
            self.assertIn("NOVEDADES.md", (ROOT / relative).read_text(encoding="utf-8"))

    def test_winui_publishes_and_displays_bounded_notes(self):
        project = ET.parse(PROJECT).getroot()
        entries = [entry for entry in project.iter("Content")
                   if entry.get("Include", "").endswith(r"docs\NOVEDADES.md")]
        self.assertEqual(len(entries), 1)
        self.assertEqual(entries[0].findtext("TargetPath"), r"help\NOVEDADES.md")
        self.assertEqual(entries[0].findtext("CopyToPublishDirectory"), "Always")
        manager = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI/Services/ReleaseNotesManager.cs").read_text(encoding="utf-8")
        parser = (ROOT / "cmd/gui-winui/src/GrxFirma.WinUI.Core/Operations/ReleaseNotesSections.cs").read_text(encoding="utf-8")
        view = ABOUT_VIEW.read_text(encoding="utf-8")
        for marker in ("64 * 1024", "UTF8Encoding(false, true)",
                       "FileAttributes.ReparsePoint", "lastSeenVersion"):
            self.assertIn(marker, manager)
        self.assertIn("Heading", parser)
        self.assertIn('Click="OnReleaseNotesClick"', view)
        self.assertIn('TextWrapping="Wrap"', view)


if __name__ == "__main__":
    unittest.main()
