# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import tempfile
import unittest
import zipfile


SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "validate_core_aar.py"
SPEC = importlib.util.spec_from_file_location("validate_core_aar", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ValidateCoreAarTest(unittest.TestCase):
    def test_accepts_complete_contract_and_required_abis(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            aar = self._build_aar(pathlib.Path(temp), complete=True)
            actual = MODULE.validate(aar, MODULE.sha256(aar))
            self.assertEqual(64, len(actual))

    def test_rejects_facade_without_android_factory(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            aar = self._build_aar(pathlib.Path(temp), complete=False)
            with self.assertRaisesRegex(MODULE.ValidationError, "newAndroidFacade"):
                MODULE.validate(aar, None)

    def test_rejects_missing_native_abi(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            aar = self._build_aar(root, complete=True, include_x86_64=False)
            with self.assertRaisesRegex(MODULE.ValidationError, "x86_64"):
                MODULE.validate(aar, None)

    def test_rejects_empty_expected_hash(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            aar = self._build_aar(pathlib.Path(temp), complete=True)
            with self.assertRaisesRegex(MODULE.ValidationError, "64 hexadecimales"):
                MODULE.validate(aar, "")

    @staticmethod
    def _build_aar(
        root: pathlib.Path,
        complete: bool,
        include_x86_64: bool = True,
    ) -> pathlib.Path:
        source = root / "src/mobilebind"
        classes = root / "classes"
        source.mkdir(parents=True)
        classes.mkdir()
        factory = (
            "public static Facade newAndroidFacade(String files, String noBackup) "
            "{ return new Facade(); }"
            if complete
            else "public static Facade newFacade() { return new Facade(); }"
        )
        (source / "Mobilebind.java").write_text(
            f"package mobilebind; public final class Mobilebind {{ {factory} }}",
            encoding="utf-8",
        )
        (source / "Facade.java").write_text(
            """
            package mobilebind;
            public final class Facade {
              public String mobileContractJSON() { return "{}"; }
              public void clearSession() {}
              public String selectCertificateJSON(String value) { return value; }
              public String importCertificateJSON(String value) { return value; }
              public String importCertificateBytesJSON(byte[] value, String password) { return password; }
              public String importCertificateSecretBytesJSON(byte[] value, byte[] password) { return "{}"; }
              public String signJSON(String value) { return value; }
              public String sealPreviewJSON(String value) { return value; }
              public String inspectSignatureJSON(String value) { return value; }
              public String processBatchJSON(String value) { return value; }
              public String createHashJSON(String value) { return value; }
              public String checkHashJSON(String value) { return value; }
              public String protectJSON(String value, byte[] secret) { return value; }
              public String unprotectJSON(String value, byte[] secret) { return value; }
              public String validateVeriFactuJSON(String value) { return value; }
              public String createENIDocumentJSON(String value) { return value; }
              public String validateENIJSON(String value) { return value; }
              public String eniCatalogsJSON() { return "{}"; }
              public String csvLegendJSON(String value) { return value; }
              public String certificateDetailsJSON() { return "{}"; }
              public String checkCertificateRevocationJSON(String value) { return value; }
              public String diagnosticsJSON() { return "{}"; }
              public String probeTimestampAuthorityJSON(String value) { return value; }
              public String readVeriFactuQRJSON(String value) { return value; }
              public String queryVeriFactuQRJSON(String value) { return value; }
              public String checkUpdateJSON(String value) { return value; }
              public String verifyJSON(String value) { return value; }
            }
            """,
            encoding="utf-8",
        )
        subprocess.run(
            ["javac", "-d", str(classes), *map(str, source.glob("*.java"))],
            check=True,
            capture_output=True,
        )
        jar = root / "classes.jar"
        subprocess.run(
            ["jar", "--create", "--file", str(jar), "-C", str(classes), "."], check=True
        )

        aar = root / "grxfirma.aar"
        with zipfile.ZipFile(aar, "w") as archive:
            archive.write(jar, "classes.jar")
            for abi in ("armeabi-v7a", "arm64-v8a"):
                archive.writestr(f"jni/{abi}/libgojni.so", b"ELF-test")
            if include_x86_64:
                archive.writestr("jni/x86_64/libgojni.so", b"ELF-test")
        return aar


if __name__ == "__main__":
    unittest.main()
