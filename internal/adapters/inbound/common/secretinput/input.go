// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package secretinput centraliza la política de entrada de secretos de los
// procesos propios. Los secretos se leen por stdin (sin eco cuando es un TTY)
// y nunca como valores de una bandera de línea de comandos.
package secretinput

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const maxSecretBytes = 16 * 1024

// ErrorCode identifica fallos de entrada de secretos sin ligar la política a
// un idioma ni incluir nunca el valor sensible.
type ErrorCode string

const (
	CodeArgvSecretValue    ErrorCode = "secret.argv_value"
	CodeArgvSourceValue    ErrorCode = "secret.argv_source_value"
	CodeSensitiveOption    ErrorCode = "secret.sensitive_option"
	CodeStdinUnavailable   ErrorCode = "secret.stdin_unavailable"
	CodeTerminalRead       ErrorCode = "secret.terminal_read"
	CodeStdinRead          ErrorCode = "secret.stdin_read"
	CodeSecretTooLarge     ErrorCode = "secret.too_large"
	CodeSecretNUL          ErrorCode = "secret.nul"
	CodeEnvironmentCleanup ErrorCode = "secret.environment_cleanup"
)

// Error conserva únicamente metadatos no sensibles para que las capas de
// entrada puedan localizar el mensaje de usuario.
type Error struct {
	Code    ErrorCode
	Flag    string
	Name    string
	Hint    string
	Limit   int
	Wrapped error
}

func (e *Error) Error() string {
	if e == nil {
		return "secret input error"
	}
	detail := e.Flag
	if detail == "" {
		detail = e.Name
	}
	if detail == "" {
		return "secret input error: " + string(e.Code)
	}
	return fmt.Sprintf("secret input error: %s (%s)", e.Code, detail)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Wrapped
}

// Translator coincide con ports.Localizador.T sin acoplar este paquete al
// puerto de presentación.
type Translator func(id string, args ...any) string

// UserMessage traduce un Error tipado. Los fallbacks son deliberadamente
// genéricos y no incorporan Wrapped para evitar que un error de bajo nivel
// termine reflejando material sensible.
func UserMessage(err error, translate Translator) string {
	var inputErr *Error
	if !errors.As(err, &inputErr) {
		return translated(translate, "security.secret.error", "no se pudo procesar la entrada secreta")
	}
	switch inputErr.Code {
	case CodeArgvSecretValue:
		hint := localizedHint(translate, inputErr.Hint)
		return translated(
			translate,
			"security.secret.argv_value",
			"la opción %q ya no admite secretos en argv (CWE-214); %s",
			"-"+inputErr.Flag,
			hint,
		)
	case CodeArgvSourceValue:
		return translated(
			translate,
			"security.secret.argv_source_value",
			"la opción de fuente segura %q no admite un valor en argv (CWE-214)",
			"-"+inputErr.Flag,
		)
	case CodeSensitiveOption:
		return translated(
			translate,
			"security.secret.sensitive_option",
			"la clave sensible %q no se admite mediante -opcion (CWE-214); use la entrada segura específica",
			inputErr.Name,
		)
	case CodeStdinUnavailable:
		return translated(translate, "security.secret.stdin_unavailable", "stdin no está disponible para leer el secreto")
	case CodeTerminalRead:
		return translated(translate, "security.secret.terminal_read", "no se pudo leer el secreto sin eco")
	case CodeStdinRead:
		return translated(translate, "security.secret.stdin_read", "no se pudo leer el secreto desde stdin")
	case CodeSecretTooLarge:
		return translated(
			translate,
			"security.secret.too_large",
			"el secreto supera el límite de %d bytes",
			inputErr.Limit,
		)
	case CodeSecretNUL:
		return translated(translate, "security.secret.nul", "el secreto contiene un byte NUL no permitido")
	case CodeEnvironmentCleanup:
		return translated(
			translate,
			"security.secret.environment_cleanup",
			"no se pudo retirar %s del entorno heredable",
			inputErr.Name,
		)
	default:
		return translated(translate, "security.secret.error", "no se pudo procesar la entrada secreta")
	}
}

func translated(translate Translator, id, fallback string, args ...any) string {
	if translate != nil {
		if value := translate(id, args...); value != "" && value != id {
			return value
		}
	}
	if len(args) == 0 {
		return fallback
	}
	return fmt.Sprintf(fallback, args...)
}

