// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package truststore_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/truststore"
	"grxfirma/internal/domain"
)

// helper: crea un GestorConfianza con configDir temporal (sin allowlist del sistema).
func nuevoGestorTmp(t *testing.T, headless bool) (*truststore.GestorConfianza, string) {
	t.Helper()
	dir := t.TempDir()
	g, err := truststore.New(dir, headless)
	if err != nil {
		t.Fatalf("New fallo: %v", err)
	}
	return g, dir
}

// helper: escribe una allowlist del sistema en la ruta del paquete (se sobreescribe la constante
// interna usando el mecanismo de inyeccion del constructor de prueba que acepta la ruta).
// Como la ruta del sistema esta fija en /etc/grxfirma/allowed-domains.json y no podemos
// escribir ahi en tests, utilizamos NewWithSystemAllowlist que acepta la ruta como parametro.
func nuevoGestorConAllowlist(t *testing.T, allowlistPath string, headless bool) *truststore.GestorConfianza {
	t.Helper()
	dir := t.TempDir()
	g, err := truststore.NewWithOptions(dir, truststore.Options{
		SystemAllowlistFile: allowlistPath,
		Headless:            headless,
		TOFUEnabled:         true,
	})
	if err != nil {
		t.Fatalf("NewWithSystemAllowlist fallo: %v", err)
	}
	return g
}

func nuevoGestorConOpciones(t *testing.T, headless, tofuEnabled bool) *truststore.GestorConfianza {
	t.Helper()
	dir := t.TempDir()
	g, err := truststore.NewWithOptions(dir, truststore.Options{
		SystemAllowlistFile: truststore.SystemAllowlistPath,
		Headless:            headless,
		TOFUEnabled:         tofuEnabled,
	})
	if err != nil {
		t.Fatalf("NewWithOptions fallo: %v", err)
	}
	return g
}

