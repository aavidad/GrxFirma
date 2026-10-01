// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"testing"
)

func TestXMLAssessment_SignedInfoSpoofCannotReplaceAuthenticatedReferences(t *testing.T) {
	key, cert := certForTest(t, "Synthetic XML authenticated fields QA")
	original := []byte("contenido sintético autorizado")
	changed := []byte("contenido sintético manipulado")
	document := assessmentSignature(t, key, cert, `<root>`, `</root>`, "one", original, false, false)
	if result, err := assessDocument(t, document, original); err != nil || !result.Valid {
		t.Fatalf("precondición: firma original no válida: %+v %v", result, err)
	}
	info, err := extractXMLLiteralElement(document, "ds:SignedInfo")
	if err != nil {
		t.Fatal(err)
	}
	var parsed signedInfoForVerify
	if err := xml.Unmarshal(info, &parsed); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(changed)
	// No se vuelve a usar la clave. El atacante inserta campos homónimos en
	// otro namespace y un digest elegido por él, delante del SignedInfo real.
	fake := bytes.Replace(info, []byte(nsXMLDSig), []byte("urn:qa:unsigned-lookalike"), 1)
	fake = bytes.Replace(fake, []byte(parsed.References[0].DigestValue), []byte(base64.StdEncoding.EncodeToString(digest[:])), 1)
	attack := bytes.Replace(document, info, append(append([]byte(nil), fake...), info...), 1)
	result, err := assessDocument(t, attack, changed)
	if err == nil {
		t.Fatalf("referencias no autenticadas sustituyen las firmadas: %+v", result)
	}
}
