// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cscremota_test

import (
	"context"
	"sync/atomic"
	"testing"

	"grxfirma/internal/testsupport/csctest"
)

func TestSesionMuestraElHostDeLosMetadatosOAuth(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Emisor = "/emisor" })
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, _ := nuevaSesion(t, s, &permitida)
	d, err := sesion.Configurar(context.Background(), s.URL, csctest.ClientID)
	if err != nil {
		t.Fatalf("Configurar: %v", err)
	}
	if d.HostOAuth == "" || d.HostOAuth != d.HostServicio {
		t.Fatalf("hosts: %+v", d)
	}
	if _, _, err := sesion.Conectar(context.Background()); err != nil {
		t.Fatalf("Conectar: %v", err)
	}
}

func TestSesionConectaConListadoPaginado(t *testing.T) {
	s := csctest.Nuevo(t)
	// Los extra comparten certificado con la credencial RSA: se descartan
	// por repetidos, pero solo si el listado se recorre entero.
	s.Configurar(func(s *csctest.Servidor) { s.CredencialesExtra, s.TamPagina, s.Multisign = 3, 2, 4 })
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, _ := nuevaSesion(t, s, &permitida)
	if _, err := sesion.Configurar(context.Background(), s.URL, csctest.ClientID); err != nil {
		t.Fatal(err)
	}
	creds, omitidas, err := sesion.Conectar(context.Background())
	if err != nil || len(creds) != 2 || omitidas != 3 {
		t.Fatalf("creds=%d omitidas=%d err=%v", len(creds), omitidas, err)
	}
	for _, c := range creds {
		if c.Multisign != 4 {
			t.Fatalf("multisign = %d", c.Multisign)
		}
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.PeticionesListado != 3 {
			t.Fatalf("páginas = %d", s.PeticionesListado)
		}
	})
}
