// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	pdfsign "github.com/digitorus/pdfsign/sign"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
)

// Firma visible PAdES con los parámetros de AutoFirma Java, que son los que
// envían las sedes: signaturePositionOnPage{LowerLeft,UpperRight}{X,Y},
// signaturePage/signaturePages (negativas desde el final), layer2Text con
// patrones ($$SUBJECTCN$$, $$SIGNDATE=dd/MM/yyyy$$...), signatureRubricImage
// y signatureRotation. Se traducen a las opciones del motor de sello.

var javaSelloClaves = []string{
	"signaturePositionOnPageLowerLeftX", "signaturePositionOnPageLowerLeftY",
	"signaturePositionOnPageUpperRightX", "signaturePositionOnPageUpperRightY",
}

func traducirSelloVisibleJava(options map[string]string, pdfData []byte, cert *x509.Certificate, ahora time.Time) (map[string]string, error) {
	if strings.TrimSpace(valorOpcion(options, "signatureField")) != "" {
		// En un campo existente la posición la da el propio campo; solo se
		// traduce el texto del sello.
		out := make(map[string]string, len(options)+1)
		for k, v := range options {
			out[k] = v
		}
		if texto := valorOpcion(options, "layer2Text"); strings.TrimSpace(texto) != "" {
			out["visibleSealText"] = expandirLayer2Text(texto, cert, options, ahora)
		}
		return out, nil
	}
	if pos := strings.TrimSpace(options[domain.OpcionPosicionSello]); pos != "" {
		var err error
		if options, err = posicionSelloElegida(options, pos, pdfData); err != nil {
			return nil, err
		}
	}
	coords := make([]float64, 0, 4)
	for _, clave := range javaSelloClaves {
		raw := strings.TrimSpace(valorOpcion(options, clave))
		if raw == "" {
			return options, nil
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("posición de firma visible inválida en %s", clave)
		}
		coords = append(coords, v)
	}
	llx, lly, urx, ury := coords[0], coords[1], coords[2], coords[3]
	if urx <= llx || ury <= lly {
		return nil, fmt.Errorf("el área de la firma visible está vacía o invertida")
	}
	out := make(map[string]string, len(options)+8)
	for k, v := range options {
		out[k] = v
	}
	out["visibleSeal"] = "true"
	out["visibleSealRectX"] = strconv.FormatFloat(llx, 'f', -1, 64)
	out["visibleSealRectY"] = strconv.FormatFloat(lly, 'f', -1, 64)
	out["visibleSealRectW"] = strconv.FormatFloat(urx-llx, 'f', -1, 64)
	out["visibleSealRectH"] = strconv.FormatFloat(ury-lly, 'f', -1, 64)

	paginas := strings.TrimSpace(valorOpcion(options, "signaturePages"))
	if paginas == "" {
		paginas = strings.TrimSpace(valorOpcion(options, "signaturePage"))
	}
	if paginas != "" {
		resueltas, err := resolverPaginasJava(paginas, pdfData, options)
		if err != nil {
			return nil, err
		}
		out["page"] = resueltas
	}
	if rubrica := strings.TrimSpace(valorOpcion(options, "signatureRubricImage")); rubrica != "" && strings.TrimSpace(valorOpcion(options, "visibleSealImageBase64")) == "" {
		out["visibleSealImageBase64"] = rubrica
	}
	if rot := strings.TrimSpace(valorOpcion(options, "signatureRotation")); rot != "" && strings.TrimSpace(valorOpcion(options, "rotation")) == "" {
		out["rotation"] = rot
	}
	if texto := valorOpcion(options, "layer2Text"); strings.TrimSpace(texto) != "" {
		out["visibleSealText"] = expandirLayer2Text(texto, cert, options, ahora)
	}
	return out, nil
}

