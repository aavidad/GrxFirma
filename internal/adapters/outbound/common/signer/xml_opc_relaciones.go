// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
)

// Transformación de relaciones de OPC (ECMA-376 parte 2, §13.2.4.24). Office
// y AutoFirma firman las partes .rels con ella: el resumen cubre solo las
// relaciones elegidas por Id (RelationshipReference/@SourceId) o por tipo
// (RelationshipsGroupReference/@SourceType), ordenadas por Id, sin contenido
// ni espacios entre ellas y con TargetMode="Internal" cuando falta. Después
// se canoniza con la C14N que indique la referencia.

const nsRelacionesOPC = "http://schemas.openxmlformats.org/package/2006/relationships"

// relacionOPC es una relación de una parte .rels.
type relacionOPC struct {
	id, tipo, destino, modo string
	attrs                   []xml.Attr
}

// relacionNoFirmada es una relación de una parte .rels firmada con la
// transformación de relaciones que la firma no elige y por tanto no cubre.
type relacionNoFirmada struct {
	parte    string
	relacion relacionOPC
}

func (r relacionNoFirmada) String() string {
	return r.parte + "#" + r.relacion.id
}

// seleccionRelaciones son los parámetros de la transformación.
type seleccionRelaciones struct {
	ids   map[string]bool
	tipos map[string]bool
}

// leerSeleccionRelaciones interpreta los hijos del ds:Transform. Se comparan
// por nombre local: Office declara el prefijo mdssi en cada hijo y AutoFirma
// en el propio ds:Transform, que no forma parte del innerxml. Cualquier otro
// hijo es una transformación que no se sabe aplicar.
func leerSeleccionRelaciones(parametros string) (seleccionRelaciones, bool) {
	sel := seleccionRelaciones{ids: map[string]bool{}, tipos: map[string]bool{}}
	decoder := xml.NewDecoder(strings.NewReader(parametros))
	profundidad := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return sel, true
		}
		if err != nil {
			return sel, false
		}
		switch nodo := token.(type) {
		case xml.StartElement:
			profundidad++
			if profundidad > 1 {
				return sel, false
			}
			var atributo string
			destino := sel.ids
			switch nodo.Name.Local {
			case "RelationshipReference":
				atributo = "SourceId"
			case "RelationshipsGroupReference":
				atributo, destino = "SourceType", sel.tipos
			default:
				return sel, false
			}
			valor, encontrado := "", false
			for _, attr := range nodo.Attr {
				switch {
				case attr.Name.Space == "xmlns", attr.Name.Space == "" && attr.Name.Local == "xmlns":
				case attr.Name.Space == "" && attr.Name.Local == atributo && !encontrado:
					valor, encontrado = attr.Value, true
				default:
					return sel, false
				}
			}
			if !encontrado || valor == "" {
				return sel, false
			}
			destino[valor] = true
		case xml.EndElement:
			profundidad--
		case xml.CharData:
			if len(bytes.TrimSpace(nodo)) > 0 {
				return sel, false
			}
		case xml.Comment:
		default:
			return sel, false
		}
	}
}

// leerRelacionesOPC lee una parte .rels. Un Id repetido hace ambigua la
// selección, así que es un error.
func leerRelacionesOPC(datos []byte) ([]relacionOPC, error) {
	decoder := xml.NewDecoder(bytes.NewReader(datos))
	var relaciones []relacionOPC
	ids := map[string]bool{}
	profundidad := 0
	raiz := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parte de relaciones ilegible: %w", err)
		}
		switch nodo := token.(type) {
		case xml.StartElement:
			profundidad++
			switch {
			case profundidad == 1:
				if raiz || nodo.Name != (xml.Name{Space: nsRelacionesOPC, Local: "Relationships"}) {
					return nil, errors.New("la parte no es de relaciones OPC")
				}
				raiz = true
			case profundidad == 2 && nodo.Name == (xml.Name{Space: nsRelacionesOPC, Local: "Relationship"}):
				relacion := relacionOPC{}
				for _, attr := range nodo.Attr {
					switch {
					case attr.Name.Space == "xmlns", attr.Name.Space == "" && attr.Name.Local == "xmlns":
						continue
					case attr.Name.Space != "":
						// Fuera del esquema de OPC: no se sabe cómo lo
						// serializa quien firma.
						return nil, fmt.Errorf("relación con un atributo no admitido: %s", attr.Name.Local)
					}
					switch attr.Name.Local {
					case "Id":
						relacion.id = attr.Value
					case "Type":
						relacion.tipo = attr.Value
					case "Target":
						relacion.destino = attr.Value
					case "TargetMode":
						relacion.modo = attr.Value
					}
					relacion.attrs = append(relacion.attrs, xml.Attr{Name: xml.Name{Local: attr.Name.Local}, Value: attr.Value})
				}
				if relacion.id == "" || ids[relacion.id] {
					return nil, fmt.Errorf("relación sin Id o con Id repetido: %q", relacion.id)
				}
				ids[relacion.id] = true
				relaciones = append(relaciones, relacion)
			default:
				// La transformación descarta cualquier otro elemento, así
				// que la firma no lo cubriría: se rechaza la parte.
				return nil, fmt.Errorf("parte de relaciones con un elemento no admitido: %s", nodo.Name.Local)
			}
		case xml.EndElement:
			profundidad--
		}
	}
	if !raiz {
		return nil, errors.New("la parte no es de relaciones OPC")
	}
	return relaciones, nil
}

