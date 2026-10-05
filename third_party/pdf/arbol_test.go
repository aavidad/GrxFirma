// Copyright 2026 Alberto Avidad Fernández. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// pdfSintetico escribe un PDF con los objetos indicados (el primero es el
// catálogo, objeto 1) y una tabla xref clásica.
func pdfSintetico(objs []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func abrirSintetico(t *testing.T, objs []string) *Reader {
	t.Helper()
	data := pdfSintetico(objs)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// conPlazo ejecuta f y falla si no termina a tiempo: sin los límites, los
// árboles maliciosos dejaban el lector en un bucle infinito.
func conPlazo(t *testing.T, f func()) {
	t.Helper()
	hecho := make(chan any, 1)
	go func() {
		defer func() { hecho <- recover() }()
		f()
	}()
	plazo := 10 * time.Second
	if d, ok := t.Deadline(); ok && time.Until(d) < plazo {
		plazo = time.Until(d) / 2
	}
	select {
	case p := <-hecho:
		if p != nil {
			panic(p)
		}
	case <-time.After(plazo):
		t.Fatal("el recorrido no terminó: bucle sin límite ante un PDF malicioso")
	}
}

// arbolCiclico: /Kids de un nodo intermedio apunta a su padre.
func arbolCiclico() []string {
	return []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 5 >>",
		"<< /Type /Pages /Parent 2 0 R /Kids [2 0 R] /Count 5 >>",
	}
}

// arbolProfundo encadena n nodos Pages con una sola página al final.
func arbolProfundo(n int) []string {
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>"}
	for i := 0; i < n; i++ {
		id := 2 + i
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf(" /Parent %d 0 R", id-1)
		}
		objs = append(objs, fmt.Sprintf("<< /Type /Pages%s /Kids [%d 0 R] /Count 1 >>", parent, id+1))
	}
	objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] >>", n+1))
	return objs
}

func TestBuscarPagina_KidsCiclicoDevuelveError(t *testing.T) {
	r := abrirSintetico(t, arbolCiclico())
	conPlazo(t, func() {
		p, err := r.BuscarPagina(1)
		if !errors.Is(err, ErrArbolPaginas) || !p.V.IsNull() {
			t.Errorf("se esperaba un error de árbol de páginas, no %v", err)
		}
		if !r.Page(1).V.IsNull() {
			t.Error("Page debe devolver una página nula ante un ciclo")
		}
	})
}

func TestBuscarPagina_ProfundidadExcesivaDevuelveError(t *testing.T) {
	r := abrirSintetico(t, arbolProfundo(10000))
	conPlazo(t, func() {
		if _, err := r.BuscarPagina(1); !errors.Is(err, ErrArbolPaginas) {
			t.Errorf("se esperaba un error por profundidad, no %v", err)
		}
	})
	// Un árbol razonable sigue funcionando.
	r = abrirSintetico(t, arbolProfundo(20))
	if p, err := r.BuscarPagina(1); err != nil || p.V.Key("Type").Name() != "Page" {
		t.Fatalf("árbol de 20 niveles: %v", err)
	}
}

func TestPagina_HerenciaConParentCiclico(t *testing.T) {
	r := abrirSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 4 0 R >>",
		"<< /Type /Pages /Parent 5 0 R >>",
		"<< /Type /Pages /Parent 4 0 R >>",
	})
	conPlazo(t, func() {
		p := r.Page(1)
		if p.V.IsNull() {
			t.Error("no se encontró la página")
			return
		}
		for _, clave := range []string{"Resources", "MediaBox", "CropBox", "Rotate"} {
			if v := p.Heredado(clave); !v.IsNull() {
				t.Errorf("%s: valor inesperado %v", clave, v)
			}
		}
		if !p.Resources().IsNull() {
			t.Error("Resources debe ser nulo")
		}
	})
}

func TestPagina_HerenciaConParentProfundo(t *testing.T) {
	objs := arbolProfundo(1000)
	// El nodo raíz lleva la caja; la página está a 1000 niveles.
	objs[1] = "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>"
	objs[len(objs)-1] = fmt.Sprintf("<< /Type /Page /Parent %d 0 R >>", len(objs)-1)
	r := abrirSintetico(t, objs)
	conPlazo(t, func() {
		p := Page{V: r.Resolve(objptr{}, objptr{uint32(len(objs)), 0})}
		if p.V.Key("Type").Name() != "Page" {
			t.Error("no se resolvió la página")
			return
		}
		if v := p.Heredado("MediaBox"); !v.IsNull() {
			t.Error("la herencia debe cortarse al superar la profundidad máxima")
		}
	})
	// Herencia normal.
	r = abrirSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 /Rotate 90 /MediaBox [0 0 10 20] >>",
		"<< /Type /Page /Parent 2 0 R >>",
	})
	p := r.Page(1)
	if p.Heredado("Rotate").Int64() != 90 || p.Heredado("MediaBox").Len() != 4 {
		t.Fatal("no se heredan /Rotate ni /MediaBox del nodo Pages")
	}
}

