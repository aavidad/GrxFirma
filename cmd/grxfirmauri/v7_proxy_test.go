// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"net/http"
	"testing"
)

func TestV7ProxyHTTPDependeDelProxyNoDelDestino(t *testing.T) {
	for _, ps := range []string{"http", "https"} {
		for _, ts := range []string{"http", "https"} {
			t.Run(ps+"/"+ts, func(t *testing.T) {
				machinepolicy.UnsetForTest(t, "permitir_proxy_http")
				excepciones.SetForTest(t, "proxy_http", false)
				t.Setenv("HTTP_PROXY", ps+"://usuario:secreto@proxy.example:8443")
				t.Setenv("HTTPS_PROXY", ps+"://usuario:secreto@proxy.example:8443")
				t.Setenv("NO_PROXY", "")
				enabled, typ := true, "system"
				loader := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
					return docWithProxy(ports.ConfiguracionUsuarioProxy{Enabled: &enabled, Type: &typ}), nil
				}
				transport := construirTransporteHTTPCompartidoConDependencias(t.TempDir(), loader, nil)
				ctx, warnings := domain.ConAvisosOperacion(context.Background())
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts+"://8.8.8.8/list.crl", nil)
				if err != nil {
					t.Fatal(err)
				}
				proxy, err := transport.Proxy(req)
				var block *excepciones.Block
				if ps == "http" {
					if !errors.As(err, &block) || block.ID != "proxy_http" || proxy != nil {
						t.Fatalf("HTTP proxy no bloqueado: %v %v", proxy, err)
					}
				} else {
					if err != nil || proxy == nil || proxy.Scheme != "https" || proxy.User == nil {
						t.Fatalf("HTTPS proxy bloqueado por destino %s: %v %v", ts, proxy, err)
					}
					if len(warnings.Claves()) != 0 {
						t.Fatal("proxy HTTPS da aviso HTTP", warnings.Claves())
					}
				}
			})
		}
	}
}
