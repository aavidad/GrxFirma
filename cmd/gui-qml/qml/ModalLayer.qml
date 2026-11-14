// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Window

// Capa de trabajo con recorrido de tabulación cerrado y retorno del foco.
Rectangle {
    id: layer
    property Item initialFocusItem: null
    property Item returnFocusItem: null
    signal escapePressed()

    function containsItem(item) {
        for (let current = item; current; current = current.parent) {
            if (current === layer) return true
        }
        return false
    }
    function enter() {
        const host = layer.Window.window
        const previous = host ? host.activeFocusItem : null
        if (previous && !containsItem(previous)) returnFocusItem = previous
        if (initialFocusItem) initialFocusItem.forceActiveFocus(Qt.TabFocusReason)
    }
    function restoreFocus() {
        if (returnFocusItem && returnFocusItem.visible && returnFocusItem.enabled)
            returnFocusItem.forceActiveFocus(Qt.OtherFocusReason)
    }
    function moveFocus(forward) {
        const host = layer.Window.window
        const first = host ? host.activeFocusItem : null
        if (!first) { enter(); return }
        let next = first.nextItemInFocusChain(forward)
        const visited = [first]
        while (next && visited.indexOf(next) < 0) {
            visited.push(next)
            if (containsItem(next) && next.visible && next.enabled && next.activeFocusOnTab) {
                next.forceActiveFocus(forward ? Qt.TabFocusReason : Qt.BacktabFocusReason)
                return
            }
            next = next.nextItemInFocusChain(forward)
        }
    }
    onVisibleChanged: {
        if (visible) Qt.callLater(enter)
        else Qt.callLater(restoreFocus)
    }
    Component.onCompleted: if (visible) Qt.callLater(enter)
    // Los controles consumen Tab antes de propagar Keys a sus antecesores.
    // Los atajos de ventana mantienen el recorrido dentro de esta capa.
    Shortcut {
        sequence: "Tab"
        context: Qt.WindowShortcut
        enabled: layer.visible
        onActivated: layer.moveFocus(true)
    }
    Shortcut {
        sequences: ["Shift+Tab", "Backtab"]
        context: Qt.WindowShortcut
        enabled: layer.visible
        onActivated: layer.moveFocus(false)
    }
    Keys.onPressed: function(event) {
        if (event.key === Qt.Key_Tab || event.key === Qt.Key_Backtab) {
            moveFocus(event.key !== Qt.Key_Backtab && !(event.modifiers & Qt.ShiftModifier))
            event.accepted = true
        } else if (event.key === Qt.Key_Escape) {
            event.accepted = true
            escapePressed()
        }
    }
}
