# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
import zipfile


BUILD_PATH = Path(__file__).resolve().parents[1] / "build.py"
SPEC = importlib.util.spec_from_file_location("grxfirma_extension_build", BUILD_PATH)
assert SPEC is not None and SPEC.loader is not None
build = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(build)


class FirefoxArtifactTests(unittest.TestCase):
    manifest = {
        "version": "1.2.3",
        "browser_specific_settings": {"gecko": {"id": "grxfirma@aavidad.github.io"}},
    }

    def write_xpi(
        self,
        path: Path,
        *,
        manifest: dict | None = None,
        signature: str | None = "META-INF/mozilla.rsa",
    ) -> None:
        with zipfile.ZipFile(path, "w") as archive:
            archive.writestr("manifest.json", json.dumps(manifest or self.manifest))
            if signature:
                archive.writestr(signature, b"test-signature-envelope")

    def test_accepts_signed_xpi_with_matching_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            artifact = Path(tmp) / "extension.xpi"
            self.write_xpi(artifact)

            build.validate_signed_firefox_xpi(artifact, self.manifest)

    def test_rejects_xpi_without_signature_envelope(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            artifact = Path(tmp) / "extension.xpi"
            self.write_xpi(artifact, signature=None)

            with self.assertRaisesRegex(ValueError, "no contiene firma Mozilla"):
                build.validate_signed_firefox_xpi(artifact, self.manifest)

    def test_rejects_mismatched_version_or_extension_id(self) -> None:
        cases = (
            {**self.manifest, "version": "9.9.9"},
            {
                **self.manifest,
                "browser_specific_settings": {"gecko": {"id": "otro@example.test"}},
            },
        )
        with tempfile.TemporaryDirectory() as tmp:
            for index, packaged_manifest in enumerate(cases):
                artifact = Path(tmp) / f"extension-{index}.xpi"
                self.write_xpi(artifact, manifest=packaged_manifest)
                with self.assertRaises(ValueError):
                    build.validate_signed_firefox_xpi(artifact, self.manifest)

    def test_signed_xpi_must_match_every_filtered_source_file(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "manifest.json").write_text(
                json.dumps(self.manifest), encoding="utf-8"
            )
            (source / "background.js").write_text("const host = 'io.github.aavidad.grxfirma';\n", encoding="utf-8")
            artifact = root / "signed.xpi"
            with zipfile.ZipFile(artifact, "w") as archive:
                archive.writestr("manifest.json", (source / "manifest.json").read_bytes())
                archive.writestr("background.js", (source / "background.js").read_bytes())
                archive.writestr("META-INF/mozilla.rsa", b"test-signature-envelope")
            build.validate_signed_firefox_xpi(artifact, self.manifest, source)

            with zipfile.ZipFile(artifact, "w") as archive:
                archive.writestr("manifest.json", (source / "manifest.json").read_bytes())
                archive.writestr("background.js", b"const host = 'com.dipgra.autofirma';\n")
                archive.writestr("META-INF/mozilla.rsa", b"test-signature-envelope")
            with self.assertRaisesRegex(ValueError, "difiere de la fuente"):
                build.validate_signed_firefox_xpi(artifact, self.manifest, source)

            with zipfile.ZipFile(artifact, "w") as archive:
                archive.writestr("manifest.json", (source / "manifest.json").read_bytes())
                archive.writestr("background.js", (source / "background.js").read_bytes())
                archive.writestr("extra.js", b"unexpected")
                archive.writestr("META-INF/mozilla.rsa", b"test-signature-envelope")
            with self.assertRaisesRegex(ValueError, "no coincide con la fuente"):
                build.validate_signed_firefox_xpi(artifact, self.manifest, source)

    def test_metadata_binds_xpi_with_sha256(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "manifest.json").write_text(
                json.dumps(self.manifest), encoding="utf-8"
            )
            artifact = root / "extension.xpi"
            self.write_xpi(artifact)
            metadata_path = root / "extension.metadata.json"
            original_metadata_path = build.FIREFOX_METADATA
            build.FIREFOX_METADATA = metadata_path
            try:
                build.write_firefox_metadata(source, artifact, True, "test")
            finally:
                build.FIREFOX_METADATA = original_metadata_path

            metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
            self.assertTrue(metadata["signed"])
            self.assertEqual(metadata["version"], "1.2.3")
            self.assertEqual(metadata["extension_id"], "grxfirma@aavidad.github.io")
            self.assertEqual(metadata["xpi_sha256"], build.sha256_file(artifact))


class PackagedSurfaceTests(unittest.TestCase):
    def test_archive_is_reproducible_across_mtimes_and_timezones(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "manifest.json").write_text(
                json.dumps({"version": "1.0.0"}), encoding="utf-8"
            )
            script = source / "background.js"
            script.write_text("const value = 1;\n", encoding="utf-8")
            first = root / "first.zip"
            second = root / "second.zip"

            previous_epoch = os.environ.get("SOURCE_DATE_EPOCH")
            previous_tz = os.environ.get("TZ")
            try:
                os.environ["SOURCE_DATE_EPOCH"] = "1700000001"
                os.environ["TZ"] = "UTC"
                build.build_archive(source, first)

                os.utime(script, (1900000000, 1900000000))
                script.chmod(0o755)
                os.environ["TZ"] = "Pacific/Kiritimati"
                build.build_archive(source, second)
            finally:
                if previous_epoch is None:
                    os.environ.pop("SOURCE_DATE_EPOCH", None)
                else:
                    os.environ["SOURCE_DATE_EPOCH"] = previous_epoch
                if previous_tz is None:
                    os.environ.pop("TZ", None)
                else:
                    os.environ["TZ"] = previous_tz

            self.assertEqual(first.read_bytes(), second.read_bytes())
            with zipfile.ZipFile(first) as archive:
                entries = archive.infolist()
            self.assertEqual(
                {entry.date_time for entry in entries},
                {(2023, 11, 14, 22, 13, 20)},
            )
            self.assertTrue(
                all((entry.external_attr >> 16) == 0o100644 for entry in entries)
            )

    def test_archive_rejects_invalid_source_date_epoch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "manifest.json").write_text(
                json.dumps({"version": "1.0.0"}), encoding="utf-8"
            )
            previous_epoch = os.environ.get("SOURCE_DATE_EPOCH")
            try:
                os.environ["SOURCE_DATE_EPOCH"] = "not-an-epoch"
                with self.assertRaisesRegex(ValueError, "SOURCE_DATE_EPOCH"):
                    build.build_archive(source, root / "extension.zip")
            finally:
                if previous_epoch is None:
                    os.environ.pop("SOURCE_DATE_EPOCH", None)
                else:
                    os.environ["SOURCE_DATE_EPOCH"] = previous_epoch

    def test_excludes_legacy_credential_and_session_code(self) -> None:
        forbidden = {
            "crypto_utils.js",
            "content_scripts/autologin.js",
            "content_scripts/session_sync.js",
        }
        for variant in ("chromium", "firefox"):
            packaged = {
                relative.as_posix()
                for _, relative in build.iter_source_files(build.SRC_DIR / variant)
            }
            self.assertTrue(
                forbidden.isdisjoint(packaged),
                f"{variant} empaqueta superficie heredada: {sorted(forbidden & packaged)}",
            )

    def test_options_are_new_and_legacy_vault_does_not_enter_archive(self) -> None:
        for variant in ("chromium", "firefox"):
            source = build.SRC_DIR / variant
            manifest = json.loads((source / "manifest.json").read_text(encoding="utf-8"))
            self.assertEqual(manifest.get("options_ui", {}).get("page"), "options.html")
            packaged = {
                relative.as_posix() for _, relative in build.iter_source_files(source)
            }
            self.assertTrue({"options.html", "options.js", "trusted_sites.js"} <= packaged)
            self.assertFalse(any(path.startswith("signer/") for path in packaged))
            for path in ("options.html", "options.js"):
                content = (source / path).read_text(encoding="utf-8")
                for legacy_token in ("grx_vault", "vaultPass", "session_sync", "autologin"):
                    self.assertNotIn(legacy_token, content)

    def test_rejects_packaged_bearer_even_before_creating_archive(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "manifest.json").write_text('{"version":"1.1.0"}', encoding="utf-8")
            config = source / "config.js"
            config.write_text('var CONFIG = { LOCAL_REST_BEARER: "secret" };', encoding="utf-8")
            output = root / "extension.zip"
            with self.assertRaisesRegex(ValueError, "LOCAL_REST_BEARER"):
                build.build_archive(source, output)
            self.assertFalse(output.exists())

            config.write_text('var CONFIG = { LOCAL_REST_BEARER: "" };', encoding="utf-8")
            build.build_archive(source, output)
            self.assertTrue(output.is_file())

    def test_rejects_forbidden_package_paths(self) -> None:
        for path in ("crypto_utils.js", "content_scripts/autologin.js",
                     "content_scripts/session_sync.js", "signer/signer.js"):
            with self.assertRaisesRegex(ValueError, "heredados"):
                build.validate_package_paths({"manifest.json", path})

    def test_manifests_do_not_activate_legacy_authentication(self) -> None:
        for variant in ("chromium", "firefox"):
            manifest = json.loads(
                (build.SRC_DIR / variant / "manifest.json").read_text(encoding="utf-8")
            )
            background = (build.SRC_DIR / variant / "background.js").read_text(
                encoding="utf-8"
            )
            scripts = {
                script
                for content_script in manifest.get("content_scripts", [])
                for script in content_script.get("js", [])
            }
            scripts.update(manifest.get("background", {}).get("scripts", []))
            service_worker = manifest.get("background", {}).get("service_worker")
            if service_worker:
                scripts.add(service_worker)
            self.assertNotIn("content_scripts/autologin.js", scripts)
            self.assertNotIn("content_scripts/session_sync.js", scripts)
            self.assertNotIn("crypto_utils.js", scripts)
            for legacy_token in ("grx_vault", "CryptoUtils", "GRXGO_API"):
                self.assertNotIn(legacy_token, background)

        firefox = json.loads(
            (build.SRC_DIR / "firefox" / "manifest.json").read_text(encoding="utf-8")
        )
        collected = firefox["browser_specific_settings"]["gecko"][
            "data_collection_permissions"
        ]["required"]
        self.assertNotIn("authenticationInfo", collected)


if __name__ == "__main__":
    unittest.main()
