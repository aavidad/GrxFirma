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
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const (
	contenidoPrevioSalida = "firmado-anterior-que-no-debe-perderse"
	contenidoNuevoSalida  = "firmado-nuevo"
)

func manejadorFirmaPolitica(preferencia string) *Manejador {
	m := &Manejador{
		Loc: localizador.Para("es"),
		Firmar: &stubFirmar{result: application.SignResult{
			Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte(contenidoNuevoSalida)},
		}},
	}
	if preferencia != "" {
		valor := preferencia
		m.Settings = &stubTypedSettings{doc: ports.DocumentoConfiguracionUsuario{
			Firma: ports.ConfiguracionUsuarioFirma{Overwrite: &valor},
		}}
	}
	return m
}

func leerSalidaPrueba(t *testing.T, ruta string) string {
	t.Helper()
	data, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer %s: %v", filepath.Base(ruta), err)
	}
	return string(data)
}

// Con cada política, con y sin outputPath y con y sin fichero previo, el
// motor decide qué se escribe y devuelve la ruta real.
func TestSign_PoliticaSobrescrituraSeAplicaSiempreEnElMotor(t *testing.T) {
	type caso struct {
		nombre      string
		overwrite   string
		preferencia string
		confirmada  bool
		conRuta     bool
		existe      bool
		wantOK      bool
		wantFinal   string // nombre base esperado de la ruta devuelta
		wantPrevio  bool   // el fichero previo sigue intacto
	}
	casos := []caso{}
	for _, conRuta := range []bool{true, false} {
		casos = append(casos,
			caso{nombre: "rename sin previo", overwrite: "rename", conRuta: conRuta, wantOK: true, wantFinal: "doc_firmado.pdf"},
			caso{nombre: "rename con previo", overwrite: "rename", conRuta: conRuta, existe: true, wantOK: true, wantFinal: "doc_firmado_001.pdf", wantPrevio: true},
			caso{nombre: "fail sin previo", overwrite: "fail", conRuta: conRuta, wantOK: true, wantFinal: "doc_firmado.pdf"},
			caso{nombre: "fail con previo", overwrite: "fail", conRuta: conRuta, existe: true, wantOK: false, wantPrevio: true},
			caso{nombre: "force sin previo", overwrite: "force", conRuta: conRuta, wantOK: true, wantFinal: "doc_firmado.pdf"},
			caso{nombre: "force con previo", overwrite: "force", conRuta: conRuta, existe: true, wantOK: true, wantFinal: "doc_firmado.pdf"},
			caso{nombre: "valor desconocido renombra", overwrite: "ask", preferencia: "force", conRuta: conRuta, existe: true, wantOK: true, wantFinal: "doc_firmado_001.pdf", wantPrevio: true},
			caso{nombre: "sin valor ni preferencia renombra", conRuta: conRuta, existe: true, wantOK: true, wantFinal: "doc_firmado_001.pdf", wantPrevio: true},
			caso{nombre: "sin valor usa preferencia fail", preferencia: "fail", conRuta: conRuta, existe: true, wantOK: false, wantPrevio: true},
			caso{nombre: "sin valor usa preferencia force", preferencia: "force", conRuta: conRuta, existe: true, wantOK: true, wantFinal: "doc_firmado.pdf"},
			caso{nombre: "confirmada con rename reemplaza", overwrite: "rename", confirmada: true, conRuta: conRuta, existe: true, wantOK: true, wantFinal: "doc_firmado.pdf"},
			caso{nombre: "confirmada con fail reemplaza", overwrite: "fail", confirmada: true, conRuta: conRuta, existe: true, wantOK: true, wantFinal: "doc_firmado.pdf"},
			caso{nombre: "confirmada sin previo", confirmada: true, conRuta: conRuta, wantOK: true, wantFinal: "doc_firmado.pdf"},
		)
	}
	for _, c := range casos {
		c := c
		nombre := c.nombre + map[bool]string{true: " con outputPath", false: " sin outputPath"}[c.conRuta]
		t.Run(nombre, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "doc.pdf")
			if err := os.WriteFile(input, []byte("%PDF-1.4 prueba"), 0o600); err != nil {
				t.Fatal(err)
			}
			destino := filepath.Join(dir, "doc_firmado.pdf")
			if c.existe {
				if err := os.WriteFile(destino, []byte(contenidoPrevioSalida), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			params := paramsFirma{
				InputPath:          input,
				Format:             "pades",
				Overwrite:          c.overwrite,
				OverwriteConfirmed: c.confirmada,
				SaveToDisk:         true,
			}
			if c.conRuta {
				params.OutputPath = destino
			}
			m := manejadorFirmaPolitica(c.preferencia)
			resp := m.despachar(context.Background(), peticionJSON(t, "sign", params))
			if resp.OK != c.wantOK {
				t.Fatalf("OK=%v, want %v (error=%q)", resp.OK, c.wantOK, resp.Error)
			}
			if c.wantPrevio {
				if got := leerSalidaPrueba(t, destino); got != contenidoPrevioSalida {
					t.Fatalf("el fichero previo se perdió: %q", got)
				}
			}
			if !c.wantOK {
				if !strings.Contains(resp.Error, "Ya existe") || !strings.Contains(resp.Error, destino) {
					t.Fatalf("error poco claro: %q", resp.Error)
				}
				return
			}
			data, ok := resp.Data.(resultadoFirma)
			if !ok {
				t.Fatalf("Data = %T", resp.Data)
			}
			if data.OutputPath != filepath.Join(dir, c.wantFinal) {
				t.Fatalf("ruta devuelta = %q, want %q", data.OutputPath, filepath.Join(dir, c.wantFinal))
			}
			if got := leerSalidaPrueba(t, data.OutputPath); got != contenidoNuevoSalida {
				t.Fatalf("contenido de la ruta devuelta = %q", got)
			}
		})
	}
}

