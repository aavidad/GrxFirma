// Copyright 2026 Alberto Avidad Fernández. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

// Pruebas de regresión de E2-03: un PDF malicioso no debe colgar el lector
// (bucle sin fin), agotar la pila (fatal en Go, sin recover posible) ni
// reservar memoria sin límite. Cada caso se ejecuta con un plazo: si no
// termina, la prueba falla en lugar de quedarse colgada.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"
)

const plazoPruebaLector = 3 * time.Second

// objetoPrueba es un objeto del PDF sintético. Si flujo > 0, el objeto está
// comprimido en ese flujo de objetos (entrada de xref de tipo 2) y texto no
// se escribe; si no, texto es el cuerpo entre "N 0 obj" y "endobj".
type objetoPrueba struct {
	num    int
	texto  string
	flujo  int
	indice int
}

// construirPDFPrueba escribe un PDF con una xref en flujo (W [1 4 2]) que
// recoge todos los objetos con desplazamientos reales. extraXRef se añade al
// diccionario del flujo de xref y transformar (si no es nil) recodifica sus
// datos, por ejemplo para comprimirlos.
func construirPDFPrueba(objetos []objetoPrueba, raiz int, extraXRef string, transformar func([]byte) []byte) []byte {
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n%" + strings.Repeat("x", 230) + "\n")
	desplazamientos := map[int]int{}
	maximo := 0
	sort.Slice(objetos, func(i, j int) bool { return objetos[i].num < objetos[j].num })
	for _, o := range objetos {
		if o.num > maximo {
			maximo = o.num
		}
		if o.flujo > 0 {
			continue
		}
		desplazamientos[o.num] = file.Len()
		fmt.Fprintf(&file, "%d 0 obj\n%s\nendobj\n", o.num, o.texto)
	}
	numXRef := maximo + 1
	tamano := numXRef + 1
	desplazamientos[numXRef] = file.Len()
	datos := make([]byte, tamano*7)
	binary.BigEndian.PutUint16(datos[5:7], 65535)
	for _, o := range objetos {
		pos := o.num * 7
		if o.flujo > 0 {
			datos[pos] = 2
			binary.BigEndian.PutUint32(datos[pos+1:pos+5], uint32(o.flujo))  // #nosec G115 -- prueba.
			binary.BigEndian.PutUint16(datos[pos+5:pos+7], uint16(o.indice)) // #nosec G115 -- prueba.
			continue
		}
		datos[pos] = 1
		binary.BigEndian.PutUint32(datos[pos+1:pos+5], uint32(desplazamientos[o.num])) // #nosec G115 -- prueba.
	}
	pos := numXRef * 7
	datos[pos] = 1
	binary.BigEndian.PutUint32(datos[pos+1:pos+5], uint32(desplazamientos[numXRef])) // #nosec G115 -- prueba.
	if transformar != nil {
		datos = transformar(datos)
	}
	fmt.Fprintf(&file, "%d 0 obj\n<< /Type /XRef /Size %d /Root %d 0 R /W [1 4 2] /Length %d %s >>\nstream\n", numXRef, tamano, raiz, len(datos), extraXRef)
	file.Write(datos)
	file.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&file, "startxref\n%d\n%%%%EOF\n", desplazamientos[numXRef])
	return file.Bytes()
}

// flujoObjetos devuelve el cuerpo de un flujo de objetos (/ObjStm) con los
// objetos dados, en orden, y las claves extra del diccionario.
func flujoObjetos(extra string, comprimir bool, objetos ...[2]string) string {
	var cabecera, cuerpo strings.Builder
	for _, o := range objetos {
		fmt.Fprintf(&cabecera, "%s %d ", o[0], cuerpo.Len())
		cuerpo.WriteString(o[1])
		cuerpo.WriteString("\n")
	}
	if cabecera.Len() == 0 {
		cabecera.WriteString(" ")
	}
	datos := []byte(cabecera.String() + cuerpo.String())
	filtro := ""
	if comprimir {
		datos = comprimirZlib(datos)
		filtro = "/Filter /FlateDecode"
	}
	return fmt.Sprintf("<< /Type /ObjStm /N %d /First %d /Length %d %s %s >>\nstream\n%s\nendstream",
		len(objetos), cabecera.Len(), len(datos), filtro, extra, datos)
}

func comprimirZlib(datos []byte) []byte {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, _ = w.Write(datos)
	_ = w.Close()
	return b.Bytes()
}

// recorrerPDF hace lo que hacen los llamadores reales: abrir, contar páginas,
// pedir la primera y sus recursos heredados. Los pánicos del lector se
// convierten en error, como en abrirPDF y pdfsign.Sign.
func recorrerPDF(data []byte) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("pánico recuperado: %v", p)
		}
	}()
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	_ = r.NumPage()
	_ = r.Page(1).Resources()
	return nil
}

