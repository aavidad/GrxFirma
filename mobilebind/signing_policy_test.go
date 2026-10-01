// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"encoding/base64"
	"testing"
	"time"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/testsupport/pdffixture"
)

type mobileSigningPolicyClock struct{ now time.Time }

func (c mobileSigningPolicyClock) Now() time.Time { return c.now }

func TestAndroidFacadeRejectsIdentityExpiredAfterImport(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	certificateID := importEphemeralIdentity(t, facade, "aptitud-sintetica")
	// La importación válida no autoriza una firma posterior a la caducidad.
	// Se conserva el caso de uso real y el delegado GoMobile; solo avanza su reloj.
	engine := &mobileSignerEngine{
		delegate:           desktopsigner.NuevoMotorFirmaGo(mobileSigningPolicyClock{time.Now().Add(2 * time.Hour)}),
		temporaryDirectory: facade.temporaryDirectory,
	}
	facade.signService = application.NuevoSignDocumentUseCase(facade.session, facade.session, engine, nativeExplicitApproval{}, nil, nil)
	for _, tc := range []struct {
		format, name, mime string
		content            []byte
	}{
		{"cades", "sintetico.txt", "text/plain", []byte("sintético")},
		{"pades", "sintetico.pdf", "application/pdf", pdffixture.Minimal()},
		{"xades", "sintetico.xml", "application/xml", []byte("<sintetico/>")},
	} {
		t.Run(tc.format, func(t *testing.T) {
			response, err := facade.SignJSON(mustJSON(t, signRequest{
				Name: tc.name, MIMEType: tc.mime, Format: tc.format, Action: "sign", CertificateID: certificateID,
				ContentBase64: base64.StdEncoding.EncodeToString(tc.content),
				Options:       map[string]string{"allowExpired": "true", "skipCertificateValidation": "true"},
			}))
			if err == nil || response != "" {
				t.Fatalf("firma posterior a caducidad: respuesta=%q error=%v", response, err)
			}
		})
	}
}
