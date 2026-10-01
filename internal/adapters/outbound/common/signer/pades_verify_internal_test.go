// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestExtractPDFSubFilter_AceptaAdobeDetached(t *testing.T) {
	t.Parallel()

	pdf := []byte("%PDF-1.4\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /Contents <AA> >>")
	subFilter, err := extractPDFSubFilter(pdf)
	if err != nil {
		t.Fatalf("extractPDFSubFilter devolvió error: %v", err)
	}
	if subFilter != "adbe.pkcs7.detached" {
		t.Fatalf("subFilter inesperado: %q", subFilter)
	}
}

func TestExtractPDFByteRange_AceptaWhitespaceFlexible(t *testing.T) {
	t.Parallel()

	pdf := []byte("%PDF-1.4\n<< /Type /Sig /ByteRange[\n0  100\t200 300\r] >>")
	byteRange, err := extractPDFByteRange(pdf)
	if err != nil {
		t.Fatalf("extractPDFByteRange devolvió error: %v", err)
	}
	want := [4]int{0, 100, 200, 300}
	if byteRange != want {
		t.Fatalf("ByteRange inesperado: %#v", byteRange)
	}
}

func TestExtractPDFSignatureCMS_AceptaHexConSaltos(t *testing.T) {
	t.Parallel()

	pdf := []byte("%PDF-1.4\n<< /Contents <AA55\n\tBB00\r\nCCDD> >>")
	cms, err := extractPDFSignatureCMS(pdf)
	if err != nil {
		t.Fatalf("extractPDFSignatureCMS devolvió error: %v", err)
	}
	want := []byte{0xAA, 0x55, 0xBB, 0x00, 0xCC, 0xDD}
	if string(cms) != string(want) {
		t.Fatalf("CMS inesperado: %x", cms)
	}
}

func TestValidatePDFSubFilter_RechazaNoSoportado(t *testing.T) {
	t.Parallel()

	if err := validatePDFSubFilter("adbe.pkcs7.sha1"); err == nil {
		t.Fatal("se esperaba error para SubFilter no soportado")
	}
}

func TestExtractPDFEmbeddedSignatures_ExtraeTodasLasFirmas(t *testing.T) {
	t.Parallel()

	pdf := pdfConFirmasEmbebidasPrueba(t,
		"ETSI.CAdES.detached", "AA55",
		"adbe.pkcs7.detached", "BB66",
	)
	signatures, err := extractPDFEmbeddedSignatures(pdf)
	if err != nil {
		t.Fatalf("extractPDFEmbeddedSignatures devolvió error: %v", err)
	}
	if len(signatures) != 2 {
		t.Fatalf("número de firmas inesperado: %d", len(signatures))
	}
	if signatures[0].SubFilter != "ETSI.CAdES.detached" {
		t.Fatalf("subFilter[0] inesperado: %q", signatures[0].SubFilter)
	}
	if signatures[1].SubFilter != "adbe.pkcs7.detached" {
		t.Fatalf("subFilter[1] inesperado: %q", signatures[1].SubFilter)
	}
	if signatures[0].ByteRange[0] != 0 || signatures[0].RevisionEnd != len(pdf) {
		t.Fatalf("ByteRange[0] inesperado: %#v (fin=%d)", signatures[0].ByteRange, signatures[0].RevisionEnd)
	}
	if signatures[1].ByteRange[0] != 0 || signatures[1].RevisionEnd != len(pdf) {
		t.Fatalf("ByteRange[1] inesperado: %#v (fin=%d)", signatures[1].ByteRange, signatures[1].RevisionEnd)
	}
	if string(signatures[0].CMSDER) != string([]byte{0xAA, 0x55}) {
		t.Fatalf("CMS[0] inesperado: %x", signatures[0].CMSDER)
	}
	if string(signatures[1].CMSDER) != string([]byte{0xBB, 0x66}) {
		t.Fatalf("CMS[1] inesperado: %x", signatures[1].CMSDER)
	}
}

func TestExtractPDFSignedBytes_RechazaMaxIntSinPanic(t *testing.T) {
	t.Parallel()

	pdf := []byte("%PDF-1.4\n")
	for _, byteRange := range [][4]int{
		{0, math.MaxInt, math.MaxInt, math.MaxInt},
		{0, 1, math.MaxInt, 1},
		{0, 1, 2, math.MaxInt},
	} {
		if _, err := extractPDFSignedBytes(pdf, byteRange); err == nil {
			t.Fatalf("se esperaba error para ByteRange %#v", byteRange)
		}
	}
}

func TestExtractPDFEmbeddedSignatures_RechazaHuecoAjenoAContents(t *testing.T) {
	t.Parallel()

	pdf := pdfConFirmasEmbebidasPrueba(t, "ETSI.CAdES.detached", "AA55")
	byteRange, err := extractPDFByteRange(pdf)
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte(fmt.Sprintf("%010d", byteRange[1]))
	idx := strings.Index(string(pdf), string(marker))
	if idx < 0 {
		t.Fatal("no se encontró el primer tramo para manipularlo")
	}
	copy(pdf[idx:idx+len(marker)], []byte(fmt.Sprintf("%010d", byteRange[1]-1)))

	if _, err := extractPDFEmbeddedSignatures(pdf); err == nil || !strings.Contains(err.Error(), "no coincide exactamente con /Contents") {
		t.Fatalf("error inesperado para hueco falso: %v", err)
	}
}

