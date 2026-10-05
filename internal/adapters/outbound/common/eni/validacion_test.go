// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package eni

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func expedientePrueba(t *testing.T) []byte {
	t.Helper()
	doc, err := Generar(documentoPrueba(), ahoraPrueba)
	if err != nil {
		t.Fatal(err)
	}
	// XMLDSig de prueba: estructuralmente completo, sin firma criptográfica real.
	sign := func(_ []byte, id string) ([]byte, error) {
		return []byte(`<ds:Signature xmlns:ds="` + nsDS + `"><ds:SignedInfo><ds:CanonicalizationMethod Algorithm="http://www.w3.org/2001/10/xml-exc-c14n#"/><ds:SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"/><ds:Reference URI="#` + id + `"><ds:DigestMethod Algorithm="` + funcionResumenSHA2 + `"/><ds:DigestValue>AA==</ds:DigestValue></ds:Reference></ds:SignedInfo><ds:SignatureValue>AA==</ds:SignatureValue></ds:Signature>`), nil
	}
	out, err := GenerarExpediente(MetadatosExpediente{Organos: []string{"L01180877"}, Clasificacion: "L01180877_PRO_LICENCIAS", Estado: "E01"}, []DocumentoExpediente{{XML: doc}}, sign, func(b []byte) ([]byte, error) { return b, nil }, ahoraPrueba)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestValidarXMLDocumento(t *testing.T) {
	valid, err := Generar(documentoPrueba(), ahoraPrueba)
	if err != nil {
		t.Fatal(err)
	}
	if issues := ValidarXML(append([]byte{0xef, 0xbb, 0xbf}, valid...)); len(issues) != 0 {
		t.Fatal(issues)
	}
	cases := map[string]string{
		"nombre largo":         "<" + strings.Repeat("x", 129) + "/>",
		"namespace largo":      `<x xmlns="` + strings.Repeat("x", 513) + `"/>`,
		"atributo desconocido": strings.Replace(string(valid), "<enidoc:documento ", `<enidoc:documento sorpresa="1" `, 1),
		"referencia externa":   strings.Replace(string(valid), "<enids:ReferenciaFirma>#", "<enids:ReferenciaFirma>https://example.invalid/", 1),
		"offset minutos":       strings.Replace(string(valid), "2026-09-26T12:00:00Z", "2026-09-26T12:00:00+02:60", 1),
		"namespace":            strings.Replace(string(valid), nsMetadatos, "urn:otro", 1),
		"obligatorio":          strings.Replace(string(valid), "<enidocmeta:TipoDocumental>TD14</enidocmeta:TipoDocumental>", "", 1),
		"orden":                strings.Replace(string(valid), "<enidocmeta:TipoDocumental>TD14</enidocmeta:TipoDocumental>", "<enidocmeta:Organo>L01180877</enidocmeta:Organo>", 1),
		"estado":               strings.Replace(string(valid), "EE01", "EE05", 1),
		"tipo":                 strings.Replace(string(valid), "TD14", "TD21", 1),
		"origen":               strings.Replace(string(valid), ">false<", ">ciudadano<", 1),
		"DIR3":                 strings.ReplaceAll(string(valid), "L01180877", "LLLLLLLLL"),
		"id largo":             strings.ReplaceAll(string(valid), "ES_L01180877_2026_", "ES_L01180877_2026_"+strings.Repeat("A", 31)),
		"fecha":                strings.Replace(string(valid), "2026-09-26T12:00:00Z", "2026-02-30T12:00:00Z", 1),
		"zona":                 strings.Replace(string(valid), "2026-09-26T12:00:00Z", "2026-09-26T12:00:00", 1),
		"zona extrema":         strings.Replace(string(valid), "2026-09-26T12:00:00Z", "2026-09-26T12:00:00+15:00", 1),
		"origen copia":         strings.Replace(string(valid), "EE01", "EE02", 1),
		"base64":               strings.Replace(string(valid), "<enifile:ValorBinario>", "<enifile:ValorBinario>!", 1),
		"raices":               string(valid) + string(valid),
		"DTD":                  `<!DOCTYPE x [<!ENTITY x SYSTEM "file:///etc/passwd">]>` + string(valid),
		"profundidad":          strings.Repeat("<x>", 65) + strings.Repeat("</x>", 65),
		"atributo duplicado":   strings.Replace(string(valid), "<enidoc:documento ", `<enidoc:documento x="1" x="2" `, 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if issues := ValidarXML([]byte(input)); len(issues) == 0 {
				t.Fatal("se aceptó XML inválido")
			}
		})
	}
	multi := strings.Replace(strings.Replace(string(valid), "EE01", "EE05", 1), "TD14", "TD21", 1)
	if len(ValidarXML([]byte(multi))) < 2 {
		t.Fatal("debe informar cada problema")
	}
}
func TestValidarXMLExpediente(t *testing.T) {
	valid := expedientePrueba(t)
	for name, input := range map[string]string{
		"estado":           strings.Replace(string(valid), ">E01<", ">E99<", 1),
		"clasificacion":    strings.Replace(string(valid), "L01180877_PRO_LICENCIAS", "texto libre", 1),
		"namespace indice": strings.ReplaceAll(string(valid), nsIndiceContenido, "urn:otro"),
		"orden":            strings.Replace(string(valid), "<eniexpmeta:Estado>E01</eniexpmeta:Estado>", "<eniexpmeta:Organo>L01180877</eniexpmeta:Organo>", 1),
		"fecha":            strings.ReplaceAll(string(valid), "2026-09-26T12:00:00Z", "invalida"),
		"sin firma":        strings.Replace(string(valid), "<enids:firmas>", "<enids:otra>", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if len(ValidarXML([]byte(input))) == 0 {
				t.Fatal("se aceptó expediente inválido")
			}
		})
	}
}
func TestGenerarLimitaMetadatos(t *testing.T) {
	for name, change := range map[string]func(*Documento){
		"fecha fuera xsd":   func(d *Documento) { d.Metadatos.FechaCaptura = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		"origen inyectable": func(d *Documento) { d.Metadatos.IdentificadorDocumentoOrigen = "<x/>" },
		"id limite":         func(d *Documento) { d.Metadatos.Identificador = "ES_L01180877_2026_" + strings.Repeat("x", 31) },
	} {
		t.Run(name, func(t *testing.T) {
			d := documentoPrueba()
			change(&d)
			if _, err := Generar(d, ahoraPrueba); err == nil {
				t.Fatal("se esperaba error")
			}
		})
	}
	meta := MetadatosExpediente{Organos: []string{"L01180877"}, Clasificacion: "123", Estado: "E01", Interesados: []string{"x\x00"}}
	if err := meta.Validar(); err == nil {
		t.Fatal("se aceptó carácter de control")
	}
}
func TestXSDOficialOpcional(t *testing.T) {
	lint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint no instalado")
	}
	for _, kind := range []string{"DOCUMENTO", "EXPEDIENTE"} {
		t.Run(kind, func(t *testing.T) {
			schema := os.Getenv("GRXFIRMA_ENI_XSD_" + kind)
			if schema == "" {
				t.Skip("indique GRXFIRMA_ENI_XSD_" + kind + " con la ruta del XSD oficial local")
			}
			if _, err := os.Stat(schema); err != nil {
				t.Skip("XSD oficial no disponible: " + err.Error())
			}
			var data []byte
			if kind == "DOCUMENTO" {
				data, err = Generar(documentoPrueba(), ahoraPrueba)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				data = expedientePrueba(t)
			}
			path := filepath.Join(t.TempDir(), "eni.xml")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(lint, "--nonet", "--noout", "--schema", schema, path).CombinedOutput(); err != nil {
				t.Fatalf("XSD oficial: %v\n%s", err, out)
			}
		})
	}
}

