// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// politicaOrigenes es una política de dominios de confianza que solo admite
// los orígenes indicados y anota los que se le preguntan.
type politicaOrigenes struct {
	mu        sync.Mutex
	admitidos map[string]bool
	evaluados []string
}

func nuevaPoliticaOrigenes(admitidos ...string) *politicaOrigenes {
	p := &politicaOrigenes{admitidos: map[string]bool{}}
	for _, o := range admitidos {
		p.admitidos[strings.ToLower(o)] = true
	}
	return p
}

func (p *politicaOrigenes) Evaluate(_ context.Context, origen string) (domain.TrustDecision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.evaluados = append(p.evaluados, origen)
	// Como truststore, la política no distingue mayúsculas en el host.
	if p.admitidos[strings.ToLower(origen)] {
		return domain.TrustDecision{Origin: origen, Status: domain.TrustAllowed}, nil
	}
	return domain.TrustDecision{Origin: origen, Status: domain.TrustDenied}, nil
}

func (p *politicaOrigenes) Allow(context.Context, string) error  { return nil }
func (p *politicaOrigenes) Deny(context.Context, string) error   { return nil }
func (p *politicaOrigenes) Remove(context.Context, string) error { return nil }

// politicaAbierta admite los orígenes de sesionRemotaPrueba.
func politicaAbierta() *politicaOrigenes {
	return nuevaPoliticaOrigenes("https://upload.example", "https://retrieve.example")
}

func sesionHostsDistintos() domain.ExchangeSession {
	return domain.ExchangeSession{
		RequestID:        "req-h03",
		SessionKey:       "clave",
		RetrieveEndpoint: "https://a.example/rt",
		UploadEndpoint:   "https://b.example/st",
		State:            domain.SessionActive,
	}
}

// TestRetrieveRechazaSubidaEnOtroHost: H-03. La descarga es de un portal de
// confianza y la subida va a otro host que no lo es: el núcleo no deja
// empezar, aunque el adaptador que construyó la sesión no lo comprobara.
func TestRetrieveRechazaSubidaEnOtroHost(t *testing.T) {
	t.Parallel()
	transporte := &transporteRetrieveMock{data: []byte("PETICION")}
	uc := application.NuevoRetrieveRequestUseCase(transporte, nuevaPoliticaOrigenes("https://a.example"), nil, nil)

	_, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{Session: sesionHostsDistintos()})
	if err == nil || !strings.Contains(err.Error(), "https://b.example") {
		t.Fatalf("Ejecutar() error = %v, se esperaba el rechazo de https://b.example", err)
	}
	if transporte.retrieveCalls != 0 {
		t.Fatalf("retrieveCalls = %d, el núcleo no debía llamar al transporte", transporte.retrieveCalls)
	}
}

func TestUploadRechazaSubidaEnOtroHost(t *testing.T) {
	t.Parallel()
	transporte := &transporteResultadoMock{}
	uc := application.NuevoUploadResultUseCase(transporte, nuevaPoliticaOrigenes("https://a.example"), nil, nil)

	err := uc.Ejecutar(context.Background(), application.UploadResultCommand{Session: sesionHostsDistintos(), Data: []byte("firma")})
	if err == nil || !strings.Contains(err.Error(), "https://b.example") {
		t.Fatalf("Ejecutar() error = %v, se esperaba el rechazo de https://b.example", err)
	}
	if transporte.uploadCalls != 0 {
		t.Fatalf("uploadCalls = %d, el núcleo no debía subir nada", transporte.uploadCalls)
	}
}

// TestRetrieveRechazaDescargaSinConfianza: tampoco vale al revés.
func TestRetrieveRechazaDescargaSinConfianza(t *testing.T) {
	t.Parallel()
	transporte := &transporteRetrieveMock{data: []byte("PETICION")}
	uc := application.NuevoRetrieveRequestUseCase(transporte, nuevaPoliticaOrigenes("https://b.example"), nil, nil)

	if _, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{Session: sesionHostsDistintos()}); err == nil {
		t.Fatal("Ejecutar() error = nil con la descarga fuera de la política")
	}
	if transporte.retrieveCalls != 0 {
		t.Fatalf("retrieveCalls = %d", transporte.retrieveCalls)
	}
}