func TestValidarArbolPaginas(t *testing.T) {
	for _, c := range []struct {
		nombre string
		objs   []string
		max    int
		want   int
		error  bool
	}{
		{"correcto", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 3 >>",
			"<< /Type /Page /Parent 2 0 R >>",
			"<< /Type /Pages /Parent 2 0 R /Kids [5 0 R 6 0 R] /Count 2 >>",
			"<< /Type /Page /Parent 4 0 R >>",
			"<< /Type /Page /Parent 4 0 R >>",
		}, 100, 3, false},
		{"count-enorme", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 2000000000 >>",
			"<< /Type /Page /Parent 2 0 R >>",
		}, 20000, 0, true},
		{"count-intermedio-falso", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R] /Count 2 >>",
			"<< /Type /Pages /Parent 2 0 R /Kids [4 0 R 5 0 R] /Count 7 >>",
			"<< /Type /Page /Parent 3 0 R >>",
			"<< /Type /Page /Parent 3 0 R >>",
		}, 100, 0, true},
		{"ciclo", arbolCiclico(), 100, 0, true},
		{"profundo", arbolProfundo(10000), 100, 0, true},
		{"pagina-repetida", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R 3 0 R] /Count 2 >>",
			"<< /Type /Page /Parent 2 0 R >>",
		}, 100, 0, true},
		{"demasiadas-paginas", []string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>",
			"<< /Type /Page /Parent 2 0 R >>",
			"<< /Type /Page /Parent 2 0 R >>",
			"<< /Type /Page /Parent 2 0 R >>",
		}, 2, 0, true},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			r := abrirSintetico(t, c.objs)
			conPlazo(t, func() {
				n, err := r.ValidarArbolPaginas(c.max)
				if c.error {
					if !errors.Is(err, ErrArbolPaginas) {
						t.Errorf("se esperaba un error de árbol de páginas, no %v (%d)", err, n)
					}
					return
				}
				if err != nil || n != c.want {
					t.Errorf("ValidarArbolPaginas = %d, %v; se esperaba %d", n, err, c.want)
				}
			})
		})
	}
}

// Una DAG de /Kids repetidos crece de forma exponencial si no se recuerdan
// los nodos ya vistos.
func TestValidarArbolPaginas_KidsRepetidosExponencial(t *testing.T) {
	objs := []string{"<< /Type /Catalog /Pages 2 0 R >>"}
	const niveles = 40
	for i := 0; i < niveles; i++ {
		id := 2 + i
		objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R %d 0 R] /Count 1 >>", id+1, id+1))
	}
	objs = append(objs, "<< /Type /Page >>")
	r := abrirSintetico(t, objs)
	conPlazo(t, func() {
		if _, err := r.ValidarArbolPaginas(20000); !errors.Is(err, ErrArbolPaginas) {
			t.Errorf("se esperaba un error por nodos repetidos, no %v", err)
		}
	})
}

func TestOutline_SiguienteCiclico(t *testing.T) {
	r := abrirSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R /Outlines 3 0 R >>",
		"<< /Type /Pages /Kids [] /Count 0 >>",
		"<< /First 4 0 R >>",
		"<< /Title (a) /Next 5 0 R /First 4 0 R >>",
		"<< /Title (b) /Next 4 0 R >>",
	})
	conPlazo(t, func() {
		o := r.Outline()
		if len(o.Child) == 0 || len(o.Child) > 2 {
			t.Errorf("índice inesperado: %d entradas", len(o.Child))
		}
	})
}

// Arrays anidados sin fin agotaban la pila de Go, que no se puede recuperar.
func TestLector_AnidamientoExcesivoDeObjetos(t *testing.T) {
	const n = 200000
	r := abrirSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R /Profundo 3 0 R >>",
		"<< /Type /Pages /Kids [] /Count 0 >>",
		strings.Repeat("[", n) + strings.Repeat("]", n),
	})
	conPlazo(t, func() {
		defer func() {
			p := recover()
			if p == nil || !strings.Contains(fmt.Sprint(p), "anidamiento") {
				t.Errorf("se esperaba un error por anidamiento, no %v", p)
			}
		}()
		r.Trailer().Key("Root").Key("Profundo").Len()
	})
	// Anidamiento normal.
	r = abrirSintetico(t, []string{
		"<< /Type /Catalog /Pages 2 0 R /Normal [[[1] << /A [2] >>]] >>",
		"<< /Type /Pages /Kids [] /Count 0 >>",
	})
	if r.Trailer().Key("Root").Key("Normal").Index(0).Index(0).Index(0).Int64() != 1 {
		t.Fatal("no se leen los arrays anidados normales")
	}
}

// flujoObjetos describe un flujo de objetos sintético: los objetos que
// contiene, texto extra para su diccionario y, si no está vacía, la /Length
// que se escribe en lugar de la real.
type flujoObjetos struct {
	contenidos map[int]string
	extra      string
	longitud   string
}

