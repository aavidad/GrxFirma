// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

var huellaCLIPrueba = strings.Repeat("c", 64)

type gestionConfianzaMock struct {
	confianza domain.ConfianzaForzada
	aceptadas []domain.ObstaculoConfianza
	quitadas  []string
}

func (m *gestionConfianzaMock) Listar(context.Context) (domain.ConfianzaForzada, error) {
	return m.confianza, nil
}

func (m *gestionConfianzaMock) Aceptar(_ context.Context, o domain.ObstaculoConfianza) (domain.AceptacionConfianza, error) {
	if m.confianza.Prohibida {
		return domain.AceptacionConfianza{}, domain.ErrConfianzaProhibida
	}
	a, err := domain.AceptacionConfianza{Tipo: o.Tipo, Huella: o.Huella, Sujeto: o.Sujeto, AceptadaEn: time.Now()}.Normalizar()
	if err != nil {
		return a, err
	}
	m.aceptadas = append(m.aceptadas, o)
	return a, nil
}

func (m *gestionConfianzaMock) Quitar(_ context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error) {
	m.quitadas = append(m.quitadas, string(tipo)+"|"+huella)
	return true, nil
}

type verifyForzableMock struct {
	forzable  []domain.ObstaculoConfianza
	prohibida bool
}

func (m verifyForzableMock) Execute(context.Context, application.VerifyCommand) (application.VerifyResult, error) {
	res := domain.NewVerificationFailure("CAdES", "autoridad desconocida", nil)
	res.Warnings = append(res.Warnings, domain.ClavePistaConfianzaForzable)
	return application.VerifyResult{
		Verification:     res,
		ConfianzaForzada: application.ResultadoConfianzaForzada{Forzable: m.forzable, Prohibida: m.prohibida},
	}, nil
}

func ficheroFirmaCLI(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "firma con espacio.csig")
	if err := os.WriteFile(ruta, []byte("firma"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func adaptadorConfianza(gestion *gestionConfianzaMock, forzable []domain.ObstaculoConfianza, interactivo bool, stdin string) (*Adaptador, *strings.Builder, *strings.Builder) {
	a := New(nil, nil).WithVerificador(verifyForzableMock{forzable: forzable}).WithConfianzaForzada(gestion)
	var stdout, stderr strings.Builder
	a.Stdout = &stdout
	a.Stderr = &stderr
	a.Stdin = strings.NewReader(stdin)
	a.esInteractivo = func() bool { return interactivo }
	return a, &stdout, &stderr
}

// Lote 14: al verificar, si aceptar bastaría, la CLI dice cómo hacerlo.
func TestCLIConfianzaForzada_VerificarMuestraComoAceptar(t *testing.T) {
	entrada := ficheroFirmaCLI(t)
	forzable := []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaCADesconocida, Huella: huellaCLIPrueba}}
	a, stdout, _ := adaptadorConfianza(&gestionConfianzaMock{}, forzable, false, "")
	if rc := a.Run(context.Background(), []string{"-operacion", "verificar", "-entrada", entrada}); rc != 2 {
		t.Fatalf("una firma no válida devuelve 2, no %d", rc)
	}
	salida := stdout.String()
	if !strings.Contains(salida, "-aceptar-confianza -entrada '"+entrada+"'") {
		t.Fatalf("falta la orden para aceptar: %s", salida)
	}
	if !strings.Contains(salida, domain.ClavePistaConfianzaForzable) {
		t.Fatalf("falta la pista en los avisos: %s", salida)
	}

	// Sin gestión de confianza (otros canales) no se sugiere nada.
	sin := New(nil, nil).WithVerificador(verifyForzableMock{forzable: forzable})
	var out strings.Builder
	sin.Stdout = &out
	sin.Stderr = &strings.Builder{}
	sin.Run(context.Background(), []string{"-operacion", "verificar", "-entrada", entrada})
	if strings.Contains(out.String(), "-aceptar-confianza") {
		t.Fatalf("sin gestión de confianza no se puede sugerir aceptar: %s", out.String())
	}
}

func TestCLIConfianzaForzada_AceptarSinTerminalExigeOpcion(t *testing.T) {
	entrada := ficheroFirmaCLI(t)
	forzable := []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaCADesconocida, Huella: huellaCLIPrueba, Sujeto: "CN=Raíz"}}
	gestion := &gestionConfianzaMock{}
	a, stdout, stderr := adaptadorConfianza(gestion, forzable, false, "s\n")
	if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada}); rc != 1 {
		t.Fatalf("sin terminal y sin -acepto-el-aviso debe fallar: rc=%d", rc)
	}
	if len(gestion.aceptadas) != 0 {
		t.Fatalf("se aceptó sin confirmación: %v", gestion.aceptadas)
	}
	if !strings.Contains(stdout.String(), domain.ConfianzaCADesconocida.ClaveAviso()) || !strings.Contains(stdout.String(), huellaCLIPrueba) {
		t.Fatalf("antes de pedir confirmación se muestra el aviso y la huella: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "verify.forced_trust.cli.error.needs_confirmation") {
		t.Fatalf("error esperado: %s", stderr.String())
	}

	a, _, _ = adaptadorConfianza(gestion, forzable, false, "")
	if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada, "-acepto-el-aviso"}); rc != 0 {
		t.Fatalf("con -acepto-el-aviso debe aceptar: rc=%d", rc)
	}
	if len(gestion.aceptadas) != 1 || gestion.aceptadas[0].Huella != huellaCLIPrueba {
		t.Fatalf("aceptadas: %v", gestion.aceptadas)
	}
}