// resolverPaginasJava admite "all", listas y números negativos (-1 es la
// última página), como AutoFirma Java.
func resolverPaginasJava(raw string, pdfData []byte, options map[string]string) (string, error) {
	if strings.EqualFold(raw, "all") || raw == "*" {
		return "all", nil
	}
	total := 0
	var partes []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			partes = append(partes, p) // rangos "2-4" los interpreta el motor
			continue
		}
		if n <= 0 {
			if total == 0 {
				r, err := abrirPDF(pdfData, options)
				if err != nil {
					return "", fmt.Errorf("no se pudo contar las páginas del PDF: %w", err)
				}
				total = r.NumPage()
			}
			if n == 0 {
				n = 1
			} else {
				n = total + n + 1
			}
			if n < 1 {
				return "", fmt.Errorf("página de firma visible fuera del documento: %s", p)
			}
		}
		partes = append(partes, strconv.Itoa(n))
	}
	return strings.Join(partes, ","), nil
}

var camposDNJava = map[string]asn1.ObjectIdentifier{
	"CN": {2, 5, 4, 3}, "SN": {2, 5, 4, 4}, "SURNAME": {2, 5, 4, 4}, "SERIALNUMBER": {2, 5, 4, 5},
	"C": {2, 5, 4, 6}, "L": {2, 5, 4, 7}, "ST": {2, 5, 4, 8}, "O": {2, 5, 4, 10}, "OU": {2, 5, 4, 11},
	"T": {2, 5, 4, 12}, "TITLE": {2, 5, 4, 12}, "GIVENNAME": {2, 5, 4, 42}, "G": {2, 5, 4, 42},
	"ORGANIZATIONIDENTIFIER": {2, 5, 4, 97},
}

// expandirLayer2Text sustituye los patrones de AutoFirma Java.
func expandirLayer2Text(texto string, cert *x509.Certificate, options map[string]string, ahora time.Time) string {
	var b strings.Builder
	for {
		i := strings.Index(texto, "$$")
		if i < 0 {
			b.WriteString(texto)
			break
		}
		j := strings.Index(texto[i+2:], "$$")
		if j < 0 {
			b.WriteString(texto)
			break
		}
		b.WriteString(texto[:i])
		b.WriteString(valorPatronJava(texto[i+2:i+2+j], cert, options, ahora))
		texto = texto[i+2+j+2:]
	}
	return b.String()
}

func valorPatronJava(patron string, cert *x509.Certificate, options map[string]string, ahora time.Time) string {
	nombre, arg, _ := strings.Cut(patron, "=")
	switch strings.ToUpper(strings.TrimSpace(nombre)) {
	case "SUBJECTCN":
		if cert != nil {
			return cert.Subject.CommonName
		}
	case "ISSUERCN":
		if cert != nil {
			return cert.Issuer.CommonName
		}
	case "CERTSERIAL":
		if cert != nil {
			return strings.ToUpper(cert.SerialNumber.Text(16))
		}
	case "SUBJECTFIELD":
		if cert != nil {
			return campoDN(cert.Subject.Names, arg)
		}
	case "ISSUERFIELD":
		if cert != nil {
			return campoDN(cert.Issuer.Names, arg)
		}
	case "SIGNDATE":
		formato := strings.TrimSpace(arg)
		if formato == "" {
			formato = "dd/MM/yyyy"
		}
		return ahora.In(zonaSelloDesdeOpciones(options)).Format(formatoFechaJava(formato))
	case "REASON":
		return primeraOpcionNoVacia(options, "signReason", "reason", "signatureReason")
	case "LOCATION":
		return primeraOpcionNoVacia(options, "signatureProductionCity", "signLocation", "location")
	case "CONTACT":
		return primeraOpcionNoVacia(options, "signerContact", "signContact", "contactInfo")
	}
	return ""
}

func campoDN(names []pkix.AttributeTypeAndValue, campo string) string {
	oid, ok := camposDNJava[strings.ToUpper(strings.TrimSpace(campo))]
	if !ok {
		return ""
	}
	for _, n := range names {
		if n.Type.Equal(oid) {
			return fmt.Sprint(n.Value)
		}
	}
	return ""
}

// formatoFechaJava traduce los patrones de SimpleDateFormat más usados.
func formatoFechaJava(f string) string {
	r := strings.NewReplacer("yyyy", "2006", "yy", "06", "MM", "01", "dd", "02", "HH", "15", "mm", "04", "ss", "05")
	return r.Replace(f)
}