func TestEvaluate_AllowlistSistema_RetornaTrustAllowed(t *testing.T) {
	t.Parallel()

	// Creamos un fichero de allowlist del sistema en un directorio temporal.
	tmp := t.TempDir()
	allowlistFile := filepath.Join(tmp, "allowed-domains.json")
	dominios := []string{"https://example.com", "https://sede.juntadeandalucia.es"}
	data, _ := json.Marshal(dominios)
	if err := os.WriteFile(allowlistFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	g := nuevoGestorConAllowlist(t, allowlistFile, false)
	ctx := context.Background()

	dec, err := g.Evaluate(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("Evaluate fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}
	if dec.Origin != "https://example.com" {
		t.Fatalf("origen incorrecto: %s", dec.Origin)
	}
}

func TestEvaluate_AllowlistSistema_AceptaPatronesPorHost(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	allowlistFile := filepath.Join(tmp, "allowed-domains.json")
	data, _ := json.Marshal([]string{"*.gob.es", "localhost"})
	if err := os.WriteFile(allowlistFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	g := nuevoGestorConAllowlist(t, allowlistFile, false)
	ctx := context.Background()

	dec, err := g.Evaluate(ctx, "https://sede.administracion.gob.es")
	if err != nil {
		t.Fatalf("Evaluate con patrón fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}

	dec, err = g.Evaluate(ctx, "https://localhost:63118")
	if err != nil {
		t.Fatalf("Evaluate localhost fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed para localhost, se obtuvo %s", dec.Status)
	}

	// Un patrón de dominio no autoriza orígenes sin TLS.
	dec, err = g.Evaluate(ctx, "http://localhost:63118")
	if err != nil {
		t.Fatalf("Evaluate http localhost fallo: %v", err)
	}
	if dec.Status == domain.TrustAllowed {
		t.Fatal("un patrón de dominio no debe autorizar http://")
	}
}

func TestEvaluate_AllowlistSistema_NoPreguntar(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	allowlistFile := filepath.Join(tmp, "allowed-domains.json")
	data, _ := json.Marshal([]string{"https://trusted.example.com"})
	os.WriteFile(allowlistFile, data, 0o600)

	// Modo headless: si estuviera en allowlist no deberia dar error aunque sea headless.
	g := nuevoGestorConAllowlist(t, allowlistFile, true)
	ctx := context.Background()

	dec, err := g.Evaluate(ctx, "https://trusted.example.com")
	if err != nil {
		t.Fatalf("Evaluate fallo para dominio en allowlist headless: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}
}

func TestAllow_PersisteDominio_EvalRetornaTrustAllowed(t *testing.T) {
	t.Parallel()

	g, dir := nuevoGestorTmp(t, false)
	ctx := context.Background()

	if err := g.Allow(ctx, "https://mi-sede.es"); err != nil {
		t.Fatalf("Allow fallo: %v", err)
	}

	// Verifica que el fichero se escribio.
	path := filepath.Join(dir, "trusted-domains.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se encontro fichero de decisiones: %v", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	if raw["https://mi-sede.es"] != "allowed" {
		t.Fatalf("valor inesperado en fichero: %q", raw["https://mi-sede.es"])
	}

	// Evaluate debe retornar TrustAllowed.
	dec, err := g.Evaluate(ctx, "https://mi-sede.es")
	if err != nil {
		t.Fatalf("Evaluate fallo tras Allow: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}
}

func TestAllow_ConfiarseSiempreNoVuelveAPreguntar(t *testing.T) {
	t.Parallel()

	g, _ := nuevoGestorTmp(t, false)
	ctx := context.Background()

	if err := g.Allow(ctx, "https://example.org"); err != nil {
		t.Fatalf("Allow fallo: %v", err)
	}

	// Llamamos varias veces, debe seguir retornando TrustAllowed.
	for i := 0; i < 3; i++ {
		dec, err := g.Evaluate(ctx, "https://example.org")
		if err != nil {
			t.Fatalf("Evaluate fallo en iteracion %d: %v", i, err)
		}
		if dec.Status != domain.TrustAllowed {
			t.Fatalf("iteracion %d: se esperaba TrustAllowed, se obtuvo %s", i, dec.Status)
		}
	}
}

func TestDeny_PersisteDominio_EvalRetornaTrustDenied(t *testing.T) {
	t.Parallel()

	g, dir := nuevoGestorTmp(t, false)
	ctx := context.Background()

	if err := g.Deny(ctx, "https://malo.es"); err != nil {
		t.Fatalf("Deny fallo: %v", err)
	}

	// Verifica persistencia.
	path := filepath.Join(dir, "trusted-domains.json")
	data, _ := os.ReadFile(path)
	var raw map[string]string
	json.Unmarshal(data, &raw)
	if raw["https://malo.es"] != "denied" {
		t.Fatalf("valor inesperado: %q", raw["https://malo.es"])
	}

	// Evaluate debe retornar TrustDenied.
	dec, err := g.Evaluate(ctx, "https://malo.es")
	if err != nil {
		t.Fatalf("Evaluate fallo tras Deny: %v", err)
	}
	if dec.Status != domain.TrustDenied {
		t.Fatalf("se esperaba TrustDenied, se obtuvo %s", dec.Status)
	}
}

func TestEvaluate_Headless_DesconocidoRetornaError(t *testing.T) {
	t.Parallel()

	g, _ := nuevoGestorTmp(t, true)
	ctx := context.Background()

	_, err := g.Evaluate(ctx, "https://desconocido.example.com")
	if err == nil {
		t.Fatal("se esperaba error en modo headless para origen desconocido")
	}
	if !errors.Is(err, truststore.ErrHeadlessUnknownOrigin) {
		t.Fatalf("se esperaba ErrHeadlessUnknownOrigin, se obtuvo: %v", err)
	}
}

func TestEvaluate_TOFUDeshabilitado_DesconocidoRetornaError(t *testing.T) {
	t.Parallel()

	g := nuevoGestorConOpciones(t, false, false)
	ctx := context.Background()

	_, err := g.Evaluate(ctx, "https://nuevo.example.com")
	if err == nil {
		t.Fatal("se esperaba error con TOFU deshabilitado para origen desconocido")
	}
	if !errors.Is(err, truststore.ErrTOFUDisabledUnknownOrigin) {
		t.Fatalf("se esperaba ErrTOFUDisabledUnknownOrigin, se obtuvo: %v", err)
	}
}

func TestEvaluate_OrigenDesconocido_ModoCLI_RetornaPending(t *testing.T) {
	t.Parallel()

	g, _ := nuevoGestorTmp(t, false)
	ctx := context.Background()

	dec, err := g.Evaluate(ctx, "https://nuevo.example.com")
	if err != nil {
		t.Fatalf("Evaluate fallo: %v", err)
	}
	if dec.Status != domain.TrustPending {
		t.Fatalf("se esperaba TrustPending, se obtuvo %s", dec.Status)
	}
}

func TestRemove_EliminaDecisionPersistida(t *testing.T) {
	t.Parallel()

	g, _ := nuevoGestorTmp(t, false)
	ctx := context.Background()

	if err := g.Allow(ctx, "https://temporal.es"); err != nil {
		t.Fatal(err)
	}
	dec, _ := g.Evaluate(ctx, "https://temporal.es")
	if dec.Status != domain.TrustAllowed {
		t.Fatal("deberia ser TrustAllowed antes de Remove")
	}

	if err := g.Remove(ctx, "https://temporal.es"); err != nil {
		t.Fatalf("Remove fallo: %v", err)
	}

	dec, err := g.Evaluate(ctx, "https://temporal.es")
	if err != nil {
		t.Fatalf("Evaluate tras Remove fallo: %v", err)
	}
	if dec.Status != domain.TrustPending {
		t.Fatalf("tras Remove se esperaba TrustPending, se obtuvo %s", dec.Status)
	}
}

func TestNew_CargaDecisionesPersitidasDelFichero(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Escribe decisiones previas directamente.
	raw := map[string]string{
		"https://pre-allowed.es": "allowed",
		"https://pre-denied.es":  "denied",
	}
	data, _ := json.Marshal(raw)
	os.WriteFile(filepath.Join(dir, "trusted-domains.json"), data, 0o600)

	g, err := truststore.New(dir, false)
	if err != nil {
		t.Fatalf("New fallo: %v", err)
	}
	ctx := context.Background()

	dec, _ := g.Evaluate(ctx, "https://pre-allowed.es")
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}

	dec, _ = g.Evaluate(ctx, "https://pre-denied.es")
	if dec.Status != domain.TrustDenied {
		t.Fatalf("se esperaba TrustDenied, se obtuvo %s", dec.Status)
	}
}

func TestNew_SiNoExisteFicheroSiembraDominiosPorDefecto(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	g, err := truststore.New(dir, false)
	if err != nil {
		t.Fatalf("New fallo: %v", err)
	}

	dec, err := g.Evaluate(context.Background(), "https://sede.junta-andalucia.es")
	if err != nil {
		t.Fatalf("Evaluate fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}
	dec, err = g.Evaluate(context.Background(), "https://valide.redsara.es")
	if err != nil {
		t.Fatalf("Evaluate VALIDe fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba VALIDe en la semilla de compatibilidad, se obtuvo %s", dec.Status)
	}

	data, err := os.ReadFile(filepath.Join(dir, "trusted-domains.json"))
	if err != nil {
		t.Fatalf("no se escribió trusted-domains.json: %v", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json invalido: %v", err)
	}
	if raw["*.gob.es"] != "allowed" {
		t.Fatalf("faltan dominios por defecto sembrados")
	}
	if raw["valide.redsara.es"] != "allowed" {
		t.Fatalf("falta el origen exacto de VALIDe")
	}
}

func TestNew_MigraVALIDeUnaSolaVezEnPerfilYaSembrado(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data, _ := json.Marshal(map[string]string{"*.gob.es": "allowed"})
	if err := os.WriteFile(filepath.Join(dir, "trusted-domains.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile trusted-domains.json: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "trusted-domains.seeded"),
		[]byte("seeded-default-trusted-domains\n"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile trusted-domains.seeded: %v", err)
	}

	g, err := truststore.New(dir, false)
	if err != nil {
		t.Fatalf("New fallo: %v", err)
	}
	dec, err := g.Evaluate(context.Background(), "https://valide.redsara.es")
	if err != nil {
		t.Fatalf("Evaluate VALIDe fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("la migracion debe permitir VALIDe, estado=%s", dec.Status)
	}

	if err := g.Remove(context.Background(), "valide.redsara.es"); err != nil {
		t.Fatalf("Remove VALIDe fallo: %v", err)
	}
	g2, err := truststore.New(dir, false)
	if err != nil {
		t.Fatalf("segunda apertura fallo: %v", err)
	}
	dec, err = g2.Evaluate(context.Background(), "https://valide.redsara.es")
	if err != nil {
		t.Fatalf("Evaluate tras eliminar VALIDe fallo: %v", err)
	}
	if dec.Status != domain.TrustPending {
		t.Fatalf("la migracion no debe resembrar tras eliminar; estado=%s", dec.Status)
	}
}

func TestRemove_DominioSembradoSePuedeEliminar(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	g, err := truststore.New(dir, false)
	if err != nil {
		t.Fatalf("New fallo: %v", err)
	}

	ctx := context.Background()
	if err := g.Remove(ctx, "*.gob.es"); err != nil {
		t.Fatalf("Remove fallo: %v", err)
	}

	dec, err := g.Evaluate(ctx, "https://sede.gob.es")
	if err != nil {
		t.Fatalf("Evaluate tras remove fallo: %v", err)
	}
	if dec.Status != domain.TrustPending {
		t.Fatalf("tras eliminar dominio sembrado se esperaba pending, se obtuvo %s", dec.Status)
	}
}

func TestNew_SiYaExistePerfilMigraSemillaUnaSolaVez(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := map[string]string{
		"https://demos.guadaltel.es": "allowed",
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(filepath.Join(dir, "trusted-domains.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile fallo: %v", err)
	}

	g, err := truststore.New(dir, false)
	if err != nil {
		t.Fatalf("New fallo: %v", err)
	}

	dec, err := g.Evaluate(context.Background(), "https://sede.gob.es")
	if err != nil {
		t.Fatalf("Evaluate fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba dominio sembrado allowed, se obtuvo %s", dec.Status)
	}

	if err := g.Remove(context.Background(), "*.gob.es"); err != nil {
		t.Fatalf("Remove fallo: %v", err)
	}
	g2, err := truststore.New(dir, false)
	if err != nil {
		t.Fatalf("segunda apertura fallo: %v", err)
	}
	dec, err = g2.Evaluate(context.Background(), "https://sede.gob.es")
	if err != nil {
		t.Fatalf("Evaluate tras reapertura fallo: %v", err)
	}
	if dec.Status != domain.TrustPending {
		t.Fatalf("no debe resembrar tras limpiar; estado=%s", dec.Status)
	}
}

func TestNewWithOptions_TOFUDeshabilitado_NoSiembraDominiosPorDefecto(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	g, err := truststore.NewWithOptions(dir, truststore.Options{
		Headless:    false,
		TOFUEnabled: false,
	})
	if err != nil {
		t.Fatalf("NewWithOptions fallo: %v", err)
	}

	_, err = g.Evaluate(context.Background(), "https://sede.gob.es")
	if err == nil {
		t.Fatal("se esperaba error al evaluar un dominio no permitido con TOFU deshabilitado")
	}
	if !errors.Is(err, truststore.ErrTOFUDisabledUnknownOrigin) {
		t.Fatalf("se esperaba ErrTOFUDisabledUnknownOrigin, se obtuvo: %v", err)
	}
}

func TestNewWithOptions_TOFUDeshabilitado_IgnoraAllowsPersistidos(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := map[string]string{
		"https://persistido.example.com": "allowed",
		"https://denegado.example.com":   "denied",
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(filepath.Join(dir, "trusted-domains.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile fallo: %v", err)
	}

	g, err := truststore.NewWithOptions(dir, truststore.Options{
		Headless:    false,
		TOFUEnabled: false,
	})
	if err != nil {
		t.Fatalf("NewWithOptions fallo: %v", err)
	}

	_, err = g.Evaluate(context.Background(), "https://persistido.example.com")
	if err == nil {
		t.Fatal("se esperaba error para allow persistido con TOFU deshabilitado")
	}
	if !errors.Is(err, truststore.ErrTOFUDisabledUnknownOrigin) {
		t.Fatalf("se esperaba ErrTOFUDisabledUnknownOrigin, se obtuvo: %v", err)
	}

	dec, err := g.Evaluate(context.Background(), "https://denegado.example.com")
	if err != nil {
		t.Fatalf("Evaluate para deny persistido fallo: %v", err)
	}
	if dec.Status != domain.TrustDenied {
		t.Fatalf("se esperaba TrustDenied para deny persistido, se obtuvo %s", dec.Status)
	}
}

func TestNewWithOptions_TOFUDeshabilitado_AceptaAllowlistExtra(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	g, err := truststore.NewWithOptions(dir, truststore.Options{
		Headless:     false,
		TOFUEnabled:  false,
		ExtraAllowed: []string{"*.dipgra.es", "https://portal.example.org"},
	})
	if err != nil {
		t.Fatalf("NewWithOptions fallo: %v", err)
	}

	dec, err := g.Evaluate(context.Background(), "https://sede.dipgra.es")
	if err != nil {
		t.Fatalf("Evaluate con patrón extra fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}

	dec, err = g.Evaluate(context.Background(), "https://portal.example.org")
	if err != nil {
		t.Fatalf("Evaluate con origen exacto extra fallo: %v", err)
	}
	if dec.Status != domain.TrustAllowed {
		t.Fatalf("se esperaba TrustAllowed, se obtuvo %s", dec.Status)
	}
}
