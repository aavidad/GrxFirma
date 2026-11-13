// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"grxfirma/internal/domain"
)

// Cobertura de las firmas XML: qué parte del documento queda de verdad bajo
// cada firma. Que los resúmenes cuadren solo prueba que lo referenciado no ha
// cambiado; si la firma referencia un nodo, un CONTENT o unas entradas del
// contenedor, el resto del documento puede ser de otra persona. Una firma solo
// es válida si cubre todo el contenido; si no, la cobertura es parcial, y si no
// se ha podido comprobar algo que referencia (Manifest con referencias externas
// o con la transformación de relaciones de OPC), queda sin acreditar.

const (
	algRelationshipTransform = "http://schemas.openxmlformats.org/package/2006/RelationshipTransform"

	// Textos visibles (claves de los catálogos de idiomas).
	motivoCoberturaParcial       = "La firma es correcta, pero parte del contenido del documento no está firmado."
	motivoCoberturaNoComprobada  = "No se ha podido comprobar todo lo que cubre la firma, así que no se da por válida."
	motivoFueraDeFirmaPorFormato = "La firma es válida, aunque no cubre algunos metadatos del documento porque su formato los deja fuera. En el detalle se indica cuáles."

	maxElementosSinCubrirMostrados = 8
)

// coberturaFirmaXML resume lo que referencia una firma ya verificada.
type coberturaFirmaXML struct {
	// inicio identifica el ds:Signature en el documento de la firma.
	inicio int64
	// uris son las referencias comprobadas (SignedInfo y Manifest validados).
	uris []string
	// noComprobado recoge referencias de un Manifest que no se han podido
	// resolver o comparar.
	noComprobado []string
	// relacionesNoFirmadas son las relaciones de las partes .rels firmadas
	// con la transformación de relaciones de OPC que la firma no elige.
	relacionesNoFirmadas []relacionNoFirmada
	// partesRelacionesOPC son las partes .rels comprobadas con esa
	// transformación. Solo tiene sentido en un paquete OOXML: en cualquier
	// otro contenedor se tratan como no comprobadas.
	partesRelacionesOPC []string
}

// noComprobadoFueraDeOPC es noComprobado para los contenedores que no son
// OOXML, donde la transformación de relaciones no acredita nada.
func (c coberturaFirmaXML) noComprobadoFueraDeOPC() []string {
	if len(c.partesRelacionesOPC) == 0 {
		return c.noComprobado
	}
	return append(append([]string(nil), c.noComprobado...), "manifest_relationship_transform")
}

// resultadoCobertura es la cobertura de todo el documento.
type resultadoCobertura struct {
	sinCubrir    []string
	noComprobado []string
	// fueraPorFormato es lo que el formato deja fuera de la firma por diseño
	// (metadatos de Office, metadatos del expediente ENI). No impide que la
	// firma sea válida, pero se dice qué es.
	fueraPorFormato []string
}

func (c resultadoCobertura) estado() string {
	switch {
	case len(c.sinCubrir) > 0:
		return domain.CoberturaParcial
	case len(c.noComprobado) > 0:
		return domain.CoberturaNoAcreditada
	default:
		return domain.CoberturaCompleta
	}
}

