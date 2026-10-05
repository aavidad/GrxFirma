# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import importlib.util
import plistlib
import re
import stat
import sys
import tempfile
import unittest
from pathlib import Path


SAFARI_DIR = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "grxfirma_safari_postprocess", SAFARI_DIR / "postprocess.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


def build_settings(identifier: str, name: str, token: str) -> str:
    return f"""
\t\t{token} /* {name} */ = {{
\t\t\tisa = XCBuildConfiguration;
\t\t\tbuildSettings = {{
\t\t\t\tCODE_SIGN_ENTITLEMENTS = Old.entitlements;
\t\t\t\tPRODUCT_BUNDLE_IDENTIFIER = {identifier};
\t\t\t}};
\t\t\tname = {name};
\t\t}};
"""


def target_sections(app_name: str, extension_name: str) -> tuple[str, str]:
    native_targets = f"""
/* Begin PBXNativeTarget section */
\t\tTA /* {app_name} */ = {{
\t\t\tisa = PBXNativeTarget;
\t\t\tbuildConfigurationList = LA /* app configs */;
\t\t\tname = \"{app_name}\";
\t\t\tproductType = \"com.apple.product-type.application\";
\t\t}};
\t\tTE /* {extension_name} */ = {{
\t\t\tisa = PBXNativeTarget;
\t\t\tbuildConfigurationList = LE /* extension configs */;
\t\t\tname = \"{extension_name}\";
\t\t\tproductType = \"com.apple.product-type.app-extension\";
\t\t}};
/* End PBXNativeTarget section */
"""
    configuration_lists = """
/* Begin XCConfigurationList section */
\t\tLA /* app configs */ = {
\t\t\tisa = XCConfigurationList;
\t\t\tbuildConfigurations = (
\t\t\t\tA1 /* Debug */,
\t\t\t\tA2 /* Release */,
\t\t\t);
\t\t};
\t\tLE /* extension configs */ = {
\t\t\tisa = XCConfigurationList;
\t\t\tbuildConfigurations = (
\t\t\t\tE1 /* Debug */,
\t\t\t\tE2 /* Release */,
\t\t\t);
\t\t};
/* End XCConfigurationList section */
"""
    return native_targets, configuration_lists


