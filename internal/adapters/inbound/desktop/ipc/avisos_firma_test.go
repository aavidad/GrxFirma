// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"grxfirma/internal/adapters/outbound/common/revocationclient"
	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// Lote 16d: dos avisos del motor (cofirma de una CAdES explícita sin el
// original, KD-03, y firma LT con una CRL que no cubre al firmante, 14-5)
// solo quedaban en el registro al firmar desde Qt o WinUI. Ahora el
// resultado de la firma por IPC los lleva en «warnings» (claves de catálogo)
// para que la interfaz los muestre en el idioma de la persona.

const (
	avisoPruebaCofirma = signer.AvisoCofirmaSinOriginal
	avisoPruebaCRL     = revocationclient.AvisoCRLNoAplicable
)

// firmaQueAvisa hace lo que el motor real: anota avisos en el contexto de
// la petición mientras firma.
type firmaQueAvisa struct {
	avisos []string
	result application.SignResult
}

func (f *firmaQueAvisa) Execute(ctx context.Context, _ application.SignCommand) (application.SignResult, error) {
	for _, a := range f.avisos {
		domain.AnotarAviso(ctx, a)
	}
	return f.result, nil
}

type multiQueAvisa struct {
	porLlamada [][]string
	llamada    int
	result     application.SignResult
}

func (f *multiQueAvisa) Execute(ctx context.Context, _ application.MultiCoSignCommand) (application.SignResult, error) {
	if f.llamada < len(f.porLlamada) {
		for _, a := range f.porLlamada[f.llamada] {
			domain.AnotarAviso(ctx, a)
		}
	}
	f.llamada++
	return f.result, nil
}

type loteQueAvisa struct {
	avisos []string
}

func (f *loteQueAvisa) Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	for _, a := range f.avisos {
		domain.AnotarAviso(ctx, a)
	}
	out := application.BatchResult{}
	for range cmd.Jobs {
		out.Results = append(out.Results, application.SignResult{
			Result: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("QA firmado")},
		})
	}
	return out, nil
}

// lotePorDocumento firma como ProcessBatchUseCase: cada trabajo abre su
// registro de avisos, que pasa también lo anotado al de la petición.
type lotePorDocumento struct {
	porTrabajo [][]string
}

func (f *lotePorDocumento) Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	out := application.BatchResult{}
	for i := range cmd.Jobs {
		ctxTrabajo, avisos := domain.ConAvisosOperacion(ctx)
		if i < len(f.porTrabajo) {
			for _, a := range f.porTrabajo[i] {
				domain.AnotarAviso(ctxTrabajo, a)
			}
		}
		out.Results = append(out.Results, application.SignResult{
			Result:   domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("QA firmado")},
			Warnings: avisos.Claves(),
		})
	}
	return out, nil
}

func entradaPrueba(t *testing.T, dir, nombre string) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte("QA documento sintético"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func camposJSON(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var campos map[string]json.RawMessage
	if err := json.Unmarshal(raw, &campos); err != nil {
		t.Fatal(err)
	}
	return campos
}

func TestFirmaIPCDevuelveLosAvisosDelMotor(t *testing.T) {
	firmado := application.SignResult{Result: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("QA firmado")}}
	for _, accion := range []string{"sign", "sign_multicosign"} {
		for _, inline := range []bool{false, true} {
			t.Run(accion+map[bool]string{false: "/fichero", true: "/base64"}[inline], func(t *testing.T) {
				dir := t.TempDir()
				m := &Manejador{
					Firmar:        &firmaQueAvisa{avisos: []string{avisoPruebaCofirma, avisoPruebaCRL, avisoPruebaCofirma}, result: firmado},
					MultiCofirmar: &multiQueAvisa{porLlamada: [][]string{{avisoPruebaCofirma, avisoPruebaCRL}}, result: firmado},
					ultimosCerts:  []domain.CertificateRef{{ID: "qa"}},
				}
				resp := m.despachar(context.Background(), peticionJSON(t, accion, paramsFirma{
					InputPath: entradaPrueba(t, dir, "entrada.csig"), OutputPath: filepath.Join(dir, "salida.csig"),
					CertificateID: "qa", Format: "CAdES", Action: "cosign", ReturnSignatureB64: inline,
				}))
				if !resp.OK {
					t.Fatal(resp.Error)
				}
				data := resp.Data.(resultadoFirma)
				if !slices.Equal(data.Warnings, []string{avisoPruebaCofirma, avisoPruebaCRL}) {
					t.Fatalf("warnings = %q", data.Warnings)
				}
				if _, ok := camposJSON(t, data)["warnings"]; !ok {
					t.Fatal("el JSON no lleva warnings")
				}
			})
		}
	}
}

func TestFirmaIPCSinAvisosNoLlevaWarnings(t *testing.T) {
	dir := t.TempDir()
	firmado := application.SignResult{Result: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("QA firmado")}}
	m := &Manejador{Firmar: &firmaQueAvisa{result: firmado}, ultimosCerts: []domain.CertificateRef{{ID: "qa"}}}
	resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath: entradaPrueba(t, dir, "entrada.txt"), OutputPath: filepath.Join(dir, "salida.csig"),
		CertificateID: "qa", Format: "CAdES", Action: "sign",
	}))
	if !resp.OK {
		t.Fatal(resp.Error)
	}
	if _, ok := camposJSON(t, resp.Data)["warnings"]; ok {
		t.Fatalf("una firma sin avisos no lleva warnings: %+v", resp.Data)
	}
}

