# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import shutil
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]

class EniValidationTests(unittest.TestCase):
    def test_js_validation_accepts_only_bounded_nti_metadata_and_dates(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        js = (ROOT / "cmd/gui-qml/qml/EniValidation.js").read_text(encoding="utf-8").replace(".pragma library", "")
        js += r'''
const assert = require('assert');
assert.strictEqual(organError('L01180877, A00000000'), '');
assert.notStrictEqual(organError('LLLLLLLLL'), '');
assert.notStrictEqual(organError('L01180877\n'), '');
assert.strictEqual(identifierError('ES_L01180877_2026_' + 'x'.repeat(30), false), '');
assert.notStrictEqual(identifierError('ES_L01180877_2026_' + 'x'.repeat(31), false), '');
assert.notStrictEqual(identifierError('', true), '');
assert.strictEqual(identifierError('', false), '');
assert.strictEqual(classificationError('L01180877_PRO_LICENCIAS'), '');
assert.strictEqual(classificationError('123456'), '');
assert.notStrictEqual(classificationError('<xml>'), '');
assert.notStrictEqual(interestedError('x\0'), '');
assert.notStrictEqual(formatError('<PDF>'), '');
for (const text of ['2026-10-04T12:00:00+02:00', '2024-02-29T13:42:00Z']) assert.strictEqual(dateError(text), '');
for (const text of ['2026-02-29T12:00:00Z', '2026-10-04', '2026-10-04T25:00:00Z', '2026-10-04T12:00:00+15:00', '2026-10-04T12:00:00+02:60', '0000-01-01T12:00:00Z']) assert.notStrictEqual(dateError(text), '');
const date = new Date(2026, 9, 4, 13, 42, 0);
assert.strictEqual(dateError(rfc3339(date)), '');
assert.strictEqual(new Date(rfc3339(date)).getTime(), date.getTime());
assert.strictEqual(displayDate(date), '04/10/2026');
assert.strictEqual(displayTime(date), '13:42');
assert.strictEqual(parseLocal('04/10/2026', '13:42').getTime(), date.getTime());
assert.strictEqual(parseLocal(' 4/10/2026 ', '13:42').getTime(), date.getTime());
for (const [d, t] of [['31/02/2026', '10:00'], ['04/13/2026', '10:00'], ['04/10/26', '10:00'], ['04/10/2026', '24:00'], ['04/10/2026', '10:60'], ['2026-10-04', '10:00'], ['04/10/2026', '']]) assert.strictEqual(parseLocal(d, t), null);
assert.strictEqual(dateError(rfc3339(parseLocal('29/02/2024', '00:00'))), '');
'''
        subprocess.run([node, "-e", js], check=True, cwd=ROOT)

if __name__ == "__main__":
    unittest.main()
