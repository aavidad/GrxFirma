// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package documentpicker

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"grxfirma/internal/ports"
)

func resolverFijo(doc SelectedDocument, err error) Resolver {
	return ResolveFunc(func(context.Context) (SelectedDocument, error) {
		return doc, err
	})
}

func TestPicker_TraduceADominio(t *testing.T) {
	t.Parallel()

	p := Nuevo(resolverFijo(SelectedDocument{
		Name:     "contrato.pdf",
		Content:  []byte("%PDF-1.7"),
		MIMEType: "application/pdf",
	}, nil))

	doc, err := p.Pick(context.Background())
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if doc.Name != "contrato.pdf" {
		t.Errorf("Name = %q", doc.Name)
	}
	if doc.MIMEType != "application/pdf" {
		t.Errorf("MIMEType = %q", doc.MIMEType)
	}
	if !bytes.Equal(doc.Content, []byte("%PDF-1.7")) {
		t.Errorf("Content = %q", doc.Content)
	}
}

// Nombre y tipo vacios no deben propagarse al dominio: la UI puede devolverlos
// en blanco y el documento seguiria siendo valido.
func TestPicker_RellenaNombreYTipoPorDefecto(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre       string
		entrada      SelectedDocument
		esperaNombre string
		esperaMIME   string
	}{
		{
			nombre:       "vacios",
			entrada:      SelectedDocument{Content: []byte("x")},
			esperaNombre: "documento",
			esperaMIME:   "application/octet-stream",
		},
		{
			nombre:       "solo espacios",
			entrada:      SelectedDocument{Name: "   ", MIMEType: "  ", Content: []byte("x")},
			esperaNombre: "documento",
			esperaMIME:   "application/octet-stream",
		},
		{
			nombre:       "se recortan los margenes",
			entrada:      SelectedDocument{Name: "  a.pdf ", MIMEType: " application/pdf ", Content: []byte("x")},
			esperaNombre: "a.pdf",
			esperaMIME:   "application/pdf",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			doc, err := Nuevo(resolverFijo(caso.entrada, nil)).Pick(context.Background())
			if err != nil {
				t.Fatalf("Pick() error = %v", err)
			}
			if doc.Name != caso.esperaNombre {
				t.Errorf("Name = %q, se esperaba %q", doc.Name, caso.esperaNombre)
			}
			if doc.MIMEType != caso.esperaMIME {
				t.Errorf("MIMEType = %q, se esperaba %q", doc.MIMEType, caso.esperaMIME)
			}
		})
	}
}

// Un documento sin contenido lo rechaza el dominio, no el adaptador.
func TestPicker_ContenidoVacioSeRechaza(t *testing.T) {
	t.Parallel()

	if _, err := Nuevo(resolverFijo(SelectedDocument{Name: "a.pdf"}, nil)).Pick(context.Background()); err == nil {
		t.Fatal("un documento sin contenido debe rechazarse")
	}
}

func TestPicker_SinResolver(t *testing.T) {
	t.Parallel()

	if _, err := Nuevo(nil).Pick(context.Background()); err == nil {
		t.Fatal("un picker sin resolver debe fallar en vez de entrar en panico")
	}
}

func TestPicker_PropagaElErrorDelResolver(t *testing.T) {
	t.Parallel()

	_, err := Nuevo(resolverFijo(SelectedDocument{}, ErrSeleccionCancelada)).Pick(context.Background())
	if !errors.Is(err, ErrSeleccionCancelada) {
		t.Fatalf("se esperaba ErrSeleccionCancelada y fue: %v", err)
	}
}

// El contexto se comprueba antes de abrir el selector: cancelar no debe
// llegar a mostrar un dialogo al usuario.
func TestPicker_ContextoCanceladoNoAbreElSelector(t *testing.T) {
	t.Parallel()

	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()

	abierto := false
	p := Nuevo(ResolveFunc(func(context.Context) (SelectedDocument, error) {
		abierto = true
		return SelectedDocument{Name: "a.pdf", Content: []byte("x")}, nil
	}))

	if _, err := p.Pick(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("se esperaba context.Canceled y fue: %v", err)
	}
	if abierto {
		t.Error("no debe invocarse el resolver con el contexto ya cancelado")
	}
}

func TestPicker_PickFilteredPasaElFiltroAlResolver(t *testing.T) {
	t.Parallel()

	var recibido ports.DocumentFilter
	p := Nuevo(ResolveFilteredFunc(func(_ context.Context, filter ports.DocumentFilter) (SelectedDocument, error) {
		recibido = filter
		return SelectedDocument{Name: "a.pdf", Content: []byte("%PDF-1.7"), MIMEType: "application/pdf"}, nil
	}))
	if _, err := p.PickFiltered(context.Background(), ports.DocumentFilter{Extensions: []string{"pdf"}}); err != nil {
		t.Fatalf("PickFiltered() error = %v", err)
	}
	if len(recibido.Extensions) != 1 || recibido.Extensions[0] != "pdf" {
		t.Fatalf("filtro recibido = %v", recibido.Extensions)
	}
	if _, err := p.Pick(context.Background()); err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if len(recibido.Extensions) != 0 {
		t.Fatalf("Pick() sin filtro pasó %v", recibido.Extensions)
	}
}

func TestPicker_PickFilteredConResolverSinFiltro(t *testing.T) {
	t.Parallel()

	p := Nuevo(resolverFijo(SelectedDocument{Name: "a.txt", Content: []byte("x")}, nil))
	if _, err := p.PickFiltered(context.Background(), ports.DocumentFilter{Extensions: []string{"pdf"}}); err != nil {
		t.Fatalf("PickFiltered() error = %v", err)
	}
}
