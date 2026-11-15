// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

const marcaAjenaV38 = "[sha1_leer; excepciones.sha1_leer.bloqueo]"

// cadesImplicitaV38 firma con el resumen y el nombre común que se pidan.
func cadesImplicitaV38(t *testing.T, datos []byte, resumen asn1.ObjectIdentifier, nombre string) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(38), Subject: pkix.Name{CommonName: nombre},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	firmado, err := pkcs7.NewSignedData(datos)
	if err != nil {
		t.Fatal(err)
	}
	firmado.SetDigestAlgorithm(resumen)
	if err := firmado.AddSigner(cert, priv, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	cms, err := firmado.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return cms
}

func verificarV38(t *testing.T, f *Facade, cms []byte) (string, error) {
	t.Helper()
	return f.VerifyJSON(mustJSON(t, verifyRequest{Name: "firma.csig", MIMEType: "application/pkcs7-signature",
		ContentBase64: base64.StdEncoding.EncodeToString(cms)}))
}

// contratoKotlinV38 lee el error igual que UnsafeExceptions.parseBlock (lote 33).
type contratoKotlinV38 struct {
	ErrorCode         string `json:"errorCode"`
	ExceptionID       string `json:"exceptionId"`
	Activable         *bool  `json:"activable"`
	BlockKey          string `json:"blockKey"`
	SettingKey        string `json:"settingKey"`
	AcceptanceVersion string `json:"acceptanceVersion"`
}

func TestV38VerificacionSHA1BloqueadaLlegaTipadaALaApp(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	f := newAndroidFacadeForTest(t)
	out, err := verificarV38(t, f, cadesImplicitaV38(t, []byte("documento antiguo"), pkcs7.OIDDigestAlgorithmSHA1, "Firmante de 2012"))
	if err == nil {
		t.Fatalf("un bloqueo no puede salir como resultado de firma no válida: %s", out)
	}
	var block *excepciones.Block
	if !errors.As(err, &block) || block.ID != "sha1_leer" {
		t.Fatalf("el núcleo pierde la causa tipada: %v", err)
	}
	var contrato contratoKotlinV38
	if json.Unmarshal([]byte(err.Error()), &contrato) != nil {
		t.Fatalf("el mensaje debe ser el JSON que lee la app: %q", err.Error())
	}
	if contrato.ErrorCode != "unsafe_exception_blocked" || contrato.ExceptionID != "sha1_leer" ||
		contrato.Activable == nil || !*contrato.Activable || contrato.BlockKey != "excepciones.sha1_leer.bloqueo" ||
		contrato.SettingKey != "excepciones.sha1_leer.ajuste" || contrato.AcceptanceVersion != excepciones.AcceptanceVersion {
		t.Fatalf("contrato incompleto: %+v", contrato)
	}
	if strings.Contains(err.Error(), "[sha1_leer;") {
		t.Fatalf("la marca no debe viajar a la app: %q", err.Error())
	}
}

func TestV38MarcaEnElNombreDelCertificadoNoOfreceNada(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	f := newAndroidFacadeForTest(t)
	out, err := verificarV38(t, f, cadesImplicitaV38(t, []byte("documento actual"), pkcs7.OIDDigestAlgorithmSHA256, "Ana "+marcaAjenaV38))
	if err != nil {
		t.Fatalf("una firma SHA-256 no se bloquea por el texto de su certificado: %v", err)
	}
	var response verifyResponse
	decodeResponse(t, out, &response)
	if strings.Contains(out, "exceptionId") || strings.Contains(out, "unsafe_exception") {
		t.Fatalf("el texto de un certificado ofrece una excepción: %s", out)
	}
	var subjects []string
	for _, s := range response.SignerSummaries {
		subjects = append(subjects, s.Subject)
	}
	if !strings.Contains(strings.Join(subjects, " "), "excepciones.sha1_leer.bloqueo]") {
		t.Fatalf("el nombre del firmante es un dato y se conserva: %+v", response.SignerSummaries)
	}
}

