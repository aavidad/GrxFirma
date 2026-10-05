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
        signal verifactuValidated(bool ok, var result, string message)
        signal verifactuQRFinished(string action, bool ok, var result, string message)
        property int qrQueries: 0
        property var fileReads: []
        function readVeriFactuQR(url) {}
        function readVeriFactuQRFile(path) { fileReads.push(path) }
        function queryVeriFactuQR(url) { qrQueries++ }
        function validateVeriFactu(path) {}
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
    function test_verifactu_report_and_local_qr_do_not_query() {
        fake.qrQueries = 0
        fake.verifactuValidated(true, {format: "VeriFactu", valid: false, errors: 1, warnings: 0, records: [], report: "XML: error"}, "")
        compare(panel.verifactuResult.report, "XML: error")
        compare(panel.verifactuStatus, "verifactu.invalid")
        // El resultado aparece junto a los botones de Veri*Factu, no en la validación local.
        compare(findChild(panel, "verifactuSummary").text, "verifactu.invalid")
        compare(findChild(panel, "verifactuReport").text, "XML: error")
        compare(panel.invoiceResult, null)
        fake.verifactuQRFinished("read_verifactu_qr", true, {url: "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z&numserie=A&fecha=01-01-2025&importe=1", nif: "12345678Z", numserie: "A", fecha: "01-01-2025", importe: "1"}, "")
        compare(fake.qrQueries, 0)
        compare(panel.busy, false)
    }
    function test_qr_from_file_shows_data_and_needs_explicit_query() {
        fake.qrQueries = 0
        const button = findChild(panel, "qrFromFileButton")
        verify(button !== null)
        compare(button.text, "verifactu.qr_from_file")
        fake.verifactuQRFinished("read_verifactu_qr", true, {url: "https://prewww2.aeat.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=12345678-G33&fecha=01-09-2024&importe=241.4", nif: "89890001K", numserie: "12345678-G33", fecha: "01-09-2024", importe: "241.4"}, "")
        const area = findChild(panel, "qrResultArea")
        verify(area.text.indexOf("12345678-G33") >= 0)
        compare(fake.qrQueries, 0)
        const query = findChild(panel, "qrQueryButton")
        verify(query.enabled)
        query.clicked()
        compare(fake.qrQueries, 1)
    }
    function test_aeat_answer_is_a_sentence_with_technical_detail() {
        fake.verifactuQRFinished("read_verifactu_qr", true, {url: "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=12345678Z&numserie=A&fecha=01-01-2025&importe=1", nif: "12345678Z", numserie: "A", fecha: "01-01-2025", importe: "1"}, "")
        const area = findChild(panel, "qrResultArea")
        const technical = findChild(panel, "qrTechnicalButton")
        compare(panel.qrTechnical, "")
        fake.verifactuQRFinished("query_verifactu_qr", true, {response: {resultado: "Factura no encontrada"}}, "")
        compare(area.text, "verifactu.qr_aeat_not_found")
        verify(panel.qrTechnical.length > 0)
        verify(!technical.checked)
        technical.toggle()
        verify(technical.checked)
        verify(findChild(panel, "qrTechnicalArea").text.indexOf("no encontrada") >= 0)
        fake.verifactuQRFinished("query_verifactu_qr", true, {response: {resultado: "ok"}}, "")
        compare(area.text, "verifactu.qr_aeat_found")
        fake.verifactuQRFinished("query_verifactu_qr", true, {response: {otro: 1}}, "")
        compare(area.text, "verifactu.qr_aeat_unknown")
    }
    function test_empty_state_before_checking_records() {
        compare(findChild(panel, "verifactuSummary").text, "verifactu.empty_state")
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
