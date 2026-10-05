// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15
import QtQuick.Dialogs
import "VeriFactuResponse.js" as VeriFactuResponse

Item {
    id: panel
    property var bridge
    property var localPath
    property var theme
    property var reportSaver
    property var translate: function(key) { return key }
    property bool busy: false
    property string statusText: ""
    property var invoiceResult: null
    property var verifactuResult: null
    property string verifactuStatus: ""
    property string qrTechnical: ""
    property string invoicePath: ""
    signal signRequested()
    function tr(key) { return translate(key) }
    function party(kind, tax, name, surname, second, address, post, town, province, email) {
        return {
            personTypeCode: kind, taxIdentificationNumber: tax,
            name: name, firstSurname: surname, secondSurname: second,
            address: { address: address, postCode: post, town: town, province: province },
            electronicMail: email
        }
    }
    function draft() {
        return {
            invoiceNumber: invoiceNumber.value, invoiceSeriesCode: invoiceSeries.value,
            issueDate: issueDate.value,
            seller: party(sellerIndividual.checked ? "F" : "J", sellerTax.value,
                          sellerName.value, sellerSurname.value, sellerSecond.value,
                          sellerAddress.value, sellerPost.value, sellerTown.value,
                          sellerProvince.value, sellerEmail.value),
            buyer: party("J", buyerTax.value, buyerName.value, "", "",
                         buyerAddress.value, buyerPost.value, buyerTown.value,
                         buyerProvince.value, ""),
            accountingOfficeDir3: accounting.value,
            managingBodyDir3: managing.value,
            processingUnitDir3: processing.value,
            lines: [{description: lineDescription.value, quantity: quantity.value,
                     unitPriceWithoutTax: price.value, vatRate: vat.value}],
            installmentDueDate: includePayment.checked ? dueDate.value : "",
            iban: includePayment.checked ? iban.value : "",
            invoiceDescription: invoiceDescription.value,
            fileReference: fileReference.value,
            receiverContractReference: contractReference.value
        }
    }
    component InvoiceField: ColumnLayout {
        property string label: ""
        property alias value: input.text
        property string hint: ""
        Layout.fillWidth: true
        spacing: 4
        Label { text: label; color: panel.theme.textColor; wrapMode: Text.WordWrap }
        TextField {
            id: input
            Layout.fillWidth: true
            Accessible.name: label
            Accessible.description: hint
            placeholderText: hint
            selectByMouse: true
        }
    }
    FileDialog {
        id: saveDialog
        title: tr("facturae.save_title")
        fileMode: FileDialog.SaveFile
        nameFilters: [tr("facturae.xml_filter")]
        onAccepted: {
            const path = panel.localPath(selectedFile)
            if (path === "") return
            panel.busy = true
            panel.statusText = tr("facturae.creating")
            panel.bridge.createFacturae(panel.draft(), path)
        }
    }
    FileDialog {
        id: invoiceOpenDialog
        title: tr("paridad.lote3.invoice.choose")
        fileMode: FileDialog.OpenFile
        nameFilters: [tr("paridad.lote3.invoice.filter")]
        onAccepted: {
            panel.invoicePath = panel.localPath(selectedFile)
            if (panel.invoicePath === "") return
            panel.invoiceResult = null
            panel.statusText = tr("paridad.lote3.invoice.validating")
            panel.busy = true
            panel.bridge.validateInvoice(panel.invoicePath)
        }
    }
    FileDialog {
        id: invoiceReportDialog
        title: tr("paridad.lote3.invoice.export")
        fileMode: FileDialog.SaveFile
        nameFilters: [tr("paridad.lote3.report.filter")]
        onAccepted: {
            const path = panel.localPath(selectedFile)
            if (path !== "" && panel.invoiceResult && panel.reportSaver)
                panel.reportSaver(path, panel.invoiceResult.report)
        }
    }
    FileDialog {
        id: verifactuReportDialog
        title: tr("paridad.lote3.invoice.export")
        fileMode: FileDialog.SaveFile
        nameFilters: [tr("paridad.lote3.report.filter")]
        onAccepted: {
            const path = panel.localPath(selectedFile)
            if (path !== "" && panel.verifactuResult && panel.reportSaver)
                panel.reportSaver(path, panel.verifactuResult.report)
        }
    }
    Connections {
        target: panel.bridge
        ignoreUnknownSignals: true
        function onFacturaeCreated(ok, result, message) {
            panel.busy = false
            panel.statusText = ok
                    ? tr("facturae.created").replace("%1", result.total)
                    : (message && message.indexOf("facturae.error.") === 0
                         ? tr(message) : tr("facturae.error.input"))
        }
        function onVerifactuValidated(ok, result, message) {
            panel.busy = false
            panel.verifactuResult = ok ? result : null
            // El motor resume con avisos y errores: «sin errores» a secas solo
            // cuando no hay ninguno de los dos.
            panel.verifactuStatus = ok
                    ? (result.summary ? result.summary : tr(result.valid ? "verifactu.valid" : "verifactu.invalid"))
                    : message
        }
        function onVerifactuQRFinished(action, ok, result, message) {
            panel.busy = false
            if (action === "read_verifactu_qr") {
                panel.qrTechnical = ""
            qrArea.readResult = ok ? result : null
                qrArea.text = ok ? tr("verifactu.qr_nif") + ": " + result.nif + "\n" +
                    tr("verifactu.qr_number") + ": " + result.numserie + "\n" +
                    tr("verifactu.qr_date") + ": " + result.fecha + "\n" +
                    tr("verifactu.qr_amount") + ": " + result.importe : message
            } else {
                // Una frase para la persona; el JSON queda tras «Ver respuesta técnica».
                qrArea.text = ok ? tr(VeriFactuResponse.classify(result.response)) : message
                panel.qrTechnical = ok ? JSON.stringify(result.response, null, 2) : ""
            }
        }
        function onInvoiceValidated(ok, result, message) {
            panel.busy = false
            panel.invoiceResult = ok ? result : null
            panel.statusText = ok
                    ? (result.valid ? tr("paridad.lote3.invoice.valid") : tr("paridad.lote3.invoice.invalid"))
                    : tr("paridad.lote3.invoice.failed").replace("%1", message)
        }
    }
    FileDialog {
        id: verifactuFileDialog
        title: tr("verifactu.choose")
        fileMode: FileDialog.OpenFile
        nameFilters: [tr("paridad.lote3.invoice.filter")]
        onAccepted: { panel.verifactuResult = null; panel.verifactuStatus = tr("paridad.lote3.invoice.validating"); panel.busy = true; panel.bridge.validateVeriFactu(panel.localPath(selectedFile)) }
    }
    FileDialog {
        id: qrFileDialog
        title: tr("verifactu.qr_from_file")
        fileMode: FileDialog.OpenFile
        nameFilters: [tr("verifactu.qr_file_filter")]
        onAccepted: {
            qrInput.text = ""
            qrArea.readResult = null
            panel.qrTechnical = ""
            qrArea.text = tr("verifactu.qr_reading")
            panel.busy = true
            panel.bridge.readVeriFactuQRFile(panel.localPath(selectedFile))
        }
    }
    FolderDialog {
        id: verifactuFolderDialog
        title: tr("verifactu.folder")
        onAccepted: { panel.verifactuResult = null; panel.verifactuStatus = tr("paridad.lote3.invoice.validating"); panel.busy = true; panel.bridge.validateVeriFactu(panel.localPath(selectedFolder)) }
    }
    ScrollView {
        anchors.fill: parent
        clip: true
        contentWidth: availableWidth
        ColumnLayout {
            width: Math.max(0, parent.width - 48)
            x: 24
            spacing: 18
            Label {
                text: tr("facturae.title")
                color: panel.theme.textColor
                font.pixelSize: 28
                font.bold: true
                Layout.fillWidth: true
            }
            Label {
                text: tr("facturae.intro")
                color: panel.theme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            Label { text: tr("facturae.invoice"); color: panel.theme.textColor; font.bold: true }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: invoiceNumber; label: tr("facturae.invoice_number") }
                InvoiceField { id: invoiceSeries; label: tr("facturae.series") }
                InvoiceField { id: issueDate; label: tr("facturae.issue_date"); value: Qt.formatDate(new Date(), "yyyy-MM-dd"); hint: tr("facturae.date_hint") }
            }
            Label { text: tr("facturae.seller"); color: panel.theme.textColor; font.bold: true }
            Switch {
                id: sellerIndividual
                text: tr("facturae.individual")
                Accessible.name: text
            }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: sellerTax; label: tr("facturae.tax_id") }
                InvoiceField { id: sellerName; label: sellerIndividual.checked ? tr("facturae.person_name") : tr("facturae.legal_name") }
                InvoiceField { id: sellerSurname; label: tr("facturae.first_surname"); visible: sellerIndividual.checked }
                InvoiceField { id: sellerSecond; label: tr("facturae.second_surname"); visible: sellerIndividual.checked }
                InvoiceField { id: sellerAddress; label: tr("facturae.address") }
                InvoiceField { id: sellerPost; label: tr("facturae.post_code") }
                InvoiceField { id: sellerTown; label: tr("facturae.town") }
                InvoiceField { id: sellerProvince; label: tr("facturae.province") }
                InvoiceField { id: sellerEmail; label: tr("facturae.email") }
            }
            Label { text: tr("facturae.buyer"); color: panel.theme.textColor; font.bold: true }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: buyerTax; label: tr("facturae.tax_id") }
                InvoiceField { id: buyerName; label: tr("facturae.legal_name") }
                InvoiceField { id: buyerAddress; label: tr("facturae.address") }
                InvoiceField { id: buyerPost; label: tr("facturae.post_code") }
                InvoiceField { id: buyerTown; label: tr("facturae.town") }
                InvoiceField { id: buyerProvince; label: tr("facturae.province") }
            }
            Label { text: tr("facturae.dir3"); color: panel.theme.textColor; font.bold: true }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: accounting; label: tr("facturae.accounting") }
                InvoiceField { id: managing; label: tr("facturae.managing") }
                InvoiceField { id: processing; label: tr("facturae.processing") }
            }
            Label { text: tr("facturae.item"); color: panel.theme.textColor; font.bold: true }
            GridLayout {
                columns: panel.width > 820 ? 4 : 1
                Layout.fillWidth: true
                InvoiceField { id: lineDescription; label: tr("facturae.item_description") }
                InvoiceField { id: quantity; label: tr("facturae.quantity"); value: "1" }
                InvoiceField { id: price; label: tr("facturae.price"); value: "0" }
                InvoiceField { id: vat; label: tr("facturae.vat"); value: "21" }
            }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: invoiceDescription; label: tr("facturae.description") }
                InvoiceField { id: fileReference; label: tr("facturae.file_reference") }
                InvoiceField { id: contractReference; label: tr("facturae.contract_reference") }
            }
            Switch { id: includePayment; text: tr("facturae.payment"); Accessible.name: text }
            GridLayout {
                visible: includePayment.checked
                columns: panel.width > 820 ? 2 : 1
                Layout.fillWidth: true
                InvoiceField { id: dueDate; label: tr("facturae.due_date"); hint: tr("facturae.date_hint") }
                InvoiceField { id: iban; label: tr("facturae.iban") }
            }
            Label {
                text: panel.statusText
                visible: text !== ""
                color: panel.theme.textColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
                Accessible.role: Accessible.StaticText
                Accessible.name: text
            }
            RowLayout {
                Layout.fillWidth: true
                Button { text: tr("facturae.create"); enabled: !panel.busy; Accessible.name: text; onClicked: saveDialog.open() }
                Button { text: tr("facturae.sign"); Accessible.name: text; onClicked: panel.signRequested() }
            }
            Label { text: tr("paridad.lote3.invoice.title"); font.bold: true; color: panel.theme.textColor; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                Button { text: tr("paridad.lote3.invoice.choose"); enabled: !panel.busy; Accessible.name: text; onClicked: invoiceOpenDialog.open() }
                Button { text: tr("paridad.lote3.invoice.export"); enabled: !!panel.invoiceResult; Accessible.name: text; onClicked: invoiceReportDialog.open() }
            }
            Label {
                text: panel.invoiceResult ? tr("paridad.lote3.invoice.summary").replace("%1", panel.invoiceResult.format).replace("%2", panel.invoiceResult.errors).replace("%3", panel.invoiceResult.warnings) : tr("paridad.lote3.invoice.empty")
                color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true
                Accessible.name: text
            }
            Repeater {
                model: panel.invoiceResult && panel.invoiceResult.issues ? panel.invoiceResult.issues : []
                delegate: Label {
                    required property var modelData
                    text: (modelData.level === "error" ? tr("paridad.lote3.invoice.error") : tr("paridad.lote3.invoice.warning")) + " · " + modelData.field + ": " + modelData.message
                    color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true
                    Accessible.name: text
                }
            }
            Label { text: tr("facturae.face_note"); color: panel.theme.secondaryTextColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                Button { text: tr("facturae.validate_face"); Accessible.name: text; onClicked: Qt.openUrlExternally("https://proveedores.face.gob.es/proveedores/validar-factura") }
                Button { text: tr("facturae.submit_face"); Accessible.name: text; onClicked: Qt.openUrlExternally("https://proveedores.face.gob.es/proveedores/remitir-factura") }
            }
            Rectangle { Layout.fillWidth: true; Layout.topMargin: 12; height: 1; color: panel.theme.secondaryTextColor; opacity: 0.4 }
            Label { text: tr("verifactu.title"); font.bold: true; font.pixelSize: 20; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            Label { text: tr("verifactu.scope"); textFormat: Text.PlainText; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                Button { text: tr("verifactu.choose"); enabled: !panel.busy; Accessible.name: text; onClicked: verifactuFileDialog.open() }
                Button { text: tr("verifactu.folder"); enabled: !panel.busy; Accessible.name: text; onClicked: verifactuFolderDialog.open() }
            }
            // Resultado junto a sus botones, como región viva.
            Label {
                objectName: "verifactuSummary"
                text: panel.verifactuStatus !== "" ? panel.verifactuStatus : tr("verifactu.empty_state")
                color: panel.verifactuStatus !== "" ? panel.theme.textColor : panel.theme.secondaryTextColor
                wrapMode: Text.WordWrap; Layout.fillWidth: true
                Accessible.role: Accessible.AlertMessage; Accessible.name: text
            }
            TextArea {
                objectName: "verifactuReport"
                text: panel.verifactuResult ? panel.verifactuResult.report : ""
                visible: text.length > 0; readOnly: true; wrapMode: TextEdit.Wrap; textFormat: TextEdit.PlainText
                Accessible.name: tr("verifactu.report")
                color: panel.theme.textColor; Layout.fillWidth: true
                background: Rectangle { color: panel.theme.cardColor ? panel.theme.cardColor : "transparent"; border.color: panel.theme.secondaryTextColor; radius: 4 }
            }
            Button { text: tr("verifactu.export"); visible: !!panel.verifactuResult; Accessible.name: text; onClicked: verifactuReportDialog.open() }
            Label { text: tr("verifactu.qr_title"); color: panel.theme.textColor; font.bold: true; wrapMode: Text.WordWrap; Layout.fillWidth: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            Label { text: tr("verifactu.qr_url_field"); color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                TextField {
                    id: qrInput; enabled: !panel.busy; Layout.fillWidth: true; Accessible.name: tr("verifactu.qr_url_field"); Accessible.description: tr("verifactu.qr_url_label")
                    placeholderText: tr("verifactu.qr_url_label"); selectByMouse: true
                    onTextChanged: { qrArea.readResult = null; qrArea.text = "" }
                }
                Button { objectName: "qrFromFileButton"; text: tr("verifactu.qr_from_file"); enabled: !panel.busy; Accessible.name: text; onClicked: qrFileDialog.open() }
            }
            Label { text: tr("verifactu.qr_notice"); textFormat: Text.PlainText; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                Button { text: tr("verifactu.qr_read"); enabled: !panel.busy && qrInput.text.length > 0; Accessible.name: text; onClicked: { panel.busy = true; panel.bridge.readVeriFactuQR(qrInput.text) } }
                Button { objectName: "qrQueryButton"; text: tr("verifactu.qr_query"); enabled: !panel.busy && qrArea.readResult !== null; Accessible.name: text; onClicked: { panel.busy = true; panel.bridge.queryVeriFactuQR(qrArea.readResult.url) } }
            }
            Label { text: tr("verifactu.qr_query_help"); color: panel.theme.secondaryTextColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            // Fondo del tema: con el blanco por defecto el texto claro no se veía.
            TextArea {
                id: qrArea; objectName: "qrResultArea"; property var readResult: null; readOnly: true; wrapMode: TextEdit.Wrap; textFormat: TextEdit.PlainText; Layout.fillWidth: true
                color: panel.theme.textColor; Accessible.name: tr("verifactu.qr_result")
                onTextChanged: if (text === "") panel.qrTechnical = ""
                background: Rectangle { color: panel.theme.cardColor ? panel.theme.cardColor : "transparent"; border.color: panel.theme.secondaryTextColor; radius: 4 }
            }
            Button {
                id: qrTechnicalButton; objectName: "qrTechnicalButton"; checkable: true; visible: panel.qrTechnical !== ""
                text: checked ? tr("verifactu.qr_technical_hide") : tr("verifactu.qr_technical"); Accessible.name: text
            }
            TextArea {
                objectName: "qrTechnicalArea"; visible: qrTechnicalButton.checked && panel.qrTechnical !== ""; text: panel.qrTechnical
                readOnly: true; wrapMode: TextEdit.Wrap; textFormat: TextEdit.PlainText; Layout.fillWidth: true
                color: panel.theme.textColor; Accessible.name: tr("verifactu.qr_technical")
                background: Rectangle { color: panel.theme.cardColor ? panel.theme.cardColor : "transparent"; border.color: panel.theme.secondaryTextColor; radius: 4 }
            }
            Item { Layout.preferredHeight: 24 }
        }
    }
}
