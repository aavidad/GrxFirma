// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15
import QtQuick.Dialogs 6.2

GroupBox {
    id: panel
    // Wrapped prose must not make the outer settings layout wider than its viewport.
    implicitWidth: 320
    required property var bridge
    required property bool localIpc
    required property var translate
    property var snapshot: ({state: "unavailable", available: false, editable: false, checks: []})
    property var modules: []
    property bool draftEnabled: false
    property string revision: ""
    property bool loaded: false
    property bool dirty: false
    property bool busy: false
    property bool reloadRequired: false
    property string message: ""
    readonly property bool supported: localIpc && bridge && typeof bridge.getTokenSettings === "function"
    readonly property bool canEdit: supported && loaded && snapshot.available === true && snapshot.editable === true && !busy && !reloadRequired
    title: t("title")
    Accessible.name: title

    function t(key) { return translate("token_settings." + key) }
    function load() {
        if (!supported || busy) return
        busy = true
        message = ""
        bridge.getTokenSettings()
    }
    function diagnose() {
        if (!supported || busy) return
        busy = true
        message = ""
        bridge.diagnoseTokenSettings()
    }
    function addFiles(files) {
        if (!canEdit) return
        let next = modules.slice()
        for (let i = 0; i < files.length; i++) {
            const path = bridge.tokenModuleLocalPath(files[i])
            if (!path) { message = t("local_only"); return }
            if (!next.some(function(entry) { return entry.path === path })) next.push({path: path})
        }
        if (next.length > 8) { message = t("limit"); return }
        modules = next
        dirty = true
        message = ""
    }
    function saveConfirmed() {
        if (!canEdit || !confirmation.checked || (snapshot.state === "invalid" && !replaceConfirmation.checked)) return
        busy = true
        message = ""
        bridge.saveTokenSettings({enabled: draftEnabled, modules: modules, revision: revision,
                                  confirmed: true, replaceInvalid: snapshot.state === "invalid" && replaceConfirmation.checked})
    }
    function checkLabel(check) {
        const ids = ["config", "modules", "helper", "pinentry", "pin_memory", "hardware"]
        const states = ["missing", "valid", "invalid", "unsafe", "read_only", "unavailable", "filesystem_checked", "not_checked", "reservation_checked"]
        if (ids.indexOf(check.id) < 0 || states.indexOf(check.status) < 0) return t("check_unknown")
        return t("check_" + check.id) + ": " + t("status_" + check.status)
    }

    Connections {
        target: panel.supported ? panel.bridge : null
        ignoreUnknownSignals: true
        function onTokenSettingsFinished(action, ok, result, errorMessage) {
            panel.busy = false
            if (!ok) {
                // Keep the draft, never retry a write or silently replace its revision.
                // Backend errors are fixed codes. Do not render arbitrary driver or transport text.
                panel.message = panel.t("failed")
                if (errorMessage === "token_settings_unsafe") panel.message += "\n" + panel.t("readonly")
                if (errorMessage === "token_settings_unavailable" || errorMessage === "token_settings_frontend_required") panel.message += "\n" + panel.t("unavailable")
                panel.reloadRequired = true
                return
            }
            panel.snapshot = result
            if (action === "diagnose_token_settings" && panel.loaded && result.revision !== panel.revision) {
                panel.reloadRequired = true
                panel.message = panel.t("failed")
            }
            if (action !== "diagnose_token_settings" || !panel.loaded) {
                panel.modules = result.modules || []
                panel.draftEnabled = result.enabled === true
                panel.revision = result.revision || ""
                panel.dirty = false
                panel.reloadRequired = false
                panel.loaded = true
            }
            if (action === "save_token_settings") panel.message = panel.t("saved")
        }
    }

    contentItem: ColumnLayout {
        spacing: 12
        Label { Layout.fillWidth: true; text: panel.t("intro"); wrapMode: Text.Wrap; textFormat: Text.PlainText }
        Label {
            Layout.fillWidth: true
            visible: !panel.supported || (panel.loaded && !panel.snapshot.available)
            text: panel.t("unavailable"); wrapMode: Text.Wrap; textFormat: Text.PlainText
        }
        Label {
            Layout.fillWidth: true; visible: panel.loaded
            text: panel.t("state") + ": " + panel.t("state_" + (["missing", "disabled", "configured", "invalid", "unavailable"].indexOf(panel.snapshot.state) >= 0 ? panel.snapshot.state : "unavailable"))
            wrapMode: Text.Wrap; textFormat: Text.PlainText
        }
        Label { Layout.fillWidth: true; visible: panel.loaded && panel.snapshot.available && !panel.snapshot.editable; text: panel.t("readonly"); wrapMode: Text.Wrap }
        Flow {
            Layout.fillWidth: true; spacing: 8
            Button {
                implicitHeight: 44
                text: panel.t("load"); enabled: panel.supported && !panel.busy
                onClicked: { if (panel.dirty) discardDialog.open(); else panel.load() }
            }
            Button { implicitHeight: 44; text: panel.t("diagnose"); enabled: panel.supported && !panel.busy; onClicked: panel.diagnose() }
            BusyIndicator { running: panel.busy; visible: running; width: 40; height: 40; Accessible.name: panel.t("busy") }
        }
        Label { Layout.fillWidth: true; text: panel.t("diagnose_help"); wrapMode: Text.Wrap; textFormat: Text.PlainText }
        Repeater {
            model: panel.snapshot.checks || []
            Label { required property var modelData; Layout.fillWidth: true; text: panel.checkLabel(modelData); wrapMode: Text.Wrap; textFormat: Text.PlainText }
        }
        CheckBox {
            id: enableCheck
            Layout.fillWidth: true
            Layout.minimumHeight: 44
            text: panel.t("enable"); checked: panel.draftEnabled; enabled: panel.canEdit
            contentItem: Label { text: enableCheck.text; leftPadding: enableCheck.indicator.width + enableCheck.spacing; wrapMode: Text.Wrap; verticalAlignment: Text.AlignVCenter }
            onClicked: { panel.draftEnabled = checked; panel.dirty = true }
        }
        Label { Layout.fillWidth: true; text: panel.t("driver_warning"); wrapMode: Text.Wrap; textFormat: Text.PlainText }
        Repeater {
            model: panel.modules
            RowLayout {
                required property var modelData
                required property int index
                Layout.fillWidth: true
                Label { Layout.fillWidth: true; Layout.minimumWidth: 0; text: modelData.path; wrapMode: Text.WrapAnywhere; textFormat: Text.PlainText }
                Button {
                    implicitHeight: 44
                    text: panel.t("remove"); Accessible.name: text + " " + modelData.path; enabled: panel.canEdit
                    onClicked: { let next = panel.modules.slice(); next.splice(index, 1); panel.modules = next; panel.dirty = true }
                }
            }
        }
        Flow {
            Layout.fillWidth: true; spacing: 8
            Button { implicitHeight: 44; text: panel.t("add"); enabled: panel.canEdit && panel.modules.length < 8; onClicked: modulePicker.open() }
            Button {
                implicitHeight: 44
                text: panel.t("save"); enabled: panel.canEdit && panel.dirty
                onClicked: { confirmation.checked = false; replaceConfirmation.checked = false; saveDialog.open() }
            }
        }
        Label { Layout.fillWidth: true; visible: panel.dirty; text: panel.t("draft"); wrapMode: Text.Wrap }
        Label { Layout.fillWidth: true; visible: panel.snapshot.restartRequired === true; text: panel.t("restart"); wrapMode: Text.Wrap }
        Label { Layout.fillWidth: true; visible: panel.snapshot.warning === "durability_unconfirmed"; text: panel.t("durability"); wrapMode: Text.Wrap }
        Label { Layout.fillWidth: true; visible: panel.message !== ""; text: panel.message; wrapMode: Text.Wrap; textFormat: Text.PlainText; Accessible.name: text }
    }

    FileDialog {
        id: modulePicker
        title: panel.t("add")
        fileMode: FileDialog.OpenFiles
        onAccepted: panel.addFiles(selectedFiles)
    }
    Dialog {
        id: discardDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        width: Math.min(520, parent.width - 32)
        modal: true
        title: panel.t("load")
        standardButtons: Dialog.Ok | Dialog.Cancel
        Component.onCompleted: {
            standardButton(Dialog.Ok).implicitHeight = 44
            standardButton(Dialog.Cancel).implicitHeight = 44
        }
        contentItem: Label { text: panel.t("discard"); wrapMode: Text.Wrap }
        onAccepted: panel.load()
    }
    Dialog {
        id: saveDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        width: Math.min(620, parent.width - 32)
        height: Math.min(520, parent.height - 32)
        modal: true
        title: panel.t("save")
        standardButtons: Dialog.Save | Dialog.Cancel
        onOpened: confirmation.forceActiveFocus()
        onAccepted: panel.saveConfirmed()
        Component.onCompleted: {
            standardButton(Dialog.Save).implicitHeight = 44
            standardButton(Dialog.Cancel).implicitHeight = 44
            standardButton(Dialog.Save).enabled = Qt.binding(function() {
                return panel.canEdit && confirmation.checked && (panel.snapshot.state !== "invalid" || replaceConfirmation.checked)
            })
        }
        contentItem: ScrollView {
            clip: true
            contentWidth: availableWidth
            ColumnLayout {
                width: saveDialog.availableWidth
                Label { Layout.fillWidth: true; text: panel.t("driver_warning") + "\n" + panel.t("restart"); wrapMode: Text.Wrap }
                Label { Layout.fillWidth: true; text: panel.t("enable") + ": " + panel.t(panel.draftEnabled ? "state_configured" : "state_disabled"); wrapMode: Text.Wrap }
                Repeater { model: panel.modules; Label { required property var modelData; Layout.fillWidth: true; text: modelData.path; wrapMode: Text.WrapAnywhere; textFormat: Text.PlainText } }
                CheckBox { id: confirmation; objectName: "tokenSettingsConfirmation"; Layout.fillWidth: true; text: panel.t("confirm"); contentItem: Label { text: confirmation.text; leftPadding: confirmation.indicator.width + confirmation.spacing; wrapMode: Text.Wrap; verticalAlignment: Text.AlignVCenter } }
                CheckBox { id: replaceConfirmation; objectName: "tokenSettingsReplaceConfirmation"; visible: panel.snapshot.state === "invalid"; Layout.fillWidth: true; text: panel.t("replace_invalid"); contentItem: Label { text: replaceConfirmation.text; leftPadding: replaceConfirmation.indicator.width + replaceConfirmation.spacing; wrapMode: Text.Wrap; verticalAlignment: Text.AlignVCenter } }
            }
        }
    }
}
