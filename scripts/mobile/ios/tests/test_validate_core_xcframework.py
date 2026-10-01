# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import pathlib
import plistlib
import sys
import tempfile
import unittest


SCRIPT_DIR = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPT_DIR))
import validate_core_xcframework as validator  # noqa: E402


HEADER = """
FOUNDATION_EXPORT MobilebindFacade *MobilebindNewIOSFacade(NSString *applicationSupportDir, NSString *appGroupDir, NSString *keychainAccessGroup, NSError **error);
- (NSString *)mobileContractJSON;
- (void)clearSession;
- (NSString *)selectCertificateJSON:(NSString *)payload error:(NSError **)error;
- (NSString *)importCertificateJSON:(NSString *)payload error:(NSError **)error;
- (NSString *)signJSON:(NSString *)payload error:(NSError **)error;
- (NSString *)verifyJSON:(NSString *)payload error:(NSError **)error;
- (NSString *)resolvePlatformProfileJSON:(NSError **)error;
"""


def write_plist(path: pathlib.Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("wb") as handle:
        plistlib.dump(value, handle)


def create_slice(
    root: pathlib.Path,
    identifier: str,
    variant: str,
    architectures: list[str],
    header: str = HEADER,
) -> dict:
    framework = root / identifier / "Mobilebind.framework"
    (framework / "Headers").mkdir(parents=True)
    (framework / "Modules").mkdir()
    (framework / "Headers/Mobilebind.h").write_text(header, encoding="utf-8")
    (framework / "Modules/module.modulemap").write_text(
        'framework module Mobilebind { umbrella header "Mobilebind.h" export * }\n',
        encoding="utf-8",
    )
    (framework / "Mobilebind").write_bytes(b"!<arch>\n" + b"M" * 4088)
    write_plist(
        framework / "Info.plist",
        {"CFBundlePackageType": "FMWK", "CFBundleIdentifier": "go.mobilebind"},
    )
    result: dict[str, object] = {
        "LibraryIdentifier": identifier,
        "LibraryPath": "Mobilebind.framework",
        "SupportedArchitectures": architectures,
        "SupportedPlatform": "ios",
    }
    if variant == "simulator":
        result["SupportedPlatformVariant"] = "simulator"
    return result


def create_framework(
    root: pathlib.Path, header: str = HEADER, simulator_arches: list[str] | None = None
) -> pathlib.Path:
    framework = root / "Mobilebind.xcframework"
    framework.mkdir()
    libraries = [
        create_slice(framework, "ios-arm64", "device", ["arm64"], header),
        create_slice(
            framework,
            "ios-arm64_x86_64-simulator",
            "simulator",
            simulator_arches or ["arm64", "x86_64"],
            header,
        ),
    ]
    write_plist(
        framework / "Info.plist",
        {"XCFrameworkFormatVersion": "1.0", "AvailableLibraries": libraries},
    )
    return framework


class ValidateCoreXCFrameworkTests(unittest.TestCase):
    def test_accepts_complete_device_and_simulator_contract(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            framework = create_framework(pathlib.Path(temp))
            digest = validator.validate(framework, None)
            self.assertRegex(digest, r"^[0-9a-f]{64}$")
            self.assertEqual(validator.validate(framework, digest), digest)

    def test_rejects_missing_ios_factory(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            framework = create_framework(
                pathlib.Path(temp),
                HEADER.replace("MobilebindNewIOSFacade", "MissingFactory"),
            )
            with self.assertRaisesRegex(
                validator.ValidationError, "MobilebindNewIOSFacade"
            ):
                validator.validate(framework, None)

    def test_rejects_incomplete_simulator_architectures(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            framework = create_framework(pathlib.Path(temp), simulator_arches=["arm64"])
            with self.assertRaisesRegex(validator.ValidationError, "x86_64"):
                validator.validate(framework, None)

    def test_rejects_checksum_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            framework = create_framework(pathlib.Path(temp))
            with self.assertRaisesRegex(validator.ValidationError, "no coincide"):
                validator.validate(framework, "0" * 64)

    def test_rejects_non_apple_binary(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            framework = create_framework(pathlib.Path(temp))
            binary = framework / "ios-arm64/Mobilebind.framework/Mobilebind"
            binary.write_bytes(b"M" * 4096)
            with self.assertRaisesRegex(validator.ValidationError, "Mach-O/archive"):
                validator.validate(framework, None)


if __name__ == "__main__":
    unittest.main()
