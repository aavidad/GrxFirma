// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/tsatest"
)

// tsaLocalContada arranca una TSA de prueba en 127.0.0.1 que cuenta las
// peticiones.
func tsaLocalContada(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var peticiones atomic.Int32
	responder := tsatest.NewResponder(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones.Add(1)
		responder.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &peticiones
}

func trabajoXAdEST(t *testing.T, tsaURL string) domain.SignatureJob {
	t.Helper()
	doc, err := domain.NewDocument("doc.xml", []byte("<xml><dato>valor</dato></xml>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	return domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"level": "T", "tsaURL": tsaURL},
	}
}

func capturarRegistro(t *testing.T) *bytes.Buffer {
	t.Helper()
	var registro bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&registro, nil)))
	t.Cleanup(func() { slog.SetDefault(anterior) })
	return &registro
}

// Lote 13a (retrocompatibilidad, revisa E1-12): por REST una TSA interna
// pasada como opción vuelve a funcionar; queda un aviso con el host, sin la
// ruta ni la consulta.
func TestMotorFirmaGo_TSADeOpcionesREST_RedLocalConAviso(t *testing.T) {
	srv, peticiones := tsaLocalContada(t)
	registro := capturarRegistro(t)
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)

	motor := signer.NuevoMotorFirmaGo(nil).AvisarTSADeOpcionesEnRedLocal()
	// tsaclient ya rechazaba credenciales en la URL; aquí se comprueba que el
	// aviso no lleva ruta ni consulta.
	tsaURL := srv.URL + "/ruta/privada?x=1"
	if _, err := motor.Sign(context.Background(), trabajoXAdEST(t, tsaURL), clave); err != nil {
		t.Fatalf("por REST la TSA interna debía admitirse: %v", err)
	}
	if peticiones.Load() == 0 {
		t.Fatal("no se consultó la TSA interna")
	}
	texto := registro.String()
	if !strings.Contains(texto, "tsa_opciones_red_local") || !strings.Contains(texto, "127.0.0.1") {
		t.Fatalf("falta el aviso de TSA en red local:\n%s", texto)
	}
	for _, sensible := range []string{"/ruta/privada", "x=1"} {
		if strings.Contains(texto, sensible) {
			t.Fatalf("el aviso no debe llevar %q:\n%s", sensible, texto)
		}
	}
}

// Con la lista de TSA permitidas definida, REST solo admite las de la lista.
func TestMotorFirmaGo_TSADeOpcionesREST_ListaRestringe(t *testing.T) {
	srv, peticiones := tsaLocalContada(t)
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)

	fuera := signer.NuevoMotorFirmaGo(nil).AvisarTSADeOpcionesEnRedLocal().ConTSAPermitidas([]string{"https://tsa.organismo.example"})
	if _, err := fuera.Sign(context.Background(), trabajoXAdEST(t, srv.URL), clave); err == nil {
		t.Fatal("una TSA fuera de la lista debía rechazarse por REST")
	}
	if peticiones.Load() != 0 {
		t.Fatalf("la TSA fuera de la lista recibió %d peticiones", peticiones.Load())
	}
	dentro := signer.NuevoMotorFirmaGo(nil).AvisarTSADeOpcionesEnRedLocal().ConTSAPermitidas([]string{srv.URL + "/tsa"})
	if _, err := dentro.Sign(context.Background(), trabajoXAdEST(t, srv.URL), clave); err != nil {
		t.Fatalf("una TSA de la lista debía admitirse: %v", err)
	}
	vacia := signer.NuevoMotorFirmaGo(nil).AvisarTSADeOpcionesEnRedLocal().ConTSAPermitidas([]string{})
	if _, err := vacia.Sign(context.Background(), trabajoXAdEST(t, srv.URL), clave); err == nil {
		t.Fatal("una lista definida y vacía no debía admitir ninguna TSA de opciones")
	}
}

// En los caminos web (afirma://, WebSocket, nativehost) una TSA interna sigue
// rechazada salvo que esté en la lista; la lista no restringe las públicas.
func TestMotorFirmaGo_TSADeOpcionesWeb_ListaSoloExceptua(t *testing.T) {
	srv, peticiones := tsaLocalContada(t)
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)

	otra := signer.NuevoMotorFirmaGo(nil).ConTSAPermitidas([]string{"https://tsa.organismo.example"})
	if _, err := otra.Sign(context.Background(), trabajoXAdEST(t, srv.URL), clave); err == nil {
		t.Fatal("una TSA interna fuera de la lista debía seguir rechazada en el camino web")
	}
	if peticiones.Load() != 0 {
		t.Fatalf("la TSA interna recibió %d peticiones", peticiones.Load())
	}
	listada := signer.NuevoMotorFirmaGo(nil).ConTSAPermitidas([]string{srv.URL})
	if _, err := listada.Sign(context.Background(), trabajoXAdEST(t, srv.URL), clave); err != nil {
		t.Fatalf("una TSA interna de la lista debía admitirse: %v", err)
	}
}
