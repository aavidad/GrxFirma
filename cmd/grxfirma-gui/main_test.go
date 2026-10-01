// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/common/secretinput"
	"grxfirma/internal/adapters/inbound/desktop/ipc"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/ports"
)

func TestResolverPasswordP12GUI_StdinTienePrioridad(t *testing.T) {
	got, err := resolverPasswordP12GUI(
		true,
		strings.NewReader("desde-stdin\n"),
		nil,
		"Contraseña: ",
		"desde-entorno",
	)
	if err != nil {
		t.Fatalf("resolverPasswordP12GUI() error = %v", err)
	}
	if got != "desde-stdin" {
		t.Fatalf("password = %q", got)
	}
}

func TestResolverPasswordP12GUI_EntornoSoloComoCompatibilidad(t *testing.T) {
	got, err := resolverPasswordP12GUI(
		false,
		nil,
		nil,
		"Contraseña: ",
		"desde-entorno",
	)
	if err != nil || got != "desde-entorno" {
		t.Fatalf("password = %q, err=%v", got, err)
	}
}

func TestEntornoGUI_ConsumeTodosLosSecretosAntesDeLanzarQt(t *testing.T) {
	names := []string{
		"GRXFIRMA_PKCS12_PASSWORD",
		"GRXFIRMA_REST_TOKEN",
		"GRXFIRMA_PROTECTION_SECRET_B64",
	}
	for _, name := range names {
		t.Setenv(name, "secreto-"+name)
	}
	captured, err := secretinput.ConsumeEnvironment(names, os.LookupEnv, os.Unsetenv)
	if err != nil {
		t.Fatalf("ConsumeEnvironment() error = %v", err)
	}
	for _, name := range names {
		if _, present := os.LookupEnv(name); present {
			t.Fatalf("%s siguió en el entorno heredable", name)
		}
	}
	if password, present := captured.Take("GRXFIRMA_PKCS12_PASSWORD"); !present || password == "" {
		t.Fatal("no se transfirió la contraseña P12 al único consumidor")
	}
}

func TestIdiomaPreferido_UsaDocumentoTipado(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	almacen := usersettings.New(dir)
	idioma := "gl"
	doc := ports.DocumentoConfiguracionUsuario{
		General: ports.ConfiguracionUsuarioGeneral{
			Idioma: &idioma,
		},
		Extras: map[string]any{
			"tema": "oscuro",
		},
	}
	if err := almacen.GuardarDocumento(context.Background(), doc); err != nil {
		t.Fatalf("GuardarDocumento: %v", err)
	}

	if got := idiomaPreferido(dir); got != "gl" {
		t.Fatalf("idiomaPreferido() = %q, want %q", got, "gl")
	}
}

func TestIdiomaPreferido_FallbackMapaLegacy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	almacen := usersettings.New(dir)
	if err := almacen.Guardar(context.Background(), map[string]any{
		"idioma": "eu",
		"tema":   "oscuro",
	}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	if got := idiomaPreferido(dir); got != "eu" {
		t.Fatalf("idiomaPreferido() = %q, want %q", got, "eu")
	}
}

func TestIdiomaPreferido_FallbackSistema(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	want := localizador.Detectar().Locale()
	if got := idiomaPreferido(dir); got != want {
		t.Fatalf("idiomaPreferido() = %q, want %q", got, want)
	}
}

func TestBuscarCandidatoQtEnDirectorio_ExigeFicheroEjecutable(t *testing.T) {
	dir := t.TempDir()
	candidatos := []string{"grxfirma-qt"}
	if runtime.GOOS != "windows" {
		noEjecutable := filepath.Join(dir, "grxfirma-gui-qml")
		if err := os.WriteFile(noEjecutable, []byte("stub"), 0o600); err != nil {
			t.Fatalf("no se pudo crear el candidato no ejecutable: %v", err)
		}
		candidatos = append([]string{"grxfirma-gui-qml"}, candidatos...)
	}
	ejecutable := filepath.Join(dir, "grxfirma-qt")
	if err := os.WriteFile(ejecutable, []byte("stub"), 0o700); err != nil {
		t.Fatalf("no se pudo crear el candidato ejecutable: %v", err)
	}

	got := buscarCandidatoQtEnDirectorio(dir, candidatos)
	if got != ejecutable {
		t.Fatalf("candidato encontrado = %q, se esperaba %q", got, ejecutable)
	}
}

