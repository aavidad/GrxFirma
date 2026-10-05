// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
import QtQuick
import QtTest
import "../../qml/ThemeContrast.js" as Contrast

TestCase {
    name: "ThemeContrast"
    function test_ratio_accepts_strings_and_colors() {
        fuzzyCompare(Contrast.ratio("#ffffff", "#000000"), 21, 1e-6)
        fuzzyCompare(Contrast.ratio(Qt.rgba(1, 1, 1, 1), "#000000"), 21, 1e-6)
    }
    function test_readable_text_reaches_aa() {
        // Blanco sobre cian claro no se lee: se elige texto oscuro.
        verify(Contrast.ratio("#00f2ff", Contrast.readableOn("#00f2ff", "#ffffff")) >= 4.5)
        // El texto preferido se respeta cuando ya cumple.
        compare(String(Contrast.readableOn("#12141a", "#ffffff")), "#ffffff")
    }
    function test_button_fill_is_darkened_until_white_reads() {
        const fill = Contrast.legibleFill("#2980b9")
        verify(Contrast.ratio(fill, "#ffffff") >= 4.5)
        verify(Contrast.ratio(Contrast.legibleFill("#3498db"), Contrast.readableOn(Contrast.legibleFill("#3498db"), "#ffffff")) >= 4.5)
    }
}
