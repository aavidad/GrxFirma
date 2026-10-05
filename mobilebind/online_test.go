// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/updatecheck"
	"grxfirma/internal/testsupport/tsatest"
)

func TestCertificateDetailsReportExpiryKindAndKey(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	var empty certificateDetailsResponse
	raw, err := facade.CertificateDetailsJSON()
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, raw, &empty)
	if len(empty.Certificates) != 0 || empty.ExpiringSoonDays != certificateExpiringSoonDays {
		t.Fatalf("sin identidad no hay certificados: %+v", empty)
	}

	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "clave"), "clave")
	var details certificateDetailsResponse
	raw, err = facade.CertificateDetailsJSON()
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, raw, &details)
	if len(details.Certificates) != 1 {
		t.Fatalf("detalle: %+v", details)
	}
	got := details.Certificates[0]
	if got.CertificateID != id || got.KeyType != "RSA" || got.KeyBits != 2048 || got.External {
		t.Fatalf("clave o identidad: %+v", got)
	}
	if got.Organization != "Diputacion de Granada" || got.Kind == "" {
		t.Fatalf("organización o tipo: %+v", got)
	}
	// El certificado de prueba caduca en una hora: aviso de caducidad próxima.
	if got.Status != "expiring_soon" || got.DaysLeft != 0 {
		t.Fatalf("estado de caducidad: %+v", got)
	}
	if _, err := time.Parse(time.RFC3339, got.NotAfter); err != nil {
		t.Fatalf("fecha de caducidad: %q", got.NotAfter)
	}

	facade.clock = func() time.Time { return time.Now().Add(2 * time.Hour) }
	raw, _ = facade.CertificateDetailsJSON()
	decodeResponse(t, raw, &details)
	if details.Certificates[0].Status != "expired" {
		t.Fatalf("debería constar caducado: %+v", details.Certificates[0])
	}
}

func TestDescribeCertificateLongValidity(t *testing.T) {
	_, certificate := ephemeralPKCS12(t, "clave")
	now := certificate.NotBefore.Add(time.Minute)
	got := describeCertificate(certificate, "id", false, now)
	if got.Status == "expired" || got.Status == "not_yet_valid" {
		t.Fatalf("estado: %+v", got)
	}
	got = describeCertificate(certificate, "id", false, certificate.NotBefore.Add(-time.Hour))
	if got.Status != "not_yet_valid" {
		t.Fatalf("aún no vigente: %+v", got)
	}
}

func TestCertificateRevocationUsesSessionChainAndMapsStatus(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "clave"), "clave")
	var received [][]byte
	revokedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	facade.revocationCheck = func(ctx context.Context, chain [][]byte) (commonsigner.CertificateOnlineRevocationResult, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("la consulta debe tener tiempo máximo")
		}
		received = chain
		return commonsigner.CertificateOnlineRevocationResult{
			Status: commonsigner.CertificateOnlineRevocationRevoked, Method: "ocsp",
			CheckedAt: revokedAt.Add(time.Hour), RevokedAt: revokedAt,
			Reason: "texto libre que no debe cruzar", UserMessage: "mensaje en castellano",
		}, nil
	}
	raw, err := facade.CheckCertificateRevocationJSON(mustJSON(t, certificateRequest{CertificateID: id}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "texto libre") || strings.Contains(raw, "castellano") {
		t.Fatalf("la respuesta no debe incluir textos del motor: %s", raw)
	}
	var response revocationResponse
	decodeResponse(t, raw, &response)
	if response.Status != "revoked" || response.Method != "OCSP" || response.RevokedAt != revokedAt.Format(time.RFC3339) {
		t.Fatalf("respuesta: %+v", response)
	}
	identity, _ := facade.session.snapshot(id)
	if len(received) != 1 || !bytes.Equal(received[0], identity.certificate.Raw) {
		t.Fatal("la cadena debe empezar por el certificado de la sesión")
	}

	facade.revocationCheck = func(context.Context, [][]byte) (commonsigner.CertificateOnlineRevocationResult, error) {
		return commonsigner.CertificateOnlineRevocationResult{}, errors.New("red caída")
	}
	raw, err = facade.CheckCertificateRevocationJSON(mustJSON(t, certificateRequest{CertificateID: id}))
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, raw, &response)
	if response.Status != "unavailable" {
		t.Fatalf("un fallo de red es no disponible: %+v", response)
	}

	if _, err := facade.CheckCertificateRevocationJSON(mustJSON(t, certificateRequest{CertificateID: "otro"})); err == nil {
		t.Fatal("un certificado ajeno a la sesión debe rechazarse")
	}
}

