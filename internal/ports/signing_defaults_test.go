// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"encoding/base64"
	"testing"
)

func TestAplicarOpcionesFirmaPredeterminadas_ProyectaSubfilterPAdES(t *testing.T) {
	subfilter := "adobe"
	doc := DocumentoConfiguracionUsuario{
		PAdES: ConfiguracionUsuarioPAdES{SubFilter: &subfilter},
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, "PAdES", nil)

	if got["subfilter"] != "adobe" {
		t.Fatalf("subfilter = %q; want adobe", got["subfilter"])
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_RespetaOverrideSinDistinguirMayusculas(t *testing.T) {
	subfilter := "adobe"
	doc := DocumentoConfiguracionUsuario{
		PAdES: ConfiguracionUsuarioPAdES{SubFilter: &subfilter},
	}
	explicitas := map[string]string{"SubFilter": "etsi"}

	got := AplicarOpcionesFirmaPredeterminadas(doc, "pades", explicitas)

	if got["SubFilter"] != "etsi" {
		t.Fatalf("override explícito perdido: %#v", got)
	}
	if _, duplicada := got["subfilter"]; duplicada {
		t.Fatalf("se añadió una clave duplicada con distinta capitalización: %#v", got)
	}
	if explicitas["SubFilter"] != "etsi" || len(explicitas) != 1 {
		t.Fatalf("se mutó el mapa de entrada: %#v", explicitas)
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_NoProyectaPAdESEnOtroFormato(t *testing.T) {
	subfilter := "adobe"
	doc := DocumentoConfiguracionUsuario{
		PAdES: ConfiguracionUsuarioPAdES{SubFilter: &subfilter},
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, "CAdES", nil)

	if len(got) != 0 {
		t.Fatalf("opciones PAdES filtradas a CAdES: %#v", got)
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_NoProyectaSubfilterTipadoInvalido(t *testing.T) {
	subfilter := "adbe.pkcs7.sha1"
	doc := DocumentoConfiguracionUsuario{
		PAdES: ConfiguracionUsuarioPAdES{SubFilter: &subfilter},
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, "PAdES", nil)

	if len(got) != 0 {
		t.Fatalf("se proyectó un subfilter no admitido: %#v", got)
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_ProyectaFacturaECompletaNormalizada(t *testing.T) {
	version := " 3.1 "
	policyID := " urn:oid:1.2.3.4 "
	policyHash := " Ohixl6upD6av8N7pEvDABhEL6hM= "
	qualifier := " https://www.facturae.gob.es/politica.html "
	role := " Emisor "
	city := " Granada "
	province := " Granada "
	postalCode := " 18001 "
	country := " ES "
	doc := DocumentoConfiguracionUsuario{
		FacturaE: ConfiguracionUsuarioFacturaE{
			PolicyVersion:   &version,
			PolicyID:        &policyID,
			PolicyHash:      &policyHash,
			PolicyQualifier: &qualifier,
			SignerRole:      &role,
			City:            &city,
			Province:        &province,
			PostalCode:      &postalCode,
			Country:         &country,
		},
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, " FacturaE ", nil)

	want := map[string]string{
		"facturaePolicyVersion":         "3.1",
		"policyIdentifier":              "urn:oid:1.2.3.4",
		"policyIdentifierHash":          "Ohixl6upD6av8N7pEvDABhEL6hM=",
		"policyQualifier":               "https://www.facturae.gob.es/politica.html",
		"signerClaimedRole":             "emisor",
		"signatureProductionCity":       "Granada",
		"signatureProductionProvince":   "Granada",
		"signatureProductionPostalCode": "18001",
		"signatureProductionCountry":    "ES",
	}
	if len(got) != len(want) {
		t.Fatalf("opciones FacturaE = %#v; want %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q; want %q; opciones=%#v", key, got[key], value, got)
		}
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_FacturaERespetaOverrideYNoMezclaPoliticas(t *testing.T) {
	version := "3.1"
	policyID := "urn:oid:1.2.3.4"
	policyHash := "Ohixl6upD6av8N7pEvDABhEL6hM="
	qualifier := "https://www.facturae.gob.es/politica.html"
	role := "emisor"
	city := "Granada"
	province := "Granada"
	doc := DocumentoConfiguracionUsuario{
		FacturaE: ConfiguracionUsuarioFacturaE{
			PolicyVersion:   &version,
			PolicyID:        &policyID,
			PolicyHash:      &policyHash,
			PolicyQualifier: &qualifier,
			SignerRole:      &role,
			City:            &city,
			Province:        &province,
		},
	}
	explicitas := map[string]string{
		" PolicyIdentifier ":      "urn:oid:9.9",
		"SIGNERCLAIMEDROLE":       "receptor",
		"signatureProductionCity": "Málaga",
		"opcionAjena":             "conservar",
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, "facturae", explicitas)

	for _, key := range []string{
		"facturaePolicyVersion",
		"policyIdentifier",
		"policyIdentifierHash",
		"policyQualifier",
		"signerClaimedRole",
	} {
		if _, duplicate := got[key]; duplicate {
			t.Fatalf("se añadió %q junto a un override explícito: %#v", key, got)
		}
	}
	if got[" PolicyIdentifier "] != "urn:oid:9.9" ||
		got["SIGNERCLAIMEDROLE"] != "receptor" ||
		got["signatureProductionCity"] != "Málaga" ||
		got["signatureProductionProvince"] != "Granada" ||
		got["opcionAjena"] != "conservar" {
		t.Fatalf("precedencia FacturaE inesperada: %#v", got)
	}
	if len(explicitas) != 4 || explicitas[" PolicyIdentifier "] != "urn:oid:9.9" {
		t.Fatalf("se mutó el mapa de entrada: %#v", explicitas)
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_NoProyectaFacturaETipadaInvalida(t *testing.T) {
	version := "3.2"
	policyID := "javascript:alert(1)"
	policyHash := "no-es-un-digest-sha1"
	qualifier := "file:///tmp/politica"
	role := "administrador"
	city := "Granada\x00inyectada"
	doc := DocumentoConfiguracionUsuario{
		FacturaE: ConfiguracionUsuarioFacturaE{
			PolicyVersion:   &version,
			PolicyID:        &policyID,
			PolicyHash:      &policyHash,
			PolicyQualifier: &qualifier,
			SignerRole:      &role,
			City:            &city,
		},
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, "FacturaE", nil)

	if len(got) != 0 {
		t.Fatalf("se proyectaron defaults FacturaE inválidos: %#v", got)
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_ProyectaPoliticaXAdESCompleta(t *testing.T) {
	policyID := " urn:oid:1.2.3.4.5 "
	policyHash := base64.StdEncoding.EncodeToString(make([]byte, 32))
	policyHashAlgorithm := " SHA-256 "
	policyQualifier := " https://sede.example/politica.pdf "
	doc := DocumentoConfiguracionUsuario{
		XAdES: ConfiguracionUsuarioXAdES{
			PolicyID:            &policyID,
			PolicyHash:          &policyHash,
			PolicyHashAlgorithm: &policyHashAlgorithm,
			PolicyQualifier:     &policyQualifier,
		},
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, " XAdES ", nil)

	want := map[string]string{
		"policyIdentifier":              "urn:oid:1.2.3.4.5",
		"policyIdentifierHash":          policyHash,
		"policyIdentifierHashAlgorithm": "http://www.w3.org/2001/04/xmlenc#sha256",
		"policyQualifier":               "https://sede.example/politica.pdf",
	}
	if len(got) != len(want) {
		t.Fatalf("política XAdES = %#v; want %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q; want %q; opciones=%#v", key, got[key], value, got)
		}
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_XAdESRespetaPoliticaExplicita(t *testing.T) {
	policyID := "urn:oid:1.2.3.4.5"
	policyHash := base64.StdEncoding.EncodeToString(make([]byte, 32))
	policyHashAlgorithm := "SHA-256"
	policyQualifier := "https://sede.example/politica.pdf"
	doc := DocumentoConfiguracionUsuario{
		XAdES: ConfiguracionUsuarioXAdES{
			PolicyID:            &policyID,
			PolicyHash:          &policyHash,
			PolicyHashAlgorithm: &policyHashAlgorithm,
			PolicyQualifier:     &policyQualifier,
		},
	}
	explicitas := map[string]string{
		" XAdESPolicyIdentifier ": "urn:oid:9.9",
		"opcionAjena":             "conservar",
	}

	got := AplicarOpcionesFirmaPredeterminadas(doc, "xades", explicitas)

	if len(got) != len(explicitas) ||
		got[" XAdESPolicyIdentifier "] != "urn:oid:9.9" ||
		got["opcionAjena"] != "conservar" {
		t.Fatalf("precedencia XAdES inesperada: %#v", got)
	}
	if _, duplicated := got["policyIdentifier"]; duplicated {
		t.Fatalf("se mezcló la política persistida con el override: %#v", got)
	}
	if len(explicitas) != 2 {
		t.Fatalf("se mutó el mapa de entrada: %#v", explicitas)
	}
}

func TestAplicarOpcionesFirmaPredeterminadas_NoProyectaPoliticaXAdESIncompletaOInvalida(t *testing.T) {
	validID := "urn:oid:1.2.3.4.5"
	validHash := base64.StdEncoding.EncodeToString(make([]byte, 32))
	validAlgorithm := "SHA-256"
	invalidHash := base64.StdEncoding.EncodeToString(make([]byte, 20))
	invalidQualifier := "file:///tmp/politica.pdf"

	tests := []struct {
		name   string
		config ConfiguracionUsuarioXAdES
	}{
		{
			name: "sin algoritmo",
			config: ConfiguracionUsuarioXAdES{
				PolicyID:   &validID,
				PolicyHash: &validHash,
			},
		},
		{
			name: "digest de longitud incompatible",
			config: ConfiguracionUsuarioXAdES{
				PolicyID:            &validID,
				PolicyHash:          &invalidHash,
				PolicyHashAlgorithm: &validAlgorithm,
			},
		},
		{
			name: "qualifier no remoto",
			config: ConfiguracionUsuarioXAdES{
				PolicyID:            &validID,
				PolicyHash:          &validHash,
				PolicyHashAlgorithm: &validAlgorithm,
				PolicyQualifier:     &invalidQualifier,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := DocumentoConfiguracionUsuario{XAdES: tt.config}
			if got := AplicarOpcionesFirmaPredeterminadas(doc, "XAdES", nil); len(got) != 0 {
				t.Fatalf("se proyectó una política XAdES inválida: %#v", got)
			}
		})
	}
}

func TestNecesitaOpcionesFirmaPredeterminadas_PorFormatoYPrecedencia(t *testing.T) {
	tests := []struct {
		name       string
		formato    string
		explicitas map[string]string
		want       bool
	}{
		{name: "pades sin opciones", formato: "PAdES", want: true},
		{name: "otro formato", formato: "CAdES", want: false},
		{name: "subfilter explicito", formato: "PAdES", explicitas: map[string]string{"SubFilter": "adobe"}, want: false},
		{name: "pdfsubfilter explicito", formato: "pades", explicitas: map[string]string{" PDFSubFilter ": "etsi"}, want: false},
		{name: "xades sin opciones", formato: "XAdES", want: true},
		{name: "xades con politica explicita", formato: "xades", explicitas: map[string]string{" XAdESPolicyIdentifier ": "urn:oid:9.9"}, want: false},
		{name: "facturae sin opciones", formato: "FacturaE", want: true},
		{
			name:    "facturae politica explicita pero faltan metadatos",
			formato: "FacturaE",
			explicitas: map[string]string{
				"PolicyIdentifier": "urn:oid:9.9",
			},
			want: true,
		},
		{
			name:    "facturae completa explicita",
			formato: "facturae",
			explicitas: map[string]string{
				"PolicyIdentifier":              "urn:oid:9.9",
				"signerClaimedRole":             "receptor",
				"signatureProductionCity":       "Málaga",
				"signatureProductionProvince":   "Málaga",
				"signatureProductionPostalCode": "29001",
				"signatureProductionCountry":    "ES",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NecesitaOpcionesFirmaPredeterminadas(tt.formato, tt.explicitas); got != tt.want {
				t.Fatalf("NecesitaOpcionesFirmaPredeterminadas() = %t; want %t", got, tt.want)
			}
		})
	}
}
