// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package logging

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("writer no disponible")
}

func TestNewUsaJSONEnProduccion(t *testing.T) {
	t.Setenv(envModo, "production")
	t.Setenv(envNivel, "INFO")

	var out bytes.Buffer
	logger := New("nativehost", &out)
	logger.InfoContext(context.Background(), "arranque", "op", "inicio")

	linea := out.String()
	if !strings.Contains(linea, "\"component\":\"nativehost\"") {
		t.Fatalf("se esperaba salida JSON con component, obtenido: %q", linea)
	}
	if !strings.Contains(linea, "\"op\":\"inicio\"") {
		t.Fatalf("se esperaba salida JSON con op, obtenido: %q", linea)
	}
}

func TestNewUsaTextoEnDesarrollo(t *testing.T) {
	t.Setenv(envModo, "development")
	t.Setenv(envNivel, "INFO")

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.InfoContext(context.Background(), "arranque", "op", "inicio")

	linea := out.String()
	if !strings.Contains(linea, "component=grxfirma") {
		t.Fatalf("se esperaba salida texto con component, obtenido: %q", linea)
	}
	if strings.Contains(linea, "{\"time\"") {
		t.Fatalf("no se esperaba salida JSON en desarrollo: %q", linea)
	}
}

func TestDebugNoSeEmiteConNivelInfoEnProduccion(t *testing.T) {
	t.Setenv(envModo, "production")
	t.Setenv(envNivel, "INFO")

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.DebugContext(context.Background(), "depuracion", "op", "debug")

	if out.Len() != 0 {
		t.Fatalf("no se esperaba salida DEBUG con nivel INFO: %q", out.String())
	}
}

func TestDebugSeEmiteConEnvDebugAunqueNivelSeaInfo(t *testing.T) {
	t.Setenv(envModo, "development")
	t.Setenv(envNivel, "INFO")
	t.Setenv(envDebug, "1")

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.DebugContext(context.Background(), "depuracion", "op", "debug")

	linea := out.String()
	if DebugAllowed() && !strings.Contains(linea, "depuracion") {
		t.Fatalf("se esperaba salida DEBUG con GRXFIRMA_DEBUG=1, obtenido: %q", linea)
	}
	if !DebugAllowed() && linea != "" {
		t.Fatalf("un build production no debe aceptar GRXFIRMA_DEBUG: %q", linea)
	}
}

func TestPoliticaDebugDelBuildTambienLimitaLogLevel(t *testing.T) {
	t.Setenv(envModo, "production")
	t.Setenv(envNivel, "DEBUG")
	t.Setenv(envDebug, "1")

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.DebugContext(context.Background(), "depuracion")
	if DebugAllowed() && out.Len() == 0 {
		t.Fatal("el build de desarrollo debe admitir DEBUG explícito")
	}
	if !DebugAllowed() && out.Len() != 0 {
		t.Fatalf("el build production emitió DEBUG: %q", out.String())
	}
}

func TestNewDuplicaSalidaEnFicheroCuandoSeConfigura(t *testing.T) {
	t.Setenv(envModo, "development")
	t.Setenv(envNivel, "INFO")

	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "grxfirma-debug.log")
	t.Setenv(envFich, logPath)

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.InfoContext(context.Background(), "arranque", "op", "inicio")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath): %v", err)
	}
	if !strings.Contains(string(data), "arranque") {
		t.Fatalf("se esperaba el mensaje en el fichero, obtenido: %q", string(data))
	}
	if !strings.Contains(out.String(), "arranque") {
		t.Fatalf("se esperaba tambien el mensaje en el writer original, obtenido: %q", out.String())
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("el logger debe cerrar el fichero tras cada escritura: %v", err)
	}
}

func TestNewEscribeEnFicheroAunqueElWriterPrincipalFalle(t *testing.T) {
	t.Setenv(envModo, "development")
	t.Setenv(envNivel, "INFO")

	logPath := filepath.Join(t.TempDir(), "grxfirma-debug.log")
	t.Setenv(envFich, logPath)

	logger := New("grxfirmauri", failingWriter{})
	logger.InfoContext(context.Background(), "arranque_windows_gui")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath): %v", err)
	}
	if !strings.Contains(string(data), "arranque_windows_gui") {
		t.Fatalf("el fallo de stderr no debe impedir el log persistente: %q", data)
	}
}

