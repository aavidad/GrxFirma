# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from __future__ import annotations

import importlib.util
import pathlib
import unittest
import xml.etree.ElementTree as ET


SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "validate_project.py"
SPEC = importlib.util.spec_from_file_location("validate_project", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)

ACTIVITY = '''
class MainActivity {
    fun onCreate() {
        if (BuildConfig.CORE_MODE == "production") {
            window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        }
        clearSession(); releasePersistableUriPermission(); canAcceptIncomingDocument(); UiText.Verification
    }
}
'''


def sources(**extra: str) -> dict[pathlib.Path, str]:
    files = {pathlib.Path("MainActivity.kt"): ACTIVITY}
    files.update({pathlib.Path(name): text for name, text in extra.items()})
    return files


def manifest(components: str) -> ET.Element:
    return ET.fromstring(
        '<manifest xmlns:android="http://schemas.android.com/apk/res/android">'
        f"<application>{components}</application></manifest>"
    )


class KotlinSourcesTest(unittest.TestCase):
    def test_accepts_the_reviewed_baseline(self) -> None:
        MODULE.check_kotlin_sources(sources(**{
            "AppPreferences.kt": 'val p: SharedPreferences\nconst val THEME = "theme"\np.putString(THEME, x)\np.putString("tsa_url", u)',
        }))

    def test_rejects_datastore_and_open_file_output(self) -> None:
        for code in ("import androidx.datastore.preferences.core.Preferences",
                     "val Context.store by preferencesDataStore(name = \"x\")",
                     "openFileOutput(\"secreto\", 0)"):
            with self.subTest(code=code), self.assertRaises(RuntimeError):
                MODULE.check_kotlin_sources(sources(**{"Store.kt": code}))

    def test_rejects_sensitive_keys_through_constants_and_string_sets(self) -> None:
        cases = (
            'SharedPreferences\nconst val SAVED = "cert_alias"\np.putString(SAVED, a)',
            'SharedPreferences\nconst val IDENTITIES = "cert_ids"\np.putStringSet(IDENTITIES, s)',
            'SharedPreferences\np.putStringSet("pkcs12_paths", s)',
            'SharedPreferences\np.putString(Keys.UNKNOWN, s)',
        )
        for code in cases:
            with self.subTest(code=code), self.assertRaises(RuntimeError):
                MODULE.check_kotlin_sources(sources(**{"AppPreferences.kt": code}))

    def test_rejects_kotlin_network_access(self) -> None:
        for code in ("val u = java.net.URL(x)", "URL(x).openStream()", "val c: URLConnection = y",
                     "import java.net.URL", "stream = url.openStream()"):
            with self.subTest(code=code), self.assertRaises(RuntimeError):
                MODULE.check_kotlin_sources(sources(**{"Net.kt": code}))

    def test_uri_and_encoders_are_not_network(self) -> None:
        MODULE.check_kotlin_sources(sources(**{"Links.kt": "val u = URI(x)\nURLEncoder.encode(y)"}))

    def test_requires_flag_secure_applied_in_production(self) -> None:
        mentioned_only = ACTIVITY.replace(
            "window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)", "// FLAG_SECURE pendiente")
        with self.assertRaisesRegex(RuntimeError, "FLAG_SECURE"):
            MODULE.check_kotlin_sources({pathlib.Path("MainActivity.kt"): mentioned_only})
        cleared = ACTIVITY + "\nwindow.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)"
        with self.assertRaisesRegex(RuntimeError, "FLAG_SECURE"):
            MODULE.check_kotlin_sources({pathlib.Path("MainActivity.kt"): cleared})
        guarded = ACTIVITY + '\nif (BuildConfig.CORE_MODE != "production") window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)'
        MODULE.check_kotlin_sources({pathlib.Path("MainActivity.kt"): guarded})


class ManifestComponentsTest(unittest.TestCase):
    def test_accepts_main_activity_and_private_service(self) -> None:
        MODULE.check_manifest_components(manifest(
            '<activity android:name=".MainActivity" android:exported="true"><intent-filter/></activity>'
            '<service android:name="x.Holder" android:exported="false"/>'
        ))

    def test_rejects_other_exported_components(self) -> None:
        cases = (
            '<activity android:name=".Other" android:exported="true"/>',
            '<receiver android:name=".Boot" android:exported="true"/>',
            '<service android:name=".Svc"><intent-filter/></service>',
            '<provider android:name=".Files" android:authorities="a"/>',
            '<activity-alias android:name=".Alias" android:exported="true"/>',
        )
        for component in cases:
            with self.subTest(component=component), self.assertRaises(RuntimeError):
                MODULE.check_manifest_components(manifest(component))


if __name__ == "__main__":
    unittest.main()
