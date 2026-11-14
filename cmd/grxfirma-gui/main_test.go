// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestArgumentosFrontend_VinculaQtYWinUIAlBackend(t *testing.T) {
	t.Parallel()

	const socket = `\\.\pipe\grxfirma_ipc_test`
	qt := argumentosFrontend(frontendQt, socket, 1234)
	if got := strings.Join(qt, "|"); got != "--ipc-socket|"+socket+"|--backend-pid|1234" {
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

// D-01: un XDG_RUNTIME_DIR que no es privado no se usa; se recurre al
// directorio propio ~/.local/run con permisos 0700.
func TestCalcularSocketPath_RechazaRuntimeNoPrivado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows usa named pipes")
	}
	abierto := t.TempDir()
	if err := os.Chmod(abierto, 0o755); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	home := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", abierto)
	t.Setenv("HOME", home)
	ruta, err := calcularSocketPath()
	if err != nil {
		t.Fatalf("calcularSocketPath: %v", err)
	}
	if want := filepath.Join(home, ".local", "run"); filepath.Dir(ruta) != want {
		t.Fatalf("socket en %q; se esperaba %q", filepath.Dir(ruta), want)
	}
	info, err := os.Lstat(filepath.Dir(ruta))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("el directorio del socket no es 0700: %v %v", info, err)
	}
}

// D-01: sin directorio privado no se cae nunca al temporal compartido.
func TestCalcularSocketPath_NuncaUsaElTemporalCompartido(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows usa named pipes")
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", "")
	ruta, err := calcularSocketPath()
	if err == nil {
		t.Fatalf("se generó %q sin directorio privado", ruta)
	}
}

// D-01: un ~/.local/run que es un enlace simbólico no se acepta.
func TestCalcularSocketPath_RechazaEnlaceComoDirectorio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows usa named pipes")
	}
	home := t.TempDir()
	destino := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".local"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(destino, filepath.Join(home, ".local", "run")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", home)
	if ruta, err := calcularSocketPath(); err == nil {
		t.Fatalf("se aceptó un enlace como directorio del socket: %q", ruta)
	}
}

