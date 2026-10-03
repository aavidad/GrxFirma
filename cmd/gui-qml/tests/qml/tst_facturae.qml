// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtTest 1.2
import "../../qml"

TestCase {
    id: testCase
    name: "FacturaePanel"
    width: 900
    height: 700
    when: windowShown
    property var panel
    QtObject {
        id: fake
        signal facturaeCreated(bool ok, var result, string message)
        function createFacturae(draft, path) {}
    }
    Component {
        id: factory
        FacturaePanel {
            width: 850
            height: 650
            bridge: fake
            localPath: function(url) { return String(url) }
            theme: ({textColor: "#ffffff", secondaryTextColor: "#dddddd"})
            translate: function(key) { return key }
        }
    }
    function init() {
        panel = createTemporaryObject(factory, testCase)
        verify(panel !== null)
    }
    function test_draft_and_result() {
        const draft = panel.draft()
        compare(draft.lines.length, 1)
        compare(draft.lines[0].quantity, "1")
        compare(draft.lines[0].vatRate, "21")
        compare(draft.buyer.personTypeCode, "J")
        fake.facturaeCreated(true, {total: "121.01"}, "")
        compare(panel.statusText, "facturae.created")
        fake.facturaeCreated(false, {}, "facturae.error.dir3")
        compare(panel.statusText, "facturae.error.dir3")
    }
}
