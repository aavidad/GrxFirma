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
    function tr(key) { return translate(key) }
    function codeItems(codes) {
        return codes.map(function(code) { return {code: code, label: code + " — " + panel.tr("eni.codigo." + code)} })
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
        onAccepted: panel.signaturePath = panel.localPath(selectedFile) }
    FileDialog { id: originalPicker; title: tr("paridad.lote3.eni.original"); fileMode: FileDialog.OpenFile
        onAccepted: panel.originalPath = panel.localPath(selectedFile) }
    FolderDialog { id: folderPicker; title: tr("paridad.lote3.eni.folder")
        onAccepted: panel.directoryPath = panel.localPath(selectedFolder) }
    FileDialog { id: documentSave; title: tr("paridad.lote3.eni.save_document"); fileMode: FileDialog.SaveFile
        nameFilters: [tr("paridad.lote3.eni.xml_filter")]
        onAccepted: {
            const output = panel.localPath(selectedFile)
            if (output === "" || !panel.validateFields([docOrgan, capture, docId, sourceId, format])) return
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
            if (output === "" || !panel.validateFields([fileOrgan, opened, classification, fileId, interested])) return
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
    ScrollView { id: scroller; anchors.fill: parent; clip: true; contentWidth: availableWidth
        ColumnLayout { width: Math.max(0, parent.width - 48); x: 24; spacing: 12
            Label { text: tr("paridad.lote3.eni.title"); color: panel.theme.textColor; font.pixelSize: 28; font.bold: true }
            Label { text: tr("paridad.lote3.eni.document"); color: panel.theme.textColor; font.bold: true }
            Button { text: tr("paridad.lote3.eni.signature"); Accessible.name: text; onClicked: signaturePicker.open() }
            Label { text: panel.signaturePath; color: panel.theme.textColor; wrapMode: Text.WrapAnywhere; Layout.fillWidth: true }
            RowLayout {
                Button { text: tr("paridad.lote3.eni.original"); Accessible.name: text; onClicked: originalPicker.open() }
                Button { text: tr("paridad.lote3.eni.clear_original"); enabled: panel.originalPath !== ""; Accessible.name: text; onClicked: panel.originalPath = "" }
            }
            Label { text: panel.originalPath; color: panel.theme.textColor; wrapMode: Text.WrapAnywhere; Layout.fillWidth: true }
            Label { text: tr("paridad.lote3.eni.organ"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: docOrgan; objectName: "docOrgan"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.organ"; maximumLength: 128; validation: Validation.organError }
            ComboBox { id: origin; model: [tr("paridad.lote3.eni.administration"), tr("paridad.lote3.eni.citizen")]; Accessible.name: tr("paridad.lote3.eni.origin") }
            Label { text: tr("paridad.lote3.eni.capture_date"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniDateField { id: capture; objectName: "capture"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.capture_date" }
            Label { text: tr("paridad.lote3.eni.state"); color: panel.theme.textColor; Layout.fillWidth: true }
            ComboBox { id: state; objectName: "state"; Layout.fillWidth: true; model: panel.codeItems(Catalog.EstadosElaboracion); textRole: "label"; valueRole: "code"; currentIndex: 0; Accessible.name: tr("paridad.lote3.eni.state"); onCurrentValueChanged: if (sourceId) sourceId.touched = true }
            Label { text: tr("paridad.lote3.eni.doc_type"); color: panel.theme.textColor; Layout.fillWidth: true }
            ComboBox { id: docType; objectName: "docType"; Layout.fillWidth: true; model: panel.codeItems(Catalog.TiposDocumentales); textRole: "label"; valueRole: "code"; currentIndex: 20; Accessible.name: tr("paridad.lote3.eni.doc_type") }
            Label { text: tr("paridad.lote3.eni.identifier_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: docId; objectName: "docId"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.identifier_optional"; maximumLength: 48; validation: function(text) { return Validation.identifierError(text, false) } }
            Label { text: tr("paridad.lote3.eni.source_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: sourceId; objectName: "sourceId"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.source_optional"; maximumLength: 48; validation: function(text) { return Validation.identifierError(text, ["EE02", "EE03", "EE04"].indexOf(state.currentValue) >= 0) } }
            Label { text: tr("paridad.lote3.eni.format_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: format; objectName: "format"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.format_optional"; maximumLength: 32; validation: Validation.formatError }
            Button { text: tr("paridad.lote3.eni.create_document"); enabled: !panel.busy && panel.signaturePath !== ""; Accessible.name: text; onClicked: if (panel.validateFields([docOrgan, capture, docId, sourceId, format])) documentSave.open() }
            Label { text: tr("paridad.lote3.eni.file"); color: panel.theme.textColor; font.bold: true }
            Button { text: tr("paridad.lote3.eni.folder"); Accessible.name: text; onClicked: folderPicker.open() }
            Label { text: panel.directoryPath; color: panel.theme.textColor; wrapMode: Text.WrapAnywhere; Layout.fillWidth: true }
            Label { text: tr("paridad.lote3.eni.organ"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: fileOrgan; objectName: "fileOrgan"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.organ"; maximumLength: 128; validation: Validation.organError }
            Label { text: tr("paridad.lote3.eni.open_date"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniDateField { id: opened; objectName: "opened"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.open_date" }
            Label { text: tr("paridad.lote3.eni.classification"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: classification; objectName: "classification"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.classification"; maximumLength: 44; validation: Validation.classificationError }
            Label { text: tr("paridad.lote3.eni.file_state"); color: panel.theme.textColor; Layout.fillWidth: true }
            ComboBox { id: fileState; objectName: "fileState"; Layout.fillWidth: true; model: panel.codeItems(Catalog.EstadosExpediente); textRole: "label"; valueRole: "code"; currentIndex: 0; Accessible.name: tr("paridad.lote3.eni.file_state") }
            Label { text: tr("paridad.lote3.eni.identifier_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: fileId; objectName: "fileId"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.identifier_optional"; maximumLength: 48; validation: function(text) { return Validation.identifierError(text, false) } }
            Label { text: tr("paridad.lote3.eni.interested_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            EniValidatedField { id: interested; objectName: "interested"; Layout.fillWidth: true; theme: panel.theme; translate: panel.translate; labelKey: "paridad.lote3.eni.interested_optional"; maximumLength: 256; validation: Validation.interestedError }
            Label { text: tr("paridad.lote3.eni.certificate"); color: panel.theme.textColor; Layout.fillWidth: true }
            ComboBox { id: certificate; Layout.fillWidth: true; model: panel.certificates; textRole: "subjectName"; Accessible.name: tr("paridad.lote3.eni.certificate") }
            Button { text: tr("paridad.lote3.eni.create_file"); enabled: !panel.busy && panel.directoryPath !== "" && certificate.currentIndex >= 0; Accessible.name: text; onClicked: if (panel.validateFields([fileOrgan, opened, classification, fileId, interested])) fileSave.open() }
            Label { text: panel.statusText; visible: text !== ""; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true; Accessible.name: text }
            Item { Layout.preferredHeight: 24 }
        }
    }
}
