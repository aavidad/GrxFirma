// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package secretinput

import (
	"errors"
	"strings"
	"testing"
)

func TestRejectArgv_BloqueaValoresSecretos(t *testing.T) {
	for _, args := range [][]string{
		{"-password", "secreto"},
		{"--p12-password=secreto"},
		{"-contrasena-p12", "secreto"},
		{"--rest-token", "secreto"},
		{"-token-rest=secreto"},
		{"-clave-proteccion-b64", "secreto"},
		{"--protection-secret-b64=secreto"},
		{"--password-env=secreto"},
		{"--password-stdin=secreto"},
		{"--contrasena-p12-stdin=secreto"},
		{"--clave-proteccion-stdin=secreto"},
		{"--nuevo-api-token", "secreto"},
		{"-opcion", "password=secreto"},
		{"--opcion=secret_b64=secreto"},
		{"-option", "authorization=Bearer secreto"},
	} {
		if err := RejectArgv(args); err == nil {
			t.Errorf("RejectArgv(%q) aceptó un secreto", args)
		}
	}
}

func TestRejectArgv_NoRepiteElValorSecretoEnElError(t *testing.T) {
	const secret = "valor-muy-secreto-que-no-debe-aparecer"
	err := RejectArgv([]string{"--p12-password=" + secret})
	if err == nil {
		t.Fatal("RejectArgv() aceptó el secreto")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("el error expuso el valor secreto: %q", err)
	}
}

func TestRejectArgv_NoReflejaValorDeOpcionSensible(t *testing.T) {
	const secret = "valor-opcion-que-no-debe-aparecer"
	err := RejectArgv([]string{"-opcion=secret_b64=" + secret})
	if err == nil {
		t.Fatal("RejectArgv() aceptó la opción sensible")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(UserMessage(err, nil), secret) {
		t.Fatalf("el error expuso el valor secreto: %q / %q", err, UserMessage(err, nil))
	}
	var inputErr *Error
	if !errors.As(err, &inputErr) || inputErr.Code != CodeSensitiveOption {
		t.Fatalf("error sin código esperado: %#v", err)
	}
}

func TestRejectArgv_PermiteFuentesSinValorSecreto(t *testing.T) {
	args := []string{
		"-contrasena-stdin",
		"-contrasena-p12-stdin",
		"--p12-password-stdin",
		"-clave-proteccion-stdin",
		"-certificado-pem", "/tmp/cert.pem",
		"-clave-pem", "/tmp/key.pem",
	}
	if err := RejectArgv(args); err != nil {
		t.Fatalf("RejectArgv() error = %v", err)
	}
}

func TestRead_LeeUnaLineaSinAlterarEspacios(t *testing.T) {
	got, err := Read(strings.NewReader("  secreto con espacios  \r\nresto"), nil, "")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != "  secreto con espacios  " {
		t.Fatalf("Read() = %q", got)
	}
}

func TestRead_RechazaNULYTamanioExcesivo(t *testing.T) {
	if _, err := Read(strings.NewReader("secreto\x00oculto\n"), nil, ""); err == nil {
		t.Fatal("Read() aceptó NUL")
	}
	if _, err := Read(strings.NewReader(strings.Repeat("x", maxSecretBytes+1)+"\n"), nil, ""); err == nil {
		t.Fatal("Read() aceptó un secreto sobredimensionado")
	}
}

func TestConsumeEnvironment_LimpiaTodasYTransfiereUnaSolaVez(t *testing.T) {
	values := map[string]string{
		"P12":        "p12-secreto",
		"REST":       "rest-secreto",
		"PROTECTION": "proteccion-secreto",
	}
	var unset []string
	captured, err := ConsumeEnvironment(
		[]string{"P12", "REST", "PROTECTION"},
		func(name string) (string, bool) {
			value, ok := values[name]
			return value, ok
		},
		func(name string) error {
			unset = append(unset, name)
			delete(values, name)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ConsumeEnvironment() error = %v", err)
	}
	if len(unset) != 3 || len(values) != 0 {
		t.Fatalf("entorno no consumido: unset=%#v values=%#v", unset, values)
	}
	if got, ok := captured.Take("P12"); !ok || got != "p12-secreto" {
		t.Fatalf("Take(P12) = %q, %t", got, ok)
	}
	if got, ok := captured.Take("P12"); ok || got != "" {
		t.Fatalf("segundo Take(P12) = %q, %t", got, ok)
	}
}

func TestConsumeEnvironment_IntentaLimpiarElRestoTrasUnFallo(t *testing.T) {
	var unset []string
	_, err := ConsumeEnvironment(
		[]string{"P12", "REST", "PROTECTION"},
		func(name string) (string, bool) { return name + "-valor", true },
		func(name string) error {
			unset = append(unset, name)
			if name == "REST" {
				return errors.New("fallo simulado")
			}
			return nil
		},
	)
	if len(unset) != 3 {
		t.Fatalf("solo se intentaron retirar %#v", unset)
	}
	var inputErr *Error
	if !errors.As(err, &inputErr) || inputErr.Code != CodeEnvironmentCleanup || inputErr.Name != "REST" {
		t.Fatalf("error inesperado: %#v", err)
	}
}

func TestUserMessage_UsaCodigoLocalizable(t *testing.T) {
	err := &Error{Code: CodeStdinUnavailable}
	got := UserMessage(err, func(id string, _ ...any) string {
		if id == "security.secret.stdin_unavailable" {
			return "stdin is unavailable"
		}
		return id
	})
	if got != "stdin is unavailable" {
		t.Fatalf("UserMessage() = %q", got)
	}
}