func TestBuscarBinarioQt_NoConfiaEnArgsCero(t *testing.T) {
	dir := t.TempDir()
	falso := filepath.Join(dir, "grxfirma-gui-qml")
	if err := os.WriteFile(falso, []byte("stub"), 0o700); err != nil {
		t.Fatalf("no se pudo preparar el candidato falso: %v", err)
	}

	argsOriginales := os.Args
	os.Args = append([]string{filepath.Join(dir, "lanzador-falso")}, os.Args[1:]...)
	t.Cleanup(func() { os.Args = argsOriginales })
	t.Setenv("PATH", "")

	if got := buscarBinarioQt(); got == falso {
		t.Fatalf("buscarBinarioQt confio en os.Args[0]: %q", got)
	}
}

func TestBuscarFrontendInstaladoEnHermano(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Programs", "GrxFirma")
	launcher := filepath.Join(root, "DesktopLauncher")
	if err := os.MkdirAll(launcher, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		frontend          frontendKind
		component, binary string
	}{
		{frontendQt, "DesktopQML", "grxfirma-gui-qml.exe"},
		{frontendWinUI, "DesktopWinUI", "grxfirma-winui.exe"},
	} {
		path := filepath.Join(root, entry.component, entry.binary)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o700); err != nil {
			t.Fatal(err)
		}
		if got := buscarFrontendInstaladoEnHermano(launcher, entry.frontend); got != path {
			t.Fatalf("frontend %s: %q, esperado %q", entry.frontend, got, path)
		}
		if got := buscarFrontendInstaladoEnHermano(filepath.Join(root, "otro"), entry.frontend); got != "" {
			t.Fatalf("se aceptó un directorio que no es DesktopLauncher: %q", got)
		}
	}
}

func TestNormalizarFrontend(t *testing.T) {
	t.Parallel()

	got, err := normalizarFrontend("qml")
	if err != nil || got != frontendQt {
		t.Fatalf("normalizarFrontend(qml) = %q, %v", got, err)
	}
	if _, err := normalizarFrontend("desconocido"); err == nil {
		t.Fatal("se aceptó un frontend desconocido")
	}

	got, err = normalizarFrontend("winui")
	if runtime.GOOS == "windows" {
		if err != nil || got != frontendWinUI {
			t.Fatalf("normalizarFrontend(winui) = %q, %v", got, err)
		}
	} else if err == nil {
		t.Fatal("se aceptó WinUI fuera de Windows")
	}
}

func TestCandidatosFrontend_SeparaQtYWinUI(t *testing.T) {
	t.Parallel()

	qt := strings.Join(candidatosFrontend(frontendQt), "|")
	winui := strings.Join(candidatosFrontend(frontendWinUI), "|")
	if !strings.Contains(qt, "grxfirma-gui-qml") {
		t.Fatalf("candidatos Qt incompletos: %q", qt)
	}
	if !strings.Contains(winui, "grxfirma-winui.exe") {
		t.Fatalf("candidatos WinUI incompletos: %q", winui)
	}
	if strings.Contains(strings.ToLower(qt), "winui") ||
		strings.Contains(strings.ToLower(winui), "qml") {
		t.Fatalf("se mezclaron candidatos de frontends: qt=%q winui=%q", qt, winui)
	}
}

func TestArgumentosFrontend_VinculaWinUIAlBackendSinAlterarQt(t *testing.T) {
	t.Parallel()

	const socket = `\\.\pipe\grxfirma_ipc_test`
	qt := argumentosFrontend(frontendQt, socket, 1234)
	if got := strings.Join(qt, "|"); got != "--ipc-socket|"+socket {
		t.Fatalf("argumentos Qt = %q", got)
	}
	winui := argumentosFrontend(frontendWinUI, socket, 1234)
	if got := strings.Join(winui, "|"); got != "--ipc-socket|"+socket+"|--backend-pid|1234" {
		t.Fatalf("argumentos WinUI = %q", got)
	}
}

func TestNormalizarPIDFrontend_ValidaCLI(t *testing.T) {
	t.Parallel()

	for _, value := range []uint64{0, 1, uint64(^uint32(0))} {
		got, err := normalizarPIDFrontend(value)
		if err != nil || uint64(got) != value {
			t.Fatalf("normalizarPIDFrontend(%d) = %d, %v", value, got, err)
		}
	}
	if _, err := normalizarPIDFrontend(uint64(^uint32(0)) + 1); err == nil {
		t.Fatal("se acepto un PID fuera del rango uint32")
	}
}

