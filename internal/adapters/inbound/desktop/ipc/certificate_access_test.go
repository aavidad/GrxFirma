// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type ipcCertificateAccessMock struct {
	target           string
	imported         []byte
	password         string
	receivedPassword string
	opened           string
}

func (m *ipcCertificateAccessMock) Options(context.Context) (ports.CertificateAccessOptions, error) {
	return ports.CertificateAccessOptions{
		DetectedBrowser:  "firefox",
		PreferredManager: "firefox",
		PreferredTarget:  "nss:test",
		Managers: []ports.CertificateAccessManager{
			{ID: "firefox", Label: "Firefox", Recommended: true},
		},
		ImportTargets: []ports.CertificateImportTarget{
			{ID: "nss:test", Label: "Firefox (test)", Browser: "firefox", Recommended: true},
		},
	}, nil
}

func (m *ipcCertificateAccessMock) OpenManager(_ context.Context, id string) error {
	m.opened = id
	return nil
}

func (m *ipcCertificateAccessMock) Import(_ context.Context, target string, data []byte, password string) error {
	m.target = target
	m.imported = append([]byte(nil), data...)
	m.password = password
	m.receivedPassword = strings.Clone(password)
	return nil
}

type ipcTemporaryStoreMock struct {
	ref              domain.CertificateRef
	password         string
	receivedPassword string
	removed          string
	cleared          bool
}

func (m *ipcTemporaryStoreMock) Load(_ context.Context, _ []byte, password string) (domain.CertificateRef, error) {
	m.password = password
	m.receivedPassword = strings.Clone(password)
	return m.ref, nil
}
func (m *ipcTemporaryStoreMock) List(context.Context) ([]domain.CertificateRef, error) {
	if m.cleared || m.ref.ID == "" || m.removed == m.ref.ID {
		return nil, nil
	}
	return []domain.CertificateRef{m.ref}, nil
}
func (m *ipcTemporaryStoreMock) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return nil, errors.New("no usado")
}
func (m *ipcTemporaryStoreMock) Remove(id string) { m.removed = id }
func (m *ipcTemporaryStoreMock) Clear()           { m.cleared = true }

func TestCertificateAccessOptionsAndExplicitImport(t *testing.T) {
	access := &ipcCertificateAccessMock{}
	handler := &Manejador{
		CertificateAccess: application.NuevoCertificateAccessUseCase(access),
	}

	response := handler.despachar(context.Background(), peticion{Action: "certificate_access_options"})
	if !response.OK {
		t.Fatalf("certificate_access_options error = %s", response.Error)
	}
	raw, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) || !containsJSONText(raw, `"preferredTarget":"nss:test"`) {
		t.Fatalf("opciones inesperadas: %s", raw)
	}

	password := []byte("secreto")
	params, _ := json.Marshal(paramsImportCertificateToStore{
		CredentialB64: []byte("p12"),
		PasswordB64:   &password,
		TargetID:      "nss:test",
	})
	response = handler.despachar(context.Background(), peticion{
		Action: "import_certificate_to_store", Params: params,
	})
	if !response.OK {
		t.Fatalf("import_certificate_to_store error = %s", response.Error)
	}
	if access.target != "nss:test" || string(access.imported) != "p12" {
		t.Fatalf("importación inesperada: %#v", access)
	}
	if access.receivedPassword != "secreto" {
		t.Fatalf("contraseña recibida = %q", access.receivedPassword)
	}
	for i := range access.password {
		if access.password[i] != 0 {
			t.Fatalf("byte de contraseña %d no borrado", i)
		}
	}
}

func TestTemporaryCertificateAppearsAndCanBeRemoved(t *testing.T) {
	store := &ipcTemporaryStoreMock{ref: domain.CertificateRef{
		ID: "tmp-1", Subject: "CN=Temporal", Fingerprint: "aa",
	}}
	handler := &Manejador{
		Catalogo:              store,
		TemporaryCertificates: application.NuevoTemporaryCertificateUseCase(store),
	}
	password := []byte("temporal")
	params, _ := json.Marshal(paramsUseTemporaryCertificate{
		CredentialB64: []byte("pem"),
		PasswordB64:   &password,
	})
	response := handler.despachar(context.Background(), peticion{
		Action: "use_temporary_certificate", Params: params,
	})
	if !response.OK {
		t.Fatalf("use_temporary_certificate error = %s", response.Error)
	}
	data, ok := response.Data.(temporaryCertificateJSON)
	if !ok || data.ID != "tmp-1" || !data.Temporary {
		t.Fatalf("resultado temporal inesperado: %#v", response.Data)
	}
	if len(handler.ultimosCerts) != 1 {
		t.Fatalf("el catálogo no se recargó: %#v", handler.ultimosCerts)
	}
	if store.receivedPassword != "temporal" {
		t.Fatalf("contraseña temporal recibida = %q", store.receivedPassword)
	}
	for i := range store.password {
		if store.password[i] != 0 {
			t.Fatalf("byte de contraseña temporal %d no borrado", i)
		}
	}

	removeParams, _ := json.Marshal(paramsRemoveTemporaryCertificate{CertificateID: "tmp-1"})
	response = handler.despachar(context.Background(), peticion{
		Action: "remove_temporary_certificate", Params: removeParams,
	})
	if !response.OK || store.removed != "tmp-1" || len(handler.ultimosCerts) != 0 {
		t.Fatalf("retirada inesperada: response=%#v store=%#v certs=%#v", response, store, handler.ultimosCerts)
	}
}

func TestTemporaryCertificatesAreClearedBeforeResidentMode(t *testing.T) {
	store := &ipcTemporaryStoreMock{ref: domain.CertificateRef{
		ID: "tmp-resident", Subject: "CN=Temporal", Fingerprint: "bb",
	}}
	handler := &Manejador{
		Catalogo:              store,
		TemporaryCertificates: application.NuevoTemporaryCertificateUseCase(store),
		ultimosCerts:          []domain.CertificateRef{store.ref},
	}

	response := handler.despachar(context.Background(), peticion{
		Action: "clear_temporary_certificates",
	})
	if !response.OK {
		t.Fatalf("clear_temporary_certificates error = %s", response.Error)
	}
	if !store.cleared {
		t.Fatal("el almacén temporal no se limpió")
	}
	if len(handler.ultimosCerts) != 0 {
		t.Fatalf("el catálogo conservó credenciales temporales: %#v", handler.ultimosCerts)
	}
}

func TestTemporaryCertificateRejectsMalformedBase64(t *testing.T) {
	store := &ipcTemporaryStoreMock{}
	handler := &Manejador{
		TemporaryCertificates: application.NuevoTemporaryCertificateUseCase(store),
	}
	params := json.RawMessage(`{"credentialB64":"%%%no-base64%%%"}`)
	response := handler.despachar(context.Background(), peticion{
		Action: "use_temporary_certificate", Params: params,
	})
	if response.OK || response.Error == "" {
		t.Fatalf("se esperaba error claro: %#v", response)
	}
}

func containsJSONText(data []byte, value string) bool {
	for i := 0; i+len(value) <= len(data); i++ {
		if string(data[i:i+len(value)]) == value {
			return true
		}
	}
	return false
}
