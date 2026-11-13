// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// Lote 13b, punto 4: documento electrónico ENI (NTI de Documento Electrónico)
// con la firma XAdES TF02 dentro, sobre enifile:contenido. Antes salía parcial
// porque los metadatos del documento, el atributo ref y TipoFirma no están
// firmados, como manda la norma. Ahora es válido con el aviso de lo que queda
// fuera; con cualquier otra estructura sigue siendo parcial.

const contenidoENIPrueba = `<enifile:contenido xmlns:enifile="` + nsENIContenido + `" Id="CONTENIDO_ES_L01180877_2026_X1"><enifile:ValorBinario>JVBERi0xLjcgdW5v</enifile:ValorBinario><enifile:NombreFormato>PDF</enifile:NombreFormato></enifile:contenido>`

func documentoENIConFirma(contenido, ref, metadatos, firma string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<enidoc:documento xmlns:enidoc="` + nsENIDocumento + `" xmlns:enifile="` + nsENIContenido + `" xmlns:enidocmeta="` + nsENIMetadatosDocument + `" xmlns:enids="` + nsENIFirma + `" Id="ES_L01180877_2026_X1">` +
		contenido +
		`<enidocmeta:metadatos>` + metadatos + `</enidocmeta:metadatos>` +
		`<enids:firmas><enids:firma Id="FIRMA_1" ref="` + ref + `"><enids:TipoFirma>TF02</enids:TipoFirma><enids:ContenidoFirma><enids:FirmaConCertificado>` +
		firma +
		`</enids:FirmaConCertificado></enids:ContenidoFirma></enids:firma></enids:firmas>` +
		`</enidoc:documento>`
}

const metadatosENIPrueba = `<enidocmeta:VersionNTI>` + nsENIDocumento + `</enidocmeta:VersionNTI><enidocmeta:Identificador>ES_L01180877_2026_X1</enidocmeta:Identificador><enidocmeta:Organo>L01180877</enidocmeta:Organo><enidocmeta:TipoDocumental>TD99</enidocmeta:TipoDocumental>`

func documentoENIFirmado(t *testing.T) string {
	t.Helper()
	priv, cert := certForTest(t, "Documento ENI")
	firma, err := FirmarNodoXAdES([]byte(contenidoENIPrueba), "CONTENIDO_ES_L01180877_2026_X1", &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return documentoENIConFirma(contenidoENIPrueba, "CONTENIDO_ES_L01180877_2026_X1", metadatosENIPrueba, string(firma))
}

func TestXAdESVerifier_DocumentoENIValidoConAviso(t *testing.T) {
	documento := documentoENIFirmado(t)
	vr, err := verificarXAdESTexto(t, documento)
	if err != nil || !vr.Valid || vr.Coverage != domain.CoberturaCompleta || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("documento ENI: err=%v valid=%v coverage=%q %v", err, vr.Valid, vr.Coverage, vr.Details)
	}
	if !strings.Contains(strings.Join(vr.Details, ";"), "fuera_de_firma_por_formato=metadatos,atributos:firma,TipoFirma") {
		t.Fatalf("el detalle debe decir qué queda fuera: %v", vr.Details)
	}
	if !contieneString(vr.Warnings, motivoFueraDeFirmaPorFormato) {
		t.Fatalf("falta el aviso: %v", vr.Warnings)
	}
}

func TestXAdESVerifier_DocumentoENIFueraDeNormaEsParcial(t *testing.T) {
	documento := documentoENIFirmado(t)
	casos := map[string]func(string) string{
		"elemento añadido en el documento": func(s string) string {
			return strings.Replace(s, "<enids:firmas>", `<enidocmeta:Nota>Pagar 9000 euros</enidocmeta:Nota><enids:firmas>`, 1)
		},
		"atributo añadido a enids:firma": func(s string) string {
			return strings.Replace(s, `<enids:firma Id="FIRMA_1"`, `<enids:firma importe="9000" Id="FIRMA_1"`, 1)
		},
		"TipoFirma con texto libre": func(s string) string {
			return strings.Replace(s, "<enids:TipoFirma>TF02</enids:TipoFirma>", "<enids:TipoFirma>Pagar 9000 euros</enids:TipoFirma>", 1)
		},
		"ref que no apunta al contenido": func(s string) string {
			return strings.Replace(s, `ref="CONTENIDO_`, `ref="OTRO_`, 1)
		},
		"elemento de otro espacio de nombres en metadatos": func(s string) string {
			return strings.Replace(s, "<enidocmeta:metadatos>", `<enidocmeta:metadatos><CONTENT xmlns="urn:x">Pagar 9000 euros</CONTENT>`, 1)
		},
		"texto en el documento": func(s string) string {
			return strings.Replace(s, "<enidocmeta:metadatos>", "Pagar 9000 euros<enidocmeta:metadatos>", 1)
		},
		"atributo añadido a la raíz": func(s string) string {
			return strings.Replace(s, `Id="ES_L01180877_2026_X1">`, `Id="ES_L01180877_2026_X1" importe="9000">`, 1)
		},
	}
	for nombre, cambio := range casos {
		t.Run(nombre, func(t *testing.T) {
			alterado := cambio(documento)
			if alterado == documento {
				t.Fatal("la variante no cambia el documento")
			}
			vr, err := verificarXAdESTexto(t, alterado)
			noValido(t, vr, err, nombre)
		})
	}
}

// E1-02 con la regla del documento ENI: la firma de la víctima sobre otro
// nodo, metida en un falso documento ENI con los datos del atacante en los
// metadatos, no sale válida.
func TestXAdESVerifier_FirmaAjenaEnFalsoDocumentoENI(t *testing.T) {
	priv, cert := certForTest(t, "Victima")
	nodo := `<v:Recibi xmlns:v="urn:victima" Id="R1">Recibí 10 euros</v:Recibi>`
	firma, err := FirmarNodoXAdES([]byte(nodo), "R1", &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}, nil)
	if err != nil {
		t.Fatal(err)
	}
	metadatosAtacante := `<enidocmeta:Identificador>La víctima autoriza un pago de 9000 euros</enidocmeta:Identificador>`

	// El nodo firmado en el sitio de enifile:contenido.
	vr, err := verificarXAdESTexto(t, documentoENIConFirma(nodo, "R1", metadatosAtacante, string(firma)))
	if err != nil {
		t.Fatalf("la firma ajena debería verificarse criptográficamente para que la prueba tenga sentido: %v", err)
	}
	noValido(t, vr, err, "firma ajena en falso documento ENI")
	if !strings.Contains(strings.Join(vr.Details, ";"), "metadatos") {
		t.Fatalf("los metadatos deben salir como contenido sin firmar: %v", vr.Details)
	}

	// El nodo firmado dentro de un enifile:contenido de verdad, con ref
	// correcto: la firma no cubre el contenido entero.
	contenido := `<enifile:contenido Id="C1">` + nodo + `</enifile:contenido>`
	vr, err = verificarXAdESTexto(t, documentoENIConFirma(contenido, "C1", metadatosAtacante, string(firma)))
	noValido(t, vr, err, "firma ajena dentro de enifile:contenido")
}
