# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""Contrato QML y ejecución de la conversión IDNA real de Qt."""

import json
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
QT = ROOT / "cmd/gui-qml"


class VerificationUrlIdnContract(unittest.TestCase):
    def test_qml_uses_the_registered_cpp_bridge_for_qr_and_csv(self):
        qml = (QT / "qml/main.qml").read_text(encoding="utf-8")
        for function in ("normalizedQrUrl", "normalizedCsvUrl", "validQrUrl"):
            body = qml.split("function " + function + "(", 1)[1].split("\n    }", 1)[0]
            self.assertIn("portalSeal.normalizeVerificationUrl", body)
        self.assertIn('setContextProperty("portalSeal", &portalSeal)', (QT / "main.cpp").read_text(encoding="utf-8"))
        self.assertIn("Q_INVOKABLE QString normalizeVerificationUrl", (QT / "portalsealbridge.h").read_text(encoding="utf-8"))
        self.assertIn("VerificationUrl::normalize(raw)", (QT / "portalsealbridge.cpp").read_text(encoding="utf-8"))
        self.assertIn("QUrl::toAce", (QT / "verificationurl.h").read_text(encoding="utf-8"))

    def test_native_idn_validation_against_shared_cases(self):
        if not shutil.which("c++") or not shutil.which("pkg-config"):
            self.skipTest("No hay compilador C++ o pkg-config para Qt")
        flags = subprocess.run(["pkg-config", "--cflags", "--libs", "Qt6Core"], capture_output=True, text=True)
        if flags.returncode:
            self.skipTest("Qt6Core no está instalado")
        with tempfile.TemporaryDirectory(prefix="codex-var-idn-") as folder:
            source = Path(folder) / "idn.cpp"
            binary = Path(folder) / "idn"
            source.write_text('''#include "verificationurl.h"
#include <QCoreApplication>
#include <iostream>
int main(int argc, char **argv) {
  QCoreApplication app(argc, argv);
  std::cout << VerificationUrl::normalize(QString::fromUtf8(argv[1])).toUtf8().constData();
}
''')
            subprocess.run(["c++", "-std=c++17", "-fPIC", "-I" + str(QT), str(source), "-o", str(binary), *shlex.split(flags.stdout)], check=True, capture_output=True, timeout=60)
            cases = json.loads((ROOT / "testdata/verification_urls.json").read_text(encoding="utf-8"))
            for item in cases["valid"]:
                with self.subTest(input=item["input"]):
                    result = subprocess.run([str(binary), item["input"]], check=True, capture_output=True, text=True, timeout=5)
                    self.assertEqual(result.stdout, item["expected"])
            for value in cases["invalid"]:
                with self.subTest(input=value):
                    result = subprocess.run([str(binary), value], check=True, capture_output=True, text=True, timeout=5)
                    self.assertEqual(result.stdout, "")
