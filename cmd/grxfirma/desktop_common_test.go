// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func TestExtraerModoDesktop(t *testing.T) {
	desktop, resto, err := extraerModoDesktop([]string{"-desktop", "-entrada", "doc.pdf"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !desktop.habilitado {
		t.Fatal("se esperaba desktop=true")
	}
	if desktop.frontend != "fyne" {
		t.Fatalf("frontend inesperado: %s", desktop.frontend)
	}
	if len(resto) != 2 || resto[0] != "-entrada" || resto[1] != "doc.pdf" {
		t.Fatalf("resto inesperado: %#v", resto)
	}
}

func TestExtraerModoDesktop_QT(t *testing.T) {
	desktop, resto, err := extraerModoDesktop([]string{"-qt", "-entrada", "doc.pdf"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !desktop.habilitado {
		t.Fatal("se esperaba desktop=true")
	}
	if desktop.frontend != "qt" {
		t.Fatalf("frontend inesperado: %s", desktop.frontend)
	}
	if len(resto) != 2 || resto[0] != "-entrada" || resto[1] != "doc.pdf" {
		t.Fatalf("resto inesperado: %#v", resto)
	}
}

func TestExtraerModoDesktop_FrontendExplcito(t *testing.T) {
	desktop, resto, err := extraerModoDesktop([]string{"-frontend", "qt", "-entrada", "doc.pdf"})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !desktop.habilitado {
		t.Fatal("se esperaba desktop=true")
	}
	if desktop.frontend != "qt" {
		t.Fatalf("frontend inesperado: %s", desktop.frontend)
	}
	if len(resto) != 2 || resto[0] != "-entrada" || resto[1] != "doc.pdf" {
		t.Fatalf("resto inesperado: %#v", resto)
	}
}

func TestExtraerModoDesktop_FrontendNoSoportado(t *testing.T) {
	_, _, err := extraerModoDesktop([]string{"-frontend", "gio"})
	if err == nil {
		t.Fatal("se esperaba error para frontend no soportado")
	}
}

func TestPrepararComandoBackendQt_TransfierePasswordSoloPorPipe(t *testing.T) {
	const secret = "password-privado-qt"
	cmd := prepararComandoBackendQt("/ruta/grxfirma-gui", "/ruta/identidad.p12", secret)
	joinedArgs := strings.Join(cmd.Args, " ")
	if strings.Contains(joinedArgs, secret) {
		t.Fatalf("argv contiene la contraseña: %#v", cmd.Args)
	}
	if len(cmd.Args) != 4 ||
		cmd.Args[1] != "--p12-password-stdin" ||
		cmd.Args[2] != "--p12" ||
		cmd.Args[3] != "/ruta/identidad.p12" {
		t.Fatalf("argv backend inesperado: %#v", cmd.Args)
	}
	raw, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatalf("ReadAll(stdin): %v", err)
	}
	if string(raw) != secret+"\n" {
		t.Fatalf("stdin privado inesperado: %q", raw)
	}
}

func TestFiltrarSecretosEntornoHijo(t *testing.T) {
	got := filtrarSecretosEntornoHijo([]string{
		"PATH=/usr/bin",
		"GRXFIRMA_PKCS12_PASSWORD=secreto-p12",
		"GRXFIRMA_REST_TOKEN=token",
		"GRXFIRMA_PROTECTION_SECRET_B64=clave",
		"LANG=es_ES.UTF-8",
	})
	if len(got) != 2 || got[0] != "PATH=/usr/bin" || got[1] != "LANG=es_ES.UTF-8" {
		t.Fatalf("entorno filtrado inesperado: %#v", got)
	}
}

func TestQtBackendCandidates_SeleccionaBackendGoReal(t *testing.T) {
	baseDir := filepath.Join("tmp", "grxfirma")
	candidatos := qtBackendCandidates(baseDir)
	esperado := filepath.Join(baseDir, "grxfirma-gui")
	encontrado := false
	for _, candidato := range candidatos {
		if candidato == esperado {
			encontrado = true
			break
		}
	}
	if !encontrado {
		t.Fatalf("no se encontró el candidato esperado %q en %#v", esperado, candidatos)
	}
	for _, candidato := range candidatos {
		if strings.Contains(candidato, "gui-qml") {
			t.Fatalf("el lanzador seleccionaría el frontend sin backend: %#v", candidatos)
		}
	}
}

func TestInferirFormatoDesktop(t *testing.T) {
	casos := []struct {
		raw   string
		ruta  string
		esper string
	}{
		{raw: "pades", ruta: "x.bin", esper: "PAdES"},
		{raw: "facturae", ruta: "x.bin", esper: "FacturaE"},
		{raw: "asic-xades", ruta: "x.bin", esper: "ASiC-XAdES"},
		{raw: "", ruta: "/tmp/doc.pdf", esper: "PAdES"},
		{raw: "", ruta: "/tmp/doc.asics", esper: "ASiC-XAdES"},
		{raw: "", ruta: "/tmp/doc.dsig", esper: "XMLdSig"},
		{raw: "", ruta: "/tmp/doc.xml", esper: "XAdES"},
		{raw: "", ruta: "/tmp/doc.odt", esper: "ODF"},
		{raw: "", ruta: "/tmp/doc.docx", esper: "OOXML"},
		{raw: "", ruta: "/tmp/doc.bin", esper: "CAdES"},
	}
	for _, caso := range casos {
		got := inferirFormatoDesktop(caso.raw, caso.ruta)
		if got != caso.esper {
			t.Fatalf("inferirFormatoDesktop(%q,%q)=%q want %q", caso.raw, caso.ruta, got, caso.esper)
		}
	}
}

func TestConstruirSalidaDesktop(t *testing.T) {
	if got := construirSalidaDesktop("/tmp/doc.pdf", domain.FormatPAdES); got != "/tmp/doc_firmado.pdf" {
		t.Fatalf("salida PAdES inesperada: %s", got)
	}
	if got := construirSalidaDesktop("/tmp/doc.xml", domain.FormatXAdES); got != "/tmp/doc.xsig" {
		t.Fatalf("salida XAdES inesperada: %s", got)
	}
	if got := construirSalidaDesktop("/tmp/doc.xml", domain.SignatureFormat("FacturaE")); got != "/tmp/doc_firmada.xml" {
		t.Fatalf("salida FacturaE inesperada: %s", got)
	}
	if got := construirSalidaDesktop("/tmp/doc.bin", domain.SignatureFormat("ASiC-XAdES")); got != "/tmp/doc.asics" {
		t.Fatalf("salida ASiC-XAdES inesperada: %s", got)
	}
	if got := construirSalidaDesktop("/tmp/doc.bin", domain.FormatCAdES); got != "/tmp/doc.csig" {
		t.Fatalf("salida CAdES inesperada: %s", got)
	}
}

func TestInferirOriginalRelacionado(t *testing.T) {
	dir := t.TempDir()
	firma := filepath.Join(dir, "documento.csig")
	original := filepath.Join(dir, "documento.txt")
	if err := os.WriteFile(firma, []byte("firma"), 0o600); err != nil {
		t.Fatalf("escribiendo firma: %v", err)
	}
	if err := os.WriteFile(original, []byte("original"), 0o600); err != nil {
		t.Fatalf("escribiendo original: %v", err)
	}

	if got := inferirOriginalRelacionado(firma); got != original {
		t.Fatalf("inferirOriginalRelacionado()=%q want %q", got, original)
	}
}

func TestInferirOriginalRelacionado_IgnoraPades(t *testing.T) {
	if got := inferirOriginalRelacionado("/tmp/documento_firmado.pdf"); got != "" {
		t.Fatalf("se esperaba vacío para PAdES, obtenido %q", got)
	}
}

func TestNombreDestinoP12Desktop(t *testing.T) {
	ref := domain.CertificateRef{Fingerprint: "AABBCC"}
	if got := nombreDestinoP12Desktop(ref); got != "certificado-aabbcc.p12" {
		t.Fatalf("nombre inesperado: %q", got)
	}
}

func TestNombreDestinoP12Desktop_SinHuella(t *testing.T) {
	if got := nombreDestinoP12Desktop(domain.CertificateRef{}); got != "certificado-importado.p12" {
		t.Fatalf("nombre inesperado sin huella: %q", got)
	}
}
