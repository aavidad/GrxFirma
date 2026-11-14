// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"grxfirma/internal/ports"
)

// Reenvío de un enlace afirma:// a AutoFirma, cuando la persona (en el
// diálogo), su preferencia o la política de máquina lo piden.
//
// Reglas (lote 15):
//   - El ejecutable sale solo del manejador guardado por GrxFirma, del
//     registro del equipo o del usuario (Windows) o de un lanzador .desktop
//     (Linux); nunca del enlace.
//   - Se ejecuta sin shell, con el enlace como un único argumento.
//   - Se rechazan enlaces que no empiezan por afirma: o afirmav2:, o que
//     tienen comillas, espacios o caracteres de control; órdenes que no
//     tienen la forma «programa %1» y el propio GrxFirma (evita bucles).
//   - El registro de diagnóstico no guarda el enlace, que puede llevar datos
//     del documento o claves de sesión.

// EnvReenvioAfirma marca el proceso lanzado por un reenvío. Si GrxFirma
// recibe un enlace con esta marca, no lo vuelve a reenviar.
const EnvReenvioAfirma = "GRXFIRMA_REENVIO_AFIRMA"

// maxURIReenvio queda por debajo del límite de la línea de órdenes de
// Windows (32 767 caracteres).
const maxURIReenvio = 32000

var (
	// ErrURIReenvioNoValida: el enlace no se puede pasar a otro programa.
	ErrURIReenvioNoValida = errors.New("el enlace afirma:// no se puede reenviar")
	// ErrOrdenAutoFirmaNoValida: la orden registrada no tiene la forma
	// esperada o apunta al propio GrxFirma.
	ErrOrdenAutoFirmaNoValida = errors.New("la orden registrada de AutoFirma no es reconocible")
	// ErrReenvioEnBucle: este proceso ya viene de un reenvío.
	ErrReenvioEnBucle = errors.New("el enlace ya se reenvió una vez")
)

// ValidarURIReenvio comprueba que el enlace se puede pasar a AutoFirma como
// un único argumento.
func ValidarURIReenvio(uri string) error {
	if uri == "" || len(uri) > maxURIReenvio {
		return ErrURIReenvioNoValida
	}
	lower := strings.ToLower(uri)
	if !strings.HasPrefix(lower, "afirma:") && !strings.HasPrefix(lower, "afirmav2:") {
		return ErrURIReenvioNoValida
	}
	for _, r := range uri {
		if r == '"' || r == '\'' || r == '`' || r == unicode.ReplacementChar || unicode.IsSpace(r) || unicode.IsControl(r) {
			return ErrURIReenvioNoValida
		}
	}
	return nil
}

// EsquemaURI devuelve el esquema en minúsculas, para el registro.
func EsquemaURI(uri string) string {
	if esquema, _, ok := strings.Cut(uri, ":"); ok && len(esquema) <= 16 {
		return strings.ToLower(esquema)
	}
	return ""
}

// LocalizadorAutoFirma lo implementan los selectores que saben dónde está
// AutoFirma.
type LocalizadorAutoFirma interface {
	EjecutableAutoFirma(ctx context.Context) (string, error)
}

// EjecutableAutoFirma devuelve el ejecutable de AutoFirma al que se puede
// reenviar, o ErrAutoFirmaNoInstalada.
func EjecutableAutoFirma(ctx context.Context, p ports.ProtocoloAfirma) (string, error) {
	if l, ok := p.(LocalizadorAutoFirma); ok {
		return l.EjecutableAutoFirma(ctx)
	}
	return "", ports.ErrAutoFirmaNoInstalada
}

// esPropioGrxFirma reconoce el ejecutable de GrxFirma (por ruta o por nombre)
// para no reenviarse a sí mismo.
func esPropioGrxFirma(exe string) bool {
	name := strings.ToLower(baseName(exe))
	if strings.Contains(name, "grxfirma") || strings.Contains(name, "afirmauri") {
		return true
	}
	self, err := os.Executable()
	if err != nil {
		return false
	}
	if mismoFichero(self, exe) {
		return true
	}
	return false
}

func mismoFichero(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}

// lanzar permite simular el arranque en las pruebas.
var lanzar = lanzarSinShell

// Reenviar abre AutoFirma con el enlace. exe debe venir de
// EjecutableAutoFirma; se vuelve a comprobar aquí.
func Reenviar(ctx context.Context, exe, uri string) error {
	if os.Getenv(EnvReenvioAfirma) == "1" {
		return ErrReenvioEnBucle
	}
	if err := ValidarURIReenvio(uri); err != nil {
		return err
	}
	if exe == "" || !filepath.IsAbs(exe) && !isAbsoluteWindowsPath(exe) {
		return ErrOrdenAutoFirmaNoValida
	}
	if esPropioGrxFirma(exe) || !strings.HasPrefix(strings.ToLower(baseName(exe)), "autofirma") {
		return ErrOrdenAutoFirmaNoValida
	}
	if !regularFileExists(exe) {
		return ports.ErrAutoFirmaNoInstalada
	}
	if err := lanzar(ctx, exe, uri); err != nil {
		slog.WarnContext(ctx, "afirma_reenvio_autofirma_fallido",
			"programa", baseName(exe), "esquema", EsquemaURI(uri), "longitud", len(uri), "error", err)
		return fmt.Errorf("no se pudo abrir AutoFirma: %w", err)
	}
	slog.InfoContext(ctx, "afirma_reenvio_autofirma",
		"programa", baseName(exe), "esquema", EsquemaURI(uri), "longitud", len(uri))
	return nil
}

