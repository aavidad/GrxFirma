// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type evaluadorMock struct {
	entrada  ports.EntradaDictamen
	dictamen domain.DictamenVerificacion
	err      error
}

func (e *evaluadorMock) Evaluar(_ context.Context, entrada ports.EntradaDictamen) (domain.DictamenVerificacion, error) {
	e.entrada = entrada
	return e.dictamen, e.err
}

// verificadorSeparadoMock admite el original aportado, como CAdES separado.
type verificadorSeparadoMock struct {
	verifierMock
}

func (m *verificadorSeparadoMock) VerifyDetached(ctx context.Context, firmado, _ domain.Document, anclas domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	return m.Verify(ctx, firmado, anclas)
}

func TestVerifySignature_SinEvaluadorNoHayDictamen(t *testing.T) {
	uc := application.NuevoVerifySignatureUseCase(&trustAnchorMock{}, &verifierMock{result: domain.VerificationResult{Valid: true}}, nil)
	res, err := uc.Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: docFirmadoPrueba()})
	if err != nil || res.Dictamen != nil {
		t.Fatalf("sin evaluador el contrato anterior no cambia: %+v %v", res.Dictamen, err)
	}
}

func TestVerifySignature_ComponeDictamenConHuellasDeEco(t *testing.T) {
	anclas := domain.CertificateChain{DERCertificates: [][]byte{{0x30}}}
	evaluador := &evaluadorMock{dictamen: domain.DictamenVerificacion{
		Integridad:      domain.AspectoDictamen{Estado: domain.IntegridadValida},
		VinculoOriginal: domain.AspectoDictamen{Estado: domain.VinculoAcreditado},
		Firmantes: []domain.DictamenFirmante{{
			CertificadoHuellaSHA256: "cc",
			Cadena:                  domain.AspectoDictamen{Estado: domain.CadenaValida},
			Certificado:             domain.AspectoDictamen{Estado: domain.CertificadoVigente},
			Revocacion:              domain.AspectoDictamen{Estado: domain.RevocacionVigente},
			SelloTiempo:             domain.AspectoDictamen{Estado: domain.SelloNoPresente},
		}},
	}}
	uc := application.NuevoVerifySignatureUseCase(
		&trustAnchorMock{chain: anclas},
		&verificadorSeparadoMock{verifierMock{result: domain.VerificationResult{Valid: true, Format: "CAdES"}}},
		nil,
	).ConEvaluador(evaluador)
	firmado := docFirmadoPrueba()
	original, _ := domain.NewDocument("original.txt", []byte("original sintético"), "text/plain")
	res, err := uc.Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: firmado, OriginalDocument: &original})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Dictamen
	if d == nil || d.Estado != domain.EstadoDictamenValida || d.Contrato != domain.ContratoDictamenVerificacion {
		t.Fatalf("dictamen=%+v", d)
	}
	suma := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	if d.HuellaFirmadoSHA256 != suma(firmado.Content) || d.HuellaOriginalSHA256 != suma(original.Content) {
		t.Fatal("las huellas de eco deben calcularse sobre los contenidos recibidos")
	}
	if d.ComprobadoEn.IsZero() || d.Formato != "CAdES" || d.CertificadoHuellaSHA256 != "cc" {
		t.Fatalf("metadatos del dictamen incompletos: %+v", d)
	}
	if len(evaluador.entrada.Anclas.DERCertificates) != 1 || evaluador.entrada.Original == nil {
		t.Fatal("el evaluador debe recibir las anclas y el original")
	}

	evaluador.err = errors.New("cancelado")
	if _, err := uc.Ejecutar(context.Background(), application.VerifyCommand{SignedDocument: firmado}); err == nil {
		t.Fatal("un fallo del evaluador no puede devolver un resultado")
	}
}
