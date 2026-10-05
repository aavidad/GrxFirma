// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Dialogs
import "EniCatalog.js" as Catalog
import "EniValidation.js" as Validation

Item {
    id: panel
    property var bridge
    property var localPath
    property var theme
    property var translate: function(key) { return key }
    property var certificates: []
    property var certificateId: function(cert) { return cert ? cert.id : "" }
    property bool busy: false
    property string signaturePath: ""
    property string originalPath: ""
    property string directoryPath: ""
    property string statusText: ""
    property string documentMessage: ""
    property string fileMessage: ""
    readonly property color focusColor: theme && theme.focusColor ? theme.focusColor : "#1f5fa8"
    function tr(key) { return translate(key) }
    // Primero el nombre y después el código NTI: «Otros (TD99)».
    function codeItems(codes) {
        return codes.map(function(code) { return {code: code, label: panel.tr("eni.codigo." + code) + " (" + code + ")"} })
    }
    // Explica junto al botón qué falta y lleva el foco a la acción que lo resuelve.
    function missingDocumentInput() {
        if (panel.signaturePath !== "") { panel.documentMessage = ""; return false }
        panel.documentMessage = tr("paridad.lote3.eni.required_signature")
        signatureButton.forceActiveFocus()
        return true
    }
    function missingFileInput() {
        if (panel.directoryPath === "") {
            panel.fileMessage = tr("paridad.lote3.eni.required_folder")
            folderButton.forceActiveFocus()
            return true
        }
        if (certificate.currentIndex < 0 || panel.certificates.length === 0) {
            panel.fileMessage = tr("paridad.lote3.eni.required_certificate")
            certificate.forceActiveFocus()
            return true
        }
        panel.fileMessage = ""
        return false
    }
    function validateFields(fields) {
        let first = null
        fields.forEach(function(field) { if (!field.validateNow() && first === null) first = field })
        if (first !== null) {
            first.focusInput()
            const flick = scroller.contentItem
            const point = first.mapToItem(flick.contentItem, 0, 0)
            flick.contentY = Math.max(0, Math.min(point.y - 12, flick.contentHeight - flick.height))
            return false
        }
        return true
    }
    function documentOptions() {
        return {"eni.organo": docOrgan.text.trim(), "eni.origen": origin.currentIndex === 0 ? "administracion" : "ciudadano",
                "eni.fechaCaptura": capture.text.trim(), "eni.estado": state.currentValue,
                "eni.tipoDocumental": docType.currentValue, "eni.identificador": docId.text.trim(),
                "eni.documentoOrigen": sourceId.text.trim(), "eni.formato": format.text.trim()}
    }
    function fileOptions() {
        return {"exp.organo": fileOrgan.text.trim(), "exp.fechaApertura": opened.text.trim(),
                "exp.clasificacion": classification.text.trim(), "exp.estado": fileState.currentValue,
                "exp.identificador": fileId.text.trim(), "exp.interesado": interested.text.trim()}
    }
    FileDialog { id: signaturePicker; title: tr("paridad.lote3.eni.signature"); fileMode: FileDialog.OpenFile
        onAccepted: { panel.signaturePath = panel.localPath(selectedFile); if (panel.signaturePath !== "") panel.documentMessage = "" } }
    FileDialog { id: originalPicker; title: tr("paridad.lote3.eni.original"); fileMode: FileDialog.OpenFile
        onAccepted: panel.originalPath = panel.localPath(selectedFile) }
    FolderDialog { id: folderPicker; title: tr("paridad.lote3.eni.folder")
        onAccepted: { panel.directoryPath = panel.localPath(selectedFolder); if (panel.directoryPath !== "") panel.fileMessage = "" } }
    FileDialog { id: documentSave; title: tr("paridad.lote3.eni.save_document"); fileMode: FileDialog.SaveFile
        nameFilters: [tr("paridad.lote3.eni.xml_filter")]
        onAccepted: {
            const output = panel.localPath(selectedFile)
            if (output === "" || panel.missingDocumentInput() || !panel.validateFields([docOrgan, capture, docId, sourceId, format])) return
            panel.busy = true
            panel.statusText = tr("paridad.lote3.eni.creating")
            panel.bridge.generateENIDocument({inputPath: panel.signaturePath, originalPath: panel.originalPath,
                                              outputPath: output, options: panel.documentOptions()})
        }
    }
    FileDialog { id: fileSave; title: tr("paridad.lote3.eni.save_file"); fileMode: FileDialog.SaveFile
        nameFilters: [tr("paridad.lote3.eni.xml_filter")]
        onAccepted: {
            const output = panel.localPath(selectedFile)
            if (output === "" || panel.missingFileInput() || !panel.validateFields([fileOrgan, opened, classification, fileId, interested])) return
            const cert = panel.certificates[certificate.currentIndex]
            panel.busy = true
            panel.statusText = tr("paridad.lote3.eni.creating")
            panel.bridge.generateENIFile({directoryPath: panel.directoryPath, outputPath: output,
                                          certificateId: panel.certificateId(cert), options: panel.fileOptions()})
        }
    }
    Connections {
        target: panel.bridge
        ignoreUnknownSignals: true
        function onEniGenerated(action, ok, result, message) {
            panel.busy = false
            panel.statusText = ok ? tr("paridad.lote3.eni.created").replace("%1", result.outputPath)
                                  : tr("paridad.lote3.eni.failed").replace("%1", message)
        }
    }
    component SectionTitle: Label {
        color: panel.theme.textColor
        font.bold: true
        font.pixelSize: 18
        wrapMode: Text.WordWrap
        Layout.fillWidth: true
        Accessible.role: Accessible.Heading
        Accessible.name: text
    }
    component FieldLabel: Label {
        color: panel.theme.textColor
        wrapMode: Text.WordWrap
        Layout.fillWidth: true
    }
    // Ruta elegida o, si no hay, un estado vacío que lo dice.
    component PathLabel: Label {
        property string path: ""
        property string emptyKey: "paridad.lote3.eni.no_file"
        text: path !== "" ? path : panel.tr(emptyKey)
        color: path !== "" ? panel.theme.textColor : panel.theme.secondaryTextColor
        wrapMode: Text.WrapAnywhere
        Layout.fillWidth: true
        Accessible.name: text
    }
    component MessageLabel: Label {
        visible: text !== ""
        color: panel.theme.textColor
        wrapMode: Text.WordWrap
        Layout.fillWidth: true
        Accessible.role: Accessible.AlertMessage
        Accessible.name: text
    }
    // Botones y combos con los colores del tema y un anillo de foco visible.
    component EniButton: Button {
        id: eniButton
        Layout.fillWidth: true
        Layout.maximumWidth: 520
        Accessible.name: text
        palette.button: panel.theme.cardColor
        palette.buttonText: panel.theme.textColor
        contentItem: Label {
            text: eniButton.text
            color: panel.theme.textColor
            opacity: eniButton.enabled ? 1 : 0.6
            wrapMode: Text.WordWrap
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
        }
        background: Rectangle {
            implicitHeight: 40
            radius: 4
            color: eniButton.down ? Qt.darker(panel.theme.cardColor, 1.2) : panel.theme.cardColor
            border.color: eniButton.activeFocus ? panel.focusColor : panel.theme.secondaryTextColor
            border.width: eniButton.activeFocus ? 2 : 1
        }
    }
    component EniCombo: ComboBox {
        id: eniCombo
        Layout.fillWidth: true
        palette.button: panel.theme.cardColor
        palette.buttonText: panel.theme.textColor
        palette.window: panel.theme.cardColor
        palette.windowText: panel.theme.textColor
        palette.base: panel.theme.cardColor
        palette.text: panel.theme.textColor
        palette.highlight: panel.focusColor
        palette.highlightedText: panel.theme.cardColor
        background: Rectangle {
            implicitHeight: 40
            radius: 4
            color: panel.theme.cardColor
            border.color: eniCombo.activeFocus ? panel.focusColor : panel.theme.secondaryTextColor
            border.width: eniCombo.activeFocus ? 2 : 1
        }
    }
    ScrollView { id: scroller; anchors.fill: parent; clip: true; contentWidth: availableWidth
        ColumnLayout { width: Math.max(0, scroller.availableWidth - 48); x: 24; spacing: 12
            Label { text: tr("paridad.lote3.eni.title"); color: panel.theme.textColor; font.pixelSize: 28; font.bold: true; wrapMode: Text.WordWrap; Layout.fillWidth: true; Accessible.role: Accessible.Heading; Accessible.name: text }
            SectionTitle { text: tr("paridad.lote3.eni.document") }
            EniButton { id: signatureButton; objectName: "signatureButton"; text: tr("paridad.lote3.eni.signature"); onClicked: signaturePicker.open() }
            PathLabel { path: panel.signaturePath }
            Flow {
                Layout.fillWidth: true
                spacing: 8
                EniButton { text: tr("paridad.lote3.eni.original"); onClicked: originalPicker.open() }
                EniButton { text: tr("paridad.lote3.eni.clear_original"); enabled: panel.originalPath !== ""; onClicked: panel.originalPath = "" }
            }
            PathLabel { path: panel.originalPath }
            FieldLabel { text: tr("paridad.lote3.eni.organ_document") }
            EniValidatedField { id: docOrgan; objectName: "docOrgan"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.organ_document"; hintKey: "paridad.lote3.eni.organ_hint"; maximumLength: 128; validation: Validation.organError }
            FieldLabel { text: tr("paridad.lote3.eni.origin") }
            EniCombo { id: origin; model: [tr("paridad.lote3.eni.administration"), tr("paridad.lote3.eni.citizen")]; Accessible.name: tr("paridad.lote3.eni.origin") }
            FieldLabel { text: tr("paridad.lote3.eni.capture_date") }
            EniDateField { id: capture; objectName: "capture"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.capture_date"; timeLabelKey: "paridad.lote3.eni.capture_time" }
            FieldLabel { text: tr("paridad.lote3.eni.state") }
            EniCombo { id: state; objectName: "state"; model: panel.codeItems(Catalog.EstadosElaboracion); textRole: "label"; valueRole: "code"; currentIndex: 0; Accessible.name: tr("paridad.lote3.eni.state"); onCurrentValueChanged: if (sourceId) sourceId.touched = true }
            FieldLabel { text: tr("paridad.lote3.eni.doc_type") }
            EniCombo { id: docType; objectName: "docType"; model: panel.codeItems(Catalog.TiposDocumentales); textRole: "label"; valueRole: "code"; currentIndex: 20; Accessible.name: tr("paridad.lote3.eni.doc_type") }
            FieldLabel { text: tr("paridad.lote3.eni.identifier_optional") }
            EniValidatedField { id: docId; objectName: "docId"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.identifier_optional"; maximumLength: 48; validation: function(text) { return Validation.identifierError(text, false) } }
            FieldLabel { text: tr("paridad.lote3.eni.source_optional") }
            EniValidatedField { id: sourceId; objectName: "sourceId"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.source_optional"; maximumLength: 48; validation: function(text) { return Validation.identifierError(text, ["EE02", "EE03", "EE04"].indexOf(state.currentValue) >= 0) } }
            FieldLabel { text: tr("paridad.lote3.eni.format_optional") }
            EniValidatedField { id: format; objectName: "format"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.format_optional"; maximumLength: 32; validation: Validation.formatError }
            EniButton { objectName: "createDocumentButton"; text: tr("paridad.lote3.eni.create_document"); enabled: !panel.busy; onClicked: if (!panel.missingDocumentInput() && panel.validateFields([docOrgan, capture, docId, sourceId, format])) documentSave.open() }
            MessageLabel { objectName: "documentMessage"; text: panel.documentMessage }
            SectionTitle { text: tr("paridad.lote3.eni.file") }
            EniButton { id: folderButton; objectName: "folderButton"; text: tr("paridad.lote3.eni.folder"); onClicked: folderPicker.open() }
            PathLabel { path: panel.directoryPath; emptyKey: "paridad.lote3.eni.no_folder" }
            FieldLabel { text: tr("paridad.lote3.eni.organ_file") }
            EniValidatedField { id: fileOrgan; objectName: "fileOrgan"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.organ_file"; hintKey: "paridad.lote3.eni.organ_hint"; maximumLength: 128; validation: Validation.organError }
            FieldLabel { text: tr("paridad.lote3.eni.open_date") }
            EniDateField { id: opened; objectName: "opened"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.open_date"; timeLabelKey: "paridad.lote3.eni.open_time" }
            FieldLabel { text: tr("paridad.lote3.eni.classification") }
            EniValidatedField { id: classification; objectName: "classification"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.classification"; maximumLength: 44; validation: Validation.classificationError }
            FieldLabel { text: tr("paridad.lote3.eni.file_state") }
            EniCombo { id: fileState; objectName: "fileState"; model: panel.codeItems(Catalog.EstadosExpediente); textRole: "label"; valueRole: "code"; currentIndex: 0; Accessible.name: tr("paridad.lote3.eni.file_state") }
            FieldLabel { text: tr("paridad.lote3.eni.identifier_file") }
            EniValidatedField { id: fileId; objectName: "fileId"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.identifier_file"; maximumLength: 48; validation: function(text) { return Validation.identifierError(text, false) } }
            FieldLabel { text: tr("paridad.lote3.eni.interested_optional") }
            EniValidatedField { id: interested; objectName: "interested"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.interested_optional"; maximumLength: 256; validation: Validation.interestedError }
            FieldLabel { text: tr("paridad.lote3.eni.certificate") }
            EniCombo { id: certificate; model: panel.certificates; textRole: "subjectName"; Accessible.name: tr("paridad.lote3.eni.certificate") }
            EniButton { objectName: "createFileButton"; text: tr("paridad.lote3.eni.create_file"); enabled: !panel.busy; onClicked: if (!panel.missingFileInput() && panel.validateFields([fileOrgan, opened, classification, fileId, interested])) fileSave.open() }
            MessageLabel { objectName: "fileMessage"; text: panel.fileMessage }
            MessageLabel { text: panel.statusText }
            Item { Layout.preferredHeight: 24 }
        }
    }
}