func localizedHint(translate Translator, hint string) string {
	switch hint {
	case "rest":
		return translated(translate, "security.secret.hint.rest", "use GRXFIRMA_REST_TOKEN mediante un gestor de secretos")
	case "protection":
		return translated(
			translate,
			"security.secret.hint.protection",
			"use -clave-proteccion-stdin o, solo para compatibilidad automatizada, GRXFIRMA_PROTECTION_SECRET_B64",
		)
	case "gui-p12":
		return translated(
			translate,
			"security.secret.hint.gui_p12",
			"use -p12-password-stdin o, solo para compatibilidad automatizada, GRXFIRMA_PKCS12_PASSWORD",
		)
	case "import-p12":
		return translated(
			translate,
			"security.secret.hint.import_p12",
			"use -contrasena-p12-stdin o, solo para compatibilidad automatizada, GRXFIRMA_PKCS12_PASSWORD",
		)
	default:
		return translated(
			translate,
			"security.secret.hint.p12",
			"use -contrasena-stdin o, solo para compatibilidad automatizada, GRXFIRMA_PKCS12_PASSWORD",
		)
	}
}

// RejectArgv rechaza banderas cuyo valor sería una contraseña, token o clave
// simétrica visible en la tabla de procesos. Reconoce tanto "-flag value"
// como "--flag=value".
func RejectArgv(args []string) error {
	for index, arg := range args {
		if assignment, ok := optionAssignment(arg, args, index); ok {
			if key, sensitive := sensitiveOptionKey(assignment); sensitive {
				return &Error{Code: CodeSensitiveOption, Flag: "opcion", Name: key}
			}
		}
		name, ok := argvFlagName(arg)
		if !ok {
			continue
		}
		if isSafeSourceFlag(name) && isSecretSemanticName(name) {
			if strings.Contains(arg, "=") {
				return &Error{Code: CodeArgvSourceValue, Flag: name}
			}
			continue
		}
		if !IsSecretValueFlagName(name) {
			continue
		}
		return &Error{Code: CodeArgvSecretValue, Flag: name, Hint: safeSourceHint(name)}
	}
	return nil
}

// ValidateOptionAssignment aplica la misma política a flag.Value incluso
// cuando se usa fuera de los entrypoints protegidos por RejectArgv.
func ValidateOptionAssignment(assignment string) error {
	if key, sensitive := sensitiveOptionKey(assignment); sensitive {
		return &Error{Code: CodeSensitiveOption, Flag: "opcion", Name: key}
	}
	return nil
}

// IsSecretValueFlagName permite que el gate AST aplique la misma política a
// nuevas banderas registradas por los binarios. name puede incluir guiones
// iniciales o una asignación "=valor".
func IsSecretValueFlagName(name string) bool {
	name = normalizeFlagName(name)
	if name == "" || isSafeSourceFlag(name) {
		return false
	}
	return isSecretSemanticName(name)
}

func isSecretSemanticName(name string) bool {
	switch {
	case strings.Contains(name, "password"):
		return true
	case strings.Contains(name, "contrasena"), strings.Contains(name, "contraseña"):
		return true
	case strings.Contains(name, "secret"):
		return true
	case name == "token", strings.HasPrefix(name, "token-"), strings.HasSuffix(name, "-token"):
		return true
	case strings.Contains(name, "clave-proteccion"), strings.Contains(name, "protection-key"):
		return true
	default:
		return false
	}
}

// Read lee una sola línea acotada. En una terminal real desactiva el eco; en
// automatización acepta una línea canalizada por stdin.
func Read(reader io.Reader, promptWriter io.Writer, prompt string) (string, error) {
	if reader == nil {
		return "", &Error{Code: CodeStdinUnavailable}
	}
	if file, ok := reader.(*os.File); ok {
		// x/term models descriptors as int on every supported platform. Fd()
		// returns the same OS descriptor as uintptr, so this is the conversion
		// required by its public API rather than arithmetic on untrusted data.
		fd := int(file.Fd()) // #nosec G115 -- required os.File/x/term descriptor conversion.
		if term.IsTerminal(fd) {
			if promptWriter != nil && prompt != "" {
				_, _ = io.WriteString(promptWriter, prompt)
			}
			raw, err := term.ReadPassword(fd)
			if promptWriter != nil {
				_, _ = io.WriteString(promptWriter, "\n")
			}
			if err != nil {
				return "", &Error{Code: CodeTerminalRead, Wrapped: err}
			}
			return validateAndCopy(raw)
		}
	}

	limited := io.LimitReader(reader, maxSecretBytes+2)
	raw, err := bufio.NewReader(limited).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		zero(raw)
		return "", &Error{Code: CodeStdinRead, Wrapped: err}
	}
	return validateAndCopy(raw)
}

