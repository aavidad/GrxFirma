// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

const VeriFactuMaxXMLBytes = 10 * 1024 * 1024
const VeriFactuNamespace = "https://www2.agenciatributaria.gob.es/static_files/common/internet/dep/aplicaciones/es/aeat/tike/cont/ws/SuministroInformacion.xsd"
const VeriFactuEventNamespace = "https://www2.agenciatributaria.gob.es/static_files/common/internet/dep/aplicaciones/es/aeat/tike/cont/ws/EventosSIF.xsd"

var vfDigest = regexp.MustCompile(`^[0-9A-F]{64}$`)

type vfNode struct {
	name                xml.Name
	attrs               []xml.Attr
	children            []*vfNode
	text                strings.Builder
	start, openEnd, end int64
}

func vfError(key string) error { return vfProblem{Key: "verifactu." + key} }

type vfProblem struct {
	Field string `json:"field"`
	Key   string `json:"key"`
	Level string `json:"level"`
}

func (p vfProblem) Error() string           { return localizador.Detectar().T(p.Key) }
func (p vfProblem) LocalizationKey() string { return p.Key }

// TraducirErrorVeriFactu permite que cada interfaz use el idioma elegido.
func TraducirErrorVeriFactu(err error, t func(string) string) string {
	if p, ok := err.(interface{ LocalizationKey() string }); ok {
		return t(p.LocalizationKey())
	}
	return t("verifactu.input")
}

// CodigoErrorVeriFactu devuelve la clave del error como código estable de
// IPC (sin puntos, que los clientes no admiten), para que la interfaz decida
// sin interpretar el texto traducido: por ejemplo, si prueba la página
// siguiente de un PDF solo cuando el QR no se ha encontrado.
func CodigoErrorVeriFactu(err error) string {
	if p, ok := err.(interface{ LocalizationKey() string }); ok && p.LocalizationKey() != "" {
		return strings.ReplaceAll(p.LocalizationKey(), ".", "_")
	}
	return "verifactu_input"
}

