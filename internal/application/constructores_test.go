// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"testing"

	"grxfirma/internal/domain"
)

func TestParseSignatureFormat_AdmiteXMLDSig(t *testing.T) {
	got, err := ParseSignatureFormat("xmldsig")
	if err != nil {
		t.Fatalf("ParseSignatureFormat(xmldsig) devolvió error: %v", err)
	}
	if got != domain.SignatureFormat("XMLdSig") {
		t.Fatalf("ParseSignatureFormat(xmldsig) = %q, want %q", got, "XMLdSig")
	}
}

func TestParseSignatureFormat_NoBloqueaFormatosNuevos(t *testing.T) {
	got, err := ParseSignatureFormat("ODF")
	if err != nil {
		t.Fatalf("ParseSignatureFormat(ODF) devolvió error: %v", err)
	}
	if got != domain.SignatureFormat("ODF") {
		t.Fatalf("ParseSignatureFormat(ODF) = %q, want %q", got, "ODF")
	}
	if got, err = ParseSignatureFormat("odf"); err != nil || got != domain.SignatureFormat("ODF") {
		t.Fatalf("ParseSignatureFormat(odf) = %q, %v", got, err)
	}
	if got, err = ParseSignatureFormat("ooxml"); err != nil || got != domain.SignatureFormat("OOXML") {
		t.Fatalf("ParseSignatureFormat(ooxml) = %q, %v", got, err)
	}
	if got, err = ParseSignatureFormat("FacturaE"); err != nil || got != domain.SignatureFormat("FacturaE") {
		t.Fatalf("ParseSignatureFormat(FacturaE) = %q, %v", got, err)
	}
	if got, err = ParseSignatureFormat("asic-xades"); err != nil || got != domain.SignatureFormat("ASiC-XAdES") {
		t.Fatalf("ParseSignatureFormat(asic-xades) = %q, %v", got, err)
	}
	if got, err = ParseSignatureFormat("CMS/PKCS#7"); err != nil || got != domain.FormatCAdES {
		t.Fatalf("ParseSignatureFormat(CMS/PKCS#7) = %q, %v", got, err)
	}
}

func TestNewProcessBatchCommandWithDefaults_AplicaPlantillaYRespetaOverride(t *testing.T) {
	defaults := map[string]string{
		"visibleSeal":      "true",
		"visibleSealRectX": "36",
		"visibleSealRectY": "40",
		"visibleSealRectW": "220",
		"visibleSealRectH": "70",
		"page":             "all",
	}
	override := map[string]string{
		"page":             "1,3-5",
		"visibleSealRectX": "72",
	}
	cmd, err := NewProcessBatchCommandWithDefaults([]BatchItemInput{
		{
			Nombre:    "global.pdf",
			Contenido: []byte("%PDF-global"),
			TipoMIME:  "application/pdf",
			Formato:   "PAdES",
			Accion:    "sign",
		},
		{
			Nombre:    "override.pdf",
			Contenido: []byte("%PDF-override"),
			TipoMIME:  "application/pdf",
			Formato:   "PAdES",
			Accion:    "sign",
			Opciones:  override,
		},
	}, defaults)
	if err != nil {
		t.Fatalf("NewProcessBatchCommandWithDefaults() error = %v", err)
	}

	if got := cmd.Jobs[0].Options["page"]; got != "all" {
		t.Fatalf("page global = %q, want all", got)
	}
	if got := cmd.Jobs[1].Options["page"]; got != "1,3-5" {
		t.Fatalf("page override = %q, want 1,3-5", got)
	}
	if got := cmd.Jobs[1].Options["visibleSealRectX"]; got != "72" {
		t.Fatalf("rectX override = %q, want 72", got)
	}
	if got := cmd.Jobs[1].Options["visibleSealRectW"]; got != "220" {
		t.Fatalf("rectW heredado = %q, want 220", got)
	}

	defaults["page"] = "2"
	override["visibleSealRectX"] = "99"
	cmd.Jobs[0].Options["visibleSealRectW"] = "1"
	if got := cmd.Jobs[0].Options["page"]; got != "all" {
		t.Fatalf("la plantilla quedó aliased: page = %q", got)
	}
	if got := cmd.Jobs[1].Options["visibleSealRectX"]; got != "72" {
		t.Fatalf("el override quedó aliased: rectX = %q", got)
	}
	if got := cmd.Jobs[1].Options["visibleSealRectW"]; got != "220" {
		t.Fatalf("los trabajos comparten opciones: rectW = %q", got)
	}
}

func TestNewProcessBatchCommand_ConservaContratoSinDefaults(t *testing.T) {
	options := map[string]string{"page": "2,4"}
	cmd, err := NewProcessBatchCommand([]BatchItemInput{{
		Nombre:    "documento.pdf",
		Contenido: []byte("%PDF"),
		TipoMIME:  "application/pdf",
		Formato:   "PAdES",
		Accion:    "sign",
		Opciones:  options,
	}})
	if err != nil {
		t.Fatalf("NewProcessBatchCommand() error = %v", err)
	}
	if got := cmd.Jobs[0].Options["page"]; got != "2,4" {
		t.Fatalf("page = %q, want 2,4", got)
	}
	options["page"] = "all"
	if got := cmd.Jobs[0].Options["page"]; got != "2,4" {
		t.Fatalf("el constructor compatible no hizo copia defensiva: page = %q", got)
	}
}

func TestParseSignatureFormat_VariantesTrifasicasDeAfirma(t *testing.T) {
	casos := map[string]domain.SignatureFormat{
		"CAdEStri":    domain.FormatCAdES,
		"XAdEStri":    domain.FormatXAdES,
		"PAdEStri":    domain.FormatPAdES,
		"Adobe PDF":   domain.FormatPAdES,
		"FacturaEtri": domain.SignatureFormat("FacturaE"),
		// Nombres literales de AOSignConstants de AutoFirma Java.
		"XAdES Enveloped":            domain.FormatXAdES,
		"XAdES Detached":             domain.FormatXAdES,
		"Adobe PDF TriPhase":         domain.FormatPAdES,
		"Factura-e":                  domain.SignatureFormat("FacturaE"),
		"ODF (Open Document Format)": domain.SignatureFormat("ODF"),
		"OOXML (Office Open XML)":    domain.SignatureFormat("OOXML"),
		"XAdES-ASiC-S":               domain.SignatureFormat("ASiC-XAdES"),
		"XMLDSig Enveloped":          domain.SignatureFormat("XMLdSig"),
		"CAdES-ASiC-S":               domain.SignatureFormat("ASiC-CAdES"),
		"NONE":                       domain.SignatureFormat("PKCS1"),
		"PKCS#1":                     domain.SignatureFormat("PKCS1"),
	}
	for entrada, want := range casos {
		got, err := ParseSignatureFormat(entrada)
		if err != nil || got != want {
			t.Fatalf("ParseSignatureFormat(%q) = %q, %v; want %q", entrada, got, err, want)
		}
	}
}
