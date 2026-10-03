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
    property var certificates: []
    property var certificateId: function(cert) { return cert ? cert.id : "" }
    property bool busy: false
    property string signaturePath: ""
    property string originalPath: ""
    property string directoryPath: ""
    property string statusText: ""
    function tr(key) { return translate(key) }
    function documentOptions() {
        return {"eni.organo": docOrgan.text.trim(), "eni.origen": origin.currentIndex === 0 ? "administracion" : "ciudadano",
                "eni.fechaCaptura": capture.text.trim(), "eni.estado": state.text.trim(),
                "eni.tipoDocumental": docType.text.trim(), "eni.identificador": docId.text.trim(),
                "eni.documentoOrigen": sourceId.text.trim(), "eni.formato": format.text.trim()}
    }
    function fileOptions() {
        return {"exp.organo": fileOrgan.text.trim(), "exp.fechaApertura": opened.text.trim(),
                "exp.clasificacion": classification.text.trim(), "exp.estado": fileState.text.trim(),
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
            if (output === "") return
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
            if (output === "") return
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
    ScrollView { anchors.fill: parent; clip: true; contentWidth: availableWidth
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
            TextField { id: docOrgan; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.organ"); maximumLength: 128 }
            ComboBox { id: origin; model: [tr("paridad.lote3.eni.administration"), tr("paridad.lote3.eni.citizen")]; Accessible.name: tr("paridad.lote3.eni.origin") }
            Label { text: tr("paridad.lote3.eni.capture_date"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: capture; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.capture_date"); text: new Date().toISOString(); maximumLength: 35 }
            Label { text: tr("paridad.lote3.eni.state"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: state; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.state"); text: "EE01"; maximumLength: 4 }
            Label { text: tr("paridad.lote3.eni.doc_type"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: docType; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.doc_type"); text: "TD99"; maximumLength: 4 }
            Label { text: tr("paridad.lote3.eni.identifier_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: docId; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.identifier_optional"); maximumLength: 80 }
            Label { text: tr("paridad.lote3.eni.source_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: sourceId; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.source_optional"); maximumLength: 80 }
            Label { text: tr("paridad.lote3.eni.format_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: format; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.format_optional"); maximumLength: 32 }
            Button { text: tr("paridad.lote3.eni.create_document"); enabled: !panel.busy && panel.signaturePath !== "" && docOrgan.text.trim() !== ""; Accessible.name: text; onClicked: documentSave.open() }
            Label { text: tr("paridad.lote3.eni.file"); color: panel.theme.textColor; font.bold: true }
            Button { text: tr("paridad.lote3.eni.folder"); Accessible.name: text; onClicked: folderPicker.open() }
            Label { text: panel.directoryPath; color: panel.theme.textColor; wrapMode: Text.WrapAnywhere; Layout.fillWidth: true }
            Label { text: tr("paridad.lote3.eni.organ"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: fileOrgan; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.organ"); maximumLength: 128 }
            Label { text: tr("paridad.lote3.eni.open_date"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: opened; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.open_date"); text: new Date().toISOString(); maximumLength: 35 }
            Label { text: tr("paridad.lote3.eni.classification"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: classification; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.classification"); maximumLength: 80 }
            Label { text: tr("paridad.lote3.eni.file_state"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: fileState; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.file_state"); text: "E01"; maximumLength: 3 }
            Label { text: tr("paridad.lote3.eni.identifier_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: fileId; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.identifier_optional"); maximumLength: 80 }
            Label { text: tr("paridad.lote3.eni.interested_optional"); color: panel.theme.textColor; Layout.fillWidth: true }
            TextField { id: interested; Layout.fillWidth: true; Accessible.name: tr("paridad.lote3.eni.interested_optional"); maximumLength: 256 }
            Label { text: tr("paridad.lote3.eni.certificate"); color: panel.theme.textColor; Layout.fillWidth: true }
            ComboBox { id: certificate; Layout.fillWidth: true; model: panel.certificates; textRole: "subjectName"; Accessible.name: tr("paridad.lote3.eni.certificate") }
            Button { text: tr("paridad.lote3.eni.create_file"); enabled: !panel.busy && panel.directoryPath !== "" && fileOrgan.text.trim() !== "" && classification.text.trim() !== "" && certificate.currentIndex >= 0; Accessible.name: text; onClicked: fileSave.open() }
            Label { text: panel.statusText; visible: text !== ""; color: panel.theme.textColor; wrapMode: Text.WordWrap; Layout.fillWidth: true; Accessible.name: text }
            Item { Layout.preferredHeight: 24 }
        }
    }
}
