// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cscremota_test

import (
	"context"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/adapters/outbound/desktop/cscremota"
	"grxfirma/internal/testsupport/csctest"
)

// ID-03: la organización limita los prestadores CSC. Un servicio fuera de la
// lista se rechaza antes de contactar con él; sin lista, todo sigue igual.
func TestSesionSoloAdmiteLosServiciosDeLaPolitica(t *testing.T) {
	s := csctest.Nuevo(t)
	var contactado atomic.Bool
	s.Configurar(func(s *csctest.Servidor) {
		s.InfoOverride = func(w http.ResponseWriter, _ *http.Request) {
			contactado.Store(true)
			http.Error(w, "no", http.StatusTeapot)
		}
	})
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	var servicios []string
	sesion := cscremota.Nueva(cscremota.Opciones{
		ConfigDir: t.TempDir(),
		HTTP:      s.Client(),
		Navegador: s.Navegador(),
		CargarConfig: func() (config.Config, config.Policy, error) {
			cfg := config.Default()
			cfg.FirmaRemotaCSC = true
			return cfg, config.Policy{FirmaRemotaCSCServicios: servicios}, nil
		},
	})
	t.Cleanup(func() { _ = sesion.Close() })

	for _, lista := range [][]string{{"otro.example"}, {"mal/entrada"}, {}} {
		servicios = lista
		_, err := sesion.Configurar(context.Background(), s.URL, csctest.ClientID)
		codigo(t, err, csc.CodigoServicioNoPermitido)
		if contactado.Load() {
			t.Fatalf("con %q no debía contactar con el servicio", lista)
		}
	}

	// En la lista (con el puerto explícito), sí se contacta con él.
	servicios = []string{"otro.example", u.Host}
	_, _ = sesion.Configurar(context.Background(), s.URL, csctest.ClientID)
	if !contactado.Load() {
		t.Fatal("un servicio de la lista debía consultarse")
	}
}