func TestDiagnosticsHasNoPersonalData(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	importPKCS12(t, facade, ephemeralRSAPKCS12(t, "clave"), "clave")
	raw := facade.DiagnosticsJSON()
	if strings.Contains(raw, "Mobile Test RSA") || strings.Contains(raw, "Diputacion") {
		t.Fatalf("el diagnóstico no puede incluir el titular: %s", raw)
	}
	var response diagnosticsResponse
	decodeResponse(t, raw, &response)
	if response.Platform != "android" || response.ContractVersion != mobileContractVersion || !response.SessionIdentity ||
		response.GoVersion == "" || response.Architecture == "" {
		t.Fatalf("diagnóstico: %+v", response)
	}
}

func TestTimestampProbeAgainstLocalTSA(t *testing.T) {
	server := httptest.NewServer(tsatest.NewResponder(t))
	defer server.Close()
	facade := newAndroidFacadeForTest(t)
	raw, err := facade.ProbeTimestampAuthorityJSON(mustJSON(t, timestampProbeRequest{URL: server.URL}))
	if err != nil {
		t.Fatal(err)
	}
	var response timestampProbeResponse
	decodeResponse(t, raw, &response)
	if response.Status != "ok" || response.HTTPS || response.TSATime == "" {
		t.Fatalf("prueba TSA: %+v", response)
	}
	if response.SkewSeconds < -5 || response.SkewSeconds > 5 {
		t.Fatalf("desfase inesperado con la TSA local: %+v", response)
	}

	facade.clock = func() time.Time { return time.Now().Add(10 * time.Minute) }
	raw, _ = facade.ProbeTimestampAuthorityJSON(mustJSON(t, timestampProbeRequest{URL: server.URL}))
	decodeResponse(t, raw, &response)
	if response.SkewSeconds < 590 || response.SkewSeconds > 610 {
		t.Fatalf("un reloj adelantado debe dar desfase positivo: %+v", response)
	}
}

func TestTimestampProbeStatuses(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	called := false
	facade.timestampProbe = func(context.Context, string, []byte) ([]byte, error) {
		called = true
		return nil, errors.New("TSA respondio con HTTP 500")
	}
	for _, bad := range []string{"ftp://tsa.example", "https://usuario@tsa.example", "https://tsa.example/#x", " https://tsa.example", "https:///sin-host"} {
		raw, err := facade.ProbeTimestampAuthorityJSON(mustJSON(t, timestampProbeRequest{URL: bad}))
		if err != nil {
			t.Fatal(err)
		}
		var response timestampProbeResponse
		decodeResponse(t, raw, &response)
		if response.Status != "invalid_url" {
			t.Fatalf("%q: %+v", bad, response)
		}
	}
	if called {
		t.Fatal("una URL no válida no debe llegar a la red")
	}
	cases := map[string]error{
		"rejected":     errors.New("TSA respondio con HTTP 500"),
		"unreachable":  errors.New("error en petición HTTP a TSA; comprueba el servicio y su URL final"),
		"timeout":      context.DeadlineExceeded,
		"bad_response": errors.New("error parseando TimeStampResp"),
	}
	for want, failure := range cases {
		facade.timestampProbe = func(context.Context, string, []byte) ([]byte, error) { return nil, failure }
		raw, _ := facade.ProbeTimestampAuthorityJSON(mustJSON(t, timestampProbeRequest{URL: "https://tsa.example/tsr"}))
		var response timestampProbeResponse
		decodeResponse(t, raw, &response)
		if response.Status != want || !response.HTTPS {
			t.Fatalf("%s: %+v", want, response)
		}
	}
}

const testVeriFactuQR = "https://www2.agenciatributaria.gob.es/wlpl/TIKE-CONT/ValidarQR?nif=89890001K&numserie=ABC%26G33&fecha=01-01-2025&importe=241.4"