// La confirmación no se deduce: que la ruta venga del cliente no basta.
func TestSign_OutputPathExplicitoNoImplicaConfirmacion(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.pdf")
	destino := filepath.Join(dir, "elegido.pdf")
	for ruta, contenido := range map[string]string{input: "%PDF-1.4", destino: contenidoPrevioSalida} {
		if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resp := manejadorFirmaPolitica("rename").despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath: input, OutputPath: destino, Format: "pades", SaveToDisk: true,
	}))
	if !resp.OK {
		t.Fatalf("sign: %s", resp.Error)
	}
	if got := leerSalidaPrueba(t, destino); got != contenidoPrevioSalida {
		t.Fatalf("se reemplazó sin confirmación: %q", got)
	}
	if final := resp.Data.(resultadoFirma).OutputPath; final != filepath.Join(dir, "elegido_001.pdf") {
		t.Fatalf("ruta devuelta = %q", final)
	}
}

func TestSignMultiCosign_AplicaPolitica(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.pdf")
	destino := filepath.Join(dir, "doc_firmado.pdf")
	for ruta, contenido := range map[string]string{input: "%PDF-1.4", destino: contenidoPrevioSalida} {
		if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := &Manejador{MultiCofirmar: &stubMultiCofirmar{result: application.SignResult{
		Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte(contenidoNuevoSalida)},
	}}}
	resp := m.despachar(context.Background(), peticionJSON(t, "sign_multicosign", paramsFirma{
		InputPath: input, OutputPath: destino, Format: "pades", Action: "sign",
		Overwrite: "rename", SaveToDisk: true, AdditionalCertificateIDs: []string{"b"},
	}))
	if !resp.OK {
		t.Fatalf("sign_multicosign: %s", resp.Error)
	}
	if got := leerSalidaPrueba(t, destino); got != contenidoPrevioSalida {
		t.Fatalf("cofirma múltiple reemplazó el previo: %q", got)
	}
	if final := resp.Data.(resultadoFirma).OutputPath; final != filepath.Join(dir, "doc_firmado_001.pdf") {
		t.Fatalf("ruta devuelta = %q", final)
	}
}

func TestSignBatch_FailNoReemplazaYDevuelveRutaReal(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.pdf")
	destino := filepath.Join(dir, "doc_firmado.pdf")
	for ruta, contenido := range map[string]string{input: "%PDF-1.4", destino: contenidoPrevioSalida} {
		if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lote := func() *stubProcesarLote {
		return &stubProcesarLote{result: application.BatchResult{Results: []application.SignResult{{
			Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte(contenidoNuevoSalida)},
		}}}}
	}
	m := &Manejador{Loc: localizador.Para("es"), ProcesarLote: lote()}
	resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		InputPaths: []string{input}, Format: "pades", Action: "sign", Overwrite: "fail",
	}))
	if !resp.OK {
		t.Fatalf("sign_batch: %s", resp.Error)
	}
	items := resp.Data.(resultadoFirmaLote).Results
	if len(items) != 1 || items[0].OK || !strings.Contains(items[0].Error, "Ya existe") {
		t.Fatalf("fail no se respetó en el lote: %+v", items)
	}
	if got := leerSalidaPrueba(t, destino); got != contenidoPrevioSalida {
		t.Fatalf("el lote reemplazó el previo: %q", got)
	}

	// Sin valor en la petición manda la preferencia guardada.
	valor := "rename"
	m = &Manejador{ProcesarLote: lote(), Settings: &stubTypedSettings{doc: ports.DocumentoConfiguracionUsuario{
		Firma: ports.ConfiguracionUsuarioFirma{Overwrite: &valor},
	}}}
	resp = m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		InputPaths: []string{input}, Format: "pades", Action: "sign",
	}))
	items = resp.Data.(resultadoFirmaLote).Results
	if !resp.OK || len(items) != 1 || !items[0].OK || items[0].OutputPath != filepath.Join(dir, "doc_firmado_001.pdf") {
		t.Fatalf("rename en lote: %+v", resp)
	}
	if got := leerSalidaPrueba(t, destino); got != contenidoPrevioSalida {
		t.Fatalf("el lote reemplazó el previo: %q", got)
	}
}

