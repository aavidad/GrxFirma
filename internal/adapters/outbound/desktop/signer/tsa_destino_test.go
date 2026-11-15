// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/tsatest"
)

func TestMotorFirmaGo_TSAHTTPBloqueadaSinAjusteYPorMaquina(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	srv, peticiones := tsaLocalContada(t)
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil).PermitirTSADeOpcionesEnRedLocal()
	job := trabajoXAdEST(t, srv.URL)
	var block *excepciones.Block
	if _, err := motor.Sign(context.Background(), job, clave); !errors.As(err, &block) || block.ID != "tsa_http" {
		t.Fatalf("sin ajuste debe bloquear TSA HTTP: %v", err)
	}
	if peticiones.Load() != 0 {
		t.Fatal("se consultó la TSA antes de autorizar HTTP")
	}
	excepciones.SetForTest(t, "tsa_http", true)
	machinepolicy.SetForTest(t, "permitir_tsa_http", false)
	if _, err := motor.Sign(context.Background(), job, clave); !errors.As(err, &block) || block.ID != "tsa_http" {
		t.Fatalf("la directiva debe bloquear TSA HTTP: %v", err)
	}
	machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible"))
	if _, err := motor.Sign(context.Background(), job, clave); !errors.As(err, &block) || block.ID != "tsa_http" {
		t.Fatalf("la directiva ilegible debe bloquear TSA HTTP: %v", err)
	}
	if peticiones.Load() != 0 {
		t.Fatal("la TSA recibió una petición bloqueada")
	}
}

// TestMotorFirmaGo_TSADeOpcionesNoLlegaARedLocal reproduce E1-12: una página
// web que lanza afirma:// con extraParams tsaURL=http://127.0.0.1/... hacía
// que GrxFirma enviara un POST a la red local tras la firma del usuario. La
// tsaURL que llega en las opciones pasa por la misma política de destinos
// que la revocación: loopback, redes privadas y enlace local bloqueados.
func TestMotorFirmaGo_TSADeOpcionesNoLlegaARedLocal(t *testing.T) {
	permitirTSAHTTPPrueba(t)
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
	ctx, avisosOperacion := domain.ConAvisosOperacion(context.Background())
	if _, err := motor.Sign(ctx, casos[0], clave); err != nil {
		t.Fatalf("con la red local permitida la firma debería funcionar: %v", err)
	}
	exigirAvisoTSAHTTP(t, avisosOperacion)
	if peticiones.Load() == 0 {
		t.Fatal("con la red local permitida no se consultó la TSA")
	}
}
