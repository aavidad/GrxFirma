// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Lote 39a, punto 1: en el recorrido real de escritorio (15/11) verificar una
// CAdES SHA-1 sin el ajuste devolvía ok:true con errorCode
// unsafe_exception_blocked. Qt pintaba «Error: No válida» sin ofrecer activar
// y WinUI lo rechazaba como respuesta incoherente. El bloqueo de una
// verificación debe llegar como el de cualquier otra operación: un fallo.

type anclasVaciasV39 struct{}

func (anclasVaciasV39) Anchors(context.Context) (domain.CertificateChain, error) {
	return domain.CertificateChain{}, nil
}

func firmaCAdESSHA1V39(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2012), Subject: pkix.Name{CommonName: "Firmante de prueba 2012"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	datos, err := pkcs7.NewSignedData([]byte("documento firmado en 2012"))
	if err != nil {
		t.Fatal(err)
	}
	datos.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
	if err := datos.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	cms, err := datos.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return cms
}

// verificacionSHA1RealV39 monta el manejador con el verificador real, como el
// motor de escritorio, y un fichero .csig SHA-1 hecho en la prueba.
func verificacionSHA1RealV39(t *testing.T) (*Manejador, context.Context, peticion) {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	dir := t.TempDir()
	input := filepath.Join(dir, "doc_sha1.csig")
	if err := os.WriteFile(input, firmaCAdESSHA1V39(t), 0o600); err != nil {
		t.Fatal(err)
	}
	useCase := application.NuevoVerifySignatureUseCase(anclasVaciasV39{}, signer.NewMultiVerifier(), nil)
	m := &Manejador{ConfigDir: dir, Verificar: useCase, Loc: localizador.Para("es")}
	ctx := withUnsafeRetryOwner(conEstadoInterfaz(context.Background(), interfazVerificada))
	t.Cleanup(func() { m.clearUnsafeRetries(ctx) })
	return m, ctx, peticionJSON(t, "verify", paramsVerify{InputPath: input})
}

func TestV39VerificacionSHA1BloqueadaLlegaComoFalloTipado(t *testing.T) {
	m, ctx, p := verificacionSHA1RealV39(t)
	resp := m.despachar(ctx, p)
	if resp.OK || resp.Outcome != ipcOutcomeFailure {
		t.Fatalf("un bloqueo al verificar no puede llegar como éxito: ok=%v outcome=%q", resp.OK, resp.Outcome)
	}
	if resp.ErrorCode != "unsafe_exception_blocked" || resp.ExceptionID != "sha1_leer" || resp.RetryID == "" ||
		resp.Activable == nil || !*resp.Activable || resp.Exception == nil {
		t.Fatalf("falta el fallo tipado con la oferta de activar: %+v", resp)
	}
	if resp.Data != nil {
		t.Fatalf("un bloqueo no lleva datos de una verificación: %#v", resp.Data)
	}
	if want := localizador.Para("es").T("excepciones.sha1_leer.bloqueo"); resp.Error != want {
		t.Fatalf("el texto del fallo debe ser el del bloqueo:\n got %q\nwant %q", resp.Error, want)
	}
	// Un bloqueo de lo inseguro no es una avería: sin diagnóstico guiado, la
	// interfaz no abre el asistente ni guarda una incidencia encima del aviso.
	if resp.Diagnostic != nil {
		t.Fatalf("un bloqueo no lleva diagnóstico de avería: %+v", resp.Diagnostic)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["ok"] != false || wire["outcome"] != "failure" || wire["data"] != nil {
		t.Fatalf("en el cable debe ser un fallo sin data: %s", raw)
	}
}

func TestV39ReintentoTrasActivarVerificaYAvisa(t *testing.T) {
	m, ctx, p := verificacionSHA1RealV39(t)
	blocked := m.despachar(ctx, p)
	if blocked.RetryID == "" {
		t.Fatalf("sin reintento: %+v", blocked)
	}
	activar, _ := json.Marshal(map[string]any{"id": "sha1_leer", "accepted": true,
		"acceptanceVersion": excepciones.AcceptanceVersion, "retryId": blocked.RetryID, "origin": "aviso"})
	if r := m.despachar(ctx, peticion{Action: "unsafe_exceptions_activate", Params: activar}); !r.OK {
		t.Fatalf("activación: %+v", r)
	}
	retry, _ := json.Marshal(map[string]string{"retryId": blocked.RetryID})
	resp := m.despachar(ctx, peticion{Action: "unsafe_operation_retry", Params: retry})
	if !resp.OK || resp.ErrorCode == "unsafe_exception_blocked" {
		t.Fatalf("el reintento debe verificar: %+v", resp)
	}
	verified, ok := resp.Data.(resultadoVerificacion)
	if !ok {
		t.Fatalf("el reintento no trae la verificación: %#v", resp.Data)
	}
	if verified.Integrity.Status != string(domain.VerificationStatusValid) {
		t.Fatalf("con el ajuste la integridad SHA-1 se comprueba: %+v", verified.Integrity)
	}
	// Punto 3: el uso de la excepción va al aviso común (warnings de la
	// respuesta), no solo a las advertencias plegadas del resultado, y una
	// sola vez aunque la firma pase por varias capas del verificador.
	aviso := excepciones.Warning("sha1_leer")
	if n := countString(resp.WarningKeys, aviso); n != 1 {
		t.Fatalf("el aviso SHA-1 debe ir una vez en warnings: %v", resp.WarningKeys)
	}
}

func countString(values []string, want string) int {
	n := 0
	for v := range slices.Values(values) {
		if v == want {
			n++
		}
	}
	return n
}