func TestFacturaeCreate_RespetaPoliticaYConfirmacion(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "factura.xml")
	if err := os.WriteFile(output, []byte(contenidoPrevioSalida), 0o600); err != nil {
		t.Fatal(err)
	}
	draft := facturaeSampleDraft()
	raw, _ := json.Marshal(facturaeCreateParams{Draft: draft, OutputPath: output})
	response := (&Manejador{}).despachar(context.Background(), peticion{Action: "facturae_create", Params: raw})
	if !response.OK {
		t.Fatalf("facturae: %+v", response)
	}
	if got := leerSalidaPrueba(t, output); got != contenidoPrevioSalida {
		t.Fatalf("factura previa reemplazada sin confirmación: %q", got)
	}
	final := response.Data.(map[string]string)["outputPath"]
	if final != filepath.Join(dir, "factura_001.xml") {
		t.Fatalf("ruta devuelta = %q", final)
	}

	raw, _ = json.Marshal(facturaeCreateParams{Draft: draft, OutputPath: output, OverwriteConfirmed: true})
	response = (&Manejador{}).despachar(context.Background(), peticion{Action: "facturae_create", Params: raw})
	if !response.OK || response.Data.(map[string]string)["outputPath"] != output {
		t.Fatalf("facturae confirmada: %+v", response)
	}
	if got := leerSalidaPrueba(t, output); !strings.Contains(got, "<InvoiceTotal>") {
		t.Fatalf("la confirmación no reemplazó: %q", got)
	}
}

func TestHashCreate_RespetaPoliticaYConfirmacion(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.txt")
	output := filepath.Join(dir, "doc.hexhash")
	for ruta, contenido := range map[string]string{input: "hola", output: contenidoPrevioSalida} {
		if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := &Manejador{CrearHash: &stubCrearHash{result: application.CreateHashResult{
		Algorithm: "SHA-256", Format: application.HashFormatHex, Encoded: "abc",
	}}}
	resp := m.despachar(context.Background(), peticionJSON(t, "hash_create", paramsHashCreate{
		InputPath: input, OutputPath: output, Format: "hex",
	}))
	if !resp.OK {
		t.Fatalf("hash_create: %s", resp.Error)
	}
	if got := leerSalidaPrueba(t, output); got != contenidoPrevioSalida {
		t.Fatalf("huella previa reemplazada sin confirmación: %q", got)
	}
	if final := resp.Data.(resultadoHash).OutputPath; final != filepath.Join(dir, "doc_001.hexhash") {
		t.Fatalf("ruta devuelta = %q", final)
	}
	resp = m.despachar(context.Background(), peticionJSON(t, "hash_create", paramsHashCreate{
		InputPath: input, OutputPath: output, Format: "hex", OverwriteConfirmed: true,
	}))
	if !resp.OK || resp.Data.(resultadoHash).OutputPath != output || leerSalidaPrueba(t, output) != "abc" {
		t.Fatalf("hash_create confirmada: %+v", resp)
	}
}

func TestProtect_ForceDeLaPeticionSigueReemplazandoYFailNo(t *testing.T) {
	politica := (&Manejador{}).politicaSalidaIPC(context.Background(), "true", false)
	if forzada, _ := politicaSobrescrituraDesdeTexto("force"); politica != forzada {
		t.Fatalf("«true» debe seguir significando reemplazar, como antes en proteger")
	}
	if p, ok := politicaSobrescrituraDesdeTexto("fail"); !ok || p == politica {
		t.Fatalf("«fail» debe reconocerse y no reemplazar")
	}
}
