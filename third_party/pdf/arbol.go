// Copyright 2026 Alberto Avidad Fernández. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

import (
	"errors"
	"fmt"
)

// AutoFirmaV2: recorridos acotados del árbol de páginas. El PDF de entrada no
// es de confianza. Un /Kids que apunta a un antecesor, un /Parent circular o
// un árbol de miles de niveles dejaban el lector en un bucle infinito; los
// recorridos recursivos agotaban la pila, que en Go no se puede recuperar.

// MaxProfundidadArbolPaginas limita los niveles del árbol de páginas y de la
// cadena /Parent. Los PDF reales no pasan de unas decenas.
const MaxProfundidadArbolPaginas = 256

// ErrArbolPaginas indica un árbol de páginas malformado o malicioso.
var ErrArbolPaginas = errors.New("árbol de páginas del PDF no válido")

func errorArbol(formato string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrArbolPaginas, fmt.Sprintf(formato, args...))
}

// refIndirecta devuelve la referencia con la que el valor x aparece escrito
// en su contenedor, si es una referencia indirecta.
func refIndirecta(x interface{}) (objptr, bool) {
	p, ok := x.(objptr)
	return p, ok
}

// crudoClave devuelve el valor sin resolver de una clave de diccionario.
func crudoClave(v Value, clave string) interface{} {
	switch x := v.data.(type) {
	case dict:
		return x[name(clave)]
	case stream:
		return x.hdr[name(clave)]
	}
	return nil
}

// crudoIndice devuelve el elemento sin resolver de un array.
func crudoIndice(v Value, i int) interface{} {
	x, ok := v.data.(array)
	if !ok || i < 0 || i >= len(x) {
		return nil
	}
	return x[i]
}

// BuscarPagina devuelve la página num (desde 1) bajando por el árbol según
// los /Count de cada nodo, como Page, pero devuelve un error envuelto en
// ErrArbolPaginas si el árbol tiene ciclos o más niveles de los permitidos,
// y un error simple si la página no existe.
func (r *Reader) BuscarPagina(num int) (Page, error) {
	if num < 1 {
		return Page{}, fmt.Errorf("página %d fuera del documento", num)
	}
	resto := int64(num) - 1 // desde 0
	raiz := r.Trailer().Key("Root")
	nodo := raiz.Key("Pages")
	vistos := make(map[objptr]bool)
	if p, ok := refIndirecta(crudoClave(raiz, "Pages")); ok {
		vistos[p] = true
	}
	for nivel := 0; nodo.Key("Type").Name() == "Pages"; nivel++ {
		if nivel >= MaxProfundidadArbolPaginas {
			return Page{}, errorArbol("más de %d niveles", MaxProfundidadArbolPaginas)
		}
		if nodo.Key("Count").Int64() < resto {
			break
		}
		kids := nodo.Key("Kids")
		siguiente := Value{}
		for i := 0; i < kids.Len() && siguiente.IsNull(); i++ {
			kid := kids.Index(i)
			switch kid.Key("Type").Name() {
			case "Pages":
				c := kid.Key("Count").Int64()
				if resto < c {
					if p, ok := refIndirecta(crudoIndice(kids, i)); ok {
						if vistos[p] {
							return Page{}, errorArbol("ciclo en /Kids (objeto %d)", p.id)
						}
						vistos[p] = true
					}
					siguiente = kid
					continue
				}
				resto -= c
			case "Page":
				if resto == 0 {
					return Page{kid}, nil
				}
				resto--
			}
		}
		if siguiente.IsNull() {
			break
		}
		nodo = siguiente
	}
	return Page{}, fmt.Errorf("página %d no encontrada", num)
}

// Heredado devuelve el valor de la clave en la página o, si falta, en el
// primer antecesor /Parent que la define (Resources, MediaBox, CropBox y
// Rotate son heredables). Una cadena /Parent circular o demasiado larga
// devuelve un valor nulo.
func (p Page) Heredado(clave string) Value {
	return p.findInherited(clave)
}

// ValidarArbolPaginas recorre el árbol de páginas completo y devuelve el
// número real de páginas. Devuelve un error envuelto en ErrArbolPaginas si
// algún nodo aparece dos veces (ciclos incluidos), si hay más de
// MaxProfundidadArbolPaginas niveles, si hay más de max páginas o si el
// /Count de algún nodo Pages no coincide con las páginas que contiene. Así
// Page y los recorridos hoja a hoja dan la misma página para cada número.
func (r *Reader) ValidarArbolPaginas(max int) (int, error) {
	raizCat := r.Trailer().Key("Root")
	raiz := raizCat.Key("Pages")
	if raiz.Key("Type").Name() != "Pages" {
		return 0, errorArbol("falta el nodo /Pages raíz")
	}
	vistos := make(map[objptr]bool)
	if p, ok := refIndirecta(crudoClave(raizCat, "Pages")); ok {
		vistos[p] = true
	}
	type marco struct {
		kids        Value
		i           int
		hojasInicio int
		declarado   int64
	}
	pila := []marco{{kids: raiz.Key("Kids"), declarado: raiz.Key("Count").Int64()}}
	hojas := 0
	for len(pila) > 0 {
		m := &pila[len(pila)-1]
		if m.i >= m.kids.Len() {
			if int64(hojas-m.hojasInicio) != m.declarado {
				return 0, errorArbol("/Count %d no coincide con las %d páginas del nodo", m.declarado, hojas-m.hojasInicio)
			}
			pila = pila[:len(pila)-1]
			continue
		}
		i := m.i
		m.i++
		kid := m.kids.Index(i)
		if p, ok := refIndirecta(crudoIndice(m.kids, i)); ok {
			if vistos[p] {
				return 0, errorArbol("el objeto %d aparece más de una vez", p.id)
			}
			vistos[p] = true
		}
		switch kid.Key("Type").Name() {
		case "Pages":
			if len(pila) >= MaxProfundidadArbolPaginas {
				return 0, errorArbol("más de %d niveles", MaxProfundidadArbolPaginas)
			}
			pila = append(pila, marco{kids: kid.Key("Kids"), hojasInicio: hojas, declarado: kid.Key("Count").Int64()})
		case "Page":
			hojas++
			if hojas > max {
				return 0, errorArbol("más de %d páginas", max)
			}
		}
	}
	return hojas, nil
}
