// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// E1-06: la firma RSA tiene que autenticar el SignedInfo directo de la firma,
// el mismo cuyas referencias se evalúan. Aquí el atacante copia el SignedInfo
// legítimo de la víctima a un ds:Object delante y escribe un SignedInfo
// directo propio con el resumen de un documento inventado. Antes salía
// «compatibilidad histórica» con la víctima como firmante; tiene que ser un
// error de verificación.
func TestXMLAssessment_SignedInfoAnidadoLegitimoNoAutenticaElDirecto(t *testing.T) {
	firmado := string(operarXAdES(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", domain.ActionSign, map[string]string{"format": "XAdES Enveloped"}, "Victima"))
	legitimo := elementoLiteral(t, firmado, "<ds:SignedInfo", "</ds:SignedInfo>")
	apertura := elementoLiteral(t, firmado, "<ds:Signature", ">")

	atacante := strings.Replace(firmado, "<importe>10<", "<importe>99999<", 1)
	atacante = strings.Replace(atacante, apertura, apertura+`<ds:Object>`+legitimo+`</ds:Object>`, 1)

	// El resumen de la referencia URI="" del SignedInfo directo pasa a ser el
	// del documento inventado (la firma queda excluida por enveloped).
	partes, err := extractContextualXMLSignatures([]byte(atacante))
	if err != nil || len(partes) != 1 {
		t.Fatalf("firmas: %v %d", err, len(partes))
	}
	var info signedInfoForVerify
	if err := xml.Unmarshal([]byte(legitimo), &info); err != nil {
		t.Fatal(err)
	}
	var datos referenceForVerify
	for _, ref := range info.References {
		if strings.TrimSpace(ref.URI) == "" {
			datos = ref
		}
	}
	resumen, err := declaredReferenceDigest(partes[0].context, datos, []byte(atacante))
	if err != nil {
		t.Fatal(err)
	}
	directo := strings.Replace(legitimo, datos.DigestValue, base64.StdEncoding.EncodeToString(resumen), 1)
	if directo == legitimo {
		t.Fatal("el SignedInfo directo debía cambiar")
	}
	ultimo := strings.LastIndex(atacante, legitimo)
	atacante = atacante[:ultimo] + directo + atacante[ultimo+len(legitimo):]

	vr, _, err := NewXAdESVerifier().Verify(context.Background(), domain.Document{Name: "p.xsig", Content: []byte(atacante)}, domain.CertificateChain{})
	if err == nil {
		t.Fatalf("se esperaba error de verificación, no %q (valid=%v integridad=%s)", vr.Reason, vr.Valid, vr.Integrity.Status)
	}
}

// E1-07: una instrucción de proceso añadida antes de la raíz de un XML firmado
// con URI="" (p. ej. una hoja XSLT que cambia lo que muestra el visor) no está
// firmada: la firma ya no puede salir válida. Un DOCTYPE, con o sin
// subconjunto interno, se rechaza al verificar un XML firmado.
func TestXAdESVerifier_PIInyectadaInvalida(t *testing.T) {
	firmado := string(operarXAdES(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", domain.ActionSign, map[string]string{"format": "XAdES Enveloped"}, "Victima"))
	verificar := func(contenido string) (domain.VerificationResult, error) {
		vr, _, err := NewXAdESVerifier().Verify(context.Background(), domain.Document{Name: "p.xsig", Content: []byte(contenido)}, domain.CertificateChain{})
		return vr, err
	}
	if vr, err := verificar(firmado); err != nil || !vr.Valid {
		t.Fatalf("la firma original debe ser válida: %v %s", err, vr.Reason)
	}
	raiz := strings.Index(firmado, "<pedido")
	for nombre, inyectado := range map[string]string{
		"pi":       `<?xml-stylesheet type="text/xsl" href="https://atacante.example/x.xsl"?>`,
		"doctype":  `<!DOCTYPE pedido [<!ATTLIST importe moneda CDATA "USD">]>`,
		"externo":  `<!DOCTYPE pedido SYSTEM "https://atacante.example/p.dtd">`,
		"pi_final": "",
	} {
		atacante := firmado[:raiz] + inyectado + firmado[raiz:]
		if nombre == "pi_final" {
			atacante = firmado + `<?xml-stylesheet type="text/xsl" href="x.xsl"?>`
		}
		vr, err := verificar(atacante)
		if err == nil && vr.Valid {
			t.Fatalf("%s: la firma no puede seguir siendo válida", nombre)
		}
	}
}