func TestLimiteXMLProporcionadoAlContenido(t *testing.T) {
	// El Base64 del contenido máximo cabe con margen y el tope no excede
	// 4/3 del contenido más 4 MiB (antes 150 MiB).
	base64Max := (MaxContenidoBytes + 2) / 3 * 4
	if MaxXMLBytes < base64Max+1<<20 || MaxXMLBytes > base64Max+4<<20 || MaxXMLBytes >= 150<<20 {
		t.Fatalf("MaxXMLBytes=%d no es proporcionado a MaxContenidoBytes=%d", MaxXMLBytes, MaxContenidoBytes)
	}
	if issues := ValidarXML(make([]byte, MaxXMLBytes+1)); len(issues) != 1 || issues[0].Clave != "eni.validacion.limit" {
		t.Fatalf("XML sobre el límite: %v", issues)
	}
}

func TestValidarXMLBase64ConSaltosYVacio(t *testing.T) {
	valid, err := Generar(documentoPrueba(), ahoraPrueba)
	if err != nil {
		t.Fatal(err)
	}
	s := string(valid)
	inicio := strings.Index(s, "<enifile:ValorBinario>") + len("<enifile:ValorBinario>")
	fin := strings.Index(s, "</enifile:ValorBinario>")
	b64 := s[inicio:fin]
	var envuelto strings.Builder
	for i := 0; i < len(b64); i += 4 {
		envuelto.WriteString(" \r\n\t" + b64[i:min(i+4, len(b64))])
	}
	if issues := ValidarXML([]byte(s[:inicio] + envuelto.String() + "\n" + s[fin:])); len(issues) != 0 {
		t.Fatalf("Base64 con espacios y saltos de línea rechazado: %v", issues)
	}
	for nombre, valor := range map[string]string{"vacío": " \n\t ", "espacio Unicode": b64[:4] + " " + b64[4:], "truncado": b64[:len(b64)-1]} {
		if issues := ValidarXML([]byte(s[:inicio] + valor + s[fin:])); len(issues) == 0 {
			t.Errorf("Base64 %s aceptado", nombre)
		}
	}
}