// TestRetrieveAvisaHostsDistintosDeConfianza: retrocompatibilidad. AutoFirma
// deja al portal elegir la descarga y la subida por separado (setServlets de
// autoscript.js) y GrxFirma toma la descarga de serverUrl cuando falta
// rtservlet, así que dos hosts distintos que la política admite siguen
// funcionando, con una entrada en el registro.
func TestRetrieveAvisaHostsDistintosDeConfianza(t *testing.T) {
	var registro bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&registro, nil)))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	transporte := &transporteRetrieveMock{data: []byte("PETICION")}
	politica := nuevaPoliticaOrigenes("https://a.example", "https://b.example")
	uc := application.NuevoRetrieveRequestUseCase(transporte, politica, nil, nil)

	if _, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{Session: sesionHostsDistintos()}); err != nil {
		t.Fatalf("Ejecutar() error = %v", err)
	}
	if transporte.retrieveCalls != 1 {
		t.Fatalf("retrieveCalls = %d, want 1", transporte.retrieveCalls)
	}
	if !strings.Contains(registro.String(), "sesion_intercambio_hosts_distintos") {
		t.Fatalf("falta el aviso en el registro: %q", registro.String())
	}
	if strings.Contains(registro.String(), "req-h03") || strings.Contains(registro.String(), "clave") {
		t.Fatalf("el aviso no debe llevar el identificador ni la clave: %q", registro.String())
	}
}

// TestSesionIntercambioMismoHostPreguntaUnaVez: descarga y subida en el
// mismo portal solo se evalúan una vez (la política puede abrir un diálogo).
func TestSesionIntercambioMismoHostPreguntaUnaVez(t *testing.T) {
	t.Parallel()
	politica := nuevaPoliticaOrigenes("https://sede.example")
	sesion := domain.ExchangeSession{
		RequestID:        "r",
		RetrieveEndpoint: "https://sede.example/afirma-signature-retriever/RetrieveService",
		UploadEndpoint:   "https://SEDE.example/afirma-signature-storage/StorageService",
	}
	if err := application.ComprobarSesionIntercambio(context.Background(), politica, sesion); err != nil {
		t.Fatalf("ComprobarSesionIntercambio() error = %v", err)
	}
	if len(politica.evaluados) != 1 {
		t.Fatalf("orígenes evaluados = %v, se esperaba uno", politica.evaluados)
	}
}

// TestSesionIntercambioSinPoliticaFallaCerrado: un caso de uso que recibe un
// destino remoto no puede funcionar sin política de dominios.
func TestSesionIntercambioSinPoliticaFallaCerrado(t *testing.T) {
	t.Parallel()
	transporte := &transporteRetrieveMock{data: []byte("PETICION")}
	uc := application.NuevoRetrieveRequestUseCase(transporte, nil, nil, nil)
	_, err := uc.Ejecutar(context.Background(), application.RetrieveRequestCommand{Session: sesionRemotaPrueba()})
	if !errors.Is(err, application.ErrSinPoliticaConfianza) {
		t.Fatalf("Ejecutar() error = %v, want ErrSinPoliticaConfianza", err)
	}
	if transporte.retrieveCalls != 0 {
		t.Fatalf("retrieveCalls = %d", transporte.retrieveCalls)
	}
	if err := application.ComprobarSesionIntercambio(context.Background(), nil, sesionRemotaPrueba()); !errors.Is(err, application.ErrSinPoliticaConfianza) {
		t.Fatalf("ComprobarSesionIntercambio(nil) = %v", err)
	}
}

func TestSesionIntercambioRechazaEndpointRelativo(t *testing.T) {
	t.Parallel()
	sesion := domain.ExchangeSession{RequestID: "r", RetrieveEndpoint: "/rt", UploadEndpoint: "https://a.example/st"}
	if err := application.ComprobarSesionIntercambio(context.Background(), nuevaPoliticaOrigenes("https://a.example"), sesion); err == nil {
		t.Fatal("se esperaba error con un endpoint sin host")
	}
}