func TestNewRechazaEnlaceSimbolicoComoLog(t *testing.T) {
	t.Setenv(envModo, "development")
	t.Setenv(envNivel, "INFO")

	dir := t.TempDir()
	victima := filepath.Join(dir, "victima.log")
	if err := os.WriteFile(victima, []byte("intacto"), 0o600); err != nil {
		t.Fatalf("no se pudo preparar la victima: %v", err)
	}
	logPath := filepath.Join(dir, "grxfirma.log")
	if err := os.Symlink(victima, logPath); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}
	t.Setenv(envFich, logPath)

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.InfoContext(context.Background(), "no-seguir-enlace")

	data, err := os.ReadFile(victima)
	if err != nil {
		t.Fatalf("no se pudo leer la victima: %v", err)
	}
	if string(data) != "intacto" {
		t.Fatalf("el logger siguio el enlace simbolico: %q", data)
	}
	if !strings.Contains(out.String(), "no-seguir-enlace") {
		t.Fatal("el writer principal debe seguir operativo al rechazar el fichero")
	}
}

func TestNewRechazaEnlaceSimbolicoEnDirectorioDelLog(t *testing.T) {
	t.Setenv(envModo, "development")
	t.Setenv(envNivel, "INFO")

	dir := t.TempDir()
	destino := filepath.Join(dir, "destino")
	if err := os.Mkdir(destino, 0o700); err != nil {
		t.Fatalf("no se pudo preparar el directorio destino: %v", err)
	}
	enlace := filepath.Join(dir, "logs")
	if err := os.Symlink(destino, enlace); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}
	t.Setenv(envFich, filepath.Join(enlace, "grxfirma.log"))

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.InfoContext(context.Background(), "no-seguir-directorio")

	if _, err := os.Stat(filepath.Join(destino, "grxfirma.log")); !os.IsNotExist(err) {
		t.Fatal("el logger escribio a traves de un directorio simbolico")
	}
	if !strings.Contains(out.String(), "no-seguir-directorio") {
		t.Fatal("el writer principal debe seguir operativo al rechazar el directorio")
	}
}

func TestNewAdmiteAliasDeRaizTemporalPeroNoEnlacesInternos(t *testing.T) {
	t.Setenv(envModo, "development")
	t.Setenv(envNivel, "INFO")

	raizReal := t.TempDir()
	padreAlias := t.TempDir()
	raizAlias := filepath.Join(padreAlias, "temporal-alias")
	if err := os.Symlink(raizReal, raizAlias); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}
	t.Setenv("TMPDIR", raizAlias)
	t.Setenv("TMP", raizAlias)
	t.Setenv("TEMP", raizAlias)
	// GetTempPath2 usa SystemTemp cuando el proceso es LocalSystem.
	t.Setenv("SystemTemp", raizAlias)
	if filepath.Clean(os.TempDir()) != filepath.Clean(raizAlias) {
		t.Fatalf("la fixture no seleccionó la raíz temporal: %q", os.TempDir())
	}

	logPath := filepath.Join(raizAlias, "logs", "grxfirma.log")
	t.Setenv(envFich, logPath)
	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.InfoContext(context.Background(), "log-bajo-alias-confiable")

	data, err := os.ReadFile(filepath.Join(raizReal, "logs", "grxfirma.log"))
	if err != nil {
		t.Fatalf("no se creo el log bajo la raiz temporal canonica: %v", err)
	}
	if !strings.Contains(string(data), "log-bajo-alias-confiable") {
		t.Fatalf("contenido de log inesperado: %q", data)
	}

	destino := filepath.Join(raizReal, "destino")
	if err := os.Mkdir(destino, 0o700); err != nil {
		t.Fatalf("no se pudo crear el destino: %v", err)
	}
	enlaceInterno := filepath.Join(raizAlias, "logs-maliciosos")
	if err := os.Symlink(destino, enlaceInterno); err != nil {
		t.Fatalf("no se pudo crear el enlace interno: %v", err)
	}
	t.Setenv(envFich, filepath.Join(enlaceInterno, "escape.log"))
	logger = New("grxfirma", &out)
	logger.InfoContext(context.Background(), "no-seguir-enlace-interno")

	if _, err := os.Stat(filepath.Join(destino, "escape.log")); !os.IsNotExist(err) {
		t.Fatal("el logger escribio a traves de un enlace interno no confiable")
	}
	if !strings.Contains(out.String(), "no-seguir-enlace-interno") {
		t.Fatal("el writer principal dejo de funcionar al rechazar el enlace")
	}
}