// aplicarCoberturaXML fija en el resultado la cobertura calculada.
func aplicarCoberturaXML(result domain.VerificationResult, cobertura resultadoCobertura) domain.VerificationResult {
	var detalles []string
	if len(cobertura.sinCubrir) > 0 {
		detalles = append(detalles, "contenido_sin_firmar="+strings.Join(limitarLista(cobertura.sinCubrir), ","))
	}
	if len(cobertura.noComprobado) > 0 {
		detalles = append(detalles, "cobertura_no_comprobada="+strings.Join(limitarLista(cobertura.noComprobado), ","))
	}
	if len(cobertura.fueraPorFormato) > 0 {
		detalles = append(detalles, "fuera_de_firma_por_formato="+strings.Join(limitarLista(cobertura.fueraPorFormato), ","))
	}
	switch cobertura.estado() {
	case domain.CoberturaCompleta:
		if result.Coverage == domain.CoberturaNoAcreditada {
			// Compatibilidad histórica: ya no es válida y no se acredita nada.
			result.Details = append(result.Details, detalles...)
			result.Integrity.Details = append(result.Integrity.Details, detalles...)
			return result
		}
		result = result.ConCobertura(domain.CoberturaCompleta, "", nil)
		if len(cobertura.fueraPorFormato) > 0 {
			// Válida, pero a la vista: qué metadatos no cubre la firma.
			result.Warnings = appendUniqueString(result.Warnings, motivoFueraDeFirmaPorFormato)
			result.Details = append(result.Details, detalles...)
			result.Integrity.Details = append(result.Integrity.Details, detalles...)
		}
		return result
	case domain.CoberturaParcial:
		result = result.ConCobertura(domain.CoberturaParcial, motivoCoberturaParcial, detalles)
	default:
		result = result.ConCobertura(domain.CoberturaNoAcreditada, motivoCoberturaNoComprobada, detalles)
	}
	result.Details = sinAfirmacionesDeCobertura(result.Details)
	result.Integrity.Details = sinAfirmacionesDeCobertura(result.Integrity.Details)
	return result
}

// Con cobertura incompleta, "referencias_zip=ok" daría a entender que se ha
// comprobado todo el contenedor.
func sinAfirmacionesDeCobertura(details []string) []string {
	out := details[:0:0]
	for _, detail := range details {
		if detail != "referencias_zip=ok" {
			out = append(out, detail)
		}
	}
	return out
}

func limitarLista(valores []string) []string {
	if len(valores) <= maxElementosSinCubrirMostrados {
		return valores
	}
	out := append([]string(nil), valores[:maxElementosSinCubrirMostrados]...)
	return append(out, fmt.Sprintf("(+%d)", len(valores)-maxElementosSinCubrirMostrados))
}

// ---------------------------------------------------------------------------
// Árbol del documento
// ---------------------------------------------------------------------------

type nodoXML struct {
	nombre xml.Name
	attrs  []xml.Attr
	inicio int64
	fin    int64
	padre  int
	hijos  []int
	texto  bool
}

type arbolXML struct {
	nodos  []nodoXML
	raices []int
	porID  map[string][]int
}

func construirArbolXML(documento []byte) (*arbolXML, error) {
	decoder := xml.NewDecoder(bytes.NewReader(documento))
	arbol := &arbolXML{porID: map[string][]int{}}
	var pila []int
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch nodo := token.(type) {
		case xml.StartElement:
			padre := -1
			if len(pila) > 0 {
				padre = pila[len(pila)-1]
			}
			indice := len(arbol.nodos)
			arbol.nodos = append(arbol.nodos, nodoXML{nombre: nodo.Name, attrs: nodo.Copy().Attr, inicio: offset, fin: -1, padre: padre})
			if padre >= 0 {
				arbol.nodos[padre].hijos = append(arbol.nodos[padre].hijos, indice)
			} else {
				arbol.raices = append(arbol.raices, indice)
			}
			for _, attr := range nodo.Attr {
				if strings.EqualFold(attr.Name.Local, "Id") {
					arbol.porID[attr.Value] = append(arbol.porID[attr.Value], indice)
				}
			}
			pila = append(pila, indice)
		case xml.EndElement:
			if len(pila) == 0 {
				return nil, errors.New("cierre XML sin apertura")
			}
			arbol.nodos[pila[len(pila)-1]].fin = decoder.InputOffset()
			pila = pila[:len(pila)-1]
		case xml.CharData:
			if len(pila) > 0 && len(bytes.TrimSpace(nodo)) > 0 {
				arbol.nodos[pila[len(pila)-1]].texto = true
			}
		}
	}
	if len(pila) != 0 || len(arbol.nodos) == 0 {
		return nil, errors.New("documento XML incompleto")
	}
	return arbol, nil
}

func (a *arbolXML) esFirma(indice int) bool {
	return a.nodos[indice].nombre == xml.Name{Space: nsXMLDSig, Local: "Signature"}
}

func (a *arbolXML) esDSig(indice int, local string) bool {
	return a.nodos[indice].nombre == xml.Name{Space: nsXMLDSig, Local: local}
}