// conPlazo ejecuta f en otra gorrutina y falla la prueba si no termina a
// tiempo. Si f se queda en un bucle, la gorrutina sigue viva hasta que acaba
// el binario de pruebas, pero la prueba ya consta como fallida.
func conPlazo(t *testing.T, f func() error) error {
	t.Helper()
	hecho := make(chan error, 1)
	go func() { hecho <- f() }()
	select {
	case err := <-hecho:
		return err
	case <-time.After(plazoPruebaLector):
		t.Fatalf("el lector no terminó en %v: bucle o recursión sin límite", plazoPruebaLector)
		return nil
	}
}

// pilaReducida limita la pila de las gorrutinas durante la prueba para que
// una recursión sin límite falle pronto y sin consumir 1 GB de memoria.
func pilaReducida(t *testing.T) {
	t.Helper()
	anterior := debug.SetMaxStack(64 << 20)
	t.Cleanup(func() { debug.SetMaxStack(anterior) })
}

const catalogoPrueba = "<< /Type /Catalog /Pages 2 0 R >>"
const paginasPrueba = "<< /Type /Pages /Kids [] /Count 0 >>"

func exigirError(t *testing.T, data []byte) {
	t.Helper()
	if err := conPlazo(t, func() error { return recorrerPDF(data) }); err == nil {
		t.Fatal("se aceptó un PDF malicioso sin error")
	}
}

func TestLimites_ObjetoComprimidoEnSuPropioFlujo(t *testing.T) {
	pilaReducida(t)
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, flujo: 1},
		{num: 2, texto: paginasPrueba},
	}, 1, "", nil)
	exigirError(t, data)
}

func TestLimites_FlujosDeObjetosQueSeContienenMutuamente(t *testing.T) {
	pilaReducida(t)
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, flujo: 3},
		{num: 2, texto: paginasPrueba},
		{num: 3, flujo: 4},
		{num: 4, flujo: 3},
	}, 1, "", nil)
	exigirError(t, data)
}

func TestLimites_LongitudDelFlujoDentroDeEseMismoFlujo(t *testing.T) {
	pilaReducida(t)
	// /Length del flujo 3 es el objeto 4, que está comprimido en el flujo 3.
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, flujo: 3, indice: 0},
		{num: 2, texto: paginasPrueba},
		{num: 3, texto: strings.Replace(flujoObjetos("", false, [2]string{"1", catalogoPrueba}, [2]string{"4", "10"}), "/Length ", "/Length 4 0 R /Longitud ", 1)},
		{num: 4, flujo: 3, indice: 1},
	}, 1, "", nil)
	exigirError(t, data)
}

func TestLimites_ExtendsEnCiclo(t *testing.T) {
	for _, caso := range []struct {
		nombre  string
		objetos []objetoPrueba
	}{
		{"a sí mismo", []objetoPrueba{
			{num: 1, flujo: 3},
			{num: 2, texto: paginasPrueba},
			{num: 3, texto: flujoObjetos("/Extends 3 0 R", false)},
		}},
		{"entre dos flujos", []objetoPrueba{
			{num: 1, flujo: 3},
			{num: 2, texto: paginasPrueba},
			{num: 3, texto: flujoObjetos("/Extends 4 0 R", false)},
			{num: 4, texto: flujoObjetos("/Extends 3 0 R", false)},
		}},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			exigirError(t, construirPDFPrueba(caso.objetos, 1, "", nil))
		})
	}
}

func TestLimites_NEnormeEnFlujoDeObjetos(t *testing.T) {
	cuerpo := strings.Replace(flujoObjetos("", false), "/N 0", "/N 9223372036854775807", 1)
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, flujo: 3},
		{num: 2, texto: paginasPrueba},
		{num: 3, texto: cuerpo},
	}, 1, "", nil)
	exigirError(t, data)
}

func TestLimites_DatosSinCerrarDentroDeFlujoDeObjetos(t *testing.T) {
	for _, caso := range []struct{ nombre, objeto string }{
		{"array", "<< /Type /Catalog /Pages 2 0 R /X [1 2 3"},
		{"cadena literal", "<< /Type /Catalog /Pages 2 0 R /X (abc"},
		{"cadena hexadecimal", "<< /Type /Catalog /Pages 2 0 R /X <0102"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			data := construirPDFPrueba([]objetoPrueba{
				{num: 1, flujo: 3},
				{num: 2, texto: paginasPrueba},
				{num: 3, texto: flujoObjetos("", false, [2]string{"1", caso.objeto})},
			}, 1, "", nil)
			exigirError(t, data)
		})
	}
}

func TestLimites_FlujoDeObjetosDescomprimidoDemasiadoGrande(t *testing.T) {
	relleno := strings.Repeat("a", maxBytesFlujoObjetosPrueba+1)
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, flujo: 3},
		{num: 2, texto: paginasPrueba},
		{num: 3, texto: flujoObjetos("", true, [2]string{"1", "<< /Type /Catalog /Pages 2 0 R /X (" + relleno + ") >>"})},
	}, 1, "", nil)
	exigirError(t, data)
}

