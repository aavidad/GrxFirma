// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin && cgo

package proxysecretstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"runtime"
	"testing"

	"grxfirma/internal/ports"
)

type darwinBackendFake struct {
	storeCalls []struct {
		service string
		account string
		payload []byte
	}
	loadCalls []struct {
		service string
		account string
	}
	deleteCalls []struct {
		service string
		account string
	}
	payload  []byte
	storeErr error
	loadErr  error
	delErr   error
	status   ports.ProxySecretStoreStatus
}

func (f *darwinBackendFake) Store(service, account string, payload []byte) error {
	f.storeCalls = append(f.storeCalls, struct {
		service string
		account string
		payload []byte
	}{service: service, account: account, payload: append([]byte(nil), payload...)})
	return f.storeErr
}

func (f *darwinBackendFake) Load(service, account string) ([]byte, error) {
	f.loadCalls = append(f.loadCalls, struct {
		service string
		account string
	}{service: service, account: account})
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return append([]byte(nil), f.payload...), nil
}

func (f *darwinBackendFake) Delete(service, account string) error {
	f.deleteCalls = append(f.deleteCalls, struct {
		service string
		account string
	}{service: service, account: account})
	return f.delErr
}

func (f *darwinBackendFake) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	if f.status.Backend == "" {
		f.status = ports.ProxySecretStoreStatus{
			Available: true,
			Platform:  runtime.GOOS,
			Backend:   "keychain",
		}
	}
	return f.status, nil
}

func TestStoreDarwin_StoreEncodesEnvelope(t *testing.T) {
	t.Parallel()

	backend := &darwinBackendFake{}
	store := &Store{
		idgen:   func() (string, error) { return "proxy-secret-fixed-id", nil },
		service: darwinServiceName,
		backend: backend,
	}

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
	if len(backend.storeCalls) != 1 {
		t.Fatalf("store calls = %d, want 1", len(backend.storeCalls))
	}
	call := backend.storeCalls[0]
	if call.service != darwinServiceName || call.account != "proxy-secret-fixed-id" {
		t.Fatalf("store call inesperada: %+v", call)
	}
	var env secretEnvelope
	if err := json.Unmarshal(call.payload, &env); err != nil {
		t.Fatalf("payload json invalido: %v", err)
	}
	if env.Realm != "corp-proxy" || env.Username != "alberto" {
		t.Fatalf("envelope inesperado: %#v", env)
	}
	if decoded, _ := base64.StdEncoding.DecodeString(env.PasswordB64); string(decoded) != "secreto" {
		t.Fatalf("password envelope invalido: %#v", env)
	}
}

func TestStoreDarwin_LoadDecodesEnvelope(t *testing.T) {
	t.Parallel()

	raw, _ := json.Marshal(secretEnvelope{
		Realm:       "corp-proxy",
		Username:    "alberto",
		PasswordB64: base64.StdEncoding.EncodeToString([]byte("secreto")),
	})
	backend := &darwinBackendFake{payload: raw}
	store := &Store{
		idgen:   func() (string, error) { return "proxy-secret-fixed-id", nil },
		service: darwinServiceName,
		backend: backend,
	}

	got, err := store.Load(context.Background(), "proxy-secret-fixed-id")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Realm != "corp-proxy" || got.Username != "alberto" || string(got.Password) != "secreto" {
		t.Fatalf("material inesperado: %#v", got)
	}
}

func TestClassifyDarwinStatusReason_PreservesUsefulMessage(t *testing.T) {
	t.Parallel()

	err := errors.New("proxysecretstore: consultando Keychain por defecto: User interaction is not allowed.")
	if got := classifyDarwinStatusReason(err); got != "proxysecretstore: el Keychain requiere interaccion del usuario o esta bloqueado" {
		t.Fatalf("classifyDarwinStatusReason() = %q", got)
	}
}