func TestExtractPDFEmbeddedSignatures_LimitaNumeroDeFirmas(t *testing.T) {
	t.Parallel()

	pdf := []byte("%PDF-1.4\n" + strings.Repeat("/ByteRange ", maxPDFEmbeddedSignatures+1))
	if _, err := extractPDFEmbeddedSignatures(pdf); err == nil || !strings.Contains(err.Error(), "demasiadas firmas") {
		t.Fatalf("error inesperado para exceso de firmas: %v", err)
	}
}

func TestExtractPDFByteRange_LimitaLongitudDelToken(t *testing.T) {
	t.Parallel()

	pdf := []byte("<< /ByteRange [" + strings.Repeat(" ", maxPDFByteRangeText) + "0 1 2 3] >>")
	if _, err := extractPDFByteRange(pdf); err == nil || !strings.Contains(err.Error(), "tamaño máximo") {
		t.Fatalf("error inesperado para ByteRange excesivo: %v", err)
	}
}

func TestParsePDFDictionaryRanges_LimitaAnidamiento(t *testing.T) {
	t.Parallel()

	pdf := strings.Repeat("<<", maxPDFDictionaryNesting+1)
	if _, err := parsePDFDictionaryRanges(pdf); err == nil || !strings.Contains(err.Error(), "anidamiento") {
		t.Fatalf("error inesperado para anidamiento excesivo: %v", err)
	}
}

func FuzzExtractPDFEmbeddedSignatures_NoPanic(f *testing.F) {
	f.Add([]byte("%PDF-1.4\n<< /Type /Sig /ByteRange [0 1 2 3] /Contents <AA> >>"))
	f.Add([]byte("<<"))
	f.Add([]byte("/ByteRange [0 9223372036854775807 1 1]"))
	f.Fuzz(func(t *testing.T, pdf []byte) {
		_, _ = extractPDFEmbeddedSignatures(pdf)
	})
}

func TestMergeVerificationAspect_PrefiereValidSobreUnknown(t *testing.T) {
	t.Parallel()

	base := domain.VerificationAspect{
		Status: domain.VerificationStatusUnknown,
		Reason: "sin evaluar",
	}
	next := domain.VerificationAspect{
		Status:  domain.VerificationStatusValid,
		Reason:  "certificado firmante vigente",
		Details: []string{"subject=CN=Prueba"},
	}

	merged := mergeVerificationAspect(base, next)
	if merged.Status != domain.VerificationStatusValid {
		t.Fatalf("status fusionado = %q, want %q", merged.Status, domain.VerificationStatusValid)
	}
	if merged.Reason != "certificado firmante vigente" {
		t.Fatalf("reason fusionado = %q", merged.Reason)
	}
	if len(merged.Details) != 1 || merged.Details[0] != "subject=CN=Prueba" {
		t.Fatalf("details fusionados inesperados: %#v", merged.Details)
	}
}

func pdfConFirmasEmbebidasPrueba(t *testing.T, fields ...string) []byte {
	t.Helper()
	if len(fields) == 0 || len(fields)%2 != 0 {
		t.Fatalf("se esperaban pares subfilter/contents, obtenidos %d campos", len(fields))
	}
	const byteRangePlaceholder = "0000000000 0000000000 0000000000 0000000000"
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	for idx := 0; idx < len(fields); idx += 2 {
		fmt.Fprintf(&pdf,
			"<< /Type /Sig /ByteRange [%s] /SubFilter /%s /Contents <%s> >>\n",
			byteRangePlaceholder, fields[idx], fields[idx+1],
		)
	}
	raw := []byte(pdf.String())
	searchFrom := 0
	for idx := 0; idx < len(fields); idx += 2 {
		byteRangeMarker := []byte("/ByteRange [" + byteRangePlaceholder + "]")
		markerOffset := strings.Index(string(raw[searchFrom:]), string(byteRangeMarker))
		if markerOffset < 0 {
			t.Fatalf("no se encontró el placeholder ByteRange %d", idx/2+1)
		}
		markerOffset += searchFrom
		valueOffset := markerOffset + len("/ByteRange [")
		contentsMarker := []byte("/Contents <" + fields[idx+1] + ">")
		contentsOffset := strings.Index(string(raw[markerOffset:]), string(contentsMarker))
		if contentsOffset < 0 {
			t.Fatalf("no se encontró Contents %d", idx/2+1)
		}
		contentsFrom := markerOffset + contentsOffset + len("/Contents ")
		contentsTo := contentsFrom + len("<"+fields[idx+1]+">")
		replacement := fmt.Sprintf("%010d %010d %010d %010d", 0, contentsFrom, contentsTo, len(raw)-contentsTo)
		if len(replacement) != len(byteRangePlaceholder) {
			t.Fatalf("ByteRange de prueba demasiado grande: %q", replacement)
		}
		copy(raw[valueOffset:valueOffset+len(replacement)], replacement)
		searchFrom = contentsTo
	}
	return raw
}