func TestAppendFileWriterRechazaSymlinkTrasRotacion(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "grxfirma.log")
	writer, err := newAppendFileWriter(logPath)
	if err != nil {
		t.Fatalf("newAppendFileWriter: %v", err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("no se pudo retirar el log para simular rotacion: %v", err)
	}
	victima := filepath.Join(dir, "victima.log")
	if err := os.WriteFile(victima, []byte("intacto"), 0o600); err != nil {
		t.Fatalf("no se pudo preparar la victima: %v", err)
	}
	if err := os.Symlink(victima, logPath); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}

	if _, err := writer.Write([]byte("mensaje")); err == nil {
		t.Fatal("se esperaba error al sustituir el log por un enlace simbolico")
	}
	data, err := os.ReadFile(victima)
	if err != nil {
		t.Fatalf("no se pudo leer la victima: %v", err)
	}
	if string(data) != "intacto" {
		t.Fatalf("el escritor siguio el enlace introducido tras rotar: %q", data)
	}
}

func TestAppendFileWriterRechazaDirectorioSymlinkTrasRotacion(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	logPath := filepath.Join(logDir, "grxfirma.log")
	writer, err := newAppendFileWriter(logPath)
	if err != nil {
		t.Fatalf("newAppendFileWriter: %v", err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("no se pudo retirar el log: %v", err)
	}
	if err := os.Remove(logDir); err != nil {
		t.Fatalf("no se pudo retirar el directorio del log: %v", err)
	}
	destino := filepath.Join(dir, "destino")
	if err := os.Mkdir(destino, 0o700); err != nil {
		t.Fatalf("no se pudo crear el directorio victima: %v", err)
	}
	if err := os.Symlink(destino, logDir); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}

	if _, err := writer.Write([]byte("mensaje")); err == nil {
		t.Fatal("se esperaba error al sustituir el directorio por un enlace")
	}
	if _, err := os.Stat(filepath.Join(destino, "grxfirma.log")); !os.IsNotExist(err) {
		t.Fatal("el escritor siguio el directorio simbolico introducido tras rotar")
	}
}

