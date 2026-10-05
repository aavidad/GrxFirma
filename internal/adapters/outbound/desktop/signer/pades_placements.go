// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/idna"

	"github.com/digitorus/pdf"
	pdfsign "github.com/digitorus/pdfsign/sign"
)

const maxSealPlacements = 128

type sealPlacement struct {
	Page int `json:"page"`
	Rect struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
		H float64 `json:"h"`
	} `json:"rect"`
	Rotation int `json:"rotation"`
}

// contieneControlOFormato indica si el texto lleva caracteres de control
// (Cc) o de formato (Cf): marcas bidireccionales (U+202A-202E, U+2066-2069),
// caracteres de anchura cero (U+200B-200F), U+FEFF y similares. No se ven o
// alteran el orden visual, y permitirían mostrar en el sello o en el QR una
// dirección o un código distinto del que realmente contiene.
func contieneControlOFormato(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)
	})
}

// normalizarURLQRSello se ejecuta en el motor, también para solicitudes que
// no pasaron por una interfaz de escritorio.
func normalizarURLQRSello(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > 2048 || strings.Contains(value, "\\") || strings.ContainsFunc(value, unicode.IsSpace) || contieneControlOFormato(value) {
		return "", fmt.Errorf("el QR exige una dirección HTTPS válida, sin espacios ni caracteres de control")
	}
	if !strings.Contains(value, "://") {
		if strings.Contains(value, ":") {
			return "", fmt.Errorf("el QR exige HTTPS; no se permite otro esquema")
		}
		value = "https://" + value
	}
	if len(value) > 2048 {
		return "", fmt.Errorf("la dirección HTTPS del QR es demasiado larga")
	}
	u, err := url.Parse(value)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", fmt.Errorf("el QR exige una dirección HTTPS válida, sin usuario ni contraseña")
	}
	authorityStart := strings.Index(value, "://") + 3
	authorityEnd := strings.IndexAny(value[authorityStart:], "/?#")
	if authorityEnd < 0 {
		authorityEnd = len(value)
	} else {
		authorityEnd += authorityStart
	}
	if strings.ContainsAny(value[authorityStart:authorityEnd], "@%") {
		return "", fmt.Errorf("el QR exige un servidor HTTPS válido")
	}
	host := u.Hostname()
	if host == "" || strings.ContainsAny(host, "@%") {
		return "", fmt.Errorf("el QR exige un servidor HTTPS válido")
	}
	if net.ParseIP(host) == nil {
		// Lookup aplica NFC y las reglas STD3, ContextJ y Bidi. Registration
		// comprueba también los límites DNS y la validez de etiquetas ACE.
		host, err = idna.Lookup.ToASCII(host)
		if err == nil {
			host, err = idna.Registration.ToASCII(host)
		}
		if err != nil || strings.HasSuffix(host, ".") {
			return "", fmt.Errorf("el QR exige un servidor HTTPS válido")
		}
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("el puerto HTTPS del QR no es válido")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || len(port) > 5 || n < 1 || n > 65535 {
			return "", fmt.Errorf("el puerto HTTPS del QR no es válido")
		}
	}
	if port := u.Port(); port != "" {
		host += ":" + port
	}
	value = "https://" + host + value[authorityEnd:]
	if len(value) > 2048 {
		return "", fmt.Errorf("la dirección HTTPS del QR es demasiado larga")
	}
	return value, nil
}

func normalizarQROpciones(options map[string]string) error {
	var normalized string
	for key, raw := range options {
		if !strings.EqualFold(strings.TrimSpace(key), "qrContent") &&
			!strings.EqualFold(strings.TrimSpace(key), "visibleSealQRContent") {
			continue
		}
		delete(options, key)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value, err := normalizarURLQRSello(raw)
		if err != nil {
			return err
		}
		if normalized != "" && normalized != value {
			return fmt.Errorf("el QR tiene direcciones contradictorias")
		}
		normalized = value
	}
	if normalized != "" {
		options["qrContent"] = normalized
	}
	return nil
}

