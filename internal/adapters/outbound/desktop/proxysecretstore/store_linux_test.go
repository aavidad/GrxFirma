// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package proxysecretstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

func newTestStore(r commandRunner) *Store {
	return &Store{
		runner: r,
		tool:   "secret-tool",
		idgen: func() (string, error) {
			return "proxy-secret-fixed-id", nil
		},
	}
}

func TestStore_GuardaSecretoEnSecretTool(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	store := newTestStore(runner)

	desc, err := store.Store(context.Background(), "corp-proxy", ports.ProxySecretMaterial{
		Username: "alberto",
		Password: []byte("secreto"),
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if desc.ID != "proxy-secret-fixed-id" || desc.Realm != "corp-proxy" || desc.Username != "alberto" {
		t.Fatalf("descriptor inesperado: %#v", desc)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	if call.name != "secret-tool" {
		t.Fatalf("name = %q, want secret-tool", call.name)
	}
	args := strings.Join(call.args, " ")
	if !strings.Contains(args, "store") || !strings.Contains(args, "service "+secretToolService) || !strings.Contains(args, "id proxy-secret-fixed-id") {
		t.Fatalf("args inesperados: %#v", call.args)
	}
	var env secretEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(call.stdin)), &env); err != nil {
		t.Fatalf("stdin json invalido: %v", err)
	}
	if env.Realm != "corp-proxy" || env.Username != "alberto" {
		t.Fatalf("envelope inesperado: %#v", env)
	}
	if decoded, _ := base64.StdEncoding.DecodeString(env.PasswordB64); string(decoded) != "secreto" {
		t.Fatalf("password envelope invalido: %#v", env)
	}
}

func TestStore_LoadRecuperaYDecodificaSecreto(t *testing.T) {
	t.Parallel()

	raw, _ := json.Marshal(secretEnvelope{
		Realm:       "corp-proxy",
		Username:    "alberto",
		PasswordB64: base64.StdEncoding.EncodeToString([]byte("secreto")),
	})
	runner := &fakeRunner{outputs: [][]byte{append(raw, '\n')}}
	store := newTestStore(runner)

	got, err := store.Load(context.Background(), "proxy-secret-fixed-id")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Realm != "corp-proxy" || got.Username != "alberto" || string(got.Password) != "secreto" {
		t.Fatalf("material inesperado: %#v", got)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	if want := []string{"lookup", "service", secretToolService, "id", "proxy-secret-fixed-id"}; strings.Join(runner.calls[0].args, " ") != strings.Join(want, " ") {
		t.Fatalf("args lookup inesperados: %#v", runner.calls[0].args)
	}
}

func TestStore_DeleteLimpiaEntrada(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	store := newTestStore(runner)

	if err := store.Delete(context.Background(), "proxy-secret-fixed-id"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	if want := []string{"clear", "service", secretToolService, "id", "proxy-secret-fixed-id"}; strings.Join(runner.calls[0].args, " ") != strings.Join(want, " ") {
		t.Fatalf("args clear inesperados: %#v", runner.calls[0].args)
	}
}

func TestStore_PropagaErroresDelBackend(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{errs: []error{errors.New("secret backend failure")}}
	store := newTestStore(runner)

	_, err := store.Store(context.Background(), "corp-proxy", ports.ProxySecretMaterial{
		Username: "alberto",
		Password: []byte("secreto"),
	})
	if err == nil || !strings.Contains(err.Error(), "guardando secreto") {
		t.Fatalf("Store() error = %v, want wrapped error", err)
	}
}

func TestStore_StatusDisponibleCuandoSecretToolResponde(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	store := newTestStore(runner)

	status, err := store.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Available || status.Platform != "linux" || status.Backend != "secret-service" || status.Reason != "" {
		t.Fatalf("status inesperado: %+v", status)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	if want := []string{"lookup", "service", secretToolService, "id", "proxy-secret-fixed-id"}; strings.Join(runner.calls[0].args, " ") != strings.Join(want, " ") {
		t.Fatalf("args status inesperados: %#v", runner.calls[0].args)
	}
}

func TestStore_StatusDisponibleSiLaSondaNoEncuentraEntrada(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{errs: []error{errors.New("exit status 1")}}
	store := newTestStore(runner)

	status, err := store.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Available || status.Reason != "" {
		t.Fatalf("status inesperado: %+v", status)
	}
}

func TestStore_StatusIndicaMotivoCuandoBackendFalla(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{errs: []error{errors.New("dbus no disponible")}}
	store := newTestStore(runner)

	status, err := store.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Available {
		t.Fatalf("status.Available = true, want false")
	}
	if status.Reason != "proxysecretstore: sesion Secret Service/DBus no disponible" {
		t.Fatalf("status.Reason = %q, want canonical Secret Service reason", status.Reason)
	}
}

func TestClassifyLinuxStatusReason_DistingueSesionSecretService(t *testing.T) {
	t.Parallel()

	cases := []error{
		errors.New("Cannot autolaunch D-Bus without X11 $DISPLAY"),
		errors.New("org.freedesktop.secrets no disponible"),
		errors.New("dbus no disponible"),
		errors.New("Secret Service not available"),
	}
	for _, tc := range cases {
		if got := classifyLinuxStatusReason(tc); got != "proxysecretstore: sesion Secret Service/DBus no disponible" {
			t.Fatalf("classifyLinuxStatusReason(%q) = %q", tc, got)
		}
	}
}

func TestClassifyLinuxStatusReason_PreservaAusenciaSecretTool(t *testing.T) {
	t.Parallel()

	if got := classifyLinuxStatusReason(ErrSecretToolUnavailable); got != ErrSecretToolUnavailable.Error() {
		t.Fatalf("classifyLinuxStatusReason(ErrSecretToolUnavailable) = %q, want %q", got, ErrSecretToolUnavailable.Error())
	}
}
