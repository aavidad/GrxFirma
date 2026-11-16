// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtTest
import "../../qml/ThemeContrast.js" as Contrast

// Lote p3: la fila de una opción insegura activada se recuadra con 2 px y un
// icono. En los 14 temas de main.qml el recuadro y el icono llegan a 3:1
// frente al fondo teñido y al de la página, y el texto a 4,5:1.
TestCase {
    name: "UnsafeRowMarking"
    function themes() {
        const request = new XMLHttpRequest()
        request.open("GET", Qt.resolvedUrl("../../qml/main.qml"), false)
        request.send()
        const source = request.responseText
        const block = source.substring(source.indexOf("property var themes: ["), source.indexOf("property var currentTheme:"))
        const pattern = /backgroundColor: "(#\w+)",\s*sidebarColor: "#\w+",\s*cardColor: "(#\w+)",[\s\S]*?textColor: "(#\w+)",[\s\S]*?errorColor: "(#\w+)"/g
        const out = []
        let match
        while ((match = pattern.exec(block)) !== null)
            out.push({ background: match[1], card: match[2], text: match[3], error: match[4] })
        return out
    }
    function test_marking_reaches_contrast_in_every_theme() {
        const list = themes()
        compare(list.length, 14)
        for (let i = 0; i < list.length; i++) {
            const t = list[i]
            const fill = Contrast.tinted(t.card, t.error, 0.14)
            const mark = Contrast.accentOnBoth(fill, t.background, t.error, 3.0)
            verify(Contrast.ratio(fill, mark) >= 3.0, "recuadro/fondo teñido " + i)
            verify(Contrast.ratio(t.background, mark) >= 3.0, "recuadro/página " + i)
            verify(Contrast.ratio(fill, Contrast.readableOn(fill, t.text)) >= 4.5, "texto " + i)
        }
    }
}
