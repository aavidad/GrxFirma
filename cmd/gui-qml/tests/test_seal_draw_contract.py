# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

from pathlib import Path
import shutil
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[3]


class SealDrawContract(unittest.TestCase):
    def test_normalization_bounds_and_resize_minimum(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("Node no instalado")
        script = (ROOT / "cmd/gui-qml/qml/SealDrawGeometry.js").read_text(encoding="utf-8").replace(".pragma library", "")
        script += r'''
const assert = require('assert');
for (const p of [[.2,.3,.7,.8], [.7,.3,.2,.8], [.2,.8,.7,.3], [.7,.8,.2,.3]]) {
    const rect = normalize(...p);
    for (const [key, expected] of Object.entries({x:.2,y:.2,w:.5,h:.5}))
        assert.ok(Math.abs(rect[key] - expected) < 1e-12);
}
assert.deepStrictEqual(normalize(-2,5,4,-3), {x:0,y:0,w:1,h:1});
assert.deepStrictEqual(normalize(2,-2,3,-1), {x:1,y:1,w:0,h:0});
assert.ok(isLargeEnough(normalize(0,0,.08,.05), 500, 500));
assert.ok(!isLargeEnough(normalize(0,0,.079,.1), 500, 500));
assert.ok(!isLargeEnough(normalize(0,0,.1,.049), 500, 500));
for (const value of [NaN, Infinity, -Infinity])
    for (let index=0; index<4; index++) {
        const p=[0,0,1,1]; p[index]=value;
        assert.throws(() => normalize(...p), RangeError);
    }
for (let i=0; i<300; i++) {
    const rect=normalize(Math.sin(i)*2,Math.cos(i)*2,Math.sin(i+1)*2,Math.cos(i+1)*2);
    assert.ok(rect.x >= 0 && rect.y >= 0 && rect.w >= 0 && rect.h >= 0);
    assert.ok(rect.x+rect.w <= 1+1e-12 && rect.y+rect.h <= 1+1e-12);
}
'''
        subprocess.run([node, "-e", script], cwd=ROOT, check=True, timeout=15)

    def test_portal_and_normal_editor_share_draw_component_and_page_model(self):
        source = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
        self.assertEqual(1, source.count("SealDrawArea {"))
        for expected in ("portalSealEditor.parent = portalSealCanvas", "id: sealDrawButton",
                         "sealDrawArea.cancel(); loadPageSeal()", "window.addSealToPage()",
                         "window.savePageSeal()", "window.signSealY = rect.y",
                         'tr("sign.seal.draw_help")'):
            self.assertIn(expected, source)
        draw = (ROOT / "cmd/gui-qml/qml/SealDrawArea.qml").read_text(encoding="utf-8")
        self.assertNotIn("signSealRotation =", source[source.index("onCommitted: (rect)"):source.index("onFeedback: (key)")])
        for expected in ("Qt.Key_Escape", "Qt.ShiftModifier", "Qt.Key_Return",
                         "acceptedButtons: Qt.LeftButton", "preventStealing: true", "onCanceled: drawArea.cancel()"):
            self.assertIn(expected, draw)


if __name__ == "__main__":
    unittest.main()
