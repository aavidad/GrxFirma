// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package eni

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

// MaxXMLBytes limita también el contenido base64 de documentos existentes.
const MaxXMLBytes = 150 * 1024 * 1024
const nsDS = "http://www.w3.org/2000/09/xmldsig#"

var reClasificacion = regexp.MustCompile(`^(?:[0-9]{1,30}|[A-Z][0-9]{8}_PRO_[A-Za-z0-9_]{1,30})$`)
var reFecha = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-][0-9]{2}:[0-9]{2})$`)

// Problema identifica una infracción estructural sin incluir el XML del usuario.
type Problema struct {
	Campo string `json:"field"`
	Clave string `json:"key"`
}

func (p Problema) Error() string            { return p.String() }
func (p Problema) String() string           { return p.Campo + ": " + localizador.Detectar().T(p.Clave) }
func problema(campo, clave string) Problema { return Problema{campo, "eni.validacion." + clave} }

func textoSeguro(s string, max int) bool {
	if len(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func fechaValida(s string) bool {
	if len(s) > 35 || !reFecha.MatchString(s) {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	// time.Parse acepta offsets fuera del rango de xsd:dateTime.
	if err != nil || t.Year() < 1 || t.Year() > 9999 {
		return false
	}
	_, offset := t.Zone()
	return offset >= -14*3600 && offset <= 14*3600
}
func fechaTiempoValida(t time.Time) bool {
	return t.IsZero() || fechaValida(t.Format(time.RFC3339Nano))
}

// xmlNodo mantiene los nombres expandidos: los prefijos no son significativos.
type xmlNodo struct {
	nombre xml.Name
	attrs  []xml.Attr
	texto  string
	hijos  []*xmlNodo
}

func leerXML(data []byte) (*xmlNodo, string) {
	if len(data) == 0 || len(data) > MaxXMLBytes {
		return nil, "limit"
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root *xmlNodo
	var stack []*xmlNodo
	nodes := 0
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "xml"
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, "xml" // DTD/entidades externas nunca se procesan.
		case xml.ProcInst:
			if t.Target != "xml" || root != nil {
				return nil, "xml"
			}
		case xml.StartElement:
			nodes++
			if len(stack) >= 64 || nodes > 20000 || len(t.Attr) > 64 {
				return nil, "limit"
			}
			n := &xmlNodo{nombre: t.Name, attrs: t.Attr}
			seen := map[xml.Name]bool{}
			for _, a := range t.Attr {
				if seen[a.Name] {
					return nil, "xml"
				}
				seen[a.Name] = true
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, "xml"
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.hijos = append(parent.hijos, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(t)) != "" {
					return nil, "xml"
				}
			} else {
				n := stack[len(stack)-1]
				// Acumular el contenido sin copias cuadráticas para base64 grande.
				if len(n.texto) == 0 {
					n.texto = string(t)
				} else {
					n.texto += string(t)
				}
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, "xml"
	}
	return root, ""
}

type elemento struct {
	ns, nombre string
	min, max   int
}

func e(ns, nombre string, min, max int) elemento { return elemento{ns, nombre, min, max} }

// ValidarXML revisa el perfil ENI 1.0 de documento/expediente, incluidos
// metadatos, contenido, índice y envolturas de firma. No verifica firmas ni
// sustituye la validación XSD oficial (prueba opcional con xmllint).
func ValidarXML(data []byte) []Problema {
	root, err := leerXML(data)
	if err != "" {
		return []Problema{problema("XML", err)}
	}
	var issues []Problema
	add := func(path, key string) {
		if len(issues) < 256 {
			issues = append(issues, problema(path, key))
		}
	}
	sequence := func(n *xmlNodo, path string, rules ...elemento) {
		if strings.TrimSpace(n.texto) != "" {
			add(path, "structure")
		}
		pos := 0
		for _, r := range rules {
			count := 0
			for pos < len(n.hijos) && n.hijos[pos].nombre == (xml.Name{Space: r.ns, Local: r.nombre}) {
				count++
				pos++
			}
			if count < r.min || (r.max >= 0 && count > r.max) {
				add(path+"/"+r.nombre, "structure")
			}
		}
		if pos != len(n.hijos) {
			add(path, "structure")
		}
	}
	value := func(n *xmlNodo, path string, valid bool) {
		if len(n.hijos) != 0 {
			add(path, "structure")
		}
		if !valid {
			add(path, "value")
		}
	}
	var walk func(*xmlNodo, string)
	walk = func(n *xmlNodo, path string) {
		s := strings.TrimSpace(n.texto)
		ns, name := n.nombre.Space, n.nombre.Local
		switch {
		case ns == nsDocumento && name == "documento":
			sequence(n, path, e(nsContenido, "contenido", 1, 1), e(nsMetadatos, "metadatos", 1, 1), e(nsFirma, "firmas", 0, 1))
		case ns == nsContenido && name == "contenido":
			sequence(n, path, e(nsContenido, "ValorBinario", 1, 1), e(nsContenido, "NombreFormato", 1, 1))
		case ns == nsMetadatos && name == "metadatos":
			sequence(n, path, e(ns, "VersionNTI", 1, 1), e(ns, "Identificador", 1, 1), e(ns, "Organo", 1, 128), e(ns, "FechaCaptura", 1, 1), e(ns, "OrigenCiudadanoAdministracion", 1, 1), e(ns, "EstadoElaboracion", 1, 1), e(ns, "TipoDocumental", 1, 1))
		case ns == nsMetadatos && name == "EstadoElaboracion":
			sequence(n, path, e(ns, "ValorEstadoElaboracion", 1, 1), e(ns, "IdentificadorDocumentoOrigen", 0, 1))
			if len(n.hijos) > 0 && slices.Contains([]string{"EE02", "EE03", "EE04"}, n.hijos[0].texto) && len(n.hijos) != 2 {
				add(path+"/IdentificadorDocumentoOrigen", "source")
			}
		case ns == nsExpediente && name == "expediente":
			sequence(n, path, e(nsIndice, "indice", 1, 1), e(nsMetadatosExp, "metadatosExp", 1, 1))
		case ns == nsIndice && name == "indice":
			sequence(n, path, e(ns, "IndiceContenido", 1, 1), e(nsFirma, "firmas", 1, 1))
		case ns == nsIndice && name == "IndiceContenido":
			sequence(n, path, e(nsIndiceContenido, "FechaIndiceElectronico", 1, 1), e(nsIndiceContenido, "DocumentoIndizado", 1, 128))
		case ns == nsIndiceContenido && name == "DocumentoIndizado":
			sequence(n, path, e(ns, "IdentificadorDocumento", 1, 1), e(ns, "ValorHuella", 1, 1), e(ns, "FuncionResumen", 1, 1), e(ns, "FechaIncorporacionExpediente", 1, 1), e(ns, "OrdenDocumentoExpediente", 1, 1))
		case ns == nsMetadatosExp && name == "metadatosExp":
			sequence(n, path, e(ns, "VersionNTI", 1, 1), e(ns, "Identificador", 1, 1), e(ns, "Organo", 1, 128), e(ns, "FechaAperturaExpediente", 1, 1), e(ns, "Clasificacion", 1, 1), e(ns, "Estado", 1, 1), e(ns, "Interesado", 0, 128))
		case ns == nsFirma && name == "firmas":
			sequence(n, path, e(ns, "firma", 1, 128))
		case ns == nsFirma && name == "firma":
			sequence(n, path, e(ns, "TipoFirma", 1, 1), e(ns, "ContenidoFirma", 1, 1))
		case ns == nsFirma && name == "ContenidoFirma":
			sequence(n, path, e(ns, "FirmaConCertificado", 1, 1))
		case ns == nsFirma && name == "FirmaConCertificado":
			if len(n.hijos) != 1 {
				add(path, "structure")
			} else {
				c := n.hijos[0]
				if c.nombre != (xml.Name{Space: nsDS, Local: "Signature"}) && c.nombre != (xml.Name{Space: nsFirma, Local: "FirmaBase64"}) && c.nombre != (xml.Name{Space: nsFirma, Local: "ReferenciaFirma"}) {
					add(path, "structure")
				}
			}
		case ns == nsDS && name == "Signature":
			return // El verificador criptográfico comprueba XMLDSig.
		case name == "VersionNTI" && (ns == nsMetadatos || ns == nsMetadatosExp):
			expected := VersionNTI
			if ns == nsMetadatosExp {
				expected = versionNTIExp
			}
			value(n, path, s == expected)
		case (name == "Identificador" && (ns == nsMetadatos || ns == nsMetadatosExp)) || (name == "IdentificadorDocumentoOrigen" && ns == nsMetadatos) || (name == "IdentificadorDocumento" && ns == nsIndiceContenido):
			value(n, path, reIdentificador.MatchString(s))
		case name == "Organo" && (ns == nsMetadatos || ns == nsMetadatosExp):
			value(n, path, reDIR3.MatchString(s))
		case (ns == nsMetadatos && name == "FechaCaptura") || (ns == nsMetadatosExp && name == "FechaAperturaExpediente") || (ns == nsIndiceContenido && (name == "FechaIndiceElectronico" || name == "FechaIncorporacionExpediente")):
			value(n, path, fechaValida(s))
		case ns == nsMetadatos && name == "OrigenCiudadanoAdministracion":
			value(n, path, s == "true" || s == "false" || s == "0" || s == "1")
		case ns == nsMetadatos && name == "ValorEstadoElaboracion":
			value(n, path, slices.Contains(EstadosElaboracion(), s))
		case ns == nsMetadatos && name == "TipoDocumental":
			value(n, path, TipoDocumentalValido(s))
		case ns == nsMetadatosExp && name == "Estado":
			value(n, path, slices.Contains(EstadosExpediente(), s))
		case ns == nsMetadatosExp && name == "Clasificacion":
			value(n, path, reClasificacion.MatchString(s))
		case ns == nsMetadatosExp && name == "Interesado":
			value(n, path, s != "" && textoSeguro(s, 128))
		case ns == nsContenido && name == "NombreFormato":
			value(n, path, reFormato.MatchString(s))
		case (ns == nsContenido && name == "ValorBinario") || (ns == nsFirma && name == "FirmaBase64"):
			compact := strings.Join(strings.Fields(s), "")
			_, err := base64.StdEncoding.DecodeString(compact)
			value(n, path, err == nil && compact != "")
		case ns == nsFirma && name == "TipoFirma":
			value(n, path, tiposFirma[TipoFirma(s)])
		case ns == nsFirma && name == "ReferenciaFirma":
			value(n, path, strings.HasPrefix(s, "#") && textoSeguro(s, 128))
		case ns == nsIndiceContenido && name == "ValorHuella":
			value(n, path, regexp.MustCompile(`^[A-Fa-f0-9]{64}$`).MatchString(s))
		case ns == nsIndiceContenido && name == "FuncionResumen":
			value(n, path, s == funcionResumenSHA2)
		case ns == nsIndiceContenido && name == "OrdenDocumentoExpediente":
			value(n, path, regexp.MustCompile(`^[1-9][0-9]{0,2}$`).MatchString(s))
		default:
			add(path, "structure")
		}
		for _, c := range n.hijos {
			walk(c, path+"/"+c.nombre.Local)
		}
	}
	if root.nombre != (xml.Name{Space: nsDocumento, Local: "documento"}) && root.nombre != (xml.Name{Space: nsExpediente, Local: "expediente"}) {
		add("XML", "structure")
		return issues
	}
	walk(root, "/"+root.nombre.Local)
	return issues
}

func validarSalida(data []byte) error {
	issues := ValidarXML(data)
	if len(issues) == 0 {
		return nil
	}
	messages := make([]string, len(issues))
	for i, p := range issues {
		messages[i] = p.String()
	}
	return fmt.Errorf("%s", strings.Join(messages, "\n"))
}