func TestAppendFileWriterRecreaLogTrasRotacionRegular(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "grxfirma.log")
	writer, err := newAppendFileWriter(logPath)
	if err != nil {
		t.Fatalf("newAppendFileWriter: %v", err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("no se pudo retirar el log para simular rotacion: %v", err)
	}

	if _, err := writer.Write([]byte("mensaje")); err != nil {
		t.Fatalf("Write tras rotacion regular: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("no se pudo leer el log recreado: %v", err)
	}
	if string(data) != "mensaje" {
		t.Fatalf("contenido del log recreado = %q", data)
	}
}

func TestNewRedactaCamposSensibles(t *testing.T) {
	t.Setenv(envModo, "production")
	t.Setenv(envNivel, "INFO")

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.InfoContext(
		context.Background(),
		"prueba",
		"token", "valor-prueba-super-sensible",
		"certificatePEM", "MIIC......AB",
		"op", "inicio",
	)

	linea := out.String()
	if strings.Contains(linea, "valor-prueba-super-sensible") {
		t.Fatalf("el token no debería aparecer en claro: %q", linea)
	}
	if strings.Contains(linea, "MIIC......AB") {
		t.Fatalf("el certificado no debería aparecer en claro: %q", linea)
	}
	if !strings.Contains(linea, "\"token\":\"[REDACTED]\"") {
		t.Fatalf("se esperaba token redactado, obtenido: %q", linea)
	}
	if !strings.Contains(linea, "\"certificatePEM\":\"[REDACTED]\"") {
		t.Fatalf("se esperaba certificado redactado, obtenido: %q", linea)
	}
}

func TestNewEliminaRutaCompletaEnAtributosYSalida(t *testing.T) {
	t.Setenv(envModo, "production")
	t.Setenv(envNivel, "INFO")
	t.Setenv("HOME", "/home/usuario-prueba")

	var out bytes.Buffer
	logger := New("grxfirma", &out)
	logger.InfoContext(
		context.Background(),
		"ruta de prueba",
		"path", "/home/usuario-prueba/Documentos/privado.pdf",
	)

	linea := out.String()
	if strings.Contains(linea, "/home/usuario-prueba/") {
		t.Fatalf("el prefijo del home no deberia aparecer en claro: %q", linea)
	}
	if strings.Contains(linea, "privado.pdf") || strings.Contains(linea, "Documentos") {
		t.Fatalf("el nombre y los directorios no deberian aparecer en claro: %q", linea)
	}
	if !strings.Contains(linea, "\"path\":\"[LOCAL_PATH_REDACTED]\"") {
		t.Fatalf("se esperaba ruta eliminada, obtenido: %q", linea)
	}
}

func TestRedactPathPrefixes_WindowsStyle(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\UsuarioPrueba`)
	got := redactPathPrefixes(`C:\Users\UsuarioPrueba\Desktop\secreto.pdf`)
	if strings.Contains(got, `C:\Users\UsuarioPrueba`) {
		t.Fatalf("el prefijo USERPROFILE no deberia quedar en claro: %q", got)
	}
	if strings.Contains(got, `Desktop`) || strings.Contains(got, `secreto.pdf`) {
		t.Fatalf("la ruta relativa al perfil tampoco deberia quedar en claro: %q", got)
	}
	if !strings.Contains(got, redactedPathMarker) {
		t.Fatalf("ruta windows saneada inesperada: %q", got)
	}
}

func TestRedactPathPrefixes_NoRedactaPrefijosParciales(t *testing.T) {
	t.Setenv("HOME", "/home/usuario-prueba")
	got := redactPathPrefixes(`/home/usuario-prueba2/Documentos/publico.pdf`)
	if got != `/home/usuario-prueba2/Documentos/publico.pdf` {
		t.Fatalf("no deberia redactar coincidencias parciales, obtenido: %q", got)
	}
}

func TestRedactPathPrefixes_WindowsStyleConSlashes(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\UsuarioPrueba`)
	got := redactPathPrefixes(`C:/Users/UsuarioPrueba/Desktop/secreto.pdf`)
	if strings.Contains(got, `C:/Users/UsuarioPrueba`) {
		t.Fatalf("el prefijo USERPROFILE con slashes no deberia quedar en claro: %q", got)
	}
	if strings.Contains(got, `Desktop`) || strings.Contains(got, `secreto.pdf`) {
		t.Fatalf("la ruta relativa al perfil tampoco deberia quedar en claro: %q", got)
	}
	if !strings.Contains(got, redactedPathMarker) {
		t.Fatalf("ruta windows con slashes saneada inesperada: %q", got)
	}
}

func TestPathRedactingWriter_RedactaTextoLibre(t *testing.T) {
	t.Setenv("HOME", "/home/usuario-prueba")

	var out bytes.Buffer
	writer := pathRedactingWriter{dst: &out}
	if _, err := writer.Write([]byte(`fallo abriendo /home/usuario-prueba/privado.pdf`)); err != nil {
		t.Fatalf("Write(): %v", err)
	}

	got := out.String()
	if strings.Contains(got, "/home/usuario-prueba/") {
		t.Fatalf("el writer no deberia dejar el HOME en claro: %q", got)
	}
	if strings.Contains(got, "privado.pdf") {
		t.Fatalf("el writer no deberia dejar el nombre del documento en claro: %q", got)
	}
	if !strings.Contains(got, `fallo abriendo [LOCAL_PATH_REDACTED]`) {
		t.Fatalf("se esperaba redaccion del texto libre, obtenido: %q", got)
	}
}

