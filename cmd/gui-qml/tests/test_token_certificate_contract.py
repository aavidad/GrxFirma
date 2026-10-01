# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
import pathlib
import shutil
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")


@unittest.skipUnless(shutil.which("node"), "Node required for QML JavaScript contract")
class TokenCertificateContractTest(unittest.TestCase):
    def run_logic(self, assertion):
        names = ["certificateMatchesStructuredFilters", "syncCertificateSelection",
                 "certificateCanSign", "certificateStatusText", "certificateStatusReason",
                 "buildCertificateValidationSummary"]
        functions = []
        for name in names:
            body = QML.split("    function " + name + "(", 1)[1].split("\n    function ", 1)[0]
            functions.append("function " + name + "(" + body)
        setup = r'''
const assert = require('node:assert/strict');
global.window = global;
global.tr = value => value;
global.certificateId = c => c ? c.id : '';
global.findCertificateIndexById = (id, items) => items.findIndex(c => c.id === id);
global.selectCertificateIndex = i => { global.selectedCertData = global.certificates[i]; };
global.clearCertificateSelection = () => { global.selectedCertData = null; };
global.sanitizeMultiCosignCertificates = () => {};
global.normalizeMultiCosignPrimaryCertificate = () => {};
global.formatFingerprintForDisplay = value => value;
global.certificateTypeFilter = [];
global.certificateRequireNIF = false;
global.certificateRequireOrganization = false;
global.useOnlySignatureCertificates = true;
global.selectedCertData = null;
global.multiCosignEnabled = false;
global.multiCosignPrimaryCertificateId = '';
global.autoSelectSingleCertificate = true;
global.preferDefaultCertificate = true;
global.defaultCertificateId = 'token';
global.stickySigner = true;
global.preferredCertificateId = 'token';
global.certificates = [{id:'token', needsUnlock:true, canSign:false, caducado:false}];
'''
        result = subprocess.run([shutil.which("node"), "-e", setup + "\n".join(functions) + assertion],
                                capture_output=True, text=True, timeout=10, check=False)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_locked_token_remains_visible_without_claiming_key(self):
        self.run_logic("assert.equal(certificateMatchesStructuredFilters(certificates[0]), true);"
                       "assert.equal(certificateMatchesStructuredFilters({canSign:false}), false);")

    def test_single_default_or_sticky_token_is_not_automatic_consent(self):
        self.run_logic("syncCertificateSelection(certificates); assert.equal(selectedCertData, null);")

    def test_explicit_selection_is_preserved_on_refresh(self):
        self.run_logic("selectedCertData = certificates[0]; syncCertificateSelection(certificates);"
                       "assert.equal(selectedCertData.id, 'token');")

    def test_pending_authorization_is_not_reported_as_validated(self):
        self.run_logic("const summary = buildCertificateValidationSummary(certificates[0]);"
                       "assert.equal(summary.valid, false);"
                       "assert.equal(summary.details.certificate, 'pendiente de autorización');"
                       "assert.match(summary.message, /todavía no se ha comprobado/);"
                       "assert.equal(certificateStatusText(certificates[0]), '⚠ No válido');"
                       "assert.equal(certificateStatusReason(certificates[0]), 'Requiere autorización de la tarjeta');")


if __name__ == "__main__":
    unittest.main()