// maxBytesFlujoObjetosPrueba repite el límite del lector para que la prueba
// no dependa de una constante que todavía no existe antes del arreglo.
const maxBytesFlujoObjetosPrueba = 32 << 20

func TestLimites_ColumnsEnorme(t *testing.T) {
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, texto: catalogoPrueba},
		{num: 2, texto: paginasPrueba},
	}, 1, "/Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 1099511627776 >>", comprimirZlib)
	exigirError(t, data)
}

func TestLimites_AnidamientoProfundo(t *testing.T) {
	pilaReducida(t)
	profundo := strings.Repeat("[", 1_000_000) + strings.Repeat("]", 1_000_000)
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, texto: "<< /Type /Catalog /Pages 2 0 R /X " + profundo + " >>"},
		{num: 2, texto: paginasPrueba},
	}, 1, "", nil)
	exigirError(t, data)
}

func TestLimites_KidsQueSeContieneASiMismo(t *testing.T) {
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, texto: catalogoPrueba},
		{num: 2, texto: "<< /Type /Pages /Kids [2 0 R] /Count 1 >>"},
	}, 1, "", nil)
	err := conPlazo(t, func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("pánico recuperado: %v", p)
			}
		}()
		r, err := NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		if !r.Page(1).V.IsNull() {
			return fmt.Errorf("se devolvió una página de un árbol en ciclo")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLimites_ParentEnCicloAlHeredar(t *testing.T) {
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, texto: catalogoPrueba},
		{num: 2, texto: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{num: 3, texto: "<< /Type /Page /Parent 3 0 R >>"},
	}, 1, "", nil)
	err := conPlazo(t, func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("pánico recuperado: %v", p)
			}
		}()
		r, err := NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		if !r.Page(1).Resources().IsNull() {
			return fmt.Errorf("recursos inventados en una cadena /Parent en ciclo")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLimites_CountMayorQueLosObjetos(t *testing.T) {
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, texto: catalogoPrueba},
		{num: 2, texto: "<< /Type /Pages /Kids [] /Count 2147483647 >>"},
	}, 1, "", nil)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if n := r.NumPage(); n < 0 || n > len(r.xref) {
		t.Fatalf("NumPage devolvió %d páginas con %d objetos en el documento", n, len(r.xref))
	}
}

func TestLimites_CountNegativo(t *testing.T) {
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, texto: catalogoPrueba},
		{num: 2, texto: "<< /Type /Pages /Kids [] /Count -5 >>"},
	}, 1, "", nil)
	r, err := NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if n := r.NumPage(); n != 0 {
		t.Fatalf("NumPage devolvió %d con /Count negativo", n)
	}
}

// /Prev en ciclo ya estaba protegido en el fork (readXrefChain); la prueba
// deja fijado ese comportamiento.
func TestLimites_PrevEnCiclo(t *testing.T) {
	var file bytes.Buffer
	file.WriteString("%PDF-1.4\n%" + strings.Repeat("x", 230) + "\n")
	obj1 := file.Len()
	file.WriteString("1 0 obj\n" + catalogoPrueba + "\nendobj\n")
	obj2 := file.Len()
	file.WriteString("2 0 obj\n" + paginasPrueba + "\nendobj\n")
	xref := file.Len()
	fmt.Fprintf(&file, "xref\n0 3\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n", obj1, obj2)
	fmt.Fprintf(&file, "trailer\n<< /Size 3 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", xref, xref)
	exigirError(t, file.Bytes())
}

// Un PDF correcto con flujos de objetos y /Extends sigue leyéndose igual.
func TestLimites_PDFCorrectoConFlujosDeObjetos(t *testing.T) {
	data := construirPDFPrueba([]objetoPrueba{
		{num: 1, flujo: 3, indice: 0},
		// La xref sitúa el objeto 2 en el flujo 3, pero está en el 4, al que
		// se llega por /Extends.
		{num: 2, flujo: 3, indice: 1},
		{num: 3, texto: flujoObjetos("/Extends 4 0 R", true, [2]string{"1", catalogoPrueba})},
		{num: 4, texto: flujoObjetos("", false, [2]string{"2", "<< /Type /Pages /Kids [5 0 R] /Count 1 /Resources << /Font << >> >> >>"})},
		{num: 5, texto: "<< /Type /Page /Parent 2 0 R >>"},
	}, 1, "", nil)
	err := conPlazo(t, func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("pánico recuperado: %v", p)
			}
		}()
		r, err := NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		if n := r.NumPage(); n != 1 {
			return fmt.Errorf("NumPage = %d", n)
		}
		p := r.Page(1)
		if p.V.IsNull() || p.Resources().IsNull() {
			return fmt.Errorf("página o recursos heredados perdidos")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
