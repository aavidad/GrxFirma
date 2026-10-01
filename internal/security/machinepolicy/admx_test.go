// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package machinepolicy

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// La plantilla ADMX debe cubrir exactamente los valores que lee la
// aplicación, en la misma clave, y sus textos deben existir en todos los
// idiomas publicados.
func TestPlantillaADMXCoherente(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "packaging", "windows", "admx")
	admx, err := os.ReadFile(filepath.Join(dir, "GrxFirma.admx"))
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(admx, new(struct{})); err != nil {
		t.Fatalf("ADMX mal formado: %v", err)
	}
	var def struct {
		Policies []struct {
			Name      string `xml:"name,attr"`
			Key       string `xml:"key,attr"`
			ValueName string `xml:"valueName,attr"`
			Class     string `xml:"class,attr"`
			Elements  struct {
				Items []struct {
					ValueName string `xml:"valueName,attr"`
				} `xml:",any"`
			} `xml:"elements"`
		} `xml:"policies>policy"`
	}
	if err := xml.Unmarshal(admx, &def); err != nil {
		t.Fatal(err)
	}
	cubiertos := map[string]bool{}
	for _, p := range def.Policies {
		if p.Key != RegistryKey || p.Class != "Machine" {
			t.Errorf("%s: debe ser de equipo y escribir en %s", p.Name, RegistryKey)
		}
		if p.ValueName != "" {
			cubiertos[p.ValueName] = true
		}
		for _, e := range p.Elements.Items {
			cubiertos[e.ValueName] = true
		}
	}
	esperados := []string{
		AllowedDomains, WebsocketHabilitado, WebsocketPermitido, RestHabilitado, TofuHabilitado, DirectorioP12,
		DominiosDeConfianza, NivelLog, TimeoutOperacionSegundos, MaxTamanoDocumentoBytes,
		PermitirOrigenVacio, PermitirDESLegacy, PermitirRutasDirectas, AprobacionAutomaticaHost,
		PermitirSHA1Legacy, PermitirCMSAESECBLegacy,
	}
	for _, v := range esperados {
		if !cubiertos[v] {
			t.Errorf("la plantilla ADMX no cubre el valor %s", v)
		}
		delete(cubiertos, v)
	}
	for v := range cubiertos {
		t.Errorf("la plantilla ADMX escribe %s, que la aplicación no lee", v)
	}

	refs := func(tipo string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`\$\(`+tipo+`\.([A-Za-z0-9_]+)\)`).FindAllStringSubmatch(string(admx), -1) {
			out = append(out, m[1])
		}
		sort.Strings(out)
		return out
	}
	for _, idioma := range []string{"es-ES", "en-US"} {
		adml, err := os.ReadFile(filepath.Join(dir, idioma, "GrxFirma.adml"))
		if err != nil {
			t.Fatal(err)
		}
		var res struct {
			Strings []struct {
				ID string `xml:"id,attr"`
			} `xml:"resources>stringTable>string"`
			Presentations []struct {
				ID string `xml:"id,attr"`
			} `xml:"resources>presentationTable>presentation"`
		}
		if err := xml.Unmarshal(adml, &res); err != nil {
			t.Fatalf("%s: ADML mal formado: %v", idioma, err)
		}
		cadenas, presentaciones := map[string]bool{}, map[string]bool{}
		for _, s := range res.Strings {
			cadenas[s.ID] = true
		}
		for _, p := range res.Presentations {
			presentaciones[p.ID] = true
		}
		for _, id := range refs("string") {
			if !cadenas[id] {
				t.Errorf("%s: falta el texto %s", idioma, id)
			}
		}
		for _, id := range refs("presentation") {
			if !presentaciones[id] {
				t.Errorf("%s: falta la presentación %s", idioma, id)
			}
		}
		if strings.Count(string(adml), "REBAJA LA SEGURIDAD")+strings.Count(string(adml), "LOWERS SECURITY") != 6 {
			t.Errorf("%s: cada excepción de compatibilidad debe advertir que rebaja la seguridad", idioma)
		}
	}
}
