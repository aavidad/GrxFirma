// SPDX-License-Identifier: EUPL-1.2
package main

import (
	"context"
	"errors"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"net/http"
	"slices"
	"testing"
)

func TestV9SOCKSCredencialesExigeAjusteYAviso(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			machinepolicy.UnsetForTest(t, "permitir_proxy_http")
			excepciones.SetForTest(t, "proxy_http", false)
			t.Setenv("HTTPS_PROXY", scheme+"://usuario:clave@proxy.example:1080")
			t.Setenv("NO_PROXY", "")
			transport := &http.Transport{}
			ensureTransportProxyFromEnvironment(transport)
			ctx, warnings := domain.ConAvisosOperacion(excepciones.WithConfigDir(context.Background(), t.TempDir()))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://destino.example/documento", nil)
			if err != nil {
				t.Fatal(err)
			}
			var block *excepciones.Block
			if p, err := transport.Proxy(req); !errors.As(err, &block) || block.ID != "proxy_http" || p != nil {
				t.Fatalf("%s envía credenciales sin ajuste: %v %v", scheme, p, err)
			}
			excepciones.SetForTest(t, "proxy_http", true)
			if p, err := transport.Proxy(req); err != nil || p == nil || p.User == nil {
				t.Fatalf("%s con permiso: %v %v", scheme, p, err)
			}
			if !slices.Contains(warnings.Claves(), excepciones.Warning("proxy_http")) {
				t.Fatal(warnings.Claves())
			}
			machinepolicy.SetForTest(t, "permitir_proxy_http", false)
			if p, err := transport.Proxy(req); !errors.As(err, &block) || p != nil {
				t.Fatalf("directiva no prevalece: %v %v", p, err)
			}
			machinepolicy.ErrorForTest(t, "permitir_proxy_http", errors.New("ilegible"))
			if p, err := transport.Proxy(req); !errors.As(err, &block) || p != nil {
				t.Fatalf("directiva ilegible no cierra: %v %v", p, err)
			}
		})
	}
}