// lanzarSinShell arranca exe con el enlace como único argumento, sin shell y
// sin esperar a que termine. El proceso hijo lleva EnvReenvioAfirma.
func lanzarSinShell(_ context.Context, exe, uri string) error {
	cmd := exec.Command(exe, uri) // #nosec G204 -- ejecutable validado de AutoFirma; el enlace es un único argumento validado.
	cmd.Env = append(entornoSinMarca(os.Environ()), EnvReenvioAfirma+"=1")
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	prepararProcesoIndependiente(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func entornoSinMarca(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(strings.ToUpper(kv), EnvReenvioAfirma+"=") {
			out = append(out, kv)
		}
	}
	return out
}

// parsearOrdenAutoFirmaWindows acepta solo «programa.exe %1» o
// «programa.exe "%1"», con la ruta entre comillas o sin ellas (el instalador
// oficial la registra sin comillas y con espacios).
func parsearOrdenAutoFirmaWindows(command string) (string, bool) {
	command = strings.TrimSpace(command)
	var exe, rest string
	if strings.HasPrefix(command, `"`) {
		end := strings.Index(command[1:], `"`)
		if end < 0 {
			return "", false
		}
		exe, rest = command[1:end+1], command[end+2:]
	} else {
		lower := strings.ToLower(command)
		idx := strings.Index(lower, ".exe ")
		if idx < 0 {
			return "", false
		}
		exe, rest = command[:idx+len(".exe")], command[idx+len(".exe"):]
	}
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	switch strings.TrimSpace(rest) {
	case `%1`, `"%1"`:
	default:
		return "", false
	}
	if !strings.EqualFold(filepath.Ext(baseName(exe)), ".exe") || strings.ContainsAny(exe, "\"%\t\r\n\x00") || !isAbsoluteWindowsPath(exe) {
		return "", false
	}
	return exe, true
}

// carpetasProgramasXDG son las únicas donde se busca un Exec sin ruta: nunca
// el PATH del entorno, que puede venir manipulado.
var carpetasProgramasXDG = []string{"/usr/bin", "/usr/local/bin", "/bin"}

// parsearExecAutoFirma acepta solo «Exec=programa %u» (o %U) y devuelve la
// ruta absoluta del programa.
func parsearExecAutoFirma(execLine string) (string, bool) {
	if strings.ContainsAny(execLine, "\"'\\`$;|&<>()\x00") {
		return "", false
	}
	fields := strings.Fields(execLine)
	if len(fields) != 2 || (fields[1] != "%u" && fields[1] != "%U") {
		return "", false
	}
	prog := fields[0]
	if !strings.HasPrefix(strings.ToLower(filepath.Base(prog)), "autofirma") {
		return "", false
	}
	if filepath.IsAbs(prog) {
		return filepath.Clean(prog), true
	}
	if strings.Contains(prog, "/") {
		return "", false
	}
	for _, dir := range carpetasProgramasXDG {
		candidate := filepath.Join(dir, prog)
		if regularFileExists(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// EjecutableAutoFirma (Windows): el Autofirma.exe de la primera orden
// reconocible entre la guardada, la del equipo y la del usuario.
func (s *Selector) EjecutableAutoFirma(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	commands, err := s.autoFirmaCommands()
	if err != nil {
		return "", err
	}
	for _, command := range commands {
		exe, ok := parsearOrdenAutoFirmaWindows(command)
		if !ok || s.autoFirmaExecutableIn(command) != exe || esPropioGrxFirma(exe) {
			continue
		}
		return exe, nil
	}
	return "", ports.ErrAutoFirmaNoInstalada
}

// EjecutableAutoFirma (Linux): el programa del primer lanzador de AutoFirma
// cuyo Exec es reconocible.
func (s *XDGSelector) EjecutableAutoFirma(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.autoFirmaCandidates() {
		entry, err := s.findDesktop(id)
		if err != nil {
			return "", err
		}
		if entry == nil || !entry.afirma {
			continue
		}
		prog, ok := parsearExecAutoFirma(entry.exec)
		if !ok || !regularFileExists(prog) || esPropioGrxFirma(prog) {
			continue
		}
		return prog, nil
	}
	return "", ports.ErrAutoFirmaNoInstalada
}

// EjecutableAutoFirma deja pasar la consulta al selector envuelto.
func (c conPolitica) EjecutableAutoFirma(ctx context.Context) (string, error) {
	return EjecutableAutoFirma(ctx, c.inner)
}
