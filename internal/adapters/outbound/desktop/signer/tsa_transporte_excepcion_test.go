// SPDX-License-Identifier: EUPL-1.2
package signer_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/pdffixture"
)

type tsaTransportFunc func(*http.Request) (*http.Response, error)

func (f tsaTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestPAdESTHTTPConsultaExcepcionAntesDeLaRed(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	priv, cert := generarCertRSA(t)
	doc, err := domain.NewDocument("doc.pdf", pdffixture.Minimal(), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	const endpoint = "http://tsa.example.invalid/tsr"
	requests := 0
	client := tsaclient.New(endpoint)
	client.HTTPClient = &http.Client{Transport: tsaTransportFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("error"))}, nil
	})}
	motor := signer.NuevoMotorFirmaGo(nil).WithTimestampAuthority(client).PermitirTSADeOpcionesEnRedLocal()
	job := domain.SignatureJob{Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign, Options: map[string]string{"level": "T", "tsaURL": endpoint}}
	ctx, warnings := domain.ConAvisosOperacion(context.Background())
	_, err = motor.Sign(ctx, job, signer.NuevaClaveLocal(priv, cert))
	var blocked *excepciones.Block
	if !errors.As(err, &blocked) || blocked.ID != "tsa_http" || requests != 0 {
		t.Fatalf("PAdES-T HTTP se pidió sin permiso: err=%v requests=%d", err, requests)
	}
	if slices.Contains(warnings.Claves(), excepciones.Warning("tsa_http")) {
		t.Fatal("avisó de un uso bloqueado")
	}
}

// Los tres productores atraviesan el mismo control antes del primer POST.
// Una respuesta 500 evita depender de un servicio de prueba: basta observar
// si la petición llegó al transporte y si el aviso quedó en la operación.
func TestFirmasConSelloHTTPCompartenBloqueoYAviso(t *testing.T) {
	for _, tc := range []struct {
		name           string
		format         domain.SignatureFormat
		filename, mime string
		content        []byte
	}{
		{"CAdES-T", domain.FormatCAdES, "doc.bin", "application/octet-stream", []byte("documento")},
		{"XAdES-T", domain.FormatXAdES, "doc.xml", "application/xml", []byte("<documento/>")},
		{"PAdES-T", domain.FormatPAdES, "doc.pdf", "application/pdf", pdffixture.Minimal()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machinepolicy.UnsetForTest(t, "permitir_tsa_http")
			excepciones.SetForTest(t, "tsa_http", false)
			priv, cert := generarCertRSA(t)
			doc, err := domain.NewDocument(tc.filename, tc.content, tc.mime)
			if err != nil {
				t.Fatal(err)
			}
			const endpoint = "http://tsa.example.invalid/tsr"
			requests := 0
			client := tsaclient.New(endpoint)
			client.HTTPClient = &http.Client{Transport: tsaTransportFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("error"))}, nil
			})}
			motor := signer.NuevoMotorFirmaGo(nil).WithTimestampAuthority(client).PermitirTSADeOpcionesEnRedLocal()
			job := domain.SignatureJob{Document: doc, Format: tc.format, Action: domain.ActionSign, Options: map[string]string{"level": "T", "tsaURL": endpoint}}
			sign := func() (error, []string) {
				ctx, warnings := domain.ConAvisosOperacion(context.Background())
				_, err := motor.Sign(ctx, job, signer.NuevaClaveLocal(priv, cert))
				return err, warnings.Claves()
			}
			var block *excepciones.Block
			err, warnings := sign()
			if !errors.As(err, &block) || block.ID != "tsa_http" || requests != 0 || slices.Contains(warnings, excepciones.Warning("tsa_http")) {
				t.Fatalf("sin ajuste: err=%v requests=%d avisos=%v", err, requests, warnings)
			}
			excepciones.SetForTest(t, "tsa_http", true)
			err, warnings = sign()
			if err == nil || errors.As(err, &block) || requests != 1 || !slices.Contains(warnings, excepciones.Warning("tsa_http")) {
				t.Fatalf("permitido: err=%v requests=%d avisos=%v", err, requests, warnings)
			}
			machinepolicy.SetForTest(t, "permitir_tsa_http", false)
			err, _ = sign()
			if !errors.As(err, &block) || requests != 1 {
				t.Fatalf("máquina prohibida: err=%v requests=%d", err, requests)
			}
			machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible"))
			err, _ = sign()
			if !errors.As(err, &block) || requests != 1 {
				t.Fatalf("máquina ilegible: err=%v requests=%d", err, requests)
			}
		})
	}
}
