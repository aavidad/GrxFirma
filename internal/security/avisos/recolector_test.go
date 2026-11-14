// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package avisos

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
)

// Lote 16d: los avisos de una firma llegan al resultado de esa firma, no a
// una lista global que comparten todas las operaciones del proceso.
func TestRecolector_GuardaLasClavesDeSuOperacion(t *testing.T) {
	ctx, rec := ConRecolector(context.Background())
	Anotar(ctx, "firma.a")
	Anotar(ctx, " firma.b ")
	Anotar(ctx, "firma.a")
	Anotar(ctx, "")
	if got := rec.Claves(); !slices.Equal(got, []string{"firma.a", "firma.b"}) {
		t.Fatalf("claves = %q", got)
	}
	// La copia devuelta no deja tocar el recolector.
	rec.Claves()[0] = "otra"
	if got := rec.Claves(); got[0] != "firma.a" {
		t.Fatalf("Claves() devolvió el slice interno: %q", got)
	}
}

func TestRecolector_SinRecolectorNoHaceNada(t *testing.T) {
	Anotar(context.Background(), "firma.a")
	var rec *Recolector
	if got := rec.Claves(); got != nil {
		t.Fatalf("recolector nil = %q", got)
	}
}

func TestRecolector_CadaOperacionTieneElSuyo(t *testing.T) {
	ctxA, a := ConRecolector(context.Background())
	ctxB, b := ConRecolector(context.Background())
	Anotar(ctxA, "firma.a")
	Anotar(ctxB, "firma.b")
	// Uno anidado recoge solo lo suyo.
	ctxC, c := ConRecolector(ctxA)
	Anotar(ctxC, "firma.c")
	if !slices.Equal(a.Claves(), []string{"firma.a"}) || !slices.Equal(b.Claves(), []string{"firma.b"}) ||
		!slices.Equal(c.Claves(), []string{"firma.c"}) {
		t.Fatalf("a=%q b=%q c=%q", a.Claves(), b.Claves(), c.Claves())
	}
}

func TestRecolector_ConcurrenteYConTope(t *testing.T) {
	ctx, rec := ConRecolector(context.Background())
	var wg sync.WaitGroup
	for i := range 4 * maxAvisos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Anotar(ctx, fmt.Sprintf("firma.%d", i))
		}()
	}
	wg.Wait()
	if got := len(rec.Claves()); got != maxAvisos {
		t.Fatalf("claves = %d, tope %d", got, maxAvisos)
	}
}
