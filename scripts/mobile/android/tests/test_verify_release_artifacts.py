# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import hashlib
import importlib.util
import pathlib
import tempfile
import unittest
import zipfile
from unittest import mock


SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "verify_release_artifacts.py"
SPEC = importlib.util.spec_from_file_location("verify_release_artifacts", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class VerifyReleaseArtifactsTest(unittest.TestCase):
    def test_accepts_exact_version_commit_and_signer(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            apk = root / "GrxFirma-1.2.3-android.apk"
            aab = root / "GrxFirma-1.2.3-android.aab"
            apk.write_bytes(b"apk")
            aab.write_bytes(b"aab")
            fingerprint = hashlib.sha256(b"cert").hexdigest()
            reports = [
                (
                    "Verified using v1 scheme (JAR signing): false\n"
                    "Verified using v2 scheme (APK Signature Scheme v2): true\n"
                    "Verified using v3 scheme (APK Signature Scheme v3): true\n"
                    "Verified using v4 scheme (APK Signature Scheme v4): false\n"
                    f"Signer #1 certificate SHA-256 digest: {fingerprint}\n"
                ),
                "package: name='es.dipgra.grxfirma' versionCode='1000' versionName='0.1.0'\n",
                (
                    "A: android:name=\"es.dipgra.grxfirma.SOURCE_COMMIT\"\n"
                    f"A: android:value=\"{'a' * 40}\"\n"
                    "A: android:name=\"es.dipgra.grxfirma.CORE_SHA256\"\n"
                    f"A: android:value=\"{'b' * 64}\"\n"
                ),
            ]
            with mock.patch.object(
                MODULE, "find_build_tool", return_value=root / "tool"
            ):
                with mock.patch.object(MODULE, "run_text", side_effect=reports):
                    with mock.patch.object(MODULE, "verify_aab_signature"):
                        with mock.patch.object(MODULE, "verify_embedded_core"):
                            with mock.patch.object(
                                MODULE,
                                "apk_signing_block_ids",
                                return_value={
                                    MODULE.APK_V2_BLOCK_ID,
                                    MODULE.APK_V3_BLOCK_ID,
                                },
                            ):
                                evidence = MODULE.verify(
                                    apk,
                                    aab,
                                    root / "core.aar",
                                    fingerprint,
                                    "0.1.0",
                                    1000,
                                    "a" * 40,
                                    "b" * 64,
                                )
            self.assertEqual("a" * 40, evidence["source_commit"])
            self.assertEqual(2, len(evidence["artifacts"]))

    def test_rejects_apk_signed_by_another_certificate(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            apk = root / "GrxFirma-1.2.3-android.apk"
            aab = root / "GrxFirma-1.2.3-android.aab"
            apk.write_bytes(b"apk")
            aab.write_bytes(b"aab")
            with mock.patch.object(
                MODULE, "find_build_tool", return_value=root / "tool"
            ):
                with mock.patch.object(
                    MODULE,
                    "run_text",
                    return_value=(
                        "Verified using v1 scheme (JAR signing): false\n"
                        "Verified using v2 scheme (APK Signature Scheme v2): true\n"
                        "Verified using v3 scheme (APK Signature Scheme v3): true\n"
                        "Verified using v4 scheme (APK Signature Scheme v4): false\n"
                        f"Signer #1 certificate SHA-256 digest: {'c' * 64}\n"
                    ),
                ):
                    with self.assertRaisesRegex(ValueError, "firma del APK"):
                        with mock.patch.object(MODULE, "verify_embedded_core"):
                            with mock.patch.object(
                                MODULE,
                                "apk_signing_block_ids",
                                return_value={
                                    MODULE.APK_V2_BLOCK_ID,
                                    MODULE.APK_V3_BLOCK_ID,
                                },
                            ):
                                MODULE.verify(
                                    apk,
                                    aab,
                                    root / "core.aar",
                                    "d" * 64,
                                    "0.1.0",
                                    1000,
                                    "a" * 40,
                                    "b" * 64,
                                )

    def test_rejects_apk_without_v3(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            apk = root / "release.apk"
            aab = root / "release.aab"
            apk.write_bytes(b"apk")
            aab.write_bytes(b"aab")
            fingerprint = "d" * 64
            report = (
                "Verified using v1 scheme (JAR signing): false\n"
                "Verified using v2 scheme (APK Signature Scheme v2): true\n"
                "Verified using v3 scheme (APK Signature Scheme v3): false\n"
                "Verified using v4 scheme (APK Signature Scheme v4): false\n"
                f"Signer #1 certificate SHA-256 digest: {fingerprint}\n"
            )
            with mock.patch.object(
                MODULE, "find_build_tool", return_value=root / "tool"
            ):
                with mock.patch.object(MODULE, "run_text", return_value=report):
                    with mock.patch.object(MODULE, "verify_embedded_core"):
                        with mock.patch.object(
                            MODULE,
                            "apk_signing_block_ids",
                            return_value={
                                MODULE.APK_V2_BLOCK_ID,
                                MODULE.APK_V3_BLOCK_ID,
                            },
                        ):
                            with self.assertRaisesRegex(ValueError, "v2\\+v3"):
                                MODULE.verify(
                                    apk,
                                    aab,
                                    root / "core.aar",
                                    fingerprint,
                                    "0.1.0",
                                    1000,
                                    "a" * 40,
                                    "b" * 64,
                                )

    def test_rejects_non_reproducible_dependency_info_block(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            apk = root / "release.apk"
            aab = root / "release.aab"
            apk.write_bytes(b"apk")
            aab.write_bytes(b"aab")
            with mock.patch.object(MODULE, "verify_embedded_core"):
                with mock.patch.object(
                    MODULE,
                    "apk_signing_block_ids",
                    return_value={
                        MODULE.APK_V2_BLOCK_ID,
                        MODULE.APK_V3_BLOCK_ID,
                        MODULE.APK_DEPENDENCY_INFO_BLOCK_ID,
                    },
                ):
                    with self.assertRaisesRegex(ValueError, "dependencias"):
                        MODULE.verify(
                            apk,
                            aab,
                            root / "core.aar",
                            "d" * 64,
                            "0.1.0",
                            1000,
                            "a" * 40,
                            "b" * 64,
                        )

    def test_build_tools_are_sorted_numerically(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            sdk = pathlib.Path(temp)
            old = sdk / "build-tools" / "9.0.0" / "apksigner"
            current = sdk / "build-tools" / "36.0.0" / "apksigner"
            old.parent.mkdir(parents=True)
            current.parent.mkdir(parents=True)
            old.write_text("", encoding="utf-8")
            current.write_text("", encoding="utf-8")
            with mock.patch.dict("os.environ", {"ANDROID_HOME": str(sdk)}, clear=False):
                self.assertEqual(current, MODULE.find_build_tool("apksigner"))

    def test_strict_aab_rejects_code_four_for_expiry_or_disabled_algorithm(
        self,
    ) -> None:
        certificate = b"certificado"
        fingerprint = hashlib.sha256(certificate).hexdigest()
        imported = mock.Mock(returncode=0, stdout=b"", stderr=b"")
        for warning in (
            "The signer certificate has expired.",
            "The SHA1 algorithm is considered a security risk and is disabled.",
        ):
            with self.subTest(warning=warning):
                failed = mock.Mock(
                    returncode=4,
                    stdout=f"jar verified.\nWarning: {warning}\n",
                    stderr="",
                )
                with mock.patch.object(
                    MODULE, "aab_certificate_der", return_value=certificate
                ):
                    with mock.patch(
                        "subprocess.run", side_effect=[imported, failed]
                    ):
                        with self.assertRaisesRegex(ValueError, "estricto"):
                            MODULE.verify_aab_signature(
                                pathlib.Path("release.aab"),
                                fingerprint,
                            )

    def test_embedded_native_core_must_match_rebuilt_aar(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            aar = root / "core.aar"
            apk = root / "release.apk"
            with zipfile.ZipFile(aar, "w") as archive:
                for abi in ("armeabi-v7a", "arm64-v8a", "x86_64"):
                    archive.writestr(f"jni/{abi}/libgojni.so", f"debug-core-{abi}")
            with zipfile.ZipFile(apk, "w") as archive:
                for abi in ("armeabi-v7a", "arm64-v8a", "x86_64"):
                    archive.writestr(f"lib/{abi}/libgojni.so", f"core-{abi}")

            def strip_debug(command: list[str], **_kwargs: object) -> mock.Mock:
                library = pathlib.Path(command[-1])
                library.write_bytes(library.read_bytes().replace(b"debug-", b""))
                return mock.Mock(returncode=0)

            with mock.patch.object(
                MODULE, "find_ndk_strip", return_value=root / "llvm-strip"
            ):
                with mock.patch("subprocess.run", side_effect=strip_debug):
                    MODULE.verify_embedded_core(apk, aar, MODULE.sha256(aar))

            corrupted = root / "corrupted.apk"
            with zipfile.ZipFile(corrupted, "w") as archive:
                for abi in ("armeabi-v7a", "arm64-v8a", "x86_64"):
                    content = "otro-core" if abi == "x86_64" else f"core-{abi}"
                    archive.writestr(f"lib/{abi}/libgojni.so", content)
            with mock.patch.object(
                MODULE, "find_ndk_strip", return_value=root / "llvm-strip"
            ):
                with mock.patch("subprocess.run", side_effect=strip_debug):
                    with self.assertRaisesRegex(ValueError, "normalizado exacto"):
                        MODULE.verify_embedded_core(
                            corrupted, aar, MODULE.sha256(aar)
                        )


if __name__ == "__main__":
    unittest.main()