func TestVeriFactuQRReadAndExplicitQuery(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	raw, err := facade.ReadVeriFactuQRJSON(mustJSON(t, veriFactuQRRequest{URL: testVeriFactuQR}))
	if err != nil {
		t.Fatal(err)
	}
	var qr commonsigner.VeriFactuQR
	decodeResponse(t, raw, &qr)
	if qr.NIF != "89890001K" || qr.Number != "ABC&G33" || qr.Date != "01-01-2025" || qr.Amount != "241.4" || !qr.Verifiable || qr.Test {
		t.Fatalf("QR: %+v", qr)
	}
	for _, bad := range []string{strings.Replace(testVeriFactuQR, "https:", "http:", 1), strings.Replace(testVeriFactuQR, "www2.agenciatributaria.gob.es", "evil.test", 1)} {
		if _, err := facade.ReadVeriFactuQRJSON(mustJSON(t, veriFactuQRRequest{URL: bad})); err == nil || err.Error() != "verifactu.qr_url" {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	if _, err := facade.ReadVeriFactuQRJSON(mustJSON(t, veriFactuQRRequest{URL: strings.Replace(testVeriFactuQR, "241.4", "241,4", 1)})); err == nil || err.Error() != "verifactu.qr_params" {
		t.Fatalf("parámetros: %v", err)
	}

	queried := ""
	facade.veriFactuQuery = func(_ context.Context, url string) (json.RawMessage, error) {
		queried = url
		return json.RawMessage(`{"resultado":"ok"}`), nil
	}
	raw, err = facade.QueryVeriFactuQRJSON(mustJSON(t, veriFactuQRRequest{URL: testVeriFactuQR}))
	if err != nil || queried != testVeriFactuQR || !strings.Contains(raw, `"response":{"resultado":"ok"}`) {
		t.Fatalf("cotejo: %s %v", raw, err)
	}
	queried = ""
	if _, err := facade.QueryVeriFactuQRJSON(mustJSON(t, veriFactuQRRequest{URL: "https://evil.test/x"})); err == nil || queried != "" {
		t.Fatal("una URL ajena no debe consultarse")
	}
	facade.veriFactuQuery = func(context.Context, string) (json.RawMessage, error) { return nil, errors.New("detalle interno") }
	if _, err := facade.QueryVeriFactuQRJSON(mustJSON(t, veriFactuQRRequest{URL: testVeriFactuQR})); err == nil || err.Error() != "verifactu.qr_service" {
		t.Fatalf("fallo del servicio: %v", err)
	}
}

func TestUpdateCheckStatuses(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	cases := []struct {
		result updatecheck.Resultado
		err    error
		want   string
	}{
		{updatecheck.Resultado{UltimaVersion: "v0.0.120", HayNueva: true, Comparable: true, URL: "https://github.com/aavidad/GrxFirma/releases/tag/v0.0.120", Estado: updatecheck.EstadoPublicada}, nil, "newer"},
		{updatecheck.Resultado{UltimaVersion: "v0.0.115", Comparable: true, Estado: updatecheck.EstadoPublicada}, nil, "current"},
		{updatecheck.Resultado{UltimaVersion: "nightly", Estado: updatecheck.EstadoPublicada}, nil, "not_comparable"},
		{updatecheck.Resultado{Estado: updatecheck.EstadoSinPublicaciones}, nil, "no_releases"},
		{updatecheck.Resultado{}, &updatecheck.HTTPStatusError{StatusCode: 403}, "error"},
	}
	for _, tc := range cases {
		facade.updateCheck = func(ctx context.Context, version string) (updatecheck.Resultado, error) {
			if version != "0.0.115" {
				t.Fatalf("versión enviada: %q", version)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("la consulta debe tener tiempo máximo")
			}
			return tc.result, tc.err
		}
		raw, err := facade.CheckUpdateJSON(mustJSON(t, updateCheckRequest{CurrentVersion: "0.0.115"}))
		if err != nil {
			t.Fatal(err)
		}
		var response updateCheckResponse
		decodeResponse(t, raw, &response)
		if response.Status != tc.want {
			t.Fatalf("%s: %+v", tc.want, response)
		}
		if tc.err != nil && response.ErrorCode != "update_rate_limited" {
			t.Fatalf("código de error: %+v", response)
		}
	}
	for _, bad := range []string{"", "0.0.115 ", "0.0.115;rm", strings.Repeat("1", 40)} {
		if _, err := facade.CheckUpdateJSON(mustJSON(t, updateCheckRequest{CurrentVersion: bad})); err == nil {
			t.Fatalf("versión %q aceptada", bad)
		}
	}
}

func TestContractDeclaresThirdWaveServices(t *testing.T) {
	raw, err := buildMobileContract("android", true)
	if err != nil {
		t.Fatal(err)
	}
	var contract mobileContract
	decodeResponse(t, raw, &contract)
	for _, service := range []string{"certificate_details", "certificate_online_check", "diagnostics", "tsa_probe",
		"verifactu_qr_read", "verifactu_qr_query", "update_check"} {
		if !contract.Services[service] {
			t.Fatalf("servicio %s no declarado", service)
		}
	}
	if contract.Verification.Revocation != "embedded_evidence_only" {
		t.Fatal("la verificación de firmas sigue sin consultar la revocación en línea")
	}
}
