// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

func TestSelectorIdentidadMantieneCatalogoEnDialogoLocal(t *testing.T) {
	var titulo, prompt string
	var opciones []string
	selector := &selectorCertificadoIdentidadSistema{seleccionar: func(
		_ context.Context,
		tituloRecibido string,
		promptRecibido string,
		_ string,
		_ string,
		opcionesRecibidas []string,
	) (int, error) {
		titulo, prompt = tituloRecibido, promptRecibido
		opciones = append([]string(nil), opcionesRecibidas...)
		return 1, nil
	}}
	certificados := []domain.CertificateRef{
		{ID: "cert-1", Subject: "CN=Primera\nPersona", Issuer: "CN=AC", Fingerprint: "aa", NotAfter: time.Now().Add(time.Hour)},
		{ID: "cert-2", Subject: "CN=Segunda Persona", Issuer: "CN=AC", Fingerprint: "bb", NotAfter: time.Now().Add(time.Hour)},
	}
	seleccionado, err := selector.Seleccionar(context.Background(), ports.ContextoSeleccionIdentidad{
		Origen: "https://sede.example", Finalidad: "Acreditar identidad", Operacion: "tramite.aprobar/v1",
	}, certificados)
	if err != nil || seleccionado.ID != "cert-2" {
		t.Fatalf("seleccionar: certificado=%+v error=%v", seleccionado, err)
	}
	if !strings.Contains(titulo, "Identidad reforzada") || !strings.Contains(prompt, "https://sede.example") ||
		len(opciones) != 2 || strings.Contains(opciones[0], certificados[0].ID) {
		t.Fatalf("diálogo local inesperado: título=%q prompt=%q opciones=%q", titulo, prompt, opciones)
	}
	for _, opcion := range opciones {
		if strings.ContainsFunc(opcion, unicode.IsControl) {
			t.Fatalf("opción con control: %q", opcion)
		}
	}
}

func TestSelectorIdentidadRechazaIndiceAjeno(t *testing.T) {
	selector := &selectorCertificadoIdentidadSistema{seleccionar: func(
		context.Context, string, string, string, string, []string,
	) (int, error) {
		return 9, nil
	}}
	_, err := selector.Seleccionar(context.Background(), ports.ContextoSeleccionIdentidad{},
		[]domain.CertificateRef{{ID: "cert-1"}})
	if err == nil {
		t.Fatal("se esperaba rechazo del índice ajeno")
	}
}

type localizadorSeleccionIdentidadPrueba map[string]string

func (l localizadorSeleccionIdentidadPrueba) T(id string, argumentos ...any) string {
	texto, existe := l[id]
	if !existe {
		return id
	}
	if len(argumentos) == 0 {
		return texto
	}
	return fmt.Sprintf(texto, argumentos...)
}

func TestSelectorIdentidadLocalizaInterfaz(t *testing.T) {
	var titulo, prompt, usar, cancelar string
	selector := &selectorCertificadoIdentidadSistema{
		localizador: localizadorSeleccionIdentidadPrueba{
			"identity.local.selector.title":       "Strong identity",
			"identity.local.selector.prompt":      "Origin %s; purpose %s; operation %s",
			"identity.local.selector.certificate": "%d; %s; issuer %s; until %s; fingerprint %s",
			"identity.local.selector.use":         "Use certificate",
			"identity.local.selector.cancel":      "Cancel",
		},
		seleccionar: func(
			_ context.Context,
			tituloRecibido string,
			promptRecibido string,
			usarRecibido string,
			cancelarRecibido string,
			opciones []string,
		) (int, error) {
			titulo, prompt = tituloRecibido, promptRecibido
			usar, cancelar = usarRecibido, cancelarRecibido
			if len(opciones) != 1 || !strings.Contains(opciones[0], "issuer CN=CA") {
				t.Fatalf("opción no localizada: %q", opciones)
			}
			return 0, nil
		},
	}
	_, err := selector.Seleccionar(context.Background(), ports.ContextoSeleccionIdentidad{
		Origen: "https://office.example", Finalidad: "Prove identity", Operacion: "case.approve/v1",
	}, []domain.CertificateRef{{
		ID: "cert-1", Subject: "CN=Person", Issuer: "CN=CA", Fingerprint: "aa",
		NotAfter: time.Date(2027, time.January, 2, 0, 0, 0, 0, time.UTC),
	}})
	if err != nil {
		t.Fatalf("seleccionar: %v", err)
	}
	if titulo != "Strong identity" || !strings.Contains(prompt, "Origin https://office.example") ||
		usar != "Use certificate" || cancelar != "Cancel" {
		t.Fatalf("interfaz sin localizar: %q %q %q %q", titulo, prompt, usar, cancelar)
	}
}
