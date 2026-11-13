// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"encoding/xml"
	"strings"
)

// Perfil del expediente electrónico ENI (NTI de Expediente Electrónico,
// BOE-A-2011-13170, anexo II). La firma del índice cubre IndiceContenido; los
// metadatos del expediente (metadatosExp) y la información de la firma
// (atributo ref de enids:firma y enids:TipoFirma) quedan fuera de ella por
// norma. Esta regla solo los deja fuera cuando el documento tiene exactamente
// la estructura del expediente y la firma está donde la norma la pone, cubre
// exactamente IndiceContenido y nada más del documento. Cualquier otra cosa
// sigue el criterio general (cobertura parcial).

const (
	nsENIExpediente = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/expediente-e"
	nsENIIndice     = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/expediente-e/indice-e"
	nsENIMetadatos  = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/expediente-e/metadatos"
	nsENIFirma      = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/firma"
)

// exclusionesPerfil son los nodos que un perfil normativo deja fuera de la
// firma: subárboles enteros y atributos de elementos concretos.
type exclusionesPerfil struct {
	nodos     map[int]bool
	atributos map[int]bool
}

func (e *exclusionesPerfil) nodo(indice int) bool {
	return e != nil && e.nodos[indice]
}

func (e *exclusionesPerfil) atributosDe(indice int) bool {
	return e != nil && e.atributos[indice]
}

// exclusionesIndiceENI devuelve las exclusiones del perfil ENI para la firma
// en el nodo firma con lo que cubre, o nil si no es la firma del índice de un
// expediente ENI.
func exclusionesIndiceENI(documento []byte, arbol *arbolXML, firma int, cubiertos map[int]bool) *exclusionesPerfil {
	es := func(indice int, ns, local string) bool {
		return arbol.nodos[indice].nombre == xml.Name{Space: ns, Local: local}
	}
	hijosExactos := func(indice int, nombres ...xml.Name) []int {
		hijos := arbol.nodos[indice].hijos
		if len(hijos) != len(nombres) {
			return nil
		}
		for i, hijo := range hijos {
			if arbol.nodos[hijo].nombre != nombres[i] {
				return nil
			}
		}
		return hijos
	}
	// Envoltorio sin texto ni más atributos que Id y xmlns.
	envoltorio := func(indice int) bool {
		return !arbol.nodos[indice].texto && !atributosConContenido(arbol.nodos[indice])
	}

	if len(arbol.raices) != 1 {
		return nil
	}
	raiz := arbol.raices[0]
	if !es(raiz, nsENIExpediente, "expediente") || !envoltorio(raiz) {
		return nil
	}
	nivel1 := hijosExactos(raiz, xml.Name{Space: nsENIIndice, Local: "indice"}, xml.Name{Space: nsENIMetadatos, Local: "metadatosExp"})
	if nivel1 == nil {
		return nil
	}
	indice, metadatos := nivel1[0], nivel1[1]
	if !envoltorio(indice) || !soloEspacioDeNombres(arbol, metadatos, nsENIMetadatos) {
		return nil
	}
	nivel2 := hijosExactos(indice, xml.Name{Space: nsENIIndice, Local: "IndiceContenido"}, xml.Name{Space: nsENIFirma, Local: "firmas"})
	if nivel2 == nil {
		return nil
	}
	contenido, firmas := nivel2[0], nivel2[1]
	idContenido := valorAtributo(arbol.nodos[contenido], "Id")
	if idContenido == "" || arbol.nodoPorID(idContenido) != contenido || !envoltorio(firmas) || len(arbol.nodos[firmas].hijos) == 0 {
		return nil
	}
	// La firma cubre IndiceContenido y, aparte, solo lo suyo (SignedProperties).
	if !cubiertos[contenido] {
		return nil
	}
	for cubierto := range cubiertos {
		if cubierto != contenido && !arbol.desciende(cubierto, firma) {
			return nil
		}
	}
	excl := &exclusionesPerfil{nodos: map[int]bool{metadatos: true}, atributos: map[int]bool{}}
	firmaEnSuSitio := false
	for _, nodoFirma := range arbol.nodos[firmas].hijos {
		if !es(nodoFirma, nsENIFirma, "firma") || arbol.nodos[nodoFirma].texto || !atributosFirmaENI(arbol.nodos[nodoFirma], idContenido) {
			return nil
		}
		partes := hijosExactos(nodoFirma, xml.Name{Space: nsENIFirma, Local: "TipoFirma"}, xml.Name{Space: nsENIFirma, Local: "ContenidoFirma"})
		if partes == nil {
			return nil
		}
		tipo, contenidoFirma := partes[0], partes[1]
		if !tipoFirmaXAdESENI(documento, arbol.nodos[tipo]) || !envoltorio(contenidoFirma) {
			return nil
		}
		conCertificado := hijosExactos(contenidoFirma, xml.Name{Space: nsENIFirma, Local: "FirmaConCertificado"})
		if conCertificado == nil || !envoltorio(conCertificado[0]) {
			return nil
		}
		firmaXML := hijosExactos(conCertificado[0], xml.Name{Space: nsXMLDSig, Local: "Signature"})
		if firmaXML == nil {
			return nil
		}
		firmaEnSuSitio = firmaEnSuSitio || firmaXML[0] == firma
		excl.atributos[nodoFirma] = true
		excl.nodos[tipo] = true
	}
	if !firmaEnSuSitio {
		return nil
	}
	return excl
}

// atributosFirmaENI admite en enids:firma solo Id y ref, y ref tiene que
// apuntar al IndiceContenido firmado.
func atributosFirmaENI(nodo nodoXML, idContenido string) bool {
	ref := false
	for _, attr := range nodo.attrs {
		switch {
		case attr.Name.Space == "xmlns", attr.Name.Space == "" && attr.Name.Local == "xmlns":
		case attr.Name.Space == "" && attr.Name.Local == "Id":
		case attr.Name.Space == "" && attr.Name.Local == "ref" && attr.Value == idContenido && !ref:
			ref = true
		default:
			return false
		}
	}
	return true
}

// tipoFirmaXAdESENI: TipoFirma sin atributos ni hijos y con valor TF02 o
// TF03, los dos tipos XAdES de la NTI de documento electrónico.
func tipoFirmaXAdESENI(documento []byte, nodo nodoXML) bool {
	if len(nodo.hijos) != 0 || len(nodo.attrs) != 0 || nodo.inicio < 0 || nodo.fin > int64(len(documento)) {
		return false
	}
	var valor struct {
		Texto string `xml:",chardata"`
	}
	if xml.Unmarshal(documento[nodo.inicio:nodo.fin], &valor) != nil {
		return false
	}
	switch strings.TrimSpace(valor.Texto) {
	case "TF02", "TF03":
		return true
	}
	return false
}

// soloEspacioDeNombres exige que el elemento y todos sus descendientes sean
// del espacio de nombres indicado (no esconde firmas ni datos de otro tipo).
func soloEspacioDeNombres(arbol *arbolXML, indice int, ns string) bool {
	if arbol.nodos[indice].nombre.Space != ns {
		return false
	}
	for _, hijo := range arbol.nodos[indice].hijos {
		if !soloEspacioDeNombres(arbol, hijo, ns) {
			return false
		}
	}
	return true
}

func valorAtributo(nodo nodoXML, local string) string {
	for _, attr := range nodo.attrs {
		if attr.Name.Space == "" && attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}