// pdfConObjStm escribe un PDF con flujo de referencias cruzadas sin
// comprimir; los objetos de los flujos se marcan como comprimidos.
func pdfConObjStm(sueltos map[int]string, flujos map[int]flujoObjetos, total int) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	type entrada struct {
		tipo           byte
		campo2, campo3 int
	}
	entradas := make([]entrada, total+1)
	for id := 1; id <= total; id++ {
		if cuerpo, ok := sueltos[id]; ok {
			entradas[id] = entrada{1, b.Len(), 0}
			fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", id, cuerpo)
		}
	}
	for id := 1; id <= total; id++ {
		s, ok := flujos[id]
		if !ok {
			continue
		}
		var cab, cuerpo bytes.Buffer
		idx := 0
		for oid := 1; oid <= total; oid++ {
			txt, ok := s.contenidos[oid]
			if !ok {
				continue
			}
			fmt.Fprintf(&cab, "%d %d ", oid, cuerpo.Len())
			cuerpo.WriteString(txt + "\n")
			entradas[oid] = entrada{2, id, idx}
			idx++
		}
		datos := cab.String() + cuerpo.String()
		longitud := s.longitud
		if longitud == "" {
			longitud = fmt.Sprint(len(datos))
		}
		entradas[id] = entrada{1, b.Len(), 0}
		fmt.Fprintf(&b, "%d 0 obj\n<< /Type /ObjStm /N %d /First %d /Length %s%s >>\nstream\n%s\nendstream\nendobj\n",
			id, idx, cab.Len(), longitud, s.extra, datos)
	}
	xref := b.Len()
	var filas bytes.Buffer
	for id := 0; id <= total; id++ {
		e := entradas[id]
		filas.WriteByte(e.tipo)
		filas.Write([]byte{byte(e.campo2 >> 24), byte(e.campo2 >> 16), byte(e.campo2 >> 8), byte(e.campo2)})
		filas.Write([]byte{byte(e.campo3 >> 8), byte(e.campo3)})
	}
	// La propia xref ocupa la entrada total+1.
	filas.WriteByte(1)
	filas.Write([]byte{byte(xref >> 24), byte(xref >> 16), byte(xref >> 8), byte(xref)})
	filas.Write([]byte{0, 0})
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /XRef /Size %d /W [1 4 2] /Root 1 0 R /Length %d >>\nstream\n",
		total+1, total+2, filas.Len())
	b.Write(filas.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xref)
	return b.Bytes()
}

// Un objeto comprimido cuyo flujo de objetos toma su /Length de ese mismo
// objeto recursaba sin fin y agotaba la pila, lo que no se puede recuperar.
func TestLector_ObjStmRecursivoDevuelveError(t *testing.T) {
	data := pdfConObjStm(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /X 4 0 R >>",
		2: "<< /Type /Pages /Kids [] /Count 0 >>",
	}, map[int]flujoObjetos{
		3: {contenidos: map[int]string{4: "17"}, longitud: "4 0 R"},
	}, 4)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	conPlazo(t, func() {
		defer func() {
			if p := recover(); p == nil {
				t.Error("se esperaba un error controlado")
			}
		}()
		r.Trailer().Key("Root").Key("X").Int64()
	})
}

// Dos flujos de objetos que se extienden mutuamente y no contienen el
// objeto buscado dejaban el lector en un bucle infinito.
func TestLector_ObjStmExtendsCiclico(t *testing.T) {
	data := pdfConObjStm(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /X 6 0 R >>",
		2: "<< /Type /Pages /Kids [] /Count 0 >>",
	}, map[int]flujoObjetos{
		3: {contenidos: map[int]string{5: "1"}, extra: " /Extends 4 0 R"},
		4: {contenidos: map[int]string{7: "2"}, extra: " /Extends 3 0 R"},
	}, 7)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	// El objeto 6 se declara dentro del flujo 3, que no lo contiene.
	r.xref[6] = xref{ptr: objptr{6, 0}, inStream: true, stream: objptr{3, 0}}
	conPlazo(t, func() {
		defer func() {
			if p := recover(); p == nil {
				t.Error("se esperaba un error controlado")
			}
		}()
		r.Trailer().Key("Root").Key("X").Int64()
	})
}

// Un flujo de objetos normal se sigue leyendo.
func TestLector_ObjStmNormal(t *testing.T) {
	data := pdfConObjStm(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /X 4 0 R /Y 6 0 R >>",
		2: "<< /Type /Pages /Kids [] /Count 0 >>",
	}, map[int]flujoObjetos{
		3: {contenidos: map[int]string{4: "17"}},
		5: {contenidos: map[int]string{6: "(hola)"}, extra: " /Extends 3 0 R"},
	}, 6)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	raiz := r.Trailer().Key("Root")
	if raiz.Key("X").Int64() != 17 || raiz.Key("Y").Text() != "hola" {
		t.Fatalf("flujos de objetos: X=%v Y=%v", raiz.Key("X"), raiz.Key("Y"))
	}
}
