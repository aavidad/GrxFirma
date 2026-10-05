// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type filteredDocumentPickerStub struct {
	doc    domain.Document
	filtro *ports.DocumentFilter
}

func (s filteredDocumentPickerStub) Pick(ctx context.Context) (domain.Document, error) {
	return s.PickFiltered(ctx, ports.DocumentFilter{})
}

func (s filteredDocumentPickerStub) PickFiltered(_ context.Context, filter ports.DocumentFilter) (domain.Document, error) {
	*s.filtro = filter
	return s.doc, nil
}

func solicitudSinDatos(formatoWeb string, formato domain.SignatureFormat, options map[string]string) afirmauri.Solicitud {
	params := url.Values{}
	if formatoWeb != "" {
		params.Set("format", formatoWeb)
	}
	return afirmauri.Solicitud{
		Operacion:    afirmauri.OperacionFirma,
		Formato:      formato,
		Options:      options,
		LegacyParams: params,
	}
}

func TestLegacyDocumentFilter_PorFormato(t *testing.T) {
	casos := []struct {
		nombre     string
		formatoWeb string
		formato    domain.SignatureFormat
		options    map[string]string
		want       []string
	}{
		{"pades", "PAdES", domain.FormatPAdES, nil, []string{"pdf"}},
		{"pades trifasico", "PAdEStri", domain.FormatPAdES, nil, []string{"pdf"}},
		{"xades cualquier fichero", "XAdES", domain.FormatXAdES, nil, nil},
		{"cades cualquier fichero", "CAdES", domain.FormatCAdES, nil, nil},
		{"auto sin filtro", "AUTO", domain.FormatPAdES, nil, nil},
		{"filenameExts de la web manda", "XAdES", domain.FormatXAdES, map[string]string{"filenameExts": "xml, .XSIG,*.xml,mal/ext"}, []string{"xml", "xsig"}},
		{"clave sin distinguir mayusculas", "CAdES", domain.FormatCAdES, map[string]string{"FILENAMEEXTS": "odt;docx"}, []string{"odt", "docx"}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := legacyDocumentFilter(solicitudSinDatos(c.formatoWeb, c.formato, c.options)).Extensions
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("extensiones = %v, want %v", got, c.want)
			}
		})
	}
}

func TestLegacyDocumentFilter_FilenameExtsEnURL(t *testing.T) {
	solicitud := solicitudSinDatos("PAdES", domain.FormatPAdES, nil)
	solicitud.LegacyParams.Set("filenameExts", "pdf,pdfa")
	if got := legacyDocumentFilter(solicitud).Extensions; !reflect.DeepEqual(got, []string{"pdf", "pdfa"}) {
		t.Fatalf("extensiones = %v", got)
	}
}

func TestEsContenidoPDF(t *testing.T) {
	if !esContenidoPDF([]byte("%PDF-1.7\n...")) {
		t.Fatal("cabecera normal no reconocida")
	}
	if !esContenidoPDF(append([]byte("\xef\xbb\xbf  "), []byte("%PDF-1.4")...)) {
		t.Fatal("cabecera con bytes previos no reconocida")
	}
	if esContenidoPDF([]byte("texto plano")) {
		t.Fatal("un .txt no debe pasar por PDF")
	}
	tarde := append([]byte(strings.Repeat("x", legacyPDFHeaderWindow)), []byte("%PDF-1.7")...)
	if esContenidoPDF(tarde) {
		t.Fatal("la cabecera fuera de los primeros 1024 bytes no cuenta")
	}
}

