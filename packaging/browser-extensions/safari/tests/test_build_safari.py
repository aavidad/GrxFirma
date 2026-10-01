# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
import os
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[4]
BUILD_SCRIPT = ROOT / "packaging" / "browser-extensions" / "build-safari.sh"


class SafariBuildScriptTests(unittest.TestCase):
    def make_source(self, root: Path) -> Path:
        source = root / "source"
        source.mkdir()
        manifest = {
            "manifest_version": 3,
            "name": "Safari build test",
            "version": "1.0.0",
            "permissions": ["nativeMessaging"],
            "host_permissions": ["https://*.dipgra.es/*"],
            "background": {"service_worker": "background.js"},
        }
        (source / "manifest.json").write_text(json.dumps(manifest), encoding="utf-8")
        (source / "background.js").write_text(
            textwrap.dedent(
                """
                async function getSystemCertificatesFromRest() {}
                async function signWithSystemCertificateRest() {}
                async function verifyWithSystemCertificateRest() {}
                async function pingLocalIntegration() {}
                async function sendToNativeHost() {}
                """
            ),
            encoding="utf-8",
        )
        return source

    def make_fake_apple_tools(self, root: Path) -> Path:
        tools = root / "tools"
        tools.mkdir()
        uname = tools / "uname"
        uname.write_text(
            "#!/usr/bin/env bash\nprintf '%s\\n' Darwin\n", encoding="utf-8"
        )
        uname.chmod(0o755)
        plutil = tools / "plutil"
        plutil.write_text("#!/usr/bin/env bash\nexit 0\n", encoding="utf-8")
        plutil.chmod(0o755)

        xcrun = tools / "xcrun"
        xcrun.write_text(
            textwrap.dedent(
                r'''
                #!/usr/bin/env python3
                import os
                import shutil
                import sys
                from pathlib import Path

                args = sys.argv[1:]
                if args[:1] == ["--find"]:
                    print(Path(sys.argv[0]).resolve())
                    raise SystemExit(0)
                if args[:2] == ["xcodebuild", "-version"]:
                    print("Xcode 16.4")
                    print("Build version 16F6")
                    raise SystemExit(0)
                if args[:1] == ["xcodebuild"]:
                    raise SystemExit(0)
                if args[:2] == ["safari-web-extension-converter", "--help"]:
                    print("--macos-only --copy-resources --macos-version-minimum --force --no-open --no-prompt")
                    raise SystemExit(0)
                if args[:1] != ["safari-web-extension-converter"]:
                    raise SystemExit(2)
                if os.environ.get("MOCK_CONVERTER_FAIL") == "1":
                    print("mock converter failure", file=sys.stderr)
                    raise SystemExit(9)

                source = Path(args[1])
                project = Path(args[args.index("--project-location") + 1])
                app_name = args[args.index("--app-name") + 1]
                bundle_id = args[args.index("--bundle-identifier") + 1]
                extension_name = app_name + " Extension"
                extension_id = bundle_id + ".Extension"
                app_dir = project / app_name
                extension_dir = project / extension_name
                resources = extension_dir / "Resources"
                xcodeproj = project / (app_name + ".xcodeproj")
                app_dir.mkdir(parents=True)
                resources.mkdir(parents=True)
                xcodeproj.mkdir(parents=True)
                (app_dir / "ViewController.swift").write_text("final class ViewController {}\n")
                (extension_dir / "SafariWebExtensionHandler.swift").write_text(
                    "final class SafariWebExtensionHandler {}\n"
                )
                shutil.copy2(source / "background.js", resources / "background.js")

                def configuration(identifier, name, token):
                    return f"""
                {token} /* {name} */ = {{
                    isa = XCBuildConfiguration;
                    buildSettings = {{
                        PRODUCT_BUNDLE_IDENTIFIER = \"{identifier}\";
                    }};
                    name = {name};
                }};
                """

                pbx = f"""// !$*UTF8*$!
                /* Begin PBXNativeTarget section */
                TA /* app */ = {{
                    isa = PBXNativeTarget;
                    buildConfigurationList = LA /* app configs */;
                    name = \"{app_name}\";
                    productType = \"com.apple.product-type.application\";
                }};
                TE /* extension */ = {{
                    isa = PBXNativeTarget;
                    buildConfigurationList = LE /* extension configs */;
                    name = \"{extension_name}\";
                    productType = \"com.apple.product-type.app-extension\";
                }};
                /* End PBXNativeTarget section */
                /* Begin XCBuildConfiguration section */
                {configuration(bundle_id, "Debug", "A1")}
                {configuration(bundle_id, "Release", "A2")}
                {configuration(extension_id, "Debug", "E1")}
                {configuration(extension_id, "Release", "E2")}
                /* End XCBuildConfiguration section */
                /* Begin XCConfigurationList section */
                LA /* app configs */ = {{
                    isa = XCConfigurationList;
                    buildConfigurations = (
                        A1 /* Debug */,
                        A2 /* Release */,
                    );
                }};
                LE /* extension configs */ = {{
                    isa = XCConfigurationList;
                    buildConfigurations = (
                        E1 /* Debug */,
                        E2 /* Release */,
                    );
                }};
                /* End XCConfigurationList section */
                """
                (xcodeproj / "project.pbxproj").write_text(pbx)
                '''
            ).lstrip(),
            encoding="utf-8",
        )
        xcrun.chmod(0o755)
        return tools

    def run_build(
        self,
        source: Path,
        output: Path,
        tools: Path,
        fail_converter: bool = False,
    ) -> subprocess.CompletedProcess[str]:
        environment = os.environ.copy()
        environment.update(
            {
                "PATH": str(tools) + os.pathsep + environment["PATH"],
                "SAFARI_EXTENSION_SRC": str(source),
                "SAFARI_PROJECT_DIR": str(output),
                "SAFARI_APP_NAME": "Test Safari",
                "SAFARI_BUNDLE_ID": "es.example.grxfirma.safari",
            }
        )
        if fail_converter:
            environment["MOCK_CONVERTER_FAIL"] = "1"
        return subprocess.run(
            [str(BUILD_SCRIPT)],
            cwd=ROOT,
            env=environment,
            check=False,
            capture_output=True,
            text=True,
            timeout=30,
        )

    def test_build_publishes_only_after_validation_and_records_environment(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = self.make_source(root)
            tools = self.make_fake_apple_tools(root)
            output = root / "release" / "safari"
            output.mkdir(parents=True)
            (output / "old-output").write_text("keep until publish\n")

            result = self.run_build(source, output, tools)

            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((output / "old-output").exists())
            report = json.loads(
                (output / ".grxfirma-safari" / "integration-report.json").read_text()
            )
            self.assertEqual(report["build_environment"]["minimum_macos"], "12.3")
            self.assertEqual(
                report["build_environment"]["xcode_version"],
                ["Xcode 16.4", "Build version 16F6"],
            )
            self.assertRegex(
                report["build_environment"]["source_tree_sha256"],
                r"^[0-9a-f]{64}$",
            )
            self.assertTrue(
                (
                    output / "Test Safari Extension" / "SafariWebExtensionHandler.swift"
                ).is_file()
            )
            self.assertFalse(list(output.parent.glob(".grxfirma-safari-*")))

    def test_converter_failure_preserves_previous_output(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = self.make_source(root)
            tools = self.make_fake_apple_tools(root)
            output = root / "release" / "safari"
            output.mkdir(parents=True)
            sentinel = output / "old-output"
            sentinel.write_text("must survive\n")

            result = self.run_build(source, output, tools, fail_converter=True)

            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(sentinel.read_text(), "must survive\n")
            self.assertFalse(list(output.parent.glob(".grxfirma-safari-*")))


if __name__ == "__main__":
    unittest.main()