func TestFirmaLoteIPCDevuelveLosAvisosDelMotor(t *testing.T) {
	t.Run("lote", func(t *testing.T) {
		dir := t.TempDir()
		m := &Manejador{ProcesarLote: &loteQueAvisa{avisos: []string{avisoPruebaCRL}}, ultimosCerts: []domain.CertificateRef{{ID: "qa"}}}
		resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths:    []string{entradaPrueba(t, dir, "a.txt"), entradaPrueba(t, dir, "b.txt")},
			OutputDir:     t.TempDir(),
			CertificateID: "qa", Format: "CAdES", Action: "sign",
		}))
		if !resp.OK {
			t.Fatal(resp.Error)
		}
		data := resp.Data.(resultadoFirmaLote)
		if data.OkCount != 2 || !slices.Equal(data.Warnings, []string{avisoPruebaCRL}) {
			t.Fatalf("ok=%d warnings=%q", data.OkCount, data.Warnings)
		}
	})
	// Lote 17b (3d): con ProcesarLote cada documento lleva también sus
	// propios avisos, sin perder el resumen del lote.
	t.Run("lote_por_documento", func(t *testing.T) {
		dir := t.TempDir()
		lote := &lotePorDocumento{porTrabajo: [][]string{{avisoPruebaCofirma}, nil, {avisoPruebaCRL, avisoPruebaCofirma}}}
		m := &Manejador{ProcesarLote: lote, ultimosCerts: []domain.CertificateRef{{ID: "qa"}}}
		resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths:    []string{entradaPrueba(t, dir, "a.csig"), entradaPrueba(t, dir, "b.txt"), entradaPrueba(t, dir, "c.csig")},
			OutputDir:     t.TempDir(),
			CertificateID: "qa", Format: "CAdES", Action: "cosign",
		}))
		if !resp.OK {
			t.Fatal(resp.Error)
		}
		data := resp.Data.(resultadoFirmaLote)
		if data.OkCount != 3 {
			t.Fatalf("ok=%d resultados=%+v", data.OkCount, data.Results)
		}
		if !slices.Equal(data.Results[0].Warnings, []string{avisoPruebaCofirma}) ||
			len(data.Results[1].Warnings) != 0 ||
			!slices.Equal(data.Results[2].Warnings, []string{avisoPruebaCRL, avisoPruebaCofirma}) {
			t.Fatalf("avisos por documento = %q %q %q", data.Results[0].Warnings, data.Results[1].Warnings, data.Results[2].Warnings)
		}
		if !slices.Equal(data.Warnings, []string{avisoPruebaCofirma, avisoPruebaCRL}) {
			t.Fatalf("avisos del lote = %q", data.Warnings)
		}
	})
	t.Run("cofirma_multiple", func(t *testing.T) {
		dir := t.TempDir()
		multi := &multiQueAvisa{
			porLlamada: [][]string{{avisoPruebaCofirma}, nil, {avisoPruebaCofirma, avisoPruebaCRL}},
			result:     application.SignResult{Result: domain.SignatureResult{Format: domain.FormatCAdES, Data: []byte("QA firmado")}},
		}
		m := &Manejador{MultiCofirmar: multi, ultimosCerts: []domain.CertificateRef{{ID: "qa"}, {ID: "qb"}}}
		resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths: []string{entradaPrueba(t, dir, "a.csig"), entradaPrueba(t, dir, "b.csig"), entradaPrueba(t, dir, "c.csig")},
			OutputDir:  t.TempDir(), CertificateID: "qa", AdditionalCertificateIDs: []string{"qb"},
			Format: "CAdES", Action: "cosign",
		}))
		if !resp.OK {
			t.Fatal(resp.Error)
		}
		data := resp.Data.(resultadoFirmaLote)
		if data.OkCount != 3 {
			t.Fatalf("ok=%d resultados=%+v", data.OkCount, data.Results)
		}
		// Cada documento lleva los suyos y el lote, todos sin repetir.
		if !slices.Equal(data.Results[0].Warnings, []string{avisoPruebaCofirma}) ||
			len(data.Results[1].Warnings) != 0 ||
			!slices.Equal(data.Results[2].Warnings, []string{avisoPruebaCofirma, avisoPruebaCRL}) {
			t.Fatalf("avisos por documento = %q %q %q", data.Results[0].Warnings, data.Results[1].Warnings, data.Results[2].Warnings)
		}
		if !slices.Equal(data.Warnings, []string{avisoPruebaCofirma, avisoPruebaCRL}) {
			t.Fatalf("avisos del lote = %q", data.Warnings)
		}
	})
}

// Las claves que puede devolver el motor existen en los 11 catálogos: la
// interfaz las traduce sin mostrar nunca la clave.
func TestAvisosDeFirmaTienenTextoEnLosCatalogos(t *testing.T) {
	for _, idioma := range []string{"es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "outbound", "common", "localizador", "locales", idioma+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var catalogo map[string]string
		if err := json.Unmarshal(raw, &catalogo); err != nil {
			t.Fatal(err)
		}
		for _, clave := range []string{avisoPruebaCofirma, avisoPruebaCRL} {
			if catalogo[clave] == "" || catalogo[clave] == clave {
				t.Fatalf("%s: falta el texto de %s", idioma, clave)
			}
		}
	}
}
