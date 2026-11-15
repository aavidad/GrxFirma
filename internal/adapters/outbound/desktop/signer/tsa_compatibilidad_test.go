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
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/tsatest"
)

func permitirTSAHTTPPrueba(t *testing.T) {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", true)
}

// Usa el recibo persistido por la interfaz local, sin sustituir la decisión de usuario.
func contextoConAceptacionTSAHTTP(t *testing.T) (context.Context, *domain.AvisosOperacion) {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	_, err := excepciones.ActivateLocal(excepciones.ForLocalDesktop(ctx), dir, excepciones.ActivationRequest{
		ID: "tsa_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion, Origin: excepciones.OriginSettings,
	})
	if err != nil {
		t.Fatalf("no se pudo aceptar TSA HTTP en la configuración temporal: %v", err)
	}
	return domain.ConAvisosOperacion(ctx)
}

func exigirAvisoTSAHTTP(t *testing.T, avisos *domain.AvisosOperacion) {
	t.Helper()
	if !slices.Contains(avisos.Claves(), excepciones.Warning("tsa_http")) {
		t.Fatalf("falta aviso TSA HTTP: %q", avisos.Claves())
	}
}

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
	permitirTSAHTTPPrueba(t)
	srv, peticiones := tsaLocalContada(t)
	registro := capturarRegistro(t)
	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)

	motor := signer.NuevoMotorFirmaGo(nil).AvisarTSADeOpcionesEnRedLocal()
	// tsaclient ya rechazaba credenciales en la URL; aquí se comprueba que el
	// aviso no lleva ruta ni consulta.
	tsaURL := srv.URL + "/ruta/privada?x=1"
	ctx, avisosOperacion := domain.ConAvisosOperacion(context.Background())
	if _, err := motor.Sign(ctx, trabajoXAdEST(t, tsaURL), clave); err != nil {
		t.Fatalf("por REST la TSA interna debía admitirse: %v", err)
	}
	exigirAvisoTSAHTTP(t, avisosOperacion)
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
	permitirTSAHTTPPrueba(t)
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
	ctx, avisosOperacion := domain.ConAvisosOperacion(context.Background())
	if _, err := dentro.Sign(ctx, trabajoXAdEST(t, srv.URL), clave); err != nil {
		t.Fatalf("una TSA de la lista debía admitirse: %v", err)
	}
	exigirAvisoTSAHTTP(t, avisosOperacion)
	vacia := signer.NuevoMotorFirmaGo(nil).AvisarTSADeOpcionesEnRedLocal().ConTSAPermitidas([]string{})
	if _, err := vacia.Sign(context.Background(), trabajoXAdEST(t, srv.URL), clave); err == nil {
		t.Fatal("una lista definida y vacía no debía admitir ninguna TSA de opciones")
	}
}

// En los caminos web (afirma://, WebSocket, nativehost) una TSA interna sigue
// rechazada salvo que esté en la lista; la lista no restringe las públicas.
func TestMotorFirmaGo_TSADeOpcionesWeb_ListaSoloExceptua(t *testing.T) {
	permitirTSAHTTPPrueba(t)
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
	ctx, avisosOperacion := domain.ConAvisosOperacion(context.Background())
	if _, err := listada.Sign(ctx, trabajoXAdEST(t, srv.URL), clave); err != nil {
		t.Fatalf("una TSA interna de la lista debía admitirse: %v", err)
	}
	exigirAvisoTSAHTTP(t, avisosOperacion)
}