// transformarRelaciones aplica la transformación y la C14N indicada. Devuelve
// los octetos sobre los que se calcula el resumen y las relaciones que la
// transformación deja fuera.
func transformarRelaciones(datos []byte, sel seleccionRelaciones, c14n string) ([]byte, []relacionOPC, error) {
	relaciones, err := leerRelacionesOPC(datos)
	if err != nil {
		return nil, nil, err
	}
	var elegidas, fuera []relacionOPC
	for _, relacion := range relaciones {
		if sel.ids[relacion.id] || sel.tipos[relacion.tipo] {
			elegidas = append(elegidas, relacion)
		} else {
			fuera = append(fuera, relacion)
		}
	}
	sort.Slice(elegidas, func(i, j int) bool { return elegidas[i].id < elegidas[j].id })
	var salida bytes.Buffer
	salida.WriteString(`<Relationships xmlns="` + nsRelacionesOPC + `">`)
	for _, relacion := range elegidas {
		salida.WriteString("<Relationship")
		attrs := relacion.attrs
		if relacion.modo == "" {
			attrs = append(append([]xml.Attr(nil), attrs...), xml.Attr{Name: xml.Name{Local: "TargetMode"}, Value: "Internal"})
		}
		for _, attr := range attrs {
			salida.WriteString(" " + attr.Name.Local + `="` + escaparAtributoXML(attr.Value) + `"`)
		}
		salida.WriteString("></Relationship>")
	}
	salida.WriteString("</Relationships>")
	canonico, err := canonicalizeXML(salida.Bytes(), c14n)
	if err != nil {
		return nil, nil, err
	}
	return canonico, fuera, nil
}

func escaparAtributoXML(valor string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		`"`, "&quot;",
		"\t", "&#x9;",
		"\n", "&#xA;",
		"\r", "&#xD;",
	).Replace(valor)
}

// verificarReferenciaRelaciones comprueba una referencia de ds:Manifest a una
// parte .rels con la transformación de relaciones. Devuelve si quedó
// comprobada, el motivo si no, y las relaciones que la firma no cubre.
func verificarReferenciaRelaciones(ref referenceForVerify, resolver func(string) ([]byte, error)) (bool, string, []relacionNoFirmada, error) {
	const motivo = "manifest_relationship_transform"
	if resolver == nil {
		return false, motivo, nil, nil
	}
	// La parte se resuelve siempre: si falta del paquete, es una alteración.
	uri := strings.TrimSpace(ref.URI)
	parte, err := nombreParteOPC(uri)
	if err != nil {
		return false, "", nil, err
	}
	datos, err := resolver(uri)
	if err != nil {
		return false, "", nil, err
	}
	if len(ref.Transforms) > 2 || !strings.HasSuffix(strings.ToLower(parte), ".rels") {
		return false, motivo, nil, nil
	}
	sel, ok := leerSeleccionRelaciones(ref.Transforms[0].Parameters)
	if !ok {
		return false, motivo, nil, nil
	}
	c14n := algC14N
	if len(ref.Transforms) == 2 {
		c14n = strings.TrimSpace(ref.Transforms[1].Algorithm)
		if (c14n != algC14N && c14n != algExcC14N) || strings.TrimSpace(ref.Transforms[1].Parameters) != "" {
			return false, motivo, nil, nil
		}
	}
	transformado, fuera, err := transformarRelaciones(datos, sel, c14n)
	if err != nil {
		return false, "", nil, err
	}
	expected, err := decodificarResumen(ref.DigestValue)
	if err != nil {
		return false, "", nil, err
	}
	declared, err := digestXML(ref.DigestMethod.Algorithm, transformado)
	if err != nil {
		return false, "manifest_procedimiento_no_soportado", nil, nil
	}
	if !equalBytes(declared, expected) {
		return false, "", nil, errors.New("el resumen no coincide")
	}
	var noFirmadas []relacionNoFirmada
	for _, relacion := range fuera {
		noFirmadas = append(noFirmadas, relacionNoFirmada{parte: parte, relacion: relacion})
	}
	return true, "", noFirmadas, nil
}

// nombreParteOPC devuelve el nombre de parte de una URI de referencia (sin
// la consulta ?ContentType= ni la barra inicial), como lo resuelve el
// contenedor.
func nombreParteOPC(uri string) (string, error) {
	raw := uri
	if q := strings.IndexByte(raw, '?'); q >= 0 {
		raw = raw[:q]
	}
	nombre, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("URI de referencia no válida: %s", uri)
	}
	return strings.TrimPrefix(nombre, "/"), nil
}

// destinoRelacionPaquete resuelve el destino interno de una relación de
// _rels/.rels (las del paquete), relativo a la raíz.
func destinoRelacionPaquete(destino string) string {
	limpio := path.Clean("/" + strings.TrimPrefix(destino, "/"))
	return strings.TrimPrefix(limpio, "/")
}
