// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick 2.15
import QtTest 1.3
import "../qml"

Item {
    width: 600
    height: 800
    SealDrawArea {
        id: area
        width: 500
        height: 700
        drawMode: true
        ready: true
        initialRect: ({x: 0.2, y: 0.3, w: 0.4, h: 0.2})
    }
    TestCase {
        name: "SealDrawArea"
        when: windowShown
        SignalSpy { id: committed; target: area; signalName: "committed" }
        SignalSpy { id: feedback; target: area; signalName: "feedback" }
        SignalSpy { id: finished; target: area; signalName: "finished" }
        SignalSpy { id: moved; target: area; signalName: "keyboardMoved" }
        function init() {
            area.drawing = false
            area.drawMode = true
            area.ready = true
            area.forceActiveFocus()
            committed.clear()
            feedback.clear()
            finished.clear()
            moved.clear()
        }
        function test_reverse_drag_and_bottom_origin() {
            mousePress(area, 350, 560)
            mouseMove(area, 100, 210)
            compare(committed.count, 0)
            mouseRelease(area, 100, 210)
            compare(committed.count, 1)
            const rect = committed.signalArguments[0][0]
            fuzzyCompare(rect.x, 0.2, 1e-9)
            fuzzyCompare(rect.y, 0.2, 1e-9)
            fuzzyCompare(rect.w, 0.5, 1e-9)
            fuzzyCompare(rect.h, 0.5, 1e-9)
            compare(feedback.signalArguments[0][0], "sign.seal.draw_applied")
            // Soltar el ratón termina el modo de dibujo, como Intro.
            compare(finished.count, 1)
        }
        function test_drag_outside_page_is_clamped() {
            mousePress(area, 100, 100)
            mouseMove(area, 570, 750)
            mouseRelease(area, 570, 750)
            compare(committed.count, 1)
            const rect = committed.signalArguments[0][0]
            fuzzyCompare(rect.x, 0.2, 1e-9)
            fuzzyCompare(rect.y, 0, 1e-9)
            fuzzyCompare(rect.w, 0.8, 1e-9)
            fuzzyCompare(rect.h, 1 - 100 / 700, 1e-9)
        }
        function test_tiny_drag_does_not_commit() {
            mousePress(area, 100, 100)
            mouseRelease(area, 105, 105)
            compare(committed.count, 0)
            compare(finished.count, 0)
            compare(feedback.signalArguments[0][0], "sign.seal.draw_too_small")
        }
        function test_escape_discards_drag_until_next_press() {
            mousePress(area, 100, 100)
            mouseMove(area, 350, 500)
            keyClick(Qt.Key_Escape)
            mouseRelease(area, 350, 500)
            compare(area.drawing, false)
            compare(committed.count, 0)
            compare(feedback.signalArguments[0][0], "sign.seal.draw_cancelled")
        }
        function test_keyboard_moves_opposite_corners_and_confirms() {
            keyClick(Qt.Key_Right)
            keyClick(Qt.Key_Down, Qt.ShiftModifier)
            compare(committed.count, 0)
            keyClick(Qt.Key_Return)
            compare(committed.count, 1)
            const rect = committed.signalArguments[0][0]
            fuzzyCompare(rect.x, 0.21, 1e-9)
            fuzzyCompare(rect.y, 0.29, 1e-9)
            fuzzyCompare(rect.w, 0.39, 1e-9)
            fuzzyCompare(rect.h, 0.21, 1e-9)
        }
        function test_enter_applies_and_escape_discards_then_leave_the_mode() {
            keyClick(Qt.Key_Right)
            compare(moved.count, 1)
            fuzzyCompare(moved.signalArguments[0][0].x, 0.21, 1e-9)
            keyClick(Qt.Key_Return)
            compare(committed.count, 1)
            compare(finished.count, 1)
            area.forceActiveFocus()
            keyClick(Qt.Key_Escape)
            compare(finished.count, 2)
            compare(committed.count, 1)
        }
        function test_losing_preview_or_switching_mode_cancels() {
            keyClick(Qt.Key_Right)
            area.ready = false
            compare(area.drawing, false)
            area.ready = true
            keyClick(Qt.Key_Right)
            area.drawMode = false
            compare(area.drawing, false)
            compare(committed.count, 0)
        }
    }
}