func validateAndCopy(raw []byte) (string, error) {
	defer zero(raw)
	raw = bytes.TrimSuffix(raw, []byte{'\n'})
	raw = bytes.TrimSuffix(raw, []byte{'\r'})
	if len(raw) > maxSecretBytes {
		return "", &Error{Code: CodeSecretTooLarge, Limit: maxSecretBytes}
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		return "", &Error{Code: CodeSecretNUL}
	}
	return string(raw), nil
}

// Environment conserva temporalmente secretos retirados del entorno. Take
// transfiere cada valor una sola vez al consumidor correspondiente.
type Environment struct {
	values map[string]string
}

// ConsumeEnvironment captura y elimina todas las variables indicadas. Aunque
// falle una eliminación, intenta limpiar el resto antes de devolver el error.
func ConsumeEnvironment(
	names []string,
	lookup func(string) (string, bool),
	unset func(string) error,
) (Environment, error) {
	captured := Environment{values: make(map[string]string, len(names))}
	var firstErr error
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		value, present := "", false
		if lookup != nil {
			value, present = lookup(name)
		}
		if !present {
			continue
		}
		captured.values[name] = value
		if unset != nil {
			if err := unset(name); err != nil && firstErr == nil {
				firstErr = &Error{Code: CodeEnvironmentCleanup, Name: name, Wrapped: err}
			}
		}
	}
	return captured, firstErr
}

// Take devuelve y elimina el valor capturado para reducir su vida útil.
func (e *Environment) Take(name string) (string, bool) {
	if e == nil || e.values == nil {
		return "", false
	}
	value, ok := e.values[name]
	delete(e.values, name)
	return value, ok
}

func argvFlagName(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return "", false
	}
	return normalizeFlagName(arg), true
}

func normalizeFlagName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimLeft(name, "-")
	if before, _, found := strings.Cut(name, "="); found {
		name = before
	}
	return name
}

func isSafeSourceFlag(name string) bool {
	for _, suffix := range []string{"-stdin", "-file", "-fichero", "-fd"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func safeSourceHint(name string) string {
	switch {
	case strings.Contains(name, "rest"), strings.Contains(name, "token"):
		return "rest"
	case strings.Contains(name, "proteccion"), strings.Contains(name, "protection"):
		return "protection"
	case strings.Contains(name, "p12") && strings.Contains(name, "password"):
		return "gui-p12"
	case strings.Contains(name, "p12"):
		return "import-p12"
	default:
		return "p12"
	}
}

func optionAssignment(arg string, args []string, index int) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(arg))
	for _, prefix := range []string{"-opcion=", "--opcion=", "-option=", "--option="} {
		if strings.HasPrefix(lower, prefix) {
			return arg[len(prefix):], true
		}
	}
	switch lower {
	case "-opcion", "--opcion", "-option", "--option":
		if index+1 < len(args) {
			return args[index+1], true
		}
	}
	return "", false
}

func sensitiveOptionKey(assignment string) (string, bool) {
	key, _, found := strings.Cut(assignment, "=")
	if !found {
		return "", false
	}
	key = strings.TrimSpace(strings.ToLower(key))
	normalized := strings.NewReplacer("_", "-", ".", "-").Replace(key)
	switch {
	case strings.Contains(normalized, "password"):
	case strings.Contains(normalized, "contrasena"), strings.Contains(normalized, "contraseña"):
	case strings.Contains(normalized, "passphrase"):
	case strings.Contains(normalized, "secret"):
	case normalized == "token", strings.HasPrefix(normalized, "token-"), strings.HasSuffix(normalized, "-token"):
	case strings.Contains(normalized, "authorization"), strings.Contains(normalized, "bearer"):
	case strings.Contains(normalized, "api-key"), strings.Contains(normalized, "private-key"):
	case strings.Contains(normalized, "clave-proteccion"), strings.Contains(normalized, "protection-key"):
	case normalized == "pin", strings.HasSuffix(normalized, "-pin"):
	case strings.Contains(normalized, "credential"), strings.Contains(normalized, "credencial"):
	default:
		return "", false
	}
	return key, true
}

func zero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