// D-01: el lanzador pasa a Qt el PID del backend para que Qt compruebe con
// el sistema que el otro extremo del socket es ese proceso.
func TestArgumentosFrontend_PasaElPIDDelBackendAQt(t *testing.T) {
	t.Parallel()
	got := strings.Join(argumentosFrontend(frontendQt, "/run/user/1/s.sock", 4321), "|")
	if got != "--ipc-socket|/run/user/1/s.sock|--backend-pid|4321" {
		t.Fatalf("argumentos Qt = %q", got)
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

func TestFrontendPorDefectoEsElNativoDeCadaSistema(t *testing.T) {
	for goos, want := range map[string]string{"windows": "winui", "linux": "qt", "darwin": "qt"} {
		if got := frontendPorDefecto(goos); got != want {
			t.Errorf("frontendPorDefecto(%q) = %q, se esperaba %q", goos, got, want)
		}
	}
}

// D-03 y lote 16c (retrocompatibilidad): el servicio de usuario de Linux y
// cualquier --server manual arrancan sin --ipc-client-pid. El motor abre el
// canal (mismo UID, socket 0600), avisa en la salida de error y en el
// registro, y no admite la firma remota CSC porque no sabe quién se conecta.
func TestRunServerMode_SinPIDClienteArrancaConAvisoYSinFirmaRemota(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("la prueba habla por un socket Unix; en Windows el canal es una named pipe")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)
	ruta := filepath.Join(dir, "s.sock")

	var registro bufferSeguro
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&registro, nil)))
	defer slog.SetDefault(anterior)

	loc := localizador.Para("es")
	var salidaError bufferSeguro
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	codigo := make(chan int, 1)
	go func() {
		codigo <- runServerMode(ctx, "ipc", "", "", ruta, 0, false, loc,
			ipc.LocalTLSStartupStatus{}, filepath.Join(home, ".config", "grxfirma"), &salidaError)
	}()

	var info os.FileInfo
	espera := time.NewTicker(20 * time.Millisecond)
	defer espera.Stop()
	for {
		if info, err = os.Lstat(ruta); err == nil {
			break
		}
		select {
		case c := <-codigo:
			t.Fatalf("runServerMode sin PID terminó con %d sin abrir el canal; salida: %s", c, salidaError.String())
		case <-espera.C:
		}
		if ctx.Err() != nil {
			t.Fatal("el motor no abrió el socket a tiempo")
		}
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permisos del socket = %o; se esperaba 0600", info.Mode().Perm())
	}

	conn, err := net.Dial("unix", ruta)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	enc, dec := json.NewEncoder(conn), json.NewDecoder(conn)

	if err := enc.Encode(map[string]any{"action": "csc_configure", "params": map[string]any{
		"serviceUrl": "https://csc.example.invalid/csc/v2", "clientId": "grxfirma",
	}}); err != nil {
		t.Fatal(err)
	}
	var csc struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"errorCode"`
	}
	if err := dec.Decode(&csc); err != nil {
		t.Fatal(err)
	}
	if csc.OK || csc.ErrorCode != "csc_interfaz_no_verificada" {
		t.Fatalf("sin vínculo de PID la firma remota CSC debe rechazarse: %+v", csc)
	}

	if err := enc.Encode(map[string]any{"action": "export_diagnostic", "params": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	var diag struct {
		OK   bool `json:"ok"`
		Data struct {
			IPCClientBinding string `json:"ipcClientBinding"`
		} `json:"data"`
	}
	if err := dec.Decode(&diag); err != nil {
		t.Fatal(err)
	}
	if !diag.OK || diag.Data.IPCClientBinding != "unbound" {
		t.Fatalf("el diagnóstico debe decir que no hay vínculo: %+v", diag)
	}

	cancel()
	select {
	case c := <-codigo:
		if c != 0 {
			t.Fatalf("runServerMode = %d al cerrarse", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el motor no terminó al cancelar el contexto")
	}
	if aviso := loc.T("desktop.ipc.server.sin_vinculo_pid"); aviso == "desktop.ipc.server.sin_vinculo_pid" ||
		!strings.Contains(salidaError.String(), aviso) {
		t.Fatalf("falta el aviso en la salida de error: %q", salidaError.String())
	}
	if !strings.Contains(registro.String(), "ipc_sin_vinculacion_pid") {
		t.Fatalf("falta el aviso en el registro: %q", registro.String())
	}
}

// bufferSeguro deja escribir desde la goroutine del motor y leer desde la
// prueba sin carreras.
type bufferSeguro struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *bufferSeguro) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *bufferSeguro) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// D-04: un backend lanzado por la GUI con --ipc-parent-stdin termina y borra
// su socket cuando se cierra la entrada heredada, que es lo que pasa cuando la
// GUI muere aunque sea de forma abrupta (SIGKILL, cuelgue, OOM).
func TestRunServerMode_TerminaAlCerrarseLaEntradaDelFrontend(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("la prueba usa un socket Unix; en Windows el mecanismo es el mismo pipe de entrada")
	}
	home := t.TempDir()
	dir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)
	ruta := filepath.Join(dir, "b.sock")

	hijo := exec.Command(os.Args[0], "-test.run=^TestRunServerMode_HelperBackendVinculado$")
	hijo.Env = append(os.Environ(),
		"GRXFIRMA_TEST_BACKEND_SOCKET="+ruta,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
	)
	entrada, err := hijo.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	var salida strings.Builder
	hijo.Stdout = &salida
	hijo.Stderr = &salida
	if err := hijo.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	terminado := make(chan error, 1)
	go func() { terminado <- hijo.Wait() }()
	defer func() { _ = hijo.Process.Kill() }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Lstat(ruta); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("el backend no abrió el socket a tiempo")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Simula la muerte del frontend: se cierra su extremo de la entrada.
	_ = entrada.Close()
	select {
	case <-terminado:
	case <-time.After(2 * time.Second):
		t.Fatal("el backend sigue vivo 2 s después de cerrarse la entrada del frontend")
	}
	if _, err := os.Lstat(ruta); err == nil {
		t.Fatal("el backend terminó sin borrar su socket")
	}
}

func TestRunServerMode_HelperBackendVinculado(t *testing.T) {
	ruta := os.Getenv("GRXFIRMA_TEST_BACKEND_SOCKET")
	if ruta == "" {
		t.Skip("helper exclusivo del subproceso de prueba")
	}
	home := os.Getenv("HOME")
	code := runServerMode(context.Background(), "ipc", "", "", ruta,
		uint64(os.Getppid()), true, localizador.Para("es"),
		ipc.LocalTLSStartupStatus{}, filepath.Join(home, ".config", "grxfirma"), os.Stderr)
	if code != 0 {
		t.Fatalf("runServerMode = %d", code)
	}
}