func TestCLIConfianzaForzada_AceptarInteractivoPreguntaYRespeta(t *testing.T) {
	entrada := ficheroFirmaCLI(t)
	forzable := []domain.ObstaculoConfianza{
		{Tipo: domain.ConfianzaCertificadoComponente, Huella: huellaCLIPrueba},
		{Tipo: domain.ConfianzaRevocacionNoComprobable},
	}
	gestion := &gestionConfianzaMock{}
	a, _, _ := adaptadorConfianza(gestion, forzable, true, "n\n")
	if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada}); rc != 0 || len(gestion.aceptadas) != 0 {
		t.Fatalf("respondiendo no, no se acepta nada: rc=%d %v", rc, gestion.aceptadas)
	}
	a, _, _ = adaptadorConfianza(gestion, forzable, true, "s\n")
	if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada, "-tipo-confianza", "revocacion_no_comprobable"}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if len(gestion.aceptadas) != 1 || gestion.aceptadas[0].Tipo != domain.ConfianzaRevocacionNoComprobable {
		t.Fatalf("con -tipo-confianza solo se acepta ese caso: %v", gestion.aceptadas)
	}
}

// Si la firma no se puede aceptar (otro problema, o ya es válida), no se
// guarda nada aunque se pida.
func TestCLIConfianzaForzada_NadaQueAceptar(t *testing.T) {
	entrada := ficheroFirmaCLI(t)
	gestion := &gestionConfianzaMock{}
	a, stdout, _ := adaptadorConfianza(gestion, nil, false, "")
	if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada, "-acepto-el-aviso"}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if len(gestion.aceptadas) != 0 || !strings.Contains(stdout.String(), "verify.forced_trust.cli.nothing_to_accept") {
		t.Fatalf("no había nada que aceptar: %v %s", gestion.aceptadas, stdout.String())
	}
}

func TestCLIConfianzaForzada_ListarYQuitar(t *testing.T) {
	gestion := &gestionConfianzaMock{confianza: domain.ConfianzaForzada{Aceptaciones: []domain.AceptacionConfianza{
		{Tipo: domain.ConfianzaCADesconocida, Huella: huellaCLIPrueba, Sujeto: "CN=Raíz", AceptadaEn: time.Now(), Aviso: domain.ConfianzaCADesconocida.ClaveAviso()},
	}}}
	a, stdout, _ := adaptadorConfianza(gestion, nil, false, "")
	if rc := a.Run(context.Background(), []string{"-listar-confianza-aceptada", "-salida-json"}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if !strings.Contains(stdout.String(), `"huella": "`+huellaCLIPrueba+`"`) && !strings.Contains(stdout.String(), `"huella":"`+huellaCLIPrueba+`"`) {
		t.Fatalf("listado JSON: %s", stdout.String())
	}
	a, _, _ = adaptadorConfianza(gestion, nil, false, "")
	if rc := a.Run(context.Background(), []string{"-quitar-confianza-aceptada", "-tipo-confianza", "ca_desconocida", "-huella", huellaCLIPrueba}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if len(gestion.quitadas) != 1 || gestion.quitadas[0] != "ca_desconocida|"+huellaCLIPrueba {
		t.Fatalf("quitadas: %v", gestion.quitadas)
	}
}

func TestCLIConfianzaForzada_PoliticaProhibe(t *testing.T) {
	entrada := ficheroFirmaCLI(t)
	gestion := &gestionConfianzaMock{}
	a := New(nil, nil).WithVerificador(verifyForzableMock{prohibida: true}).WithConfianzaForzada(gestion)
	var stderr strings.Builder
	a.Stdout = &strings.Builder{}
	a.Stderr = &stderr
	a.esInteractivo = func() bool { return false }
	if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada, "-acepto-el-aviso"}); rc != 1 {
		t.Fatalf("con la política prohibiéndolo debe fallar: rc=%d", rc)
	}
	if len(gestion.aceptadas) != 0 || !strings.Contains(stderr.String(), domain.ClaveConfianzaForzadaProhibida) {
		t.Fatalf("aceptadas=%v stderr=%s", gestion.aceptadas, stderr.String())
	}
}
