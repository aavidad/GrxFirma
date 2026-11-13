// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/domain"
)

var huellaConfigPrueba = strings.Repeat("ab", 32)

func almacenConPolicy(t *testing.T, policy string) (*config.AlmacenConfianzaForzada, string) {
	t.Helper()
	userDir := t.TempDir()
	policyDir := t.TempDir()
	if policy != "" {
		if err := os.WriteFile(filepath.Join(policyDir, "policy.json"), []byte(policy), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return config.NuevoAlmacenConfianzaForzadaConPolicy(userDir, policyDir), userDir
}

func aceptacionPrueba() domain.AceptacionConfianza {
	return domain.AceptacionConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: huellaConfigPrueba, Sujeto: "CN=Raíz", AceptadaEn: time.Now()}
}

// Lote 14: lo aceptado se guarda en config.json (0600) sin tocar el resto.
func TestAlmacenConfianzaForzada_GuardaConservaYQuita(t *testing.T) {
	almacen, userDir := almacenConPolicy(t, "")
	ruta := filepath.Join(userDir, "config.json")
	if err := os.WriteFile(ruta, []byte(`{"idioma": "gl", "clave_futura": {"x": 1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := almacen.AceptarConfianzaForzada(ctx, aceptacionPrueba()); err != nil {
		t.Fatal(err)
	}
	if err := almacen.AceptarConfianzaForzada(ctx, domain.AceptacionConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1, AceptadaEn: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Repetir la misma aceptación no la duplica.
	if err := almacen.AceptarConfianzaForzada(ctx, aceptacionPrueba()); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(ruta)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("permisos de config.json=%v, se esperaba 0600", info.Mode().Perm())
		}
	}
	var campos map[string]json.RawMessage
	datos, _ := os.ReadFile(ruta)
	if err := json.Unmarshal(datos, &campos); err != nil {
		t.Fatal(err)
	}
	if string(campos["idioma"]) != `"gl"` || campos["clave_futura"] == nil {
		t.Fatalf("se perdieron otras claves de config.json: %s", datos)
	}
	var entradas []map[string]any
	if err := json.Unmarshal(campos["confianza_forzada"], &entradas); err != nil || len(entradas) != 2 {
		t.Fatalf("entradas guardadas: %s (%v)", campos["confianza_forzada"], err)
	}
	// Renovar la misma aceptación la deja al final.
	ca := entradas[1]
	for _, clave := range []string{"tipo", "aceptada_en", "aviso"} {
		if _, ok := ca[clave]; !ok {
			t.Fatalf("a la entrada le falta %q: %v", clave, ca)
		}
	}
	if ca["tipo"] != "ca_desconocida" || ca["huella"] != huellaConfigPrueba || ca["sujeto"] != "CN=Raíz" || ca["aviso"] != "verify.forced_trust.notice.ca_desconocida" {
		t.Fatalf("entrada guardada: %v", ca)
	}

	confianza, err := almacen.CargarConfianzaForzada(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if confianza.Prohibida || len(confianza.Aceptaciones) != 2 {
		t.Fatalf("carga: %+v", confianza)
	}
	if !confianza.Acepta(domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: huellaConfigPrueba}) {
		t.Fatal("la CA aceptada no se reconoce")
	}

	quitada, err := almacen.QuitarConfianzaForzada(ctx, domain.ConfianzaCADesconocida, huellaConfigPrueba)
	if err != nil || !quitada {
		t.Fatalf("quitar: %v %v", quitada, err)
	}
	quitada, err = almacen.QuitarConfianzaForzada(ctx, domain.ConfianzaCADesconocida, huellaConfigPrueba)
	if err != nil || quitada {
		t.Fatalf("quitar dos veces: %v %v", quitada, err)
	}
	confianza, _ = almacen.CargarConfianzaForzada(ctx)
	if len(confianza.Aceptaciones) != 1 || confianza.Aceptaciones[0].Tipo != domain.ConfianzaAlgoritmoSHA1 {
		t.Fatalf("tras quitar: %+v", confianza.Aceptaciones)
	}
}

// Una entrada mal formada escrita a mano se ignora; no amplía nada.
func TestAlmacenConfianzaForzada_IgnoraEntradasMalFormadas(t *testing.T) {
	almacen, userDir := almacenConPolicy(t, "")
	contenido := `{"confianza_forzada": [
		{"tipo": "ca_desconocida", "huella": "corta"},
		{"tipo": "todo", "aceptada_en": "2026-11-13T00:00:00Z"},
		{"tipo": "algoritmo_sha1", "huella": "` + huellaConfigPrueba + `"},
		{"tipo": "revocacion_no_comprobable", "aceptada_en": "2026-11-13T00:00:00Z", "aviso": "verify.forced_trust.notice.revocacion_no_comprobable"}
	]}`
	if err := os.WriteFile(filepath.Join(userDir, "config.json"), []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	confianza, err := almacen.CargarConfianzaForzada(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(confianza.Aceptaciones) != 1 || confianza.Aceptaciones[0].Tipo != domain.ConfianzaRevocacionNoComprobable {
		t.Fatalf("solo debe quedar la entrada correcta: %+v", confianza.Aceptaciones)
	}
}

// La política de máquina prohibir_confianza_forzada impide aceptar y hace
// que lo aceptado se ignore. Una política ilegible o con el valor mal
// escrito cuenta como prohibición.
func TestAlmacenConfianzaForzada_PoliticaProhibe(t *testing.T) {
	for nombre, policy := range map[string]string{
		"prohibida":      `{"prohibir_confianza_forzada": true}`,
		"ilegible":       `{no es json`,
		"valor_invalido": `{"prohibir_confianza_forzada": "si"}`,
	} {
		t.Run(nombre, func(t *testing.T) {
			almacen, userDir := almacenConPolicy(t, policy)
			previa := `{"confianza_forzada": [{"tipo": "algoritmo_sha1", "aceptada_en": "2026-11-13T00:00:00Z", "aviso": "verify.forced_trust.notice.algoritmo_sha1"}]}`
			if err := os.WriteFile(filepath.Join(userDir, "config.json"), []byte(previa), 0o600); err != nil {
				t.Fatal(err)
			}
			confianza, _ := almacen.CargarConfianzaForzada(context.Background())
			if !confianza.Prohibida {
				t.Fatal("la política debe prohibir la confianza forzada")
			}
			if confianza.Acepta(domain.ObstaculoConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1}) {
				t.Fatal("con la política prohibiéndolo, lo aceptado no puede aplicarse")
			}
			if err := almacen.AceptarConfianzaForzada(context.Background(), aceptacionPrueba()); !errors.Is(err, domain.ErrConfianzaProhibida) {
				t.Fatalf("aceptar con la política prohibiéndolo: %v", err)
			}
		})
	}

	almacen, _ := almacenConPolicy(t, `{"prohibir_confianza_forzada": false}`)
	if confianza, _ := almacen.CargarConfianzaForzada(context.Background()); confianza.Prohibida {
		t.Fatal("prohibir_confianza_forzada=false no prohíbe")
	}
}

// El usuario no puede saltarse la política escribiendo la clave en su
// config.json.
func TestAlmacenConfianzaForzada_UsuarioNoDecideLaPolitica(t *testing.T) {
	almacen, userDir := almacenConPolicy(t, `{"prohibir_confianza_forzada": true}`)
	if err := os.WriteFile(filepath.Join(userDir, "config.json"), []byte(`{"prohibir_confianza_forzada": false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if confianza, _ := almacen.CargarConfianzaForzada(context.Background()); !confianza.Prohibida {
		t.Fatal("config.json del usuario no puede levantar la prohibición de la máquina")
	}
}
