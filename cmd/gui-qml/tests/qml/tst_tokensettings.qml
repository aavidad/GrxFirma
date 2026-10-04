// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtQuick.Layouts 1.15
import QtTest 1.2
import "../../qml"

TestCase {
    id: testCase
    name: "TokenSettingsPanel"
    width: 700
    height: 800
    when: windowShown
    property var theme: ({cardColor: "#1c1f26", textColor: "#ffffff", secondaryTextColor: "#bdc3c7"})
    property var panel
    QtObject {
        id: fake
        property var calls: []
        signal tokenSettingsFinished(string action, bool ok, var snapshot, string message)
        function getTokenSettings() { calls.push("get") }
        function diagnoseTokenSettings() { calls.push("diagnose") }
        function saveTokenSettings(value) { calls.push(value) }
        function tokenModuleLocalPath(url) { return String(url).startsWith("file:///") ? String(url).substring(7) : "" }
    }
    Component { id: factory; TokenSettingsPanel { theme: testCase.theme; width: 650; bridge: fake; localIpc: true; translate: function(key) { return key } } }
    Component {
        id: layoutFactory
        ColumnLayout {
            width: Math.max(456, implicitWidth)
            TokenSettingsPanel {
                theme: testCase.theme
                objectName: "responsivePanel"
                Layout.fillWidth: true; Layout.minimumWidth: 0
                bridge: fake; localIpc: true
                translate: function(key) { return "Texto largo que debe envolver sin ensanchar los ajustes. ".repeat(4) }
            }
        }
    }
    function init() { fake.calls = []; panel = createTemporaryObject(factory, testCase); verify(panel !== null) }
    function saved(state, editable) { return {state: state, editable: editable, available: true, enabled: false, modules: [], revision: "r1", checks: []} }
    function loadEditable() { panel.load(); fake.tokenSettingsFinished("get_token_settings", true, saved("disabled", true), "") }
    function test_production_and_rest_fail_closed() {
        panel.load()
        fake.tokenSettingsFinished("get_token_settings", true, {state: "unavailable", available: false, editable: false}, "")
        verify(!panel.canEdit)
        panel.addFiles(["file:///tmp/test.so"])
        compare(panel.modules.length, 0)
        panel.localIpc = false
        verify(!panel.supported)
        const before = fake.calls.length
        panel.load(); panel.diagnose(); panel.saveConfirmed()
        compare(fake.calls.length, before)
    }
    function test_local_paths_limit_and_no_autosave() {
        loadEditable()
        panel.addFiles(["file:///tmp/a.so", "file:///tmp/a.so"])
        compare(panel.modules.length, 1)
        panel.addFiles(["https://example.test/a.so"])
        compare(panel.modules.length, 1)
        let tooMany = []
        for (let i = 0; i < 9; i++) tooMany.push("file:///tmp/" + i + ".so")
        panel.addFiles(tooMany)
        compare(panel.modules.length, 1)
        verify(panel.dirty)
        compare(fake.calls.length, 1)
        panel.saveConfirmed()
        compare(fake.calls.length, 1)
    }
    function test_error_preserves_draft_and_requires_explicit_reload() {
        loadEditable()
        panel.addFiles(["file:///tmp/a.so"])
        fake.tokenSettingsFinished("save_token_settings", false, {}, "conflict")
        verify(!panel.busy)
        verify(panel.reloadRequired)
        verify(!panel.canEdit)
        compare(panel.modules[0].path, "/tmp/a.so")
        compare(panel.revision, "r1")
        compare(fake.calls.length, 1)
        panel.load()
        fake.tokenSettingsFinished("get_token_settings", true, saved("disabled", true), "")
        verify(panel.canEdit)
        verify(!panel.dirty)
    }
    function test_diagnosis_does_not_replace_draft_revision() {
        loadEditable()
        panel.addFiles(["file:///tmp/a.so"])
        panel.diagnose()
        let next = saved("configured", true); next.revision = "r2"
        fake.tokenSettingsFinished("diagnose_token_settings", true, next, "")
        compare(panel.modules.length, 1)
        compare(panel.revision, "r1")
        verify(panel.dirty)
        verify(!panel.busy)
        verify(panel.reloadRequired)
    }
    function test_invalid_configuration_needs_both_confirmations() {
        panel.load()
        fake.tokenSettingsFinished("get_token_settings", true, saved("invalid", true), "")
        const confirm = findChild(panel, "tokenSettingsConfirmation")
        const replace = findChild(panel, "tokenSettingsReplaceConfirmation")
        verify(confirm !== null); verify(replace !== null)
        confirm.checked = true
        panel.saveConfirmed()
        compare(fake.calls.length, 1)
        replace.checked = true
        panel.saveConfirmed()
        compare(fake.calls.length, 2)
        verify(fake.calls[1].confirmed)
        verify(fake.calls[1].replaceInvalid)
        compare(fake.calls[1].revision, "r1")
        verify(panel.busy)
        const next = saved("disabled", true); next.restartRequired = true; next.warning = "durability_unconfirmed"
        fake.tokenSettingsFinished("save_token_settings", true, next, "")
        verify(!panel.busy)
        verify(panel.snapshot.restartRequired)
        verify(!panel.dirty)
    }
    function test_readonly_and_non_boolean_permissions_fail_closed() {
        panel.load()
        fake.tokenSettingsFinished("get_token_settings", true, saved("disabled", false), "")
        verify(!panel.canEdit)
        fake.tokenSettingsFinished("get_token_settings", true, saved("disabled", "false"), "")
        verify(!panel.canEdit)
    }
    function test_long_labels_do_not_expand_outer_layout() {
        const layout = createTemporaryObject(layoutFactory, testCase)
        const responsive = findChild(layout, "responsivePanel")
        fake.tokenSettingsFinished("get_token_settings", true, saved("disabled", true), "")
        wait(30)
        compare(layout.width, 456)
        compare(responsive.width, 456)
        fake.tokenSettingsFinished("get_token_settings", true, {state:"unavailable",available:false,editable:false,modules:[],checks:[]}, "")
        wait(30)
        compare(layout.width, 456)
        compare(responsive.width, 456)
    }
}