func (a *arbolXML) nodoEn(inicio int64) int {
	for indice, nodo := range a.nodos {
		if nodo.inicio == inicio {
			return indice
		}
	}
	return -1
}

func (a *arbolXML) nodoPorID(id string) int {
	if indices := a.porID[id]; len(indices) == 1 {
		return indices[0]
	}
	return -1
}

// contenidoDetached localiza el único CONTENT del documento, el mismo que
// resuelve resolveDetachedContent para una referencia externa sin contenedor.
func (a *arbolXML) contenidoDetached() int {
	encontrado := -1
	for indice, nodo := range a.nodos {
		if nodo.nombre.Local != "CONTENT" {
			continue
		}
		if encontrado >= 0 {
			return -1
		}
		encontrado = indice
	}
	return encontrado
}

func (a *arbolXML) desciende(indice, ancestro int) bool {
	for actual := indice; actual >= 0; actual = a.nodos[actual].padre {
		if actual == ancestro {
			return true
		}
	}
	return false
}

func (a *arbolXML) firmaAncestro(indice int) int {
	for actual := a.nodos[indice].padre; actual >= 0; actual = a.nodos[actual].padre {
		if a.esFirma(actual) {
			return actual
		}
	}
	return -1
}

func (a *arbolXML) contieneLocal(indice int, local string) bool {
	for _, hijo := range a.nodos[indice].hijos {
		if a.nodos[hijo].nombre.Local == local || a.contieneLocal(hijo, local) {
			return true
		}
	}
	return false
}

