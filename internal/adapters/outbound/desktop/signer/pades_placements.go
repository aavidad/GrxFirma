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

// normalizarURLQRSello se ejecuta en el motor, también para solicitudes que
// no pasaron por una interfaz de escritorio.
func normalizarURLQRSello(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > 2048 || strings.ContainsAny(value, "\\ \t\n\r") || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
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
	if host := u.Hostname(); host == "" || strings.ContainsAny(host, "@%") {
		return "", fmt.Errorf("el QR exige un servidor HTTPS válido")
	} else if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", fmt.Errorf("el QR exige un servidor HTTPS válido")
			}
			for _, c := range label {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
					return "", fmt.Errorf("el QR exige un servidor HTTPS válido")
				}
			}
		}
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("el puerto HTTPS del QR no es válido")
		}
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
		x0, y0, x1, y1, err := cajaVisiblePagina(r.Page(entry.Page))
		if err != nil {
			return err
		}
		width, height := x1-x0, y1-y0
		if !numeroFinitoSello(width) || !numeroFinitoSello(height) || width <= 0 || height <= 0 || width > 14400 || height > 14400 {
			return fmt.Errorf("dimensiones inválidas de la página %d", entry.Page)
		}
		w, h := v.W*width, v.H*height
		rect, err := cajaSelloGirado(x0+v.X*width, y0+v.Y*height, w, h, float64(entry.Rotation), x0, y0, width, height)
		if err != nil {
			return fmt.Errorf("página %d: %w", entry.Page, err)
		}
		copyOptions := make(map[string]string, len(options)+3)
		for k, val := range options {
			copyOptions[k] = val
		}
		copyOptions["rotation"] = strconv.Itoa(entry.Rotation)
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
	media := page.V.Key("MediaBox")
	crop := page.V.Key("CropBox")
	for parent, depth := page.V.Key("Parent"), 0; !parent.IsNull() && (media.Len() != 4 || crop.Len() != 4) && depth < 64; parent, depth = parent.Key("Parent"), depth+1 {
		if media.Len() != 4 {
			media = parent.Key("MediaBox")
		}
		if crop.Len() != 4 {
			crop = parent.Key("CropBox")
		}
	}
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