class PostprocessTests(unittest.TestCase):
    bundle_id = "io.github.aavidad.grxfirma.safari"
    extension_id = bundle_id + ".Extension"

    def make_project(
        self,
        root: Path,
        include_view_controller: bool = True,
        app_source_name: str = "Shared (App)",
        extension_source_name: str = "Shared (Extension)",
    ) -> Path:
        project_dir = root / "GrxFirma Safari"
        xcodeproj = project_dir / "GrxFirma Safari.xcodeproj"
        app_source = project_dir / app_source_name
        extension_source = project_dir / extension_source_name
        xcodeproj.mkdir(parents=True)
        app_source.mkdir()
        extension_source.mkdir()
        resources = extension_source / "Resources"
        resources.mkdir()
        if include_view_controller:
            (app_source / "ViewController.swift").write_text(
                "final class ViewController {}\n", encoding="utf-8"
            )
        (extension_source / "SafariWebExtensionHandler.swift").write_text(
            "final class SafariWebExtensionHandler {}\n", encoding="utf-8"
        )
        (resources / "background.js").write_text(
            "async function pingLocalIntegration() { return { success: false }; }\n",
            encoding="utf-8",
        )
        native_targets, configuration_lists = target_sections(
            app_source_name, extension_source_name
        )
        project_text = "".join(
            [
                "// !$*UTF8*$!\n",
                native_targets,
                "/* Begin XCBuildConfiguration section */\n",
                build_settings(self.bundle_id, "Debug", "A1"),
                build_settings(self.bundle_id, "Release", "A2"),
                build_settings(self.extension_id, "Debug", "E1"),
                build_settings(self.extension_id, "Release", "E2"),
                "/* End XCBuildConfiguration section */\n",
                configuration_lists,
            ]
        )
        (xcodeproj / "project.pbxproj").write_text(project_text, encoding="utf-8")
        return project_dir

    def test_installs_handler_app_and_shared_keychain_entitlements(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            project_dir = self.make_project(root)
            report = MODULE.postprocess(
                project_dir,
                SAFARI_DIR,
                self.bundle_id,
                "https://127.0.0.1:63118",
            )

            self.assertEqual(report["extension_bundle_id"], self.extension_id)
            self.assertEqual(report["app_target"]["id"], "TA")
            self.assertEqual(report["extension_target"]["id"], "TE")
            self.assertEqual(report["app_target"]["configuration_ids"], ["A1", "A2"])
            handler = (
                project_dir / "Shared (Extension)" / "SafariWebExtensionHandler.swift"
            )
            app = project_dir / "Shared (App)" / "ViewController.swift"
            self.assertIn(MODULE.MARKER, handler.read_text(encoding="utf-8"))
            self.assertIn(MODULE.MARKER, app.read_text(encoding="utf-8"))
            self.assertNotIn(
                "__KEYCHAIN_SERVICE__", handler.read_text(encoding="utf-8")
            )
            self.assertNotIn(
                "__KEYCHAIN_ACCESS_GROUP_SUFFIX__",
                handler.read_text(encoding="utf-8"),
            )
            background = (
                project_dir / "Shared (Extension)" / "Resources" / "background.js"
            ).read_text(encoding="utf-8")
            self.assertIn(MODULE.NATIVE_ONLY_START, background)
            self.assertIn("disableSignRestFallback", background)
            self.assertEqual(
                report["background"], "Shared (Extension)/Resources/background.js"
            )
            self.assertEqual(
                report["app_entitlements"],
                "Shared (App)/GrxFirmaApp.entitlements",
            )
            self.assertEqual(
                report["extension_entitlements"],
                "Shared (Extension)/GrxFirmaExtension.entitlements",
            )

            app_entitlements = plistlib.loads(
                (
                    project_dir / "Shared (App)" / "GrxFirmaApp.entitlements"
                ).read_bytes()
            )
            extension_entitlements = plistlib.loads(
                (
                    project_dir
                    / "Shared (Extension)"
                    / "GrxFirmaExtension.entitlements"
                ).read_bytes()
            )
            expected_group = "$(AppIdentifierPrefix)" + self.bundle_id + ".shared"
            self.assertEqual(
                app_entitlements["keychain-access-groups"], [expected_group]
            )
            self.assertEqual(
                extension_entitlements["keychain-access-groups"], [expected_group]
            )
            self.assertTrue(extension_entitlements["com.apple.security.network.client"])
            self.assertNotIn("com.apple.security.network.client", app_entitlements)

            pbx = (
                project_dir / "GrxFirma Safari.xcodeproj" / "project.pbxproj"
            ).read_text(encoding="utf-8")
            self.assertEqual(pbx.count("CODE_SIGN_ENTITLEMENTS"), 4)
            self.assertEqual(pbx.count("ENABLE_HARDENED_RUNTIME = YES;"), 4)
            self.assertEqual(pbx.count("ENABLE_OUTGOING_NETWORK_CONNECTIONS = YES;"), 2)

    def test_postprocess_is_idempotent(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(Path(temporary))
            first = MODULE.postprocess(
                project_dir, SAFARI_DIR, self.bundle_id, "https://127.0.0.1:63118"
            )
            pbx_path = project_dir / "GrxFirma Safari.xcodeproj" / "project.pbxproj"
            first_pbx = pbx_path.read_bytes()
            background_path = (
                project_dir / "Shared (Extension)" / "Resources" / "background.js"
            )
            first_background = background_path.read_bytes()
            second = MODULE.postprocess(
                project_dir, SAFARI_DIR, self.bundle_id, "https://127.0.0.1:63118"
            )
            self.assertEqual(first, second)
            self.assertEqual(first_pbx, pbx_path.read_bytes())
            self.assertEqual(first_background, background_path.read_bytes())
            self.assertEqual(
                background_path.read_text(encoding="utf-8").count(
                    MODULE.NATIVE_ONLY_START
                ),
                1,
            )

    def test_atomic_replacement_preserves_source_permissions(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(Path(temporary))
            handler = (
                project_dir / "Shared (Extension)" / "SafariWebExtensionHandler.swift"
            )
            handler.chmod(0o600)
            MODULE.postprocess(
                project_dir, SAFARI_DIR, self.bundle_id, "https://127.0.0.1:63118"
            )
            self.assertEqual(stat.S_IMODE(handler.stat().st_mode), 0o600)
            self.assertFalse(list(project_dir.rglob("*.tmp")))

    def test_accepts_converter_app_name_containing_extension(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(
                Path(temporary),
                app_source_name="safari-web-extension-sample",
                extension_source_name="safari-web-extension-sample Extension",
            )
            report = MODULE.postprocess(
                project_dir, SAFARI_DIR, self.bundle_id, "https://127.0.0.1:63118"
            )
            self.assertEqual(
                report["view_controller"],
                "safari-web-extension-sample/ViewController.swift",
            )

    def test_rejects_non_loopback_endpoint(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(Path(temporary))
            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://example.org:63118",
                )

    def test_rejects_malformed_endpoint_with_controlled_error(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(Path(temporary))
            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://127.0.0.1:not-a-port",
                )

    def test_rejects_unknown_converter_layout(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(
                Path(temporary), include_view_controller=False
            )
            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://127.0.0.1:63118",
                )

    def test_rejects_target_without_debug_and_release(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(Path(temporary))
            project = project_dir / "GrxFirma Safari.xcodeproj" / "project.pbxproj"
            project.write_text(
                project.read_text(encoding="utf-8").replace(
                    "\t\t\tname = Release;",
                    "\t\t\tname = Archive;",
                    1,
                ),
                encoding="utf-8",
            )
            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://127.0.0.1:63118",
                )

    def test_rejects_symlinked_generated_source(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            project_dir = self.make_project(root)
            handler = (
                project_dir / "Shared (Extension)" / "SafariWebExtensionHandler.swift"
            )
            target = root / "outside-handler.swift"
            target.write_text("final class Outside {}\n", encoding="utf-8")
            handler.unlink()
            handler.symlink_to(target)
            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://127.0.0.1:63118",
                )

    def test_validation_failure_does_not_partially_replace_sources(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            project_dir = self.make_project(root)
            handler = (
                project_dir / "Shared (Extension)" / "SafariWebExtensionHandler.swift"
            )
            original = handler.read_text(encoding="utf-8")
            audit_target = root / "outside-audit"
            audit_target.mkdir()
            (project_dir / ".grxfirma-safari").symlink_to(
                audit_target, target_is_directory=True
            )
            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://127.0.0.1:63118",
                )
            self.assertEqual(handler.read_text(encoding="utf-8"), original)

    def test_symlinked_entitlement_is_rejected_before_any_source_change(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            project_dir = self.make_project(root)
            handler = (
                project_dir / "Shared (Extension)" / "SafariWebExtensionHandler.swift"
            )
            original = handler.read_bytes()
            outside = root / "outside.entitlements"
            outside.write_text("outside\n", encoding="utf-8")
            entitlement = project_dir / "Shared (App)" / "GrxFirmaApp.entitlements"
            entitlement.symlink_to(outside)

            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://127.0.0.1:63118",
                )

            self.assertEqual(handler.read_bytes(), original)
            self.assertEqual(outside.read_text(encoding="utf-8"), "outside\n")

    def test_write_failure_rolls_back_the_whole_generated_project(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(Path(temporary))
            tracked = [
                project_dir / "Shared (Extension)" / "SafariWebExtensionHandler.swift",
                project_dir / "Shared (App)" / "ViewController.swift",
                project_dir / "Shared (Extension)" / "Resources" / "background.js",
                project_dir / "GrxFirma Safari.xcodeproj" / "project.pbxproj",
            ]
            originals = {path: path.read_bytes() for path in tracked}
            real_write = MODULE.write_bytes_if_changed
            calls = 0

            def fail_during_transaction(path, data):
                nonlocal calls
                calls += 1
                if calls == 4:
                    raise OSError("simulated write failure")
                real_write(path, data)

            MODULE.write_bytes_if_changed = fail_during_transaction
            try:
                with self.assertRaises(OSError):
                    MODULE.postprocess(
                        project_dir,
                        SAFARI_DIR,
                        self.bundle_id,
                        "https://127.0.0.1:63118",
                    )
            finally:
                MODULE.write_bytes_if_changed = real_write

            self.assertEqual({path: path.read_bytes() for path in tracked}, originals)
            self.assertFalse(
                (project_dir / "Shared (App)" / "GrxFirmaApp.entitlements").exists()
            )
            self.assertFalse((project_dir / ".grxfirma-safari").exists())

    def test_rejects_inconsistent_existing_background_markers(self):
        with tempfile.TemporaryDirectory() as temporary:
            project_dir = self.make_project(Path(temporary))
            background = (
                project_dir / "Shared (Extension)" / "Resources" / "background.js"
            )
            background.write_text(MODULE.NATIVE_ONLY_START + "\n", encoding="utf-8")
            with self.assertRaises(MODULE.PostprocessError):
                MODULE.postprocess(
                    project_dir,
                    SAFARI_DIR,
                    self.bundle_id,
                    "https://127.0.0.1:63118",
                )

    def test_templates_keep_security_invariants(self):
        handler = (SAFARI_DIR / MODULE.HANDLER_TEMPLATE).read_text(encoding="utf-8")
        app = (SAFARI_DIR / MODULE.APP_TEMPLATE).read_text(encoding="utf-8")
        native_only = (SAFARI_DIR / MODULE.NATIVE_ONLY_TEMPLATE).read_text(
            encoding="utf-8"
        )
        self.assertIn("deviceOwnerAuthentication", handler)
        self.assertIn("SignGate.acquire()", handler)
        self.assertIn('activeAccountPrefix = "active-sign-lease-"', handler)
        self.assertIn("leaseInterval: TimeInterval = 210", handler)
        self.assertIn('cooldownAccount = "sign-cooldown-until"', handler)
        self.assertIn("SignGate.release(lease, after: timedOut ? 30 : 0)", handler)
        self.assertIn("maximumConcurrentRequests = 2", handler)
        self.assertIn("nextAllowed = Date.distantPast", handler)
        self.assertIn(
            "responder.setCancellation { authentication.invalidate() }", handler
        )
        self.assertIn("responder.replaceCancellationAndStart", handler)
        self.assertIn("invalidateAndCancel", handler)
        self.assertIn("kSecUseDataProtectionKeychain", handler)
        self.assertIn("kSecAttrAccessGroup", handler)
        self.assertIn("SecTaskCopyValueForEntitlement", handler)
        self.assertIn('components.scheme == "https"', handler)
        self.assertIn('["127.0.0.1", "::1"]', handler)
        self.assertIn("maximumInputBase64Characters", handler)
        timeout = re.search(r"operationTimeout: TimeInterval = (\d+)", handler)
        self.assertIsNotNone(timeout)
        self.assertLess(int(timeout.group(1)), 180)
        lease_interval = re.search(r"leaseInterval: TimeInterval = (\d+)", handler)
        timeout_hold = re.search(r"timedOut \? (\d+) : 0", handler)
        self.assertIsNotNone(lease_interval)
        self.assertIsNotNone(timeout_hold)
        self.assertLess(
            int(timeout.group(1)) + int(timeout_hold.group(1)),
            int(lease_interval.group(1)),
        )
        self.assertIn("activeAccountPrefix + String(bucket - 1)", handler)
        self.assertIn("completionHandler(nil)", handler)
        self.assertIn("serverTrust", handler)
        self.assertIn("SecTrustEvaluateWithError", handler)
        self.assertIn("expectedTLSPin", handler)
        self.assertLess(
            handler.index("SecTrustEvaluateWithError"),
            handler.index("constantTimeEqual(actualPin, expectedTLSPin)"),
        )
        prepare_index = handler.index("BoundedHTTPClient.shared.prepare")
        self.assertLess(
            prepare_index,
            handler.index("responder.replaceCancellationAndStart", prepare_index),
        )
        self.assertIn("SHA256.hash(data: documentData)", handler)
        self.assertIn("Documento SHA-256", handler)
        self.assertNotIn("allowsAnyHTTPSCertificate", handler)
        self.assertNotIn('payload["error"]', handler)
        self.assertIn("NSSecureTextField", app)
        self.assertIn("@IBOutlet var appNameLabel", app)
        self.assertIn("@IBAction func openSafariExtensionPreferences", app)
        self.assertIn("kSecAttrAccessibleWhenUnlockedThisDeviceOnly", app)
        self.assertIn("tlsPinField", app)
        self.assertIn("kSecAttrAccessGroup", app)
        self.assertIn("previousToken", app)
        self.assertNotIn("UserDefaults", app)
        self.assertIn("disableSignRestFallback", native_only)
        self.assertIn("pingThroughSafariHandler", native_only)
        self.assertNotIn("fetch(", native_only)


if __name__ == "__main__":
    unittest.main()
