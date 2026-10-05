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
    // Idioma de la aplicación: formato del importe y limpieza de mensajes al cambiarlo.
    property string localeName: ""
    // Los mensajes propios se guardan como clave y se traducen al pintarse; los que
    // llegan ya traducidos del motor se retiran si cambia el idioma.
    property string statusKey: ""
    property string statusArg: ""
    property bool statusArgFromBackend: false
    readonly property string statusText: statusKey !== "" ? tr(statusKey).replace("%1", statusArg) : ""
    property var invoiceResult: null
    property var verifactuResult: null
    property string verifactuStatusKey: ""
    property string verifactuError: ""
    readonly property string verifactuStatus: verifactuStatusKey !== "" ? tr(verifactuStatusKey) : verifactuError
    // Estado del cuadro del QR: {kind: "reading" | "read" | "answer" | "error", ...}.
    property var qrState: null
    property string qrTechnical: ""
    property string invoicePath: ""
    signal signRequested()
    function tr(key) { return translate(key) }
    function setStatus(key, arg, fromBackend) {
        statusKey = key
        statusArg = arg || ""
        statusArgFromBackend = !!fromBackend
    }
    onLocaleNameChanged: {
        if (statusArgFromBackend) setStatus("", "", false)
        verifactuError = ""
        if (qrState && qrState.kind === "error") qrState = null
    }
    // Importe con el formato del idioma: «241,40 €» en español.
    function amountText(value, withSymbol) {
        const number = Number(value)
        if (String(value).trim() === "" || !isFinite(number)) return String(value)
        const locale = localeName !== "" ? Qt.locale(localeName) : Qt.locale()
        return withSymbol ? number.toLocaleCurrencyString(locale, "€") : number.toLocaleString(locale, "f", 2)
    }
    function qrText(state) {
        if (!state) return ""
        if (state.kind === "reading") return tr("verifactu.qr_reading")
        if (state.kind === "answer") return tr(state.key)
        if (state.kind === "read")
            return tr("verifactu.qr_nif") + ": " + state.result.nif + "\n" +
                   tr("verifactu.qr_number") + ": " + state.result.numserie + "\n" +
                   tr("verifactu.qr_date") + ": " + state.result.fecha + "\n" +
                   tr("verifactu.qr_amount") + ": " + amountText(state.result.importe, true)
        return state.message || ""
    }
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
        id: invoiceField
        property string label: ""
        property alias value: input.text
        property string hint: ""
        // Los obligatorios llevan «*» (leyenda al principio) y lo dicen al lector de pantalla.
        property bool required: false
        Layout.fillWidth: true
        spacing: 4
        Label { text: invoiceField.required ? invoiceField.label + " *" : invoiceField.label; color: panel.theme.textColor; wrapMode: Text.WordWrap }
        ThemedTextField {
            id: input
            Layout.fillWidth: true
            Accessible.name: invoiceField.required ? invoiceField.label + ", " + panel.tr("facturae.required") : invoiceField.label
            Accessible.description: invoiceField.hint
            placeholderText: hint
            selectByMouse: true
        }
    }
    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? panel.tr("Guardar") : panel.tr("Abrir")
        rejectLabel: panel.tr("Cancelar")
        id: saveDialog
        title: tr("facturae.save_title")
        fileMode: FileDialog.SaveFile
        nameFilters: [tr("facturae.xml_filter")]
        onAccepted: {
            const path = panel.localPath(selectedFile)
            if (path === "") return
            panel.busy = true
            panel.setStatus("facturae.creating")
            // El diálogo de guardar ya pidió confirmación si el fichero existía.
            panel.bridge.createFacturae(panel.draft(), path, true)
        }
    }
    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? panel.tr("Guardar") : panel.tr("Abrir")
        rejectLabel: panel.tr("Cancelar")
        id: invoiceOpenDialog
        title: tr("paridad.lote3.invoice.choose")
        fileMode: FileDialog.OpenFile
        nameFilters: [tr("paridad.lote3.invoice.filter")]
        onAccepted: {
            panel.invoicePath = panel.localPath(selectedFile)
            if (panel.invoicePath === "") return
            panel.invoiceResult = null
            panel.setStatus("paridad.lote3.invoice.validating")
            panel.busy = true
            panel.bridge.validateInvoice(panel.invoicePath)
        }
    }
    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? panel.tr("Guardar") : panel.tr("Abrir")
        rejectLabel: panel.tr("Cancelar")
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
        acceptLabel: fileMode === FileDialog.SaveFile ? panel.tr("Guardar") : panel.tr("Abrir")
        rejectLabel: panel.tr("Cancelar")
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
            if (ok)
                panel.setStatus("facturae.created", panel.amountText(result.total, false))
            else
                panel.setStatus(!(message && message.indexOf("facturae.error.") === 0) ? "facturae.error.input" : message)
        }
        function onVerifactuValidated(ok, result, message) {
            panel.busy = false
            panel.verifactuResult = ok ? result : null
            // El motor resume con avisos y errores (ya traducido): «sin errores»
            // a secas solo cuando no hay ninguno. Sin resumen, clave traducible.
            panel.verifactuStatusKey = ok && !result.summary ? (result.valid ? "verifactu.valid" : "verifactu.invalid") : ""
            panel.verifactuError = ok ? (result.summary ? result.summary : "") : message
        }
        function onVerifactuQRFinished(action, ok, result, message) {
            panel.busy = false
            if (action === "read_verifactu_qr") {
                panel.qrTechnical = ""
                qrArea.readResult = ok ? result : null
                panel.qrState = ok ? {kind: "read", result: result} : {kind: "error", message: message}
            } else {
                // Una frase para la persona; el JSON queda tras «Ver respuesta técnica».
                panel.qrState = ok ? {kind: "answer", key: VeriFactuResponse.classify(result.response)} : {kind: "error", message: message}
                panel.qrTechnical = ok ? JSON.stringify(result.response, null, 2) : ""
            }
        }
        function onInvoiceValidated(ok, result, message) {
            panel.busy = false
            panel.invoiceResult = ok ? result : null
            if (ok)
                panel.setStatus(result.valid ? "paridad.lote3.invoice.valid" : "paridad.lote3.invoice.invalid")
            else
                panel.setStatus("paridad.lote3.invoice.failed", message, true)
        }
    }
    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? panel.tr("Guardar") : panel.tr("Abrir")
        rejectLabel: panel.tr("Cancelar")
        id: verifactuFileDialog
        title: tr("verifactu.choose")
        fileMode: FileDialog.OpenFile
        nameFilters: [tr("paridad.lote3.invoice.filter")]
        onAccepted: { panel.verifactuResult = null; panel.verifactuError = ""; panel.verifactuStatusKey = "paridad.lote3.invoice.validating"; panel.busy = true; panel.bridge.validateVeriFactu(panel.localPath(selectedFile)) }
    }
    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? panel.tr("Guardar") : panel.tr("Abrir")
        rejectLabel: panel.tr("Cancelar")
        id: qrFileDialog
        title: tr("verifactu.qr_from_file")
        fileMode: FileDialog.OpenFile
        nameFilters: [tr("verifactu.qr_file_filter")]
        onAccepted: {
            qrInput.text = ""
            qrArea.readResult = null
            panel.qrTechnical = ""
            panel.qrState = {kind: "reading"}
            panel.busy = true
            panel.bridge.readVeriFactuQRFile(panel.localPath(selectedFile))
        }
    }
    FolderDialog {
        acceptLabel: panel.tr("Seleccionar carpeta")
        rejectLabel: panel.tr("Cancelar")
        id: verifactuFolderDialog
        title: tr("verifactu.folder")
        onAccepted: { panel.verifactuResult = null; panel.verifactuError = ""; panel.verifactuStatusKey = "paridad.lote3.invoice.validating"; panel.busy = true; panel.bridge.validateVeriFactu(panel.localPath(selectedFolder)) }
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
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }
            Label {
                text: tr("facturae.intro")
                color: panel.theme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            Label { text: tr("facturae.required_legend"); color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            Label { text: tr("facturae.invoice"); color: panel.theme.textColor; font.bold: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: invoiceNumber; required: true; label: tr("facturae.invoice_number") }
                InvoiceField { id: invoiceSeries; label: tr("facturae.series") }
                InvoiceField { id: issueDate; required: true; label: tr("facturae.issue_date"); value: Qt.formatDate(new Date(), "yyyy-MM-dd"); hint: tr("facturae.date_hint") }
            }
            Label { text: tr("facturae.seller"); color: panel.theme.textColor; font.bold: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            ThemedSwitch {
                id: sellerIndividual
                text: tr("facturae.individual")
                Accessible.name: text
            }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: sellerTax; required: true; label: tr("facturae.tax_id") }
                InvoiceField { id: sellerName; required: true; label: sellerIndividual.checked ? tr("facturae.person_name") : tr("facturae.legal_name") }
                InvoiceField { id: sellerSurname; required: true; label: tr("facturae.first_surname"); visible: sellerIndividual.checked }
                InvoiceField { id: sellerSecond; label: tr("facturae.second_surname"); visible: sellerIndividual.checked }
                InvoiceField { id: sellerAddress; required: true; label: tr("facturae.address") }
                InvoiceField { id: sellerPost; required: true; label: tr("facturae.post_code") }
                InvoiceField { id: sellerTown; required: true; label: tr("facturae.town") }
                InvoiceField { id: sellerProvince; required: true; label: tr("facturae.province") }
                InvoiceField { id: sellerEmail; label: tr("facturae.email") }
            }
            Label { text: tr("facturae.buyer"); color: panel.theme.textColor; font.bold: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: buyerTax; required: true; label: tr("facturae.tax_id") }
                InvoiceField { id: buyerName; required: true; label: tr("facturae.legal_name") }
                InvoiceField { id: buyerAddress; required: true; label: tr("facturae.address") }
                InvoiceField { id: buyerPost; required: true; label: tr("facturae.post_code") }
                InvoiceField { id: buyerTown; required: true; label: tr("facturae.town") }
                InvoiceField { id: buyerProvince; required: true; label: tr("facturae.province") }
            }
            Label { text: tr("facturae.dir3"); color: panel.theme.textColor; font.bold: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: accounting; required: true; label: tr("facturae.accounting") }
                InvoiceField { id: managing; required: true; label: tr("facturae.managing") }
                InvoiceField { id: processing; required: true; label: tr("facturae.processing") }
            }
            Label { text: tr("facturae.item"); color: panel.theme.textColor; font.bold: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            GridLayout {
                columns: panel.width > 820 ? 4 : 1
                Layout.fillWidth: true
                InvoiceField { id: lineDescription; required: true; label: tr("facturae.item_description") }
                InvoiceField { id: quantity; required: true; label: tr("facturae.quantity"); value: "1" }
                InvoiceField { id: price; required: true; label: tr("facturae.price"); value: "0" }
                InvoiceField { id: vat; required: true; label: tr("facturae.vat"); value: "21" }
            }
            GridLayout {
                columns: panel.width > 820 ? 3 : 1
                Layout.fillWidth: true
                InvoiceField { id: invoiceDescription; label: tr("facturae.description") }
                InvoiceField { id: fileReference; label: tr("facturae.file_reference") }
                InvoiceField { id: contractReference; label: tr("facturae.contract_reference") }
            }
            ThemedSwitch { id: includePayment; text: tr("facturae.payment"); Accessible.name: text }
            GridLayout {
                visible: includePayment.checked
                columns: panel.width > 820 ? 2 : 1
                Layout.fillWidth: true
                InvoiceField { id: dueDate; required: true; label: tr("facturae.due_date"); hint: tr("facturae.date_hint") }
                InvoiceField { id: iban; required: true; label: tr("facturae.iban") }
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
                ThemedButton { text: tr("facturae.create"); enabled: !panel.busy; Accessible.name: text; onClicked: saveDialog.open() }
                ThemedButton { text: tr("facturae.sign"); Accessible.name: text; onClicked: panel.signRequested() }
            }
            Label { text: tr("paridad.lote3.invoice.title"); font.bold: true; color: panel.theme.textColor; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                ThemedButton { text: tr("paridad.lote3.invoice.choose"); enabled: !panel.busy; Accessible.name: text; onClicked: invoiceOpenDialog.open() }
                ThemedButton { text: tr("paridad.lote3.invoice.export"); enabled: !!panel.invoiceResult; Accessible.name: text; onClicked: invoiceReportDialog.open() }
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
                ThemedButton { text: tr("facturae.validate_face"); Accessible.name: text; onClicked: Qt.openUrlExternally("https://proveedores.face.gob.es/proveedores/validar-factura") }
                ThemedButton { text: tr("facturae.submit_face"); Accessible.name: text; onClicked: Qt.openUrlExternally("https://proveedores.face.gob.es/proveedores/remitir-factura") }
            }
            Rectangle { Layout.fillWidth: true; Layout.topMargin: 12; height: 1; color: panel.theme.secondaryTextColor; opacity: 0.4 }
            Label { text: tr("verifactu.title"); font.bold: true; font.pixelSize: 20; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            Label { text: tr("verifactu.scope"); textFormat: Text.PlainText; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                ThemedButton { text: tr("verifactu.choose"); enabled: !panel.busy; Accessible.name: text; onClicked: verifactuFileDialog.open() }
                ThemedButton { text: tr("verifactu.folder"); enabled: !panel.busy; Accessible.name: text; onClicked: verifactuFolderDialog.open() }
            }
            // Resultado junto a sus botones, como región viva.
            Label {
                objectName: "verifactuSummary"
                text: panel.verifactuStatus !== "" ? panel.verifactuStatus : tr("verifactu.empty_state")
                color: panel.verifactuStatus !== "" ? panel.theme.textColor : panel.theme.secondaryTextColor
                wrapMode: Text.WordWrap; Layout.fillWidth: true
                Accessible.role: Accessible.AlertMessage; Accessible.name: text
            }
            ThemedTextArea {
                objectName: "verifactuReport"
                text: panel.verifactuResult ? panel.verifactuResult.report : ""
                visible: text.length > 0; readOnly: true; wrapMode: TextEdit.Wrap; textFormat: TextEdit.PlainText
                Accessible.name: tr("verifactu.report")
                color: panel.theme.textColor; Layout.fillWidth: true
            }
            ThemedButton { text: tr("verifactu.export"); visible: !!panel.verifactuResult; Accessible.name: text; onClicked: verifactuReportDialog.open() }
            Label { text: tr("verifactu.qr_title"); color: panel.theme.textColor; font.bold: true; wrapMode: Text.WordWrap; Layout.fillWidth: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            Label { text: tr("verifactu.qr_url_field"); color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                ThemedTextField {
                    id: qrInput; enabled: !panel.busy; Layout.fillWidth: true; Accessible.name: tr("verifactu.qr_url_field"); Accessible.description: tr("verifactu.qr_url_label")
                    placeholderText: tr("verifactu.qr_url_label"); selectByMouse: true
                    onTextChanged: { qrArea.readResult = null; panel.qrState = null }
                }
                ThemedButton { objectName: "qrFromFileButton"; text: tr("verifactu.qr_from_file"); enabled: !panel.busy; Accessible.name: text; onClicked: qrFileDialog.open() }
            }
            Label { text: tr("verifactu.qr_notice"); textFormat: Text.PlainText; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                ThemedButton { text: tr("verifactu.qr_read"); enabled: !panel.busy && qrInput.text.length > 0; Accessible.name: text; onClicked: { panel.busy = true; panel.bridge.readVeriFactuQR(qrInput.text) } }
                ThemedButton { objectName: "qrQueryButton"; text: tr("verifactu.qr_query"); enabled: !panel.busy && qrArea.readResult !== null; Accessible.name: text; onClicked: { panel.busy = true; panel.bridge.queryVeriFactuQR(qrArea.readResult.url) } }
            }
            Label { text: tr("verifactu.qr_query_help"); color: panel.theme.secondaryTextColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            // Fondo del tema: con el blanco por defecto el texto claro no se veía.
            ThemedTextArea {
                id: qrArea; objectName: "qrResultArea"; property var readResult: null; text: panel.qrText(panel.qrState); readOnly: true; wrapMode: TextEdit.Wrap; textFormat: TextEdit.PlainText; Layout.fillWidth: true
                color: panel.theme.textColor; Accessible.name: tr("verifactu.qr_result")
                onTextChanged: if (text === "") panel.qrTechnical = ""
            }
            ThemedButton {
                id: qrTechnicalButton; objectName: "qrTechnicalButton"; checkable: true; visible: panel.qrTechnical !== ""
                text: checked ? tr("verifactu.qr_technical_hide") : tr("verifactu.qr_technical"); Accessible.name: text
            }
            ThemedTextArea {
                objectName: "qrTechnicalArea"; visible: qrTechnicalButton.checked && panel.qrTechnical !== ""; text: panel.qrTechnical
                readOnly: true; wrapMode: TextEdit.Wrap; textFormat: TextEdit.PlainText; Layout.fillWidth: true
                color: panel.theme.textColor; Accessible.name: tr("verifactu.qr_technical")
            }
            Item { Layout.preferredHeight: 24 }
        }
    }
}
