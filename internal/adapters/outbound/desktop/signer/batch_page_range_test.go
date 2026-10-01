// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/testsupport/exttools"
)

// TestProcessBatch_SelloVisiblePorRangoPorDocumento (T089) verifica que una
// plantilla global de sello se aplica a los documentos sin override y que cada
// documento puede sobrescribir su página o rango. Usa el motor de firma real
// (no un mock) para ejercitar el pipeline completo.
func TestProcessBatch_SelloVisiblePorRangoPorDocumento(t *testing.T) {
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	ref := domain.CertificateRef{ID: "cert-batch", Fingerprint: "fp-batch", Subject: "CN=Batch"}

	fixture := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")
	pdf, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("leyendo fixture: %v", err)
	}

	casos := []struct {
		nombre       string
		pageOverride string
		pageEsperada string
		widgetsEsper int
	}{
		{"global.pdf", "", "2", 1},     // default global
		{"rango.pdf", "1-3", "1-3", 3}, // rango por documento
		{"lista.pdf", "2,4", "2,4", 2}, // lista por documento
	}
	items := make([]application.BatchItemInput, 0, len(casos))
	for _, c := range casos {
		var options map[string]string
		if c.pageOverride != "" {
			options = map[string]string{"page": c.pageOverride}
		}
		items = append(items, application.BatchItemInput{
			Nombre:    c.nombre,
			Contenido: pdf,
			TipoMIME:  "application/pdf",
			Formato:   "PAdES",
			Accion:    "sign",
			Opciones:  options,
		})
	}
	cmd, err := application.NewProcessBatchCommandWithDefaults(items, map[string]string{
		"visibleSeal":      "true",
		"visibleSealRectX": "36",
		"visibleSealRectY": "36",
		"visibleSealRectW": "220",
		"visibleSealRectH": "70",
		"page":             "2",
	})
	if err != nil {
		t.Fatalf("NewProcessBatchCommandWithDefaults: %v", err)
	}
	for i, c := range casos {
		if got := cmd.Jobs[i].Options["page"]; got != c.pageEsperada {
			t.Fatalf("caso %q: page fusionada = %q, want %q", c.nombre, got, c.pageEsperada)
		}
		if got := cmd.Jobs[i].Options["visibleSeal"]; got != "true" {
			t.Fatalf("caso %q: visibleSeal global = %q, want true", c.nombre, got)
		}
	}
	cmd.CertificateID = ref.ID

	uc := application.NuevoProcessBatchUseCase(
		&catalogoUnCert{ref: ref},
		&clavesUnCert{ref: ref, clave: clave},
		signer.NuevoMotorFirmaGo(nil),
		aprobadorSiempre{},
		application.NuevoAuditUseCase(relojFijo{}, loggerNulo{}),
		publicadorNulo{},
	)

	res, err := uc.Ejecutar(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Ejecutar lote: %v", err)
	}
	if len(res.Errores) != 0 {
		t.Fatalf("errores en el lote: %v", res.Errores)
	}
	if len(res.Results) != len(casos) {
		t.Fatalf("resultados = %d, want %d", len(res.Results), len(casos))
	}

	tieneQpdf := exttools.Available(t, "qpdf")
	for i, c := range casos {
		data := res.Results[i].Result.Data
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			t.Fatalf("caso %q: la salida no es un PDF", c.pageEsperada)
		}
		if got := bytes.Count(data, []byte("/Subtype /Widget")); got != c.widgetsEsper {
			t.Fatalf("caso %q: %d widgets, se esperaban %d", c.pageEsperada, got, c.widgetsEsper)
		}
		if got := bytes.Count(data, []byte("/ByteRange")); got != 1 {
			t.Fatalf("caso %q: %d firmas, se esperaba 1", c.pageEsperada, got)
		}
		if tieneQpdf {
			signedPath := filepath.Join(t.TempDir(), "lote.pdf")
			if werr := os.WriteFile(signedPath, data, 0o600); werr != nil {
				t.Fatalf("escribir temporal: %v", werr)
			}
			if out, cerr := exec.Command("qpdf", "--check", signedPath).CombinedOutput(); cerr != nil {
				t.Fatalf("caso %q: qpdf --check falló: %v\n%s", c.pageEsperada, cerr, out)
			}
		}
	}
}

func TestMotorFirmaGo_SellosDistintosPorPaginaUnaFirma(t *testing.T) {
	fixture := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")
	pdf, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	placements := `[{"page":1,"rect":{"x":0.1,"y":0.1,"w":0.3,"h":0.1},"rotation":0},{"page":2,"rect":{"x":0.4,"y":0.2,"w":0.2,"h":0.15},"rotation":90}]`
	firmado := firmarFormulario(t, pdf, map[string]string{
		"visibleSeal": "true", "visibleSealPlacements": placements,
		"qrContent": "verifica.ejemplo/ruta",
	})
	if got := bytes.Count(firmado, []byte("/Subtype /Widget")); got != 2 {
		t.Fatalf("widgets=%d", got)
	}
	if got := bytes.Count(firmado, []byte("/ByteRange")); got != 1 {
		t.Fatalf("firmas=%d", got)
	}
	for _, expected := range []string{
		"/Rect [61.200000 79.200000 244.800000 158.400000]",
		"/Rect [235.800000 165.000000 358.200000 283.800000]",
		"/BBox [0 0 183.600000 79.200000]",
		"/BBox [0 0 122.400000 118.800000]",
	} {
		if !bytes.Contains(firmado, []byte(expected)) {
			t.Errorf("falta geometría propia del widget: %s", expected)
		}
	}
	comprobarPDFFirmado(t, firmado)
}

func TestMotorFirmaGo_SelloUnicoDesdeLista(t *testing.T) {
	fixture := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")
	pdf, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	firmado := firmarFormulario(t, pdf, map[string]string{
		"visibleSealPlacements": `[{"page":3,"rect":{"x":0.2,"y":0.2,"w":0.3,"h":0.1},"rotation":45}]`,
	})
	if got := bytes.Count(firmado, []byte("/Subtype /Widget")); got != 1 {
		t.Fatalf("widgets=%d", got)
	}
	if got := bytes.Count(firmado, []byte("/ByteRange")); got != 1 {
		t.Fatalf("firmas=%d", got)
	}
	comprobarPDFFirmado(t, firmado)
}

// --- dobles de test mínimos para los puertos de application ---

type catalogoUnCert struct{ ref domain.CertificateRef }

func (c *catalogoUnCert) List(context.Context) ([]domain.CertificateRef, error) {
	return []domain.CertificateRef{c.ref}, nil
}

type clavesUnCert struct {
	ref   domain.CertificateRef
	clave ports.SigningKey
}

func (p *clavesUnCert) KeyFor(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	return p.clave, nil
}

type aprobadorSiempre struct{}

func (aprobadorSiempre) Request(context.Context, string) (bool, error) { return true, nil }

type loggerNulo struct{}

func (loggerNulo) Log(context.Context, ports.Evidence) error { return nil }

type publicadorNulo struct{}

func (publicadorNulo) Publish(context.Context, ports.Event) error { return nil }

type relojFijo struct{}

func (relojFijo) Now() time.Time { return time.Unix(0, 0) }
