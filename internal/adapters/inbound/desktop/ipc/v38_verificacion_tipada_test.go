// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// verificadorSHA1V38 hace lo mismo que signer.marcarAlgoritmoAntiguo: pide el
// ajuste con el contexto de la operación y deja la causa tipada en el
// resultado. El motivo visible es otro texto, para que solo la causa tipada
// pueda identificar el bloqueo.
type verificadorSHA1V38 struct{ firmante string }

func (v verificadorSHA1V38) Execute(ctx context.Context, _ application.VerifyCommand) (application.VerifyResult, error) {
	result := domain.NewVerificationFailure("CAdES", "firma no evaluada", nil)
	if err := excepciones.Check("sha1_leer", ctx); err != nil {
		result = result.WithPolicyError(err)
	}
	result.SignerSummaries = []domain.VerificationSignerSummary{{ID: "1", Subject: v.firmante}}
	return application.VerifyResult{Verification: result.Normalize()}, nil
}

// verificadorTextoV38 devuelve un resultado cuyo texto imita un bloqueo sin
// que el motor haya pedido ningún ajuste.
type verificadorTextoV38 struct{ texto string }

func (v verificadorTextoV38) Execute(context.Context, application.VerifyCommand) (application.VerifyResult, error) {
	result := domain.NewVerificationFailure("CAdES", v.texto, nil)
	result.Warnings = []string{v.texto}
	result.SignerSummaries = []domain.VerificationSignerSummary{{ID: "1", Subject: "CN=" + v.texto}}
	return application.VerifyResult{Verification: result.Normalize()}, nil
}

func verificarIPCV38(t *testing.T, verificador VerifySignatureUseCase) respuesta {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	dir := t.TempDir()
	input := filepath.Join(dir, "firma.csig")
	if err := os.WriteFile(input, []byte("firma"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manejador{ConfigDir: dir, Verificar: verificador, Loc: localizador.Para("es")}
	ctx := withUnsafeRetryOwner(conEstadoInterfaz(context.Background(), interfazVerificada))
	t.Cleanup(func() { m.clearUnsafeRetries(ctx) })
	return m.despachar(ctx, peticionJSON(t, "verify", paramsVerify{InputPath: input}))
}

func TestV38VerificacionBloqueadaPorSHA1OfreceActivarPorLaCausaTipada(t *testing.T) {
	resp := verificarIPCV38(t, verificadorSHA1V38{firmante: "CN=Firmante de 2012"})
	if resp.ErrorCode != "unsafe_exception_blocked" || resp.ExceptionID != "sha1_leer" ||
		resp.BlockKey != "excepciones.sha1_leer.bloqueo" || resp.SettingKey != "excepciones.sha1_leer.ajuste" {
		t.Fatalf("la verificación bloqueada no llega tipada: %+v", resp)
	}
	if resp.Activable == nil || !*resp.Activable || resp.RetryID == "" {
		t.Fatalf("sin ajuste debe ofrecer activarlo y repetir la verificación: %+v", resp)
	}
	if resp.Exception == nil || resp.Exception.AcceptanceVersion != excepciones.AcceptanceVersion {
		t.Fatalf("faltan los datos de la aceptación: %+v", resp.Exception)
	}
}

func TestV38MarcaEnElNombreDelCertificadoNoOfreceNadaEnIPC(t *testing.T) {
	marca := "Ana [sha1_leer; excepciones.sha1_leer.bloqueo]"
	for name, verificador := range map[string]VerifySignatureUseCase{
		"firmante":          verificadorSHA1V38{firmante: "CN=" + marca},
		"texto sin ajuste":  verificadorTextoV38{texto: marca},
		"frase del bloqueo": verificadorTextoV38{texto: excepciones.DescribeAt(t.TempDir(), "sha1_leer").Error()},
	} {
		t.Run(name, func(t *testing.T) {
			if name == "firmante" {
				// Con el ajuste aceptado Check no bloquea: solo queda el texto del certificado.
				excepciones.SetForTest(t, "sha1_leer", true)
			}
			resp := verificarIPCV38(t, verificador)
			if resp.ExceptionID != "" || resp.RetryID != "" || resp.ErrorCode == "unsafe_exception_blocked" ||
				(resp.Activable != nil && *resp.Activable) {
				t.Fatalf("un texto de terceros decide la excepción: %+v", resp)
			}
		})
	}
}

func TestV38BlockErrorYaNoLlevaLaMarca(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	text := excepciones.DescribeAt(t.TempDir(), "sha1_leer").Error()
	if strings.Contains(text, "[sha1_leer;") || strings.Contains(text, "excepciones.sha1_leer") {
		t.Fatalf("el texto del bloqueo enseña la marca o la clave: %q", text)
	}
}
