// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtTest
import "../../qml/UnsafeText.js" as UnsafeText

// Lote 39h: la ayuda larga se trocea por apartados, el modo organización no
// explica cómo activar ni deshacer, cada nota se dice una vez y la fecha de
// activación lleva el año con cuatro cifras.
TestCase {
    name: "UnsafeText"
    readonly property string help: "Qué hace este ajuste: mientras está desactivado, GrxFirma detiene esto.\n\n" +
        "Por qué es inseguro: riesgo.\n\nCuándo tiene sentido: pocas veces.\n\nAlternativa: otra.\n\n" +
        "Qué cambia: al activar, se permite.\n\nCómo deshacerlo: desactive el ajuste.\n\n" +
        "Bloqueo de la organización: consulte al administrador."
    function test_sections_split_title_and_body() {
        const parts = UnsafeText.sections(help)
        compare(parts.length, 7)
        compare(parts[0].title, "Qué hace este ajuste")
        compare(parts[0].body, "Mientras está desactivado, GrxFirma detiene esto.")
        compare(parts[6].title, "Bloqueo de la organización")
        // Francés (espacio antes de los dos puntos) y chino (dos puntos anchos).
        compare(UnsafeText.splitSection("Ce qui change : si vous activez").title, "Ce qui change")
        compare(UnsafeText.splitSection("Ce qui change : si vous activez").body, "Si vous activez")
        compare(UnsafeText.splitSection("如何撤销：在“安全”部分停用").title, "如何撤销")
        compare(UnsafeText.splitSection("如何撤销：在“安全”部分停用").body, "在“安全”部分停用")
        // Sin dos puntos, o demasiado lejos, no hay título.
        compare(UnsafeText.splitSection("Texto sin título").title, "")
        compare(UnsafeText.sections("A: b\r\n\r\nC: d").length, 2)
    }
    function test_organization_help_says_who_decides_instead_of_how_to_undo() {
        const org = "Quién lo decide: la organización."
        const parts = UnsafeText.helpSections(help, org, true)
        compare(parts.length, 5)
        compare(parts.map(function(p) { return p.title }).join("|"),
                "Qué hace este ajuste|Por qué es inseguro|Cuándo tiene sentido|Alternativa|Quién lo decide")
        compare(UnsafeText.helpSections(help, org, false).length, 7)
    }
    function test_organization_decides_only_by_policy() {
        verify(UnsafeText.organizationDecides({fixed: true, enabled: true, activable: true}))
        verify(UnsafeText.organizationDecides({fixed: true, enabled: false, activable: false}))
        verify(UnsafeText.organizationDecides({activable: false, organization_key: "excepciones.x.organizacion"}))
        verify(!UnsafeText.organizationDecides({activable: false, config_unavailable: true, organization_key: "excepciones.x.organizacion"}))
        verify(!UnsafeText.organizationDecides({activable: true, fixed: false}))
    }
    function test_policy_note_is_said_once() {
        compare(UnsafeText.policyNoteKey({id: "sha1_leer", fixed: true, enabled: true}), "excepciones.pantalla.solo_lectura")
        compare(UnsafeText.policyNoteKey({id: "sha1_leer", fixed: true, enabled: false, organization_key: "excepciones.sha1_leer.organizacion"}),
                "excepciones.sha1_leer.organizacion")
        compare(UnsafeText.policyNoteKey({id: "sha1_leer", fixed: true, enabled: false}), "")
        // El estado ya dice que la directiva no se puede leer.
        compare(UnsafeText.policyNoteKey({id: "sha1_leer", fixed: true, policy_unreadable: true}), "")
        compare(UnsafeText.policyNoteKey({id: "sha1_leer", activable: false, config_unavailable: true}), "excepciones.directorio_no_disponible")
        compare(UnsafeText.policyNoteKey({id: "sha1_leer", activable: true, enabled: true}), "")
    }
    function test_activation_date_uses_the_catalogue_layout_and_four_digit_year() {
        const raw = "2026-11-15T16:44:00Z"
        const local = new Date(raw)
        function two(n) { return n < 10 ? "0" + n : String(n) }
        const dmy = two(local.getDate()) + "/" + two(local.getMonth() + 1) + "/2026 " + two(local.getHours()) + ":" + two(local.getMinutes())
        compare(UnsafeText.dateText(raw, "02/01/2006 15:04"), dmy)
        compare(UnsafeText.dateText(raw, "02.01.2006 15:04"), dmy.replace(/\//g, "."))
        compare(UnsafeText.dateText(raw, "2006-01-02 15:04").substring(0, 4), "2026")
        // Sin formato en el catálogo, el del motor; sin fecha, nada.
        compare(UnsafeText.dateText(raw, "format.datetime_minutes"), dmy)
        compare(UnsafeText.dateText("", "02/01/2006 15:04"), "")
        compare(UnsafeText.dateText("no es una fecha", "02/01/2006 15:04"), "")
    }
}
