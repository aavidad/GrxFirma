// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	pdfsign "github.com/digitorus/pdfsign/sign"
	restin "grxfirma/internal/adapters/inbound/common/rest"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/verificacionlocal"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

func TestDictamenV2_DosFirmasConCRLLocal(t *testing.T) {
	e := nuevoEscenario(t)
	original, first := e.firmarPAdES(t)
	dir := t.TempDir()
	firstFile := escribir(t, dir, "primera.pdf", first)
	secondFile := filepath.Join(dir, "segunda.pdf")
	if err := pdfsign.SignFile(firstFile, secondFile, pdfsign.SignData{
		Signature: pdfsign.SignDataSignature{
			Info:      pdfsign.SignDataSignatureInfo{Name: "Segundo firmante sintético", Date: time.Now()},
			CertType:  pdfsign.ApprovalSignature,
			SubFilter: pdfsign.SignatureSubFilterETSICAdESDetached,
		},
		Signer:            e.pki.firmante.clave,
		DigestAlgorithm:   crypto.SHA256,
		Certificate:       e.pki.firmante.cert,
		CertificateChains: [][]*x509.Certificate{{e.pki.firmante.cert, e.pki.intermedia.cert}},
	}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(secondFile)
	if err != nil {
		t.Fatal(err)
	}
	anchors, err := verificacionlocal.CargarAnclas(e.anclas)
	if err != nil {
		t.Fatal(err)
	}
	crl, err := verificacionlocal.NuevoAlmacenCRL(e.crlDir)
	if err != nil {
		t.Fatal(err)
	}
	uc := application.NuevoVerifySignatureUseCase(anchors, commonsigner.NewMultiVerifierOffline(), nil).
		ConEvaluador(verificacionlocal.Nuevo(verificacionlocal.Configuracion{CRL: crl}))
	adapter := restin.New(nil, uc, nil).WithBearerToken("token-sintetico-de-prueba")
	request, _ := json.Marshal(map[string]string{
		"name": "firmado.pdf", "content_base64": base64.StdEncoding.EncodeToString(second),
		"original_content_base64": base64.StdEncoding.EncodeToString(original),
		"contrato_solicitado":     "autofirmav2.dictamen-verificacion.v2",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(request)).WithContext(context.Background())
	req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
	adapter.RoutesSoloVerificacion().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Valid    bool `json:"valid"`
		Dictamen struct {
			Estado          string `json:"estado"`
			Motivo          string `json:"motivo"`
			VinculoOriginal struct {
				Estado string `json:"estado"`
			} `json:"vinculoOriginal"`
			Firmas []struct {
				Orden   int `json:"orden"`
				Cambios struct {
					Estado string `json:"estado"`
				} `json:"cambiosDesdeAnterior"`
			} `json:"firmas"`
		} `json:"dictamen"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Dictamen.Estado != string(domain.EstadoDictamenValida) || result.Dictamen.VinculoOriginal.Estado != domain.VinculoAcreditado || len(result.Dictamen.Firmas) != 2 {
		t.Fatalf("dictamen v2 inesperado: %s", rec.Body.String())
	}
	for i, sig := range result.Dictamen.Firmas {
		if sig.Orden != i+1 || sig.Cambios.Estado != "permitidos" {
			t.Fatalf("firma %d: %+v", i+1, sig)
		}
	}

	// An active page-content object rewritten after the last signature is
	// visible to the reader through the next real xref update.
	inspection, err := commonsigner.InspeccionarPAdESV2(second, 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	last := inspection.Revisiones[len(inspection.Revisiones)-1]
	var modified bytes.Buffer
	modified.Write(second)
	contentOffset := modified.Len()
	stream := "BT /F1 12 Tf 72 700 Td (CAMBIADO) Tj ET"
	fmt.Fprintf(&modified, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	xref := modified.Len()
	fmt.Fprintf(&modified, "xref\n4 1\n%010d 00000 n \ntrailer\n<< /Size %d /Root %d %d R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", contentOffset, len(last.Entries), last.RootObject, last.RootGeneration, last.StartXRef, xref)
	request, _ = json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(modified.Bytes()), "original_content_base64": base64.StdEncoding.EncodeToString(original), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(request))
	req.Header.Set("Authorization", "Bearer token-sintetico-de-prueba")
	adapter.RoutesSoloVerificacion().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"cambiosPosteriores":{"estado":"no_permitidos"`)) {
		t.Fatalf("cambio de página posterior no detectado: HTTP %d %s", rec.Code, rec.Body.String())
	}
}
