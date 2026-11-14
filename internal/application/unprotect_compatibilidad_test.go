// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"encoding/json"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type clavesFijas struct{}

func (clavesFijas) DecryptionKeys(context.Context, domain.Document) ([]domain.ProtectionKeyMaterial, error) {
	return []domain.ProtectionKeyMaterial{{RecipientID: "dest"}}, nil
}

type motorConCompatibilidad struct{ salida domain.UnprotectedPayload }

func (m motorConCompatibilidad) Protect(context.Context, domain.ProtectionJob, []domain.ProtectionRecipient) (domain.ProtectedPayload, error) {
	return domain.ProtectedPayload{}, nil
}

func (m motorConCompatibilidad) Unprotect(context.Context, domain.Document, []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error) {
	return m.salida, nil
}

// clavesConAviso simula el keyring que avisa al dar una clave guardada en
// fichero (perfil alto sin almacén de secretos, lote 19).
type clavesConAviso struct{ clave string }

func (c clavesConAviso) DecryptionKeys(ctx context.Context, _ domain.Document) ([]domain.ProtectionKeyMaterial, error) {
	domain.AnotarAviso(ctx, c.clave)
	return []domain.ProtectionKeyMaterial{{RecipientID: "dest"}}, nil
}

// Lote 19: el aviso que anota el keyring al buscar las claves llega al
// resultado, junto a los del motor y sin repetirse.
func TestUnprotect_LlevaLosAvisosDelKeyringAlResultado(t *testing.T) {
	t.Parallel()
	doc, err := domain.NewDocument("acta.pdf", []byte("contenido"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	protegido, err := domain.NewDocument("acta.pdf.afp", []byte("{}"), domain.MIMETypeProtectedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	const clave = "proteccion.alto.sin_almacen.aviso"
	uc := application.NuevoUnprotectDocumentUseCase(clavesConAviso{clave: clave}, motorConCompatibilidad{salida: domain.UnprotectedPayload{
		Document: doc, Profile: domain.ProtectionProfileStrong, RecipientID: "dest",
		Avisos: []string{"aviso del motor", clave},
	}}, nil, nil)

	res, err := uc.Ejecutar(context.Background(), application.UnprotectCommand{ProtectedDocument: protegido})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Unprotected.Avisos; len(got) != 2 || got[0] != "aviso del motor" || got[1] != clave {
		t.Fatalf("avisos = %q; want el del motor y el del keyring una sola vez", got)
	}

	uc = application.NuevoUnprotectDocumentUseCase(clavesConAviso{clave: clave}, motorConCompatibilidad{salida: domain.UnprotectedPayload{
		Document: doc, Profile: domain.ProtectionProfileStrong, RecipientID: "dest",
	}}, nil, nil)
	res, err = uc.Ejecutar(context.Background(), application.UnprotectCommand{ProtectedDocument: protegido})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Unprotected.Avisos; len(got) != 1 || got[0] != clave {
		t.Fatalf("avisos = %q; want el del keyring", got)
	}
}

// Lote 13b, puntos 5 y 6: lo que se abre solo por compatibilidad llega al
// llamador como aviso y queda en la auditoría como código.
func TestUnprotect_RegistraLaCompatibilidadEnLaAuditoria(t *testing.T) {
	t.Parallel()
	doc, err := domain.NewDocument("acta.pdf", []byte("contenido"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	protegido, err := domain.NewDocument("acta.pdf.afp", []byte("{}"), domain.MIMETypeProtectedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	logger := &loggerMock{}
	uc := application.NuevoUnprotectDocumentUseCase(clavesFijas{}, motorConCompatibilidad{salida: domain.UnprotectedPayload{
		Document: doc, Profile: domain.ProtectionProfileCompat, RecipientID: "dest",
		Avisos:   []string{"aviso para la persona"},
		Detalles: []string{"cifrado_cms_heredado=1.2.840.113549.3.7\nfalso"},
	}}, application.NuevoAuditUseCase(relojMock{}, logger), nil)

	res, err := uc.Ejecutar(context.Background(), application.UnprotectCommand{ProtectedDocument: protegido})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unprotected.Avisos) != 1 {
		t.Fatalf("el aviso no llega al llamador: %v", res.Unprotected.Avisos)
	}
	if len(logger.registros) != 1 {
		t.Fatalf("registros = %d", len(logger.registros))
	}
	var registro struct {
		Compatibilidad []string `json:"compatibilidad"`
	}
	if err := json.Unmarshal(logger.registros[0].Payload, &registro); err != nil {
		t.Fatal(err)
	}
	if len(registro.Compatibilidad) != 1 || registro.Compatibilidad[0] != "cifrado_cms_heredado=1.2.840.113549.3.7 falso" {
		t.Fatalf("la auditoría no registra la compatibilidad saneada: %q", registro.Compatibilidad)
	}
}