func TestV38ConElAjusteElResultadoLlevaElAvisoUnaVez(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	excepciones.SetForTest(t, "sha1_leer", true)
	f := newAndroidFacadeForTest(t)
	out, err := verificarV38(t, f, cadesImplicitaV38(t, []byte("documento antiguo"), pkcs7.OIDDigestAlgorithmSHA1, "Firmante de 2012"))
	if err != nil {
		t.Fatalf("con el ajuste se verifica: %v", err)
	}
	var response verifyResponse
	decodeResponse(t, out, &response)
	aviso := excepciones.Warning("sha1_leer")
	avisos := 0
	for _, w := range response.Warnings {
		if w == aviso {
			avisos++
		}
	}
	if avisos != 1 {
		t.Fatalf("el aviso debe salir una vez: %+v", response.Warnings)
	}
	if response.IntegrityStatus == string(domain.VerificationStatusValid) &&
		!slices.Contains(response.Warnings, "movil.verificacion.cadena_no_evaluada") {
		t.Fatalf("falta la clave de la cadena no evaluada: %+v", response.Warnings)
	}
}

type verificadorV38 struct {
	result application.VerifyResult
	err    error
}

func (v verificadorV38) Execute(context.Context, application.VerifyCommand) (application.VerifyResult, error) {
	return v.result, v.err
}

func TestV38SafeOperationErrorNoBorraLaCausaNiAceptaTexto(t *testing.T) {
	dir := t.TempDir()
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	block := excepciones.DescribeAt(dir, "sha1_leer")
	payload := mustJSON(t, verifyRequest{Name: "firma.csig", MIMEType: "application/pkcs7-signature",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("firma"))})

	conError := newFacade(nil, verificadorV38{err: fmt.Errorf("motor: %w", block)}, nil)
	conError.configDir = dir
	if _, err := conError.VerifyJSON(payload); !strings.Contains(fmt.Sprint(err), `"exceptionId":"sha1_leer"`) {
		t.Fatalf("un bloqueo del motor se convirtió en error genérico: %v", err)
	}

	enResultado := domain.NewVerificationFailure("CAdES", block.Error(), nil).WithPolicyError(block)
	conResultado := newFacade(nil, verificadorV38{result: application.VerifyResult{Verification: enResultado}}, nil)
	conResultado.configDir = dir
	if _, err := conResultado.VerifyJSON(payload); !strings.Contains(fmt.Sprint(err), `"exceptionId":"sha1_leer"`) {
		t.Fatalf("la causa tipada del resultado no llega a la app: %v", err)
	}

	// La misma frase, o la marca antigua, sin causa tipada: resultado normal.
	texto := domain.NewVerificationFailure("CAdES", block.Error()+" "+marcaAjenaV38, nil)
	sinCausa := newFacade(nil, verificadorV38{result: application.VerifyResult{Verification: texto}}, nil)
	sinCausa.configDir = dir
	out, err := sinCausa.VerifyJSON(payload)
	if err != nil || strings.Contains(out, "exceptionId") {
		t.Fatalf("el texto no puede crear un bloqueo: out=%s err=%v", out, err)
	}
}

func TestV38ErrorTipadoSoloEnvuelveBloqueos(t *testing.T) {
	if err := errors.New("otro"); errorTipado(err) != err {
		t.Fatal("un error ajeno no cambia")
	}
	if errorTipado(nil) != nil {
		t.Fatal("nil sigue siendo nil")
	}
	block := excepciones.DescribeAt(t.TempDir(), "tsa_http")
	typed := errorTipado(fmt.Errorf("x: %w", block))
	if errorTipado(typed) != typed {
		t.Fatal("no se envuelve dos veces")
	}
	var back *excepciones.Block
	if !errors.As(typed, &back) || back.ID != "tsa_http" {
		t.Fatal("errors.As debe seguir encontrando el bloqueo")
	}
	if strings.HasPrefix(typed.Error(), "{") == false {
		t.Fatalf("el mensaje es JSON: %q", typed.Error())
	}
}
