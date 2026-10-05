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
    function test_accent_keeps_hue_or_reaches_target() {
        // Ya legible: se respeta tal cual.
        compare(String(Contrast.accentOn("#12141a", "#3498db")), "#3498db")
        // Fondos claros y oscuros de los temas con acentos poco legibles.
        const cases = [["#f5f6fa", "#3498db"], ["#ffffff", "#f39c12"], ["#f0ebe1", "#d4af37"],
                       ["#223244", "#2980b9"], ["#1b263b", "#d98841"], ["#0a140a", "#008800"]]
        for (let i = 0; i < cases.length; i++) {
            verify(Contrast.ratio(cases[i][0], Contrast.accentOn(cases[i][0], cases[i][1])) >= 4.5, cases[i].join(" "))
            verify(Contrast.ratio(cases[i][0], Contrast.accentOn(cases[i][0], cases[i][1], 3.0)) >= 3.0, cases[i].join(" "))
        }
    }
}
