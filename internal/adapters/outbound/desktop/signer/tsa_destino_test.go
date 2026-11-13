// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/tsatest"
)

// TestMotorFirmaGo_TSADeOpcionesNoLlegaARedLocal reproduce E1-12: una página
// web que lanza afirma:// con extraParams tsaURL=http://127.0.0.1/... hacía
// que GrxFirma enviara un POST a la red local tras la firma del usuario. La
// tsaURL que llega en las opciones pasa por la misma política de destinos
// que la revocación: loopback, redes privadas y enlace local bloqueados.
func TestMotorFirmaGo_TSADeOpcionesNoLlegaARedLocal(t *testing.T) {
	var peticiones atomic.Int32
	responder := tsatest.NewResponder(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones.Add(1)
		responder.ServeHTTP(w, r)
	}))
	defer srv.Close()

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	trabajo := func(formato domain.SignatureFormat, nombre, mime string, contenido []byte) domain.SignatureJob {
		doc, _ := domain.NewDocument(nombre, contenido, mime)
		return domain.SignatureJob{
			Document: doc, Format: formato, Action: domain.ActionSign,
			Options: map[string]string{"level": "T", "tsaURL": srv.URL},
		}
	}
	casos := []domain.SignatureJob{
		trabajo(domain.FormatXAdES, "doc.xml", "application/xml", []byte("<xml><dato>valor</dato></xml>")),
		trabajo(domain.FormatCAdES, "doc.txt", "text/plain", []byte("contenido")),
	}
	for _, job := range casos {
		if _, err := signer.NuevoMotorFirmaGo(nil).Sign(context.Background(), job, clave); err == nil {
			t.Fatalf("%s: la firma con tsaURL de red local debería fallar", job.Format)
		}
	}
	if n := peticiones.Load(); n != 0 {
		t.Fatalf("la TSA de red local recibió %d peticiones", n)
	}

	// Con la TSA local permitida expresamente (escritorio, ajustes del
	// usuario) la misma petición sí sale.
	motor := signer.NuevoMotorFirmaGo(nil).PermitirTSADeOpcionesEnRedLocal()
	if _, err := motor.Sign(context.Background(), casos[0], clave); err != nil {
		t.Fatalf("con la red local permitida la firma debería funcionar: %v", err)
	}
	if peticiones.Load() == 0 {
		t.Fatal("con la red local permitida no se consultó la TSA")
	}
}
