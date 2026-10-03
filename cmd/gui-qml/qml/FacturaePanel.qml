// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15
import QtQuick.Dialogs 6.2

Item {
    id: panel
    property var bridge
    property var localPath
    property var theme
    property var translate: function(key) { return key }
    property bool busy: false
    property string statusText: ""
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
    Connections {
        target: panel.bridge
        ignoreUnknownSignals: true
        function onFacturaeCreated(ok, result, message) {
            panel.busy = false
            panel.statusText = ok
                    ? tr("facturae.created").replace("%1", result.total)
                    : tr(message && message.indexOf("facturae.error.") === 0
                         ? message : "facturae.error.input")
        }
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
            Label { text: tr("facturae.face_note"); color: panel.theme.secondaryTextColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            RowLayout {
                Layout.fillWidth: true
                Button { text: tr("facturae.validate_face"); Accessible.name: text; onClicked: Qt.openUrlExternally("https://proveedores.face.gob.es/proveedores/validar-factura") }
                Button { text: tr("facturae.submit_face"); Accessible.name: text; onClicked: Qt.openUrlExternally("https://proveedores.face.gob.es/proveedores/remitir-factura") }
            }
            Item { Layout.preferredHeight: 24 }
        }
    }
}