func TestBuildLocalInteractiveSignCommand_PAdESFiltraYRechazaNoPDF(t *testing.T) {
	doc, err := domain.NewDocument("notas.txt", []byte("hola"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	var filtro ports.DocumentFilter
	h := &legacyWebSocketHandler{documentos: filteredDocumentPickerStub{doc: doc, filtro: &filtro}}
	_, _, err = h.buildLocalInteractiveSignCommand(context.Background(), solicitudSinDatos("PAdES", domain.FormatPAdES, nil))
	if !reflect.DeepEqual(filtro.Extensions, []string{"pdf"}) {
		t.Fatalf("filtro del selector = %v, want [pdf]", filtro.Extensions)
	}
	var noPDF *legacyDocumentoNoPDFError
	if !errors.As(err, &noPDF) {
		t.Fatalf("error = %v, want legacyDocumentoNoPDFError", err)
	}
	if isLegacyCancellation(err) {
		t.Fatal("el aviso de documento no PDF no puede tratarse como cancelación")
	}
	if !strings.HasPrefix(err.Error(), "SAF_09: ") {
		t.Fatalf("la web debe recibir un código explícito: %q", err.Error())
	}
}

func TestBuildLocalInteractiveSignCommand_PAdESAceptaPDF(t *testing.T) {
	doc, err := domain.NewDocument("bueno.pdf", []byte("%PDF-1.7\n%%EOF"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	var filtro ports.DocumentFilter
	h := &legacyWebSocketHandler{documentos: filteredDocumentPickerStub{doc: doc, filtro: &filtro}}
	cmd, _, err := h.buildLocalInteractiveSignCommand(context.Background(), solicitudSinDatos("PAdES", domain.FormatPAdES, nil))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if cmd.Format != domain.FormatPAdES {
		t.Fatalf("formato = %s", cmd.Format)
	}
}

func TestBuildLocalInteractiveSignCommand_CAdESAdmiteCualquierFichero(t *testing.T) {
	doc, err := domain.NewDocument("notas.txt", []byte("hola"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	var filtro ports.DocumentFilter
	h := &legacyWebSocketHandler{documentos: filteredDocumentPickerStub{doc: doc, filtro: &filtro}}
	if _, _, err := h.buildLocalInteractiveSignCommand(context.Background(), solicitudSinDatos("CAdES", domain.FormatCAdES, nil)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(filtro.Extensions) != 0 {
		t.Fatalf("CAdES no debe filtrar: %v", filtro.Extensions)
	}
}

// Ninguna traducción del aviso puede contener «cancel»: el flujo legacy
// convertiría el error en una cancelación silenciosa.
func TestLegacyDocumentoNoPDF_TraduccionesCompletasYSinCancel(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "adapters", "outbound", "common", "localizador", "locales")
	ficheros, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(ficheros) != 11 {
		t.Fatalf("catálogos = %d (%v)", len(ficheros), err)
	}
	for _, f := range ficheros {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var cat map[string]string
		if err := json.Unmarshal(raw, &cat); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, id := range []string{legacyDocumentoNoPDFTitleID, legacyDocumentoNoPDFDetailID, legacyFiltroPermitidosID, legacyFiltroTodosID} {
			v, ok := cat[id]
			if !ok || strings.TrimSpace(v) == "" {
				t.Fatalf("%s: falta %q", filepath.Base(f), id)
			}
			if strings.Contains(strings.ToLower(v), "cancel") {
				t.Fatalf("%s: %q contiene «cancel»", filepath.Base(f), v)
			}
		}
		if !strings.Contains(cat[legacyFiltroPermitidosID], "%s") {
			t.Fatalf("%s: falta el marcador %%s", filepath.Base(f))
		}
	}
}

// El aviso usa la misma fórmula que las aplicaciones de escritorio cuando no
// se firma («No se ha firmado: …»), en todos los idiomas.
func TestLegacyDocumentoNoPDF_MismaFormulaQueEscritorio(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "adapters", "outbound", "common", "localizador", "locales")
	ficheros, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(ficheros) != 11 {
		t.Fatalf("catálogos = %d (%v)", len(ficheros), err)
	}
	formula := func(texto string) string {
		if i := strings.IndexAny(texto, ":："); i > 0 {
			return texto[:i]
		}
		return ""
	}
	for _, f := range ficheros {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var cat map[string]string
		if err := json.Unmarshal(raw, &cat); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, clave := range []string{"winui.firmar.sello_sin_vista_previa_antes_de_firmar", "sign.seal.preview_unavailable_before_sign"} {
			esperada := formula(cat[clave])
			if esperada == "" || formula(cat[legacyDocumentoNoPDFDetailID]) != esperada {
				t.Errorf("%s: el aviso no PDF %q no empieza como %q (%s)", filepath.Base(f), cat[legacyDocumentoNoPDFDetailID], esperada, clave)
			}
		}
	}
}