// estiloTextoSello es el texto y la tipografía del sello pedidos por la web,
// y el idioma y la zona horaria con que se escriben sus rótulos y su fecha.
type estiloTextoSello struct {
	texto     string
	tamPuntos float64
	color     *color.NRGBA
	escala    float64 // píxeles por punto, lo fija el generador
	textos    *localizador.Localizador
	zona      *time.Location
	emisor    string // emisor del certificado para «Emitido por»
}

// Opciones del sello que fijan su idioma y su zona horaria. La interfaz que
// pide la firma envía su idioma (o el fijado en la configuración, p. ej.
// castellano para documentos de la Administración) y, en el móvil, la zona
// del dispositivo. Sin ellas se mantiene el castellano y la zona del sistema.
const (
	OpcionIdiomaSello = "sealLanguage"
	OpcionZonaSello   = "sealTimeZone"
)

func idiomaSelloDesdeOpciones(options map[string]string) *localizador.Localizador {
	idioma := localizador.Idioma(valorOpcion(options, OpcionIdiomaSello))
	if idioma == "" {
		idioma = "es"
	}
	return localizador.Para(idioma)
}

func zonaSelloDesdeOpciones(options map[string]string) *time.Location {
	if zona := localizador.Zona(valorOpcion(options, OpcionZonaSello)); zona != nil {
		return zona
	}
	return time.Local
}

func estiloTextoDesdeOpciones(options map[string]string) estiloTextoSello {
	e := estiloTextoSello{
		texto:  valorOpcion(options, "visibleSealText"),
		textos: idiomaSelloDesdeOpciones(options),
		zona:   zonaSelloDesdeOpciones(options),
		emisor: valorOpcion(options, opcionEmisorSelloInterna),
	}
	if raw := strings.TrimSpace(valorOpcion(options, "layer2FontSize")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
			e.tamPuntos = v
		}
	}
	if c, ok := colorJava(valorOpcion(options, "layer2FontColor")); ok {
		e.color = &c
	}
	return e
}

// colorJava admite los nombres de color de AutoFirma Java y #RRGGBB.
func colorJava(raw string) (color.NRGBA, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	nombres := map[string]color.NRGBA{
		"black": {0, 0, 0, 255}, "white": {255, 255, 255, 255}, "gray": {128, 128, 128, 255},
		"lightgray": {192, 192, 192, 255}, "darkgray": {64, 64, 64, 255}, "red": {255, 0, 0, 255},
		"pink": {255, 175, 175, 255}, "orange": {255, 200, 0, 255}, "yellow": {255, 255, 0, 255},
		"green": {0, 255, 0, 255}, "magenta": {255, 0, 255, 255}, "cyan": {0, 255, 255, 255},
		"blue": {0, 0, 255, 255},
	}
	if c, ok := nombres[raw]; ok {
		return c, true
	}
	if len(raw) == 7 && raw[0] == '#' {
		if rgb, err := hex.DecodeString(raw[1:]); err == nil && len(rgb) == 3 {
			return color.NRGBA{rgb[0], rgb[1], rgb[2], 255}, true
		}
	}
	return color.NRGBA{}, false
}