func aplicarPosicionesSello(signData *pdfsign.SignData, options map[string]string, pdfData []byte) error {
	var raw string
	var found bool
	for key, value := range options {
		if !strings.EqualFold(strings.TrimSpace(key), "visibleSealPlacements") {
			continue
		}
		if found {
			return fmt.Errorf("la lista de sellos está duplicada")
		}
		raw, found = strings.TrimSpace(value), true
	}
	if raw == "" {
		if found {
			return fmt.Errorf("la lista de sellos no puede estar vacía")
		}
		return nil
	}
	for key, value := range options {
		if strings.EqualFold(strings.TrimSpace(key), "signatureField") && strings.TrimSpace(value) != "" {
			return fmt.Errorf("no se pueden combinar sellos por página con un campo de firma existente")
		}
	}
	if len(raw) > 32*1024 {
		return fmt.Errorf("la lista de sellos supera el límite")
	}
	var entries []sealPlacement
	if err := json.Unmarshal([]byte(raw), &entries); err != nil || len(entries) == 0 || len(entries) > maxSealPlacements {
		return fmt.Errorf("la lista de sellos debe contener entre 1 y %d páginas válidas", maxSealPlacements)
	}
	r, err := abrirPDF(pdfData, options)
	if err != nil {
		return err
	}
	seen := make(map[int]bool, len(entries))
	placements := make([]pdfsign.PageAppearance, 0, len(entries))
	for _, entry := range entries {
		if entry.Page < 1 || uint64(entry.Page) > uint64(^uint32(0)) || entry.Page > r.NumPage() || seen[entry.Page] || entry.Rotation < 0 || entry.Rotation > 359 {
			return fmt.Errorf("página o giro no válido en la lista de sellos")
		}
		seen[entry.Page] = true
		v := entry.Rect
		if math.IsNaN(v.X) || math.IsNaN(v.Y) || math.IsNaN(v.W) || math.IsNaN(v.H) ||
			math.IsInf(v.X, 0) || math.IsInf(v.Y, 0) || math.IsInf(v.W, 0) || math.IsInf(v.H, 0) ||
			v.X < 0 || v.Y < 0 || v.W <= 0 || v.H <= 0 || v.X+v.W > 1 || v.Y+v.H > 1 {
			return fmt.Errorf("el rectángulo del sello debe quedar dentro de la página %d", entry.Page)
		}
		marco, err := marcoVisiblePagina(r.Page(entry.Page))
		if err != nil {
			return fmt.Errorf("página %d: %w", entry.Page, err)
		}
		// Las fracciones se refieren a la página tal como se ve (con /Rotate).
		width, height := marco.dimensionesVisibles()
		w, h := v.W*width, v.H*height
		rect, giroImagen, err := marco.colocarSello(v.X*width, v.Y*height, w, h, float64(entry.Rotation))
		if err != nil {
			return fmt.Errorf("página %d: %w", entry.Page, err)
		}
		copyOptions := opcionesConGiroImagen(options, giroImagen)
		img, err := componerImagenSelloParaFirma(signData.Signature.Info, copyOptions, w, h)
		if err != nil {
			return err
		}
		placements = append(placements, pdfsign.PageAppearance{
			Page:  uint32(entry.Page), // #nosec G115 -- entry.Page was checked against uint32 above.
			Rect:  rect,
			Image: img,
		})
	}
	signData.Appearance.Visible = true
	signData.Appearance.PerPage = placements
	return nil
}

// La caja debe proceder del documento real; no se usa el A4 de reserva que
// sirve para colocar elementos opcionales en documentos antiguos.
func cajaVisiblePagina(page pdf.Page) (float64, float64, float64, float64, error) {
	// Heredado corta las cadenas /Parent circulares o demasiado largas.
	media := page.Heredado("MediaBox")
	crop := page.Heredado("CropBox")
	if media.Len() != 4 {
		return 0, 0, 0, 0, fmt.Errorf("la página PDF no tiene una caja real válida")
	}
	box := media
	if crop.Len() == 4 {
		box = crop
	}
	x0, y0, x1, y1 := box.Index(0).Float64(), box.Index(1).Float64(), box.Index(2).Float64(), box.Index(3).Float64()
	for _, v := range []float64{x0, y0, x1, y1} {
		if !numeroFinitoSello(v) {
			return 0, 0, 0, 0, fmt.Errorf("la caja de la página PDF no es finita")
		}
	}
	if x1 <= x0 || y1 <= y0 {
		return 0, 0, 0, 0, fmt.Errorf("la caja de la página PDF no es válida")
	}
	if crop.Len() == 4 {
		mx0, my0, mx1, my1 := media.Index(0).Float64(), media.Index(1).Float64(), media.Index(2).Float64(), media.Index(3).Float64()
		if !numeroFinitoSello(mx0) || !numeroFinitoSello(my0) || !numeroFinitoSello(mx1) || !numeroFinitoSello(my1) || x0 < mx0 || y0 < my0 || x1 > mx1 || y1 > my1 {
			return 0, 0, 0, 0, fmt.Errorf("la caja visible de la página PDF queda fuera de MediaBox")
		}
	}
	return x0, y0, x1, y1, nil
}
