// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import "SealDrawGeometry.js" as Geometry

Item {
    id: drawArea
    property bool drawMode: false
    property bool ready: false
    property bool drawing: false
    property real firstX: 0
    property real firstY: 0
    property real secondX: 0
    property real secondY: 0
    property var initialRect: ({x: 0.1, y: 0.1, w: 0.3, h: 0.1})
    property string accessibleName: ""
    property string accessibleHelp: ""
    property color outlineColor: "#2980b9"
    readonly property var rectangle: Geometry.normalize(firstX, firstY, secondX, secondY)
    signal committed(var rect)
    signal feedback(string key)
    visible: drawMode
    enabled: ready
    activeFocusOnTab: true
    Accessible.role: Accessible.Pane
    Accessible.name: accessibleName
    Accessible.description: accessibleHelp

    function cancel() {
        if (!drawing) return
        drawing = false
        feedback("sign.seal.draw_cancelled")
    }
    function confirm() {
        if (!drawing || !ready) return
        const rect = rectangle
        drawing = false
        if (!Geometry.isLargeEnough(rect, width, height)) {
            feedback("sign.seal.draw_too_small")
            return
        }
        committed(rect)
        feedback("sign.seal.draw_applied")
    }
    function beginKeyboard() {
        firstX = initialRect.x
        firstY = 1 - initialRect.y - initialRect.h
        secondX = firstX + initialRect.w
        secondY = firstY + initialRect.h
        drawing = true
    }
    function handleKey(event) {
        if (!drawMode || !ready) return
        if (event.key === Qt.Key_Escape) {
            cancel()
        } else if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter) {
            if (!drawing) beginKeyboard()
            confirm()
        } else {
            const dx = event.key === Qt.Key_Left ? -0.01 : event.key === Qt.Key_Right ? 0.01 : 0
            const dy = event.key === Qt.Key_Up ? -0.01 : event.key === Qt.Key_Down ? 0.01 : 0
            if (!dx && !dy) return
            if (!drawing) beginKeyboard()
            if (event.modifiers & Qt.ShiftModifier) {
                secondX = Math.max(0, Math.min(1, secondX + dx))
                secondY = Math.max(0, Math.min(1, secondY + dy))
            } else {
                firstX = Math.max(0, Math.min(1, firstX + dx))
                firstY = Math.max(0, Math.min(1, firstY + dy))
            }
        }
        event.accepted = true
    }
    Keys.onPressed: (event) => handleKey(event)
    onDrawModeChanged: { cancel(); if (drawMode && ready) forceActiveFocus(); else focus = false }
    onReadyChanged: { if (!ready) { cancel(); focus = false } }
    onWidthChanged: cancel()
    onHeightChanged: cancel()

    Rectangle {
        visible: drawArea.drawing
        x: drawArea.rectangle.x * drawArea.width
        y: (1 - drawArea.rectangle.y - drawArea.rectangle.h) * drawArea.height
        width: drawArea.rectangle.w * drawArea.width
        height: drawArea.rectangle.h * drawArea.height
        color: "#302980b9"
        border.color: drawArea.outlineColor
        border.width: 2
    }
    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.LeftButton
        cursorShape: Qt.CrossCursor
        preventStealing: true
        onPressed: (mouse) => {
            drawArea.forceActiveFocus()
            drawArea.firstX = Math.max(0, Math.min(1, mouse.x / width))
            drawArea.firstY = Math.max(0, Math.min(1, mouse.y / height))
            drawArea.secondX = drawArea.firstX
            drawArea.secondY = drawArea.firstY
            drawArea.drawing = true
        }
        onPositionChanged: (mouse) => {
            if (!pressed || !drawArea.drawing) return
            drawArea.secondX = Math.max(0, Math.min(1, mouse.x / width))
            drawArea.secondY = Math.max(0, Math.min(1, mouse.y / height))
        }
        onReleased: (mouse) => {
            if (!drawArea.drawing) return
            drawArea.secondX = Math.max(0, Math.min(1, mouse.x / width))
            drawArea.secondY = Math.max(0, Math.min(1, mouse.y / height))
            drawArea.confirm()
        }
        onCanceled: drawArea.cancel()
    }
}