// imagenesJava traduce el estampado de imagen de AutoFirma Java: image
// (Base64), imagePage (-1 última, 0 todas) e imagePositionOnPage*.
func imagenesJava(options map[string]string, pdfData []byte) ([]pdfsign.StampImage, error) {
	raw := strings.TrimSpace(valorOpcion(options, "image"))
	if raw == "" {
		return nil, nil
	}
	if i := strings.Index(raw, ","); i >= 0 && strings.Contains(strings.ToLower(raw[:i]), "base64") {
		raw = raw[i+1:]
	}
	if base64.StdEncoding.DecodedLen(len(raw)) > 10*1024*1024 {
		return nil, fmt.Errorf("la imagen a estampar supera 10 MiB")
	}
	img, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("imagen a estampar no válida: %w", err)
	}
	var rect [4]float64
	for i, clave := range []string{"imagePositionOnPageLowerLeftX", "imagePositionOnPageLowerLeftY", "imagePositionOnPageUpperRightX", "imagePositionOnPageUpperRightY"} {
		v, err := strconv.ParseFloat(strings.TrimSpace(valorOpcion(options, clave)), 64)
		if err != nil {
			return nil, fmt.Errorf("posición de la imagen no válida en %s", clave)
		}
		rect[i] = v
	}
	if rect[2] <= rect[0] || rect[3] <= rect[1] {
		return nil, fmt.Errorf("el área de la imagen está vacía o invertida")
	}
	r, err := abrirPDF(pdfData, options)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el PDF: %w", err)
	}
	total := r.NumPage()
	pagina := -1
	if v := strings.TrimSpace(valorOpcion(options, "imagePage")); v != "" {
		if pagina, err = strconv.Atoi(v); err != nil {
			return nil, fmt.Errorf("imagePage no válida: %s", v)
		}
	}
	var paginas []int
	switch {
	case pagina == 0:
		for p := 1; p <= total; p++ {
			paginas = append(paginas, p)
		}
	case pagina < 0:
		paginas = []int{total + pagina + 1}
	default:
		paginas = []int{pagina}
	}
	out := make([]pdfsign.StampImage, 0, len(paginas))
	for _, p := range paginas {
		if p < 1 || p > total {
			return nil, fmt.Errorf("página de la imagen fuera del documento: %d", p)
		}
		out = append(out, pdfsign.StampImage{Page: uint32(p), Rect: rect, Image: img}) // #nosec G115 -- 1 <= p <= NumPage.
	}
	return out, nil
}

// Tamaño y margen del sello situado por el usuario, en puntos.
const (
	anchoSelloElegido  = 200.0
	altoSelloElegido   = 70.0
	margenSelloElegido = 36.0
)

// posicionSelloElegida traduce la posición elegida por el usuario a las
// coordenadas de AutoFirma Java sobre la página indicada.
func posicionSelloElegida(options map[string]string, pos string, pdfData []byte) (map[string]string, error) {
	if !domain.PosicionSelloValida(pos) {
		return nil, fmt.Errorf("posición de sello no válida: %q", pos)
	}
	pagina := strings.TrimSpace(options[domain.OpcionPaginaSello])
	if pagina == "" {
		pagina = "1"
	}
	r, err := abrirPDF(pdfData, options)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el PDF: %w", err)
	}
	total := r.NumPage()
	n, err := strconv.Atoi(pagina)
	if err != nil || n == 0 || n > total || -n > total {
		return nil, fmt.Errorf("página del sello fuera del documento: %s", pagina)
	}
	if n < 0 {
		n = total + n + 1
	}
	x0, y0, x1, y1 := cajaPagina(r.Page(n))
	ancho, alto := math.Min(anchoSelloElegido, x1-x0-2*margenSelloElegido), altoSelloElegido
	if ancho <= 0 || y1-y0 < alto+2*margenSelloElegido {
		return nil, fmt.Errorf("la página %d es demasiado pequeña para el sello", n)
	}
	var llx, lly float64
	switch {
	case strings.HasSuffix(pos, "izquierda"):
		llx = x0 + margenSelloElegido
	case strings.HasSuffix(pos, "centro"):
		llx = x0 + (x1-x0-ancho)/2
	default:
		llx = x1 - margenSelloElegido - ancho
	}
	if strings.HasPrefix(pos, "superior") {
		lly = y1 - margenSelloElegido - alto
	} else {
		lly = y0 + margenSelloElegido
	}
	out := make(map[string]string, len(options)+5)
	for k, v := range options {
		out[k] = v
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	out["signaturePositionOnPageLowerLeftX"] = f(llx)
	out["signaturePositionOnPageLowerLeftY"] = f(lly)
	out["signaturePositionOnPageUpperRightX"] = f(llx + ancho)
	out["signaturePositionOnPageUpperRightY"] = f(lly + alto)
	out["signaturePage"] = strconv.Itoa(n)
	delete(out, "signaturePages")
	return out, nil
}
