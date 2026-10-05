// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtTest
import "../../qml/VerificationSigners.js" as Signers

TestCase {
    name: "VerificationSigners"

    // DN tal como lo entrega el motor en la verificación de 0.0.118.
    readonly property string syntheticDn: "SERIALNUMBER=IDCES-00000000T,CN=PRUEBA SINTETICA QA - 00000000T,O=Pruebas Sinteticas,C=ES,2.5.4.4=#130953494e544554494341,2.5.4.42=#1306505255454241"

    function test_common_name_identifier_and_organization() {
        const parsed = Signers.parseDistinguishedName(syntheticDn)
        compare(parsed.common, "PRUEBA SINTETICA QA - 00000000T")
        compare(parsed.identifier, "IDCES-00000000T")
        compare(parsed.organization, "Pruebas Sinteticas")
        compare(Signers.readableName(syntheticDn), "PRUEBA SINTETICA QA - 00000000T")
    }

    function test_hex_attributes_are_decoded() {
        compare(Signers.decodeDerString("1306505255454241"), "PRUEBA")
        compare(Signers.decodeDerString("0c064d55c3914f5a"), "MUÑOZ")
        compare(Signers.decodeDerString("1e04004d00d1"), "MÑ")
        // Longitud que no cuadra o etiqueta que no es texto: nada.
        compare(Signers.decodeDerString("130550"), "")
        compare(Signers.decodeDerString("020101"), "")
        compare(Signers.decodeDerString("zz"), "")
    }

    function test_given_name_and_surname_when_no_cn() {
        const dn = "2.5.4.42=#1306505255454241,2.5.4.4=#13084d415254494e4553,SERIALNUMBER=IDCES-1"
        compare(Signers.readableName(dn), "PRUEBA MARTINES")
    }

    function test_escaped_values() {
        compare(Signers.readableName("CN=Ayuntamiento\\, Servicio,O=X"), "Ayuntamiento, Servicio")
        compare(Signers.readableName("CN=Mu\\c3\\b1oz"), "Muñoz")
        compare(Signers.readableName("C=ES"), "C=ES")
    }

    function test_issuer_with_organization() {
        compare(Signers.readableIssuer("CN=AC FNMT Usuarios,OU=Ceres,O=FNMT-RCM,C=ES"), "AC FNMT Usuarios (FNMT-RCM)")
        compare(Signers.readableIssuer("CN=FNMT-RCM,O=FNMT-RCM"), "FNMT-RCM")
    }

    function test_signing_time_only_with_known_source() {
        compare(Signers.signingTimeText("2026-10-05T08:30:00Z", "", "02/01/2006 15:04:05"), "")
        compare(Signers.signingTimeText("2026-10-05T08:30:00Z", "otro", "02/01/2006 15:04:05"), "")
        compare(Signers.signingTimeText("ayer", "timestamp", "02/01/2006 15:04:05"), "")
        const text = Signers.signingTimeText("2026-10-05T08:30:00Z", "timestamp", "2006-01-02 15:04:05")
        verify(/^2026-10-0[45] \d{2}:\d{2}:00 \(UTC([+-]\d{2}:\d{2})?\)$/.test(text), text)
    }

    function test_go_layout_tokens() {
        const date = new Date(2026, 0, 2, 3, 4, 5)
        verify(Signers.formatGoLayout(date, "02.01.2006 15:04:05").indexOf("02.01.2026 03:04:05 (UTC") === 0)
    }
}