func TestNewEliminaErroresPayloadsEIdentificadoresDelLogPersistente(t *testing.T) {
	t.Setenv(envModo, "production")
	t.Setenv(envNivel, "DEBUG")
	t.Setenv("HOME", "/home/usuario-prueba")

	dir := t.TempDir()
	logPath := filepath.Join(dir, "legacy.log")
	t.Setenv(envFich, logPath)

	var out bytes.Buffer
	logger := New("legacy/websocket", &out)
	logger.ErrorContext(
		context.Background(),
		"legacy_operation_failed",
		"error", errors.New("SAF_25: open /home/usuario-prueba/Documentos/nomina-secreta.pdf: permission denied token=abc123"),
		"message_prefix", "afirma://sign?dat=PAYLOAD-SECRETO&token=TOKEN-SECRETO",
		"payload_len", 123,
		"request_id", "request-super-secreto",
		"filename", "nomina-secreta.pdf",
	)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath): %v", err)
	}
	got := string(data)
	for _, secret := range []string{
		"nomina-secreta.pdf",
		"/home/usuario-prueba",
		"PAYLOAD-SECRETO",
		"TOKEN-SECRETO",
		"request-super-secreto",
		"abc123",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("el log persistente contiene %q: %s", secret, got)
		}
	}
	for _, safe := range []string{
		"legacy_operation_failed",
		"permission_denied",
		"SAF_25",
		redactedSecretMarker,
		redactedIDMarker,
		redactedPathMarker,
		`"payload_len":123`,
	} {
		if !strings.Contains(got, safe) {
			t.Fatalf("el log persistente no conserva %q: %s", safe, got)
		}
	}
}

func TestPathRedactingWriterEliminaTextoLibreFueraDeHome(t *testing.T) {
	var out bytes.Buffer
	writer := pathRedactingWriter{dst: &out}
	raw := strings.Join([]string{
		`fallo leyendo /mnt/documentos/nomina-secreta.pdf`,
		`request afirma://sign?dat=PAYLOAD-SECRETO&token=TOKEN-SECRETO`,
		`endpoint https://portal.example/private/upload?token=TOKEN-SECRETO`,
		`socket wss://127.0.0.1:63119/private email=alice@example.test dni=12345678Z`,
		`session_id=SESSION-SECRETA json {"request_id":"REQUEST-SECRETO","token":"TOKEN-JSON-SECRETO","filename":"nomina-secreta.pdf"}`,
		"-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("A", 96) + "\n-----END PRIVATE KEY-----",
	}, "\n")
	if _, err := writer.Write([]byte(raw)); err != nil {
		t.Fatalf("Write(): %v", err)
	}

	got := out.String()
	for _, secret := range []string{
		"/mnt/documentos",
		"nomina-secreta.pdf",
		"PAYLOAD-SECRETO",
		"TOKEN-SECRETO",
		"TOKEN-JSON-SECRETO",
		"SESSION-SECRETA",
		"REQUEST-SECRETO",
		"portal.example",
		"127.0.0.1",
		"alice@example.test",
		"12345678Z",
		strings.Repeat("A", 32),
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("el writer contiene %q: %s", secret, got)
		}
	}
	for _, marker := range []string{
		redactedPathMarker,
		"[AFIRMA_REQUEST_REDACTED]",
		redactedEndpointMarker,
		redactedIDMarker,
		"[PEM_REDACTED]",
	} {
		if !strings.Contains(got, marker) {
			t.Fatalf("el writer no contiene %q: %s", marker, got)
		}
	}
}

func TestNewEliminaDireccionesYCuerposEstructurados(t *testing.T) {
	t.Setenv(envModo, "production")
	var out bytes.Buffer
	logger := New("legacy", &out)
	logger.Info(
		"request_received",
		"remote_addr", "192.0.2.10:43120",
		"host", "private.example.test",
		"body", map[string]any{"dat": "short-secret"},
	)
	got := out.String()
	for _, secret := range []string{
		"192.0.2.10", "private.example.test", "short-secret",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("el log contiene %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, redactedEndpointMarker) ||
		!strings.Contains(got, redactedSecretMarker) {
		t.Fatalf("faltan marcadores seguros: %s", got)
	}
}