// Los nombres expandidos y límites se aplican antes de canonicalizar o verificar.
func vfParse(data []byte) (*vfNode, error) {
	if len(data) == 0 || len(data) > VeriFactuMaxXMLBytes {
		return nil, vfError("limit")
	}
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	var root *vfNode
	var stack []*vfNode
	nodes := 0
	declared := false
	for {
		start := d.InputOffset()
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, vfError("xml")
		}
		switch x := t.(type) {
		case xml.Directive:
			return nil, vfError("xml")
		case xml.ProcInst:
			if x.Target != "xml" || root != nil || declared {
				return nil, vfError("xml")
			}
			declared = true
		case xml.StartElement:
			nodes++
			if nodes > 20000 || len(stack) >= 64 || len(x.Attr) > 64 || len(x.Name.Local) > 128 || len(x.Name.Space) > 512 {
				return nil, vfError("limit")
			}
			seen := map[xml.Name]bool{}
			for _, a := range x.Attr {
				if seen[a.Name] {
					return nil, vfError("xml")
				}
				seen[a.Name] = true
			}
			n := &vfNode{name: x.Name, attrs: x.Attr, start: start, openEnd: d.InputOffset()}
			if len(stack) == 0 {
				if root != nil {
					return nil, vfError("xml")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.children = append(p.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack[len(stack)-1].end = d.InputOffset()
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(x)
			} else if strings.TrimSpace(string(x)) != "" {
				return nil, vfError("xml")
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, vfError("xml")
	}
	return root, nil
}
func (n *vfNode) child(ns, name string) *vfNode {
	if n != nil {
		for _, c := range n.children {
			if c.name == (xml.Name{Space: ns, Local: name}) {
				return c
			}
		}
	}
	return nil
}
func (n *vfNode) value(path ...string) string {
	if n == nil {
		return ""
	}
	ns := n.name.Space
	for _, p := range path {
		n = n.child(ns, p)
		if n == nil {
			return ""
		}
	}
	return strings.Trim(n.text.String(), " \t\r\n")
}
func vfRoot(n *vfNode) bool {
	return n != nil && ((n.name.Space == VeriFactuNamespace && (n.name.Local == "RegistroAlta" || n.name.Local == "RegistroAnulacion")) || (n.name.Space == VeriFactuEventNamespace && n.name.Local == "RegistroEvento"))
}

// EsRegistroVeriFactu reconoce exclusivamente la raíz y el espacio de nombres oficial.
func EsRegistroVeriFactu(data []byte) bool { n, e := vfParse(data); return e == nil && vfRoot(n) }

type vfRule struct {
	Kind     string   `json:"k"`
	Name     string   `json:"n"`
	Type     string   `json:"t"`
	Ref      string   `json:"r"`
	Min      int      `json:"min"`
	Max      int      `json:"max"`
	Value    string   `json:"v"`
	Base     string   `json:"base"`
	Children []vfRule `json:"c"`
}
type vfSchema struct {
	Roots map[string]vfRule `json:"roots"`
	Types map[string]vfRule `json:"types"`
}

var vfSchemas = func() map[string]vfSchema {
	var m map[string]vfSchema
	if err := json.Unmarshal([]byte(verifactuSchemaData), &m); err != nil {
		panic(err)
	}
	return m
}()

func vfStructure(n *vfNode, unsigned bool) []vfProblem {
	var issues []vfProblem
	add := func(field, key string) {
		if len(issues) < 256 {
			issues = append(issues, vfProblem{field, "verifactu." + key, "error"})
		}
	}
	if !vfRoot(n) {
		add("XML", "root")
		return issues
	}
	s := vfSchemas["SuministroInformacion"]
	if n.name.Local == "RegistroEvento" {
		s = vfSchemas["EventosSIF"]
	}
	var validate func(*vfNode, vfRule, string)
	var particle func([]*vfNode, int, vfRule, string) (int, bool)
	var begins func(*vfNode, vfRule) bool
	begins = func(n *vfNode, r vfRule) bool {
		if r.Kind == "element" {
			if r.Ref == "ds:Signature" {
				return n.name == (xml.Name{Space: nsXMLDSig, Local: "Signature"})
			}
			return n.name == (xml.Name{Space: nRootNS(s), Local: r.Name})
		}
		for _, c := range r.Children {
			if begins(n, c) {
				return true
			}
			if r.Kind == "sequence" && c.Min > 0 {
				break
			}
		}
		return false
	}
	particle = func(nodes []*vfNode, pos int, r vfRule, path string) (int, bool) {
		if r.Kind == "element" {
			count := 0
			for pos < len(nodes) && begins(nodes[pos], r) && count < r.Max {
				validate(nodes[pos], r, path+"/"+nodes[pos].name.Local)
				pos++
				count++
			}
			min := r.Min
			if unsigned && r.Ref == "ds:Signature" {
				min = 0
			}
			if count < min {
				field := r.Name
				if field == "" {
					field = r.Ref
				}
				add(path+"/"+field, "structure")
				return pos, false
			}
			return pos, true
		}
		if r.Kind == "choice" {
			for _, c := range r.Children {
				if pos < len(nodes) && begins(nodes[pos], c) {
					return particle(nodes, pos, c, path)
				}
			}
			if r.Min > 0 {
				add(path, "structure")
				return pos, false
			}
			return pos, true
		}
		for _, c := range r.Children {
			var ok bool
			pos, ok = particle(nodes, pos, c, path)
			if !ok {
				return pos, false
			}
		}
		return pos, true
	}
	validate = func(n *vfNode, r vfRule, path string) {
		if r.Ref == "ds:Signature" {
			return
		} // El verificador criptográfico revisa XMLDSig/XAdES.
		for _, a := range n.attrs {
			if a.Name.Space != "xmlns" && !(a.Name.Space == "" && a.Name.Local == "xmlns") {
				add(path, "structure")
			}
		}
		if r.Type != "" {
			if r.Type == "fecha" {
				v := n.value()
				d, err := time.Parse("02-01-2006", v)
				if err != nil || len(v) != 10 || d.Format("02-01-2006") != v || d.Year() < 1 {
					add(path, "value")
				}
			}
			if t, ok := s.Types[r.Type]; ok {
				validate(n, t, path)
			} else if r.Type == "dateTime" {
				if !vfDateTime(n.value()) {
					add(path, "value")
				}
				if len(n.children) > 0 {
					add(path, "structure")
				}
			} else {
				add(path, "structure")
			}
			return
		}
		switch r.Kind {
		case "element", "complexType":
			if strings.TrimSpace(n.text.String()) != "" {
				add(path, "structure")
			}
			pos := 0
			for _, c := range r.Children {
				pos, _ = particle(n.children, pos, c, path)
			}
			if pos != len(n.children) {
				add(path, "structure")
			}
		case "simpleType":
			for _, c := range r.Children {
				validate(n, c, path)
			}
		case "restriction":
			if len(n.children) > 0 {
				add(path, "structure")
			}
			v := n.text.String()
			length := utf8.RuneCountInString(v)
			enum := false
			match := false
			for _, f := range r.Children {
				num, _ := strconv.Atoi(f.Value)
				bad := false
				switch f.Kind {
				case "enumeration":
					enum = true
					match = match || v == f.Value
				case "length":
					bad = length != num
				case "minLength":
					bad = length < num
				case "maxLength":
					bad = length > num
				case "pattern":
					re, e := regexp.Compile("^(?:" + f.Value + ")$")
					bad = e != nil || !re.MatchString(v)
				}
				if bad {
					add(path, "value")
				}
			}
			if enum && !match {
				add(path, "value")
			}
		}
	}
	validate(n, s.Roots[n.name.Local], n.name.Local)
	return issues
}
func nRootNS(s vfSchema) string {
	if _, ok := s.Roots["RegistroEvento"]; ok {
		return VeriFactuEventNamespace
	}
	return VeriFactuNamespace
}
func vfDateTime(v string) bool {
	if len(v) > 35 {
		return false
	}
	t, e := time.Parse(time.RFC3339Nano, v)
	if e != nil || t.Year() < 1 || t.Year() > 9999 {
		return false
	}
	_, z := t.Zone()
	return z >= -14*3600 && z <= 14*3600
}
