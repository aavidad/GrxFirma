// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtTest
import "../../qml/CatalogText.js" as CatalogText

// Lote 39a, vuelta 2. Con el catálogo inglés de verdad: el aviso de uso
// llega del puente como QStringList, que en QML es una secuencia y no un
// Array; antes se descartaba y no salía el aviso común.
TestCase {
    name: "CatalogText"
    // Una list<string> es la misma secuencia que entrega una QStringList.
    property list<string> bridgeKeys: ["excepciones.rsa_corta.aviso", "excepciones.sha1_leer.aviso",
                                       "excepciones.rsa_corta.aviso", "no es una clave", "excepciones.no_existe.aviso"]
    property var english: ({})

    function readFile(relative) {
        const request = new XMLHttpRequest()
        request.open("GET", Qt.resolvedUrl(relative), false)
        request.send(null)
        return request.responseText
    }
    function translate(key) {
        return Object.prototype.hasOwnProperty.call(english, key) ? english[key] : key
    }
    function initTestCase() {
        english = JSON.parse(readFile("../../../../internal/adapters/outbound/common/localizador/locales/en.json"))
        verify(english["excepciones.rsa_corta.aviso"] !== undefined)
    }

    function test_bridge_sequence_shows_the_usage_notice() {
        verify(!Array.isArray(bridgeKeys), "la prueba debe usar una secuencia, no un Array")
        const shown = CatalogText.warningTexts(bridgeKeys, translate, 16)
        compare(shown, [english["excepciones.rsa_corta.aviso"], english["excepciones.sha1_leer.aviso"]])
    }
    function test_plain_arrays_and_bad_input() {
        compare(CatalogText.warningTexts(["excepciones.sha1_leer.aviso"], translate, 16),
                [english["excepciones.sha1_leer.aviso"]])
        compare(CatalogText.warningTexts("excepciones.sha1_leer.aviso", translate, 16), [])
        compare(CatalogText.warningTexts(null, translate, 16), [])
        compare(CatalogText.warningTexts(undefined, translate, 16), [])
        compare(CatalogText.warningTexts(["excepciones.sha1_leer.aviso", "excepciones.rsa_corta.aviso"], translate, 1).length, 1)
    }
}