// atributosConContenido indica si un elemento envoltorio lleva atributos que
// no sean declaraciones de espacio de nombres o su Id. El envoltorio <AFIRMA>
// de AutoFirma (y de GrxFirma) lleva además metadatos de generación (Version,
// GeneratedAt) que ningún visor presenta como contenido del documento.
func atributosConContenido(nodo nodoXML) bool {
	if nodo.nombre.Space == "" && nodo.nombre.Local == "AFIRMA" {
		return false
	}
	for _, attr := range nodo.attrs {
		switch {
		case attr.Name.Space == "xmlns", attr.Name.Space == "" && attr.Name.Local == "xmlns":
		case strings.EqualFold(attr.Name.Local, "Id"):
		default:
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Cobertura de un documento XML independiente (XAdES, XMLdSig, FacturaE)
// ---------------------------------------------------------------------------

// coberturaDocumentoXML exige que cada firma de primer nivel cubra todo el
// contenido del documento. Se toleran como estructura: los propios elementos
// de firma (SignedInfo, SignatureValue, KeyInfo y el ds:Object de las
// propiedades XAdES) y los envoltorios sin texto ni atributos propios que solo
// contienen lo firmado y las firmas (p. ej. <AFIRMA> de AutoFirma).
func coberturaDocumentoXML(documento []byte, firmas []coberturaFirmaXML) (resultadoCobertura, error) {
	arbol, err := construirArbolXML(documento)
	if err != nil {
		return resultadoCobertura{}, err
	}
	var out resultadoCobertura
	vistos := map[string]bool{}
	anotar := func(lista *[]string, valor string) {
		if !vistos[valor] {
			vistos[valor] = true
			*lista = append(*lista, valor)
		}
	}
	type firmaEnArbol struct {
		nodo            int
		cubiertos       map[int]bool
		documentoEntero bool
	}
	var enArbol []firmaEnArbol
	// Objetos de datos propios de cada firma (hijos ds:Object que ella misma
	// cubre). En una cofirma enveloping cada firmante lleva su copia del dato.
	objetosPropios := map[int]bool{}
	for _, firma := range firmas {
		for _, motivo := range firma.noComprobadoFueraDeOPC() {
			anotar(&out.noComprobado, motivo)
		}
		nodoFirma := arbol.nodoEn(firma.inicio)
		if nodoFirma < 0 || !arbol.esFirma(nodoFirma) {
			anotar(&out.noComprobado, "firma_no_localizada")
			continue
		}
		actual := firmaEnArbol{nodo: nodoFirma, cubiertos: map[int]bool{}}
		for _, uri := range firma.uris {
			uri = strings.TrimSpace(uri)
			switch {
			case uri == "":
				actual.documentoEntero = true
			case strings.HasPrefix(uri, "#"):
				if indice := arbol.nodoPorID(strings.TrimPrefix(uri, "#")); indice >= 0 {
					actual.cubiertos[indice] = true
				}
			default:
				if indice := arbol.contenidoDetached(); indice >= 0 {
					actual.cubiertos[indice] = true
				}
			}
		}
		for indice := range actual.cubiertos {
			if arbol.esDSig(indice, "Object") && arbol.nodos[indice].padre == nodoFirma {
				objetosPropios[indice] = true
			}
		}
		enArbol = append(enArbol, actual)
	}
	fuera := map[string]bool{}
	for _, firma := range enArbol {
		if firma.documentoEntero || esContrafirma(arbol, firma.nodo, firma.cubiertos) {
			continue
		}
		excl := exclusionesIndiceENI(documento, arbol, firma.nodo, firma.cubiertos)
		sinCubrir, porPerfil := arbol.contenidoSinCubrir(firma.cubiertos, objetosPropios, excl)
		for _, elemento := range sinCubrir {
			anotar(&out.sinCubrir, elemento)
		}
		for _, elemento := range porPerfil {
			if !fuera[elemento] {
				fuera[elemento] = true
				out.fueraPorFormato = append(out.fueraPorFormato, elemento)
			}
		}
	}
	return out, nil
}

// esContrafirma acepta una firma anidada dentro de otra cuando todo lo que
// referencia fuera de sí misma está dentro de la firma que la contiene (la
// contrafirma XAdES firma el SignatureValue de su madre).
func esContrafirma(arbol *arbolXML, firma int, cubiertos map[int]bool) bool {
	madre := arbol.firmaAncestro(firma)
	if madre < 0 {
		return false
	}
	alguno := false
	for indice := range cubiertos {
		if arbol.desciende(indice, firma) {
			continue
		}
		if !arbol.desciende(indice, madre) {
			return false
		}
		alguno = true
	}
	return alguno
}

// contenidoSinCubrir devuelve lo que la firma no cubre y, aparte, lo que un
// perfil normativo (excl) deja fuera de ella a propósito.
func (a *arbolXML) contenidoSinCubrir(cubiertos, objetosPropios map[int]bool, excl *exclusionesPerfil) ([]string, []string) {
	// Un nodo es estructural si contiene algo cubierto o una firma.
	estructural := make([]bool, len(a.nodos))
	marcar := func(indice int) {
		for actual := indice; actual >= 0 && !estructural[actual]; actual = a.nodos[actual].padre {
			estructural[actual] = true
		}
	}
	for indice := range cubiertos {
		marcar(indice)
	}
	for indice := range a.nodos {
		if a.esFirma(indice) {
			marcar(indice)
		}
	}
	var sinCubrir, porPerfil []string
	var visitar func(int)
	visitar = func(indice int) {
		if cubiertos[indice] {
			return
		}
		nodo := a.nodos[indice]
		if excl.nodo(indice) {
			porPerfil = append(porPerfil, nodo.nombre.Local)
			return
		}
		switch {
		case a.esFirma(indice):
			if nodo.texto {
				sinCubrir = append(sinCubrir, "texto:"+nodo.nombre.Local)
			}
			for _, hijo := range nodo.hijos {
				switch {
				case a.esDSig(hijo, "SignedInfo"), a.esDSig(hijo, "SignatureValue"), a.esDSig(hijo, "KeyInfo"):
				case a.esDSig(hijo, "Object") && a.contieneLocal(hijo, "QualifyingProperties"):
				case objetosPropios[hijo]:
					// Dato de otra firma ya verificada que esa firma cubre.
				default:
					visitar(hijo)
				}
			}
		case estructural[indice]:
			if nodo.texto {
				sinCubrir = append(sinCubrir, "texto:"+nodo.nombre.Local)
			}
			if atributosConContenido(nodo) {
				if excl.atributosDe(indice) {
					porPerfil = append(porPerfil, "atributos:"+nodo.nombre.Local)
				} else {
					sinCubrir = append(sinCubrir, "atributos:"+nodo.nombre.Local)
				}
			}
			for _, hijo := range nodo.hijos {
				visitar(hijo)
			}
		default:
			sinCubrir = append(sinCubrir, nodo.nombre.Local)
		}
	}
	for _, raiz := range a.raices {
		visitar(raiz)
	}
	return sinCubrir, porPerfil
}

// ---------------------------------------------------------------------------
// Cobertura de un contenedor ZIP (ODF, OOXML, ASiC)
// ---------------------------------------------------------------------------

// coberturaContenedor exige que cada firma cubra todas las entradas del
// contenedor salvo las que la norma deja fuera (excluida).
func coberturaContenedor(entradas []string, excluida func(string) bool, cubiertasPorFirma []map[string]bool, noComprobado []string) resultadoCobertura {
	out := resultadoCobertura{noComprobado: dedup(noComprobado)}
	vistos := map[string]bool{}
	for _, cubiertas := range cubiertasPorFirma {
		for _, entrada := range entradas {
			if excluida(entrada) || cubiertas[entrada] || vistos[entrada] {
				continue
			}
			vistos[entrada] = true
			out.sinCubrir = append(out.sinCubrir, entrada)
		}
	}
	sort.Strings(out.sinCubrir)
	return out
}

// Metadatos de un paquete OOXML que Office deja fuera de la firma por diseño
// (ECMA-376 parte 2 deja al firmante elegir las partes; Word y AutoFirma no
// firman las propiedades del documento). Lista cerrada: nombre exacto (OPC no
// distingue mayúsculas) y tipo de contenido exacto.
var metadatosOOXMLFueraDeFirma = map[string]string{
	"docprops/core.xml":            "application/vnd.openxmlformats-package.core-properties+xml",
	"docprops/app.xml":             "application/vnd.openxmlformats-officedocument.extended-properties+xml",
	"docprops/_rels/core.xml.rels": "application/vnd.openxmlformats-package.relationships+xml",
	"docprops/_rels/app.xml.rels":  "application/vnd.openxmlformats-package.relationships+xml",
}

// Entradas que Word deja en [trash]/ al guardar: no son partes OPC (los
// corchetes no se admiten en un nombre de parte) y ningún lector las abre.
var reEntradaPapeleraWord = regexp.MustCompile(`^\[trash\]/[0-9A-Fa-f]{4}\.dat$`)

// Relaciones que Office no elige al firmar _rels/.rels: el tipo y el destino
// tienen que ser exactamente estos.
var relacionesPaqueteFueraDeFirma = map[string]string{
	"http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties":   "docprops/core.xml",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties": "docprops/app.xml",
	"http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/origin":   "_xmlsignatures/origin.sigs",
}

const tipoRelacionFirmaDigital = "http://schemas.openxmlformats.org/package/2006/relationships/digital-signature/signature"

func (c *contenedorZIP) metadatoFueraDeFirma(nombre string) bool {
	if reEntradaPapeleraWord.MatchString(nombre) {
		return true
	}
	esperado, ok := metadatosOOXMLFueraDeFirma[strings.ToLower(nombre)]
	return ok && strings.EqualFold(c.contentTypes.ContentTypeFor(nombre), esperado)
}

// relacionFueraDeFirmaPermitida: relaciones de _rels/.rels hacia los
// metadatos y el origen de firmas, y las de las partes de firma digital.
func relacionFueraDeFirmaPermitida(r relacionNoFirmada) bool {
	if r.relacion.modo != "" && r.relacion.modo != "Internal" {
		return false
	}
	parte := strings.ToLower(r.parte)
	if parte == "_xmlsignatures/_rels/origin.sigs.rels" {
		return r.relacion.tipo == tipoRelacionFirmaDigital
	}
	if parte != "_rels/.rels" {
		return false
	}
	destino, ok := relacionesPaqueteFueraDeFirma[r.relacion.tipo]
	return ok && strings.ToLower(destinoRelacionPaquete(r.relacion.destino)) == destino
}

// coberturaOOXML es coberturaContenedor para un paquete OOXML: además de las
// entradas, cada relación que una firma deja fuera de una parte .rels cuenta
// como contenido sin firmar, salvo las de la lista cerrada de metadatos. Los
// metadatos que ninguna firma cubre se dicen en fueraPorFormato.
func coberturaOOXML(c *contenedorZIP, cubiertasPorFirma []map[string]bool, relacionesPorFirma [][]relacionNoFirmada, noComprobado []string) resultadoCobertura {
	out := coberturaContenedor(c.nombres, func(nombre string) bool {
		return entradaOOXMLSinFirmar(nombre) || c.metadatoFueraDeFirma(nombre)
	}, cubiertasPorFirma, noComprobado)
	vistos := map[string]bool{}
	for _, cubiertas := range cubiertasPorFirma {
		for _, nombre := range c.nombres {
			if cubiertas[nombre] || vistos[nombre] || entradaOOXMLSinFirmar(nombre) || !c.metadatoFueraDeFirma(nombre) {
				continue
			}
			vistos[nombre] = true
			out.fueraPorFormato = append(out.fueraPorFormato, nombre)
		}
	}
	for _, relaciones := range relacionesPorFirma {
		for _, relacion := range relaciones {
			etiqueta := "relacion:" + relacion.String()
			if relacionFueraDeFirmaPermitida(relacion) || vistos[etiqueta] {
				continue
			}
			vistos[etiqueta] = true
			out.sinCubrir = append(out.sinCubrir, etiqueta)
		}
	}
	sort.Strings(out.sinCubrir)
	sort.Strings(out.fueraPorFormato)
	return out
}

func dedup(valores []string) []string {
	var out []string
	vistos := map[string]bool{}
	for _, valor := range valores {
		if !vistos[valor] {
			vistos[valor] = true
			out = append(out, valor)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// ds:Manifest
// ---------------------------------------------------------------------------

// verificarManifiestos comprueba las referencias de los ds:Manifest que la
// firma cubre desde su SignedInfo (OOXML de Office, XAdES externa con
// Manifest). Que cuadre el resumen del Manifest solo prueba que la lista de
// resúmenes no ha cambiado; cada referencia de la lista se resuelve y se
// compara aquí. Un resumen distinto es un error; lo que no se puede resolver
// o calcular (URI externas sin el dato, RelationshipTransform de OPC) queda
// como no comprobado y la firma no puede salir válida.
func verificarManifiestos(context xmlSignatureContext, signedInfoXML []byte, resolver func(string) ([]byte, error)) (coberturaFirmaXML, error) {
	uris, noComprobado, partes, relaciones, err := verificarManifiestosDetalle(context, signedInfoXML, resolver)
	return coberturaFirmaXML{uris: uris, noComprobado: noComprobado, partesRelacionesOPC: partes, relacionesNoFirmadas: relaciones}, err
}

func verificarManifiestosDetalle(context xmlSignatureContext, signedInfoXML []byte, resolver func(string) ([]byte, error)) ([]string, []string, []string, []relacionNoFirmada, error) {
	var info signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &info); err != nil {
		return nil, nil, nil, nil, err
	}
	var manifiestos []int
	var arbol *arbolXML
	for _, ref := range info.References {
		uri := strings.TrimSpace(ref.URI)
		if !strings.HasPrefix(uri, "#") {
			continue
		}
		if arbol == nil {
			var err error
			if arbol, err = construirArbolXML(context.document); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		destino := arbol.nodoPorID(strings.TrimPrefix(uri, "#"))
		if destino < 0 {
			continue
		}
		switch {
		case arbol.esDSig(destino, "Manifest"):
			manifiestos = append(manifiestos, destino)
		case arbol.esDSig(destino, "Object"):
			for _, hijo := range arbol.nodos[destino].hijos {
				if arbol.esDSig(hijo, "Manifest") {
					manifiestos = append(manifiestos, hijo)
				}
			}
		}
	}
	var uris, noComprobado []string
	var relaciones []relacionNoFirmada
	var partes []string
	total := 0
	for _, manifiesto := range manifiestos {
		nodo := arbol.nodos[manifiesto]
		var lista struct {
			References []referenceForVerify `xml:"Reference"`
		}
		if err := xml.Unmarshal(context.document[nodo.inicio:nodo.fin], &lista); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("leyendo ds:Manifest: %w", err)
		}
		if len(lista.References) == 0 {
			return nil, nil, nil, nil, errors.New("ds:Manifest sin referencias")
		}
		if total += len(lista.References); total > maxReferenciasXML {
			return nil, nil, nil, nil, fmt.Errorf("ds:Manifest con demasiadas referencias (más de %d)", maxReferenciasXML)
		}
		for indice, ref := range lista.References {
			comprobada, motivo, fuera, err := verificarReferenciaManifiesto(context, ref, resolver)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("referencia %d del Manifest: %w", indice+1, err)
			}
			if !comprobada {
				noComprobado = append(noComprobado, motivo)
				continue
			}
			uris = append(uris, ref.URI)
			if usaTransformacionRelaciones(ref) {
				partes = append(partes, ref.URI)
				relaciones = append(relaciones, fuera...)
			}
		}
	}
	return uris, noComprobado, partes, relaciones, nil
}

func verificarReferenciaManifiesto(context xmlSignatureContext, ref referenceForVerify, resolver func(string) ([]byte, error)) (bool, string, []relacionNoFirmada, error) {
	if strings.TrimSpace(ref.DigestMethod.Parameters) != "" {
		return false, "manifest_digest_con_parametros", nil, nil
	}
	for indice, transform := range ref.Transforms {
		algorithm := strings.TrimSpace(transform.Algorithm)
		if algorithm == algRelationshipTransform {
			if indice != 0 {
				// Solo se sabe aplicar sobre la parte .rels tal cual.
				return false, "manifest_relationship_transform", nil, nil
			}
			return verificarReferenciaRelaciones(ref, resolver)
		}
		if strings.TrimSpace(transform.Parameters) != "" && !esXPathExcluirFirmas(transform) {
			return false, "manifest_transformacion_con_parametros", nil, nil
		}
		switch algorithm {
		case algEnveloped, algExcC14N, algC14N, algBase64Transform, algXPathFilter:
		default:
			return false, "manifest_transformacion_no_soportada", nil, nil
		}
	}
	uri := strings.TrimSpace(ref.URI)
	externa := uri != "" && !strings.HasPrefix(uri, "#")
	var (
		target []byte
		err    error
	)
	switch {
	case externa && resolver != nil:
		// Dentro de un contenedor, una parte que falta es una alteración.
		if target, err = resolver(uri); err != nil {
			return false, "", nil, err
		}
	case externa:
		if target, err = resolveDetachedContent(context.document, uri); err != nil {
			return false, "manifest_referencia_externa", nil, nil
		}
	default:
		if target, err = resolveReferenceTarget(context.document, uri, nil); err != nil {
			return false, "", nil, err
		}
	}
	expected, err := decodificarResumen(ref.DigestValue)
	if err != nil {
		return false, "", nil, err
	}
	declared, err := declaredReferenceDigest(context, ref, target)
	if err != nil {
		return false, "manifest_procedimiento_no_soportado", nil, nil
	}
	if !equalBytes(declared, expected) {
		return false, "", nil, errors.New("el resumen no coincide")
	}
	return true, "", nil, nil
}

func usaTransformacionRelaciones(ref referenceForVerify) bool {
	for _, transform := range ref.Transforms {
		if strings.TrimSpace(transform.Algorithm) == algRelationshipTransform {
			return true
		}
	}
	return false
}

func decodificarResumen(value string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(compactBase64(value))
}

// coberturaDeSignedInfo anota las URI de las referencias del SignedInfo.
func coberturaDeSignedInfo(context xmlSignatureContext, signedInfoXML []byte) (coberturaFirmaXML, error) {
	var info signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &info); err != nil {
		return coberturaFirmaXML{}, err
	}
	cobertura := coberturaFirmaXML{inicio: context.signatureStart}
	for _, ref := range info.References {
		cobertura.uris = append(cobertura.uris, ref.URI)
	}
	return cobertura, nil
}
