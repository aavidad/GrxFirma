# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import importlib.util
import hashlib
import io
import os
import pathlib
import tempfile
import unittest
from unittest import mock


SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "validate_release_signing.py"
SPEC = importlib.util.spec_from_file_location("validate_release_signing", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ValidateReleaseSigningTest(unittest.TestCase):
    def test_rejects_missing_credentials(self) -> None:
        clean = {name: "" for name in MODULE.REQUIRED}
        with mock.patch.dict(os.environ, clean, clear=False):
            with mock.patch("sys.stderr", new=io.StringIO()):
                self.assertEqual(1, MODULE.main())

    def test_accepts_complete_external_credentials(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            keystore = pathlib.Path(temp, "release.p12")
            keystore.write_bytes(b"placeholder")
            certificate = b"certificado-publico"
            values = {
                "GRXFIRMA_ANDROID_KEYSTORE": str(keystore),
                "GRXFIRMA_ANDROID_KEYSTORE_PASSWORD": "secret",
                "GRXFIRMA_ANDROID_KEY_ALIAS": "release",
                "GRXFIRMA_ANDROID_KEY_PASSWORD": "secret",
                "GRXFIRMA_ANDROID_SIGNING_CERT_SHA256": hashlib.sha256(
                    certificate
                ).hexdigest(),
                "GRXFIRMA_ANDROID_SOURCE_COMMIT": "a" * 40,
            }
            with mock.patch.dict(os.environ, values, clear=False):
                with mock.patch.object(
                    MODULE, "git_text", side_effect=["a" * 40, ""]
                ):
                    with mock.patch.object(
                        MODULE,
                        "export_signing_certificate",
                        return_value=certificate,
                    ):
                        with mock.patch("sys.stdout", new=io.StringIO()):
                            self.assertEqual(0, MODULE.main())

    def test_rejects_mismatched_signing_fingerprint(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            keystore = pathlib.Path(temp, "release.p12")
            keystore.write_bytes(b"placeholder")
            values = {
                "GRXFIRMA_ANDROID_KEYSTORE": str(keystore),
                "GRXFIRMA_ANDROID_KEYSTORE_PASSWORD": "secret",
                "GRXFIRMA_ANDROID_KEY_ALIAS": "release",
                "GRXFIRMA_ANDROID_KEY_PASSWORD": "secret",
                "GRXFIRMA_ANDROID_SIGNING_CERT_SHA256": "0" * 64,
                "GRXFIRMA_ANDROID_SOURCE_COMMIT": "a" * 40,
            }
            with mock.patch.dict(os.environ, values, clear=False):
                with mock.patch.object(
                    MODULE, "git_text", side_effect=["a" * 40, ""]
                ):
                    with mock.patch.object(
                        MODULE,
                        "export_signing_certificate",
                        return_value=b"otro-certificado",
                    ):
                        with mock.patch("sys.stderr", new=io.StringIO()):
                            self.assertEqual(1, MODULE.main())


if __name__ == "__main__":
    unittest.main()
