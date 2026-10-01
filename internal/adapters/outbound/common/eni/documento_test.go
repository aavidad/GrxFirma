// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package eni

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func documentoPrueba() Documento {
	return Documento{
		Contenido:     []byte("%PDF-1.7 firmado"),
		NombreFormato: "PDF",
		Metadatos: Metadatos{
			Organos:           []string{"L01180877"},
			EstadoElaboracion: "EE01",
			TipoDocumental:    "TD14",
		},
		Firmas: []Firma{{Tipo: FirmaPAdES}},
	}
}

var ahoraPrueba = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func TestGenerar_EstructuraYMetadatos(t *testing.T) {
	out, err := Generar(documentoPrueba(), ahoraPrueba)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		XMLName xml.Name
		Id      string `xml:"Id,attr"`
	}
	if err := xml.Unmarshal(out, &v); err != nil {
		t.Fatalf("XML no válido: %v\n%s", err, out)
	}
	if v.XMLName.Space != nsDocumento || v.XMLName.Local != "documento" {
		t.Fatalf("raíz inesperada: %v", v.XMLName)
	}
	if !regexp.MustCompile(`^ES_L01180877_2026_[A-Z0-9]{30}$`).MatchString(v.Id) {
		t.Fatalf("identificador generado no válido: %s", v.Id)
	}
	for _, esperado := range []string{
		"<enidocmeta:VersionNTI>" + VersionNTI + "</enidocmeta:VersionNTI>",
		"<enidocmeta:Organo>L01180877</enidocmeta:Organo>",
		"<enidocmeta:FechaCaptura>2026-09-26T12:00:00Z</enidocmeta:FechaCaptura>",
		"<enidocmeta:OrigenCiudadanoAdministracion>false</enidocmeta:OrigenCiudadanoAdministracion>",
		"<enidocmeta:TipoDocumental>TD14</enidocmeta:TipoDocumental>",
		"<enids:TipoFirma>TF06</enids:TipoFirma>",
		"<enids:ReferenciaFirma>#CONTENIDO_" + v.Id + "</enids:ReferenciaFirma>",
	} {
		if !strings.Contains(string(out), esperado) {
			t.Errorf("falta %s", esperado)
		}
	}
	guardarParaXSD(t, "pades.xml", out)
}

func TestGenerar_CAdESSeparadaYCopia(t *testing.T) {
	d := documentoPrueba()
	d.Metadatos.Identificador = "ES_L01180877_2026_EXP000123"
	d.Metadatos.EstadoElaboracion = "EE02"
	d.Metadatos.IdentificadorDocumentoOrigen = "ES_A00000000_2025_ORIGINAL1"
	d.Metadatos.OrigenAdministracion = true
	d.Firmas = []Firma{{Tipo: FirmaCAdESExplicit, Datos: []byte{0x30, 0x80}}}
	out, err := Generar(d, ahoraPrueba)
	if err != nil {
		t.Fatal(err)
	}
	for _, esperado := range []string{"Id=\"ES_L01180877_2026_EXP000123\"", "<enids:FirmaBase64>MIA=</enids:FirmaBase64>", "IdentificadorDocumentoOrigen>ES_A00000000_2025_ORIGINAL1<", ">true<"} {
		if !strings.Contains(string(out), esperado) {
			t.Errorf("falta %s", esperado)
		}
	}
	guardarParaXSD(t, "cades.xml", out)
}

func TestValidar_RechazaMetadatosIncorrectos(t *testing.T) {
	casos := map[string]func(*Documento){
		"sin órgano":       func(d *Documento) { d.Metadatos.Organos = nil },
		"DIR3 corto":       func(d *Documento) { d.Metadatos.Organos = []string{"L0118"} },
		"estado":           func(d *Documento) { d.Metadatos.EstadoElaboracion = "EE05" },
		"copia sin origen": func(d *Documento) { d.Metadatos.EstadoElaboracion = "EE03" },
		"tipo documental":  func(d *Documento) { d.Metadatos.TipoDocumental = "TD21" },
		"identificador":    func(d *Documento) { d.Metadatos.Identificador = "ES-mal" },
		"formato":          func(d *Documento) { d.NombreFormato = "<pdf>" },
		"firma sin datos":  func(d *Documento) { d.Firmas = []Firma{{Tipo: FirmaCAdESExplicit}} },
		"sin contenido":    func(d *Documento) { d.Contenido = nil },
	}
	for nombre, cambiar := range casos {
		d := documentoPrueba()
		cambiar(&d)
		if _, err := Generar(d, ahoraPrueba); err == nil {
			t.Errorf("%s: se esperaba error", nombre)
		}
	}
}

// guardarParaXSD permite validar la salida con los XSD oficiales y xmllint
// (GRXFIRMA_ENI_SALIDA=directorio), que no forman parte de la suite.
func guardarParaXSD(t *testing.T, nombre string, data []byte) {
	if dir := os.Getenv("GRXFIRMA_ENI_SALIDA"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, nombre), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