func TestIniciarFrontendVinculado_StartFallidoDeniegaGate(t *testing.T) {
	if !ipc.SoportaVinculacionPIDFrontend() {
		t.Skip("la plataforma no expone el PID del peer IPC")
	}

	srv := ipc.New(nil)
	if err := srv.PrepararVinculacionPIDFrontend(); err != nil {
		t.Fatalf("PrepararVinculacionPIDFrontend: %v", err)
	}
	cmd := exec.Command(filepath.Join(t.TempDir(), "frontend-inexistente"))
	if err := iniciarFrontendVinculado(cmd, srv, true); err == nil {
		t.Fatal("Start de un frontend inexistente no fallo")
	}
	if err := srv.PublicarPIDFrontend(uint32(os.Getpid())); err == nil {
		t.Fatal("el gate permitio publicar despues de que Start lo denegara")
	}
}

func TestBuscarCandidatoFrontendEnDirectorio_AdmiteSubdirectorioWinUI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subdir := filepath.Join(dir, "winui")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bin := filepath.Join(subdir, "grxfirma-winui.exe")
	if err := os.WriteFile(bin, []byte("stub"), 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := buscarCandidatoFrontendEnDirectorio(
		dir,
		[]string{filepath.Join("winui", "grxfirma-winui.exe")},
	)
	if got != bin {
		t.Fatalf("candidato WinUI = %q, want %q", got, bin)
	}
}

func TestEntornoConBusSesion_AgregaSocketUnixValidado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("los sockets Unix de sesion no se prueban en Windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("no se pudieron fijar permisos privados en XDG_RUNTIME_DIR: %v", err)
	}
	busPath := filepath.Join(dir, "bus")
	listener, err := net.Listen("unix", busPath)
	if err != nil {
		t.Skipf("el sistema no permite crear un socket Unix: %v", err)
	}
	defer listener.Close()

	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", dir)
	got := ultimaVariable(entornoConBusSesion(), "DBUS_SESSION_BUS_ADDRESS")
	want := "unix:path=" + busPath
	if got != want {
		t.Fatalf("direccion del bus = %q, se esperaba %q", got, want)
	}
}

func TestEntornoConBusSesion_RechazaBusQueNoEsSocket(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bus"), []byte("no es un socket"), 0o600); err != nil {
		t.Fatalf("no se pudo preparar el bus falso: %v", err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", dir)

	if got := ultimaVariable(entornoConBusSesion(), "DBUS_SESSION_BUS_ADDRESS"); got != "" {
		t.Fatalf("se acepto un bus que no es socket: %q", got)
	}
}

func TestEntornoConBusSesion_RechazaRuntimeRelativo(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", "runtime-relativo")

	if got := ultimaVariable(entornoConBusSesion(), "DBUS_SESSION_BUS_ADDRESS"); got != "" {
		t.Fatalf("se acepto un XDG_RUNTIME_DIR relativo: %q", got)
	}
}

func TestCalcularSocketPath_GeneraEndpointsUnicos(t *testing.T) {
	t.Parallel()

	primero, err := calcularSocketPath()
	if err != nil {
		t.Fatalf("calcularSocketPath(primero): %v", err)
	}
	segundo, err := calcularSocketPath()
	if err != nil {
		t.Fatalf("calcularSocketPath(segundo): %v", err)
	}
	if primero == segundo {
		t.Fatalf("se reutilizo el endpoint IPC: %q", primero)
	}
	if !strings.Contains(primero, fmt.Sprintf("%d_", os.Getpid())) {
		t.Fatalf("el endpoint no contiene el PID de la instancia")
	}
	if runtime.GOOS == "windows" && !strings.HasPrefix(primero, `\\.\pipe\`) {
		t.Fatalf("endpoint Windows no es un named pipe local: %q", primero)
	}
}

func ultimaVariable(entorno []string, nombre string) string {
	prefijo := nombre + "="
	for i := len(entorno) - 1; i >= 0; i-- {
		if strings.HasPrefix(entorno[i], prefijo) {
			return strings.TrimPrefix(entorno[i], prefijo)
		}
	}
	return ""
}
