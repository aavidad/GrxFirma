// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/digitorus/pdf"
	pdfsign "github.com/digitorus/pdfsign/sign"
	qrcode "github.com/skip2/go-qrcode"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
)

// construirValidationDataPAdES recolecta el material de validación (cadena,
// OCSP, CRL) para incrustar el /DSS de PAdES B-LT. La evidencia del
// certificado firmante es obligatoria; la de los intermedios, mejor esfuerzo.
func (m *MotorFirmaGo) construirValidationDataPAdES(ctx context.Context, clave *ClaveLocal) (pdfsign.ValidationData, error) {
	if m == nil || m.revocation == nil {
		return pdfsign.ValidationData{}, errors.New("requiere un proveedor de revocación configurado")
	}
	cadenas := construirCadenaPdfsign(clave)
	if len(cadenas) == 0 || len(cadenas[0]) < 2 {
		return pdfsign.ValidationData{}, errors.New("requiere la cadena de certificación completa (emisor no disponible)")
	}
	cadena := cadenas[0]

	var vd pdfsign.ValidationData
	for _, cert := range cadena {
		vd.Certs = append(vd.Certs, cert.Raw)
	}
	for i := 0; i+1 < len(cadena); i++ {
		evidencia, err := m.revocation.Fetch(ctx, cadena[i], cadena[i+1])
		if err != nil {
			if i == 0 {
				return pdfsign.ValidationData{}, fmt.Errorf("sin evidencia de revocación (OCSP/CRL) para el certificado firmante: %w", err)
			}
			continue
		}
		vd.OCSPs = append(vd.OCSPs, evidencia.OCSPResponses...)
		vd.CRLs = append(vd.CRLs, evidencia.CRLs...)
	}
	return vd, nil
}

// solicitaLTVPAdES indica si el llamador pidió incrustar material de
// validación (/DSS): perfil B-LT/B-LTA o la opción booleana "ltv".
func solicitaLTVPAdES(options map[string]string) bool {
	if solicitaPerfilLT(options) || solicitaPerfilLTA(options) {
		return true
	}
	return valorBoolOpcionPAdES(options, "ltv", false)
}

func firmarPAdESConPdfsign(doc domain.Document, clave *ClaveLocal, options map[string]string, validation pdfsign.ValidationData, tsa pdfsign.TSA) (domain.SignatureResult, error) {
	if clave == nil || clave.cert == nil || clave.priv == nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES pdfsign: clave local incompleta")
	}

	tmpDir, err := os.MkdirTemp("", "grxfirma-pdfsign-*")
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES pdfsign: creando directorio temporal: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.pdf")
	outputPath := filepath.Join(tmpDir, "signed.pdf")
	if err := os.WriteFile(inputPath, doc.Content, 0o600); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES pdfsign: escribiendo PDF temporal: %w", err)
	}

	options, err = traducirSelloVisibleJava(options, doc.Content, clave.cert, time.Now())
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES: %w", err)
	}
	if err := normalizarQROpciones(options); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES: %w", err)
	}
	if solicitaSelloVisiblePAdES(options) && strings.TrimSpace(valorOpcion(options, "visibleSealPlacements")) == "" && strings.TrimSpace(valorOpcion(options, "signatureField")) == "" {
		r, openErr := abrirPDF(doc.Content, options)
		if openErr != nil {
			return domain.SignatureResult{}, openErr
		}
		_, pages, pagesErr := parseVisibleSealPageSelection(valorOpcion(options, "page"))
		if pagesErr != nil {
			return domain.SignatureResult{}, pagesErr
		}
		page := 1
		if len(pages) > 0 {
			page = int(pages[0])
		}
		if page > r.NumPage() {
			return domain.SignatureResult{}, fmt.Errorf("PAdES: página del sello fuera del documento")
		}
		x0, y0, x1, y1, boxErr := cajaVisiblePagina(r.Page(page))
		if boxErr != nil {
			return domain.SignatureResult{}, boxErr
		}
		options["visibleSealPageWidth"] = strconv.FormatFloat(x1-x0, 'f', -1, 64)
		options["visibleSealPageHeight"] = strconv.FormatFloat(y1-y0, 'f', -1, 64)
		options["visibleSealPageX"] = strconv.FormatFloat(x0, 'f', -1, 64)
		options["visibleSealPageY"] = strconv.FormatFloat(y0, 'f', -1, 64)
	}

	info := pdfsign.SignDataSignatureInfo{
		Name:        nombreFirmantePAdES(clave),
		Location:    descripcionCertificadoPAdES(clave),
		Reason:      "Firma electrónica avanzada",
		ContactInfo: "",
		Date:        time.Now().Local(),
	}
	aplicarMetadatosFirmaPdfsign(&info, options)
	signData := pdfsign.SignData{
		Signature: pdfsign.SignDataSignature{
			Info:       info,
			CertType:   pdfsign.ApprovalSignature,
			DocMDPPerm: pdfsign.AllowFillingExistingFormFieldsAndSignaturesPerms,
			SubFilter:  subFilterPdfsign(options),
		},
		Signer:            clave.priv,
		DigestAlgorithm:   hashFirmaPAdES(options),
		Certificate:       clave.cert,
		CertificateChains: construirCadenaPdfsign(clave),
	}
	if err := aplicarOpcionesAparienciaPdfsign(&signData, options); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := aplicarPosicionesSello(&signData, options, doc.Content); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES: %w", err)
	}
	if signData.Appearance.Stamps, err = imagenesJava(options, doc.Content); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES: %w", err)
	}
	leyenda, err := leyendaCSV(options, doc.Content)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES: %w", err)
	}
	signData.Appearance.Stamps = append(signData.Appearance.Stamps, leyenda...)
	signData.TSA = tsa
	signData.ValidationData = validation

	if err := firmarConLector(inputPath, outputPath, doc.Content, options, signData); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES pdfsign: %w", err)
	}

	out, err := securefile.ReadFileLimit(outputPath, 100*1024*1024)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("PAdES pdfsign: leyendo salida: %w", err)
	}
	if len(out) == 0 {
		return domain.SignatureResult{}, fmt.Errorf("PAdES pdfsign: PDF firmado vacío")
	}

	algoritmo := "PAdES-Basic-Detached"
	if subFilterPdfsign(options) == pdfsign.SignatureSubFilterAdobePKCS7Detached {
		algoritmo += "-Adobe"
	}
	if solicitaPerfilT(options) {
		algoritmo = "PAdES-B-T"
		if subFilterPdfsign(options) == pdfsign.SignatureSubFilterAdobePKCS7Detached {
			algoritmo += "-Adobe"
		}
	}
	if !validation.IsEmpty() {
		if solicitaPerfilT(options) {
			algoritmo = "PAdES-B-LT"
		} else {
			algoritmo = "PAdES-Basic-LTV"
		}
		if subFilterPdfsign(options) == pdfsign.SignatureSubFilterAdobePKCS7Detached {
			algoritmo += "-Adobe"
		}
	}

	return domain.SignatureResult{
		Format:    domain.FormatPAdES,
		Data:      out,
		Algorithm: algoritmo,
	}, nil
}

// hashFirmaPAdES aplica el resumen pedido por el portal (algorithm=SHA512withRSA
// en JCyL o Aragón). SHA-1 y los valores desconocidos se elevan a SHA-256.
func hashFirmaPAdES(options map[string]string) crypto.Hash {
	raw := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(valorOpcion(options, "algorithm")), "-", ""))
	switch {
	case strings.HasPrefix(raw, "SHA384"):
		return crypto.SHA384
	case strings.HasPrefix(raw, "SHA512"):
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

func subFilterPdfsign(options map[string]string) pdfsign.SignatureSubFilter {
	if resolverSubFilterPAdES(options) == string(pdfsign.SignatureSubFilterAdobePKCS7Detached) {
		return pdfsign.SignatureSubFilterAdobePKCS7Detached
	}
	return pdfsign.SignatureSubFilterETSICAdESDetached
}

func aplicarMetadatosFirmaPdfsign(info *pdfsign.SignDataSignatureInfo, options map[string]string) {
	if info == nil {
		return
	}
	if reason := primeraOpcionNoVacia(options, "reason", "signReason", "signatureReason"); reason != "" {
		info.Reason = reason
	}
	if location := primeraOpcionNoVacia(options, "location", "signLocation", "signatureLocation"); location != "" {
		info.Location = location
	}
	if contact := primeraOpcionNoVacia(options, "contactInfo", "contact", "signContact", "signatureContact"); contact != "" {
		info.ContactInfo = contact
	}
}

func aplicarOpcionesAparienciaPdfsign(signData *pdfsign.SignData, options map[string]string) error {
	if signData == nil {
		return nil
	}
	opacidadSello, err := opacidadSelloDesdeOpciones(options)
	if err != nil {
		return err
	}
	signData.Appearance.SealOpacityPercent = &opacidadSello
	if err := normalizarQROpciones(options); err != nil {
		return err
	}
	if strings.TrimSpace(valorOpcion(options, "visibleSealPlacements")) != "" {
		signData.Appearance.Visible = true
		return nil
	}
	if campo := strings.TrimSpace(valorOpcion(options, "signatureField")); campo != "" {
		// signatureField de AutoFirma Java: se firma en el campo vacío del
		// formulario, con el sello ajustado a su rectángulo.
		signData.Appearance.FieldName = campo
		img, err := generarImagenSelloPAdES(signData.Signature.Info, 250, 64, true, codigoQRSelloDesdeOpciones(options),
			resumenFirmantesSelloDesdeOpciones(options), nil, estiloTextoDesdeOpciones(options))
		if err != nil {
			return fmt.Errorf("PAdES pdfsign: no se pudo generar el sello del campo: %w", err)
		}
		signData.Appearance.Image = img
		return nil
	}
	if !solicitaSelloVisiblePAdES(options) {
		return nil
	}
	x := valorFloatOpcionPAdES(options, "visibleSealRectX", 0.62*595.28)
	y := valorFloatOpcionPAdES(options, "visibleSealRectY", 0.04*841.89)
	w := valorFloatOpcionPAdES(options, "visibleSealRectW", 0.34*595.28)
	h := valorFloatOpcionPAdES(options, "visibleSealRectH", 0.12*841.89)
	if err := validarGeometriaSello(options, x, y, w, h); err != nil {
		return fmt.Errorf("PAdES pdfsign: %w", err)
	}
	pageX := valorFloatOpcionPAdES(options, "visibleSealPageX", 0)
	pageY := valorFloatOpcionPAdES(options, "visibleSealPageY", 0)
	if valorBoolOpcionPAdES(options, "visibleSealRectRelativeToCrop", false) {
		x += pageX
		y += pageY
	}
	signData.Appearance.Visible = true
	if err := aplicarPaginasAparienciaPdfsign(&signData.Appearance, options); err != nil {
		return err
	}
	pageW := valorFloatOpcionPAdES(options, "visibleSealPageWidth", 0)
	pageH := valorFloatOpcionPAdES(options, "visibleSealPageHeight", 0)
	rect, err := cajaSelloGirado(x, y, w, h, valorFloatOpcionPAdES(options, "rotation", 0), pageX, pageY, pageW, pageH)
	if err != nil {
		return fmt.Errorf("PAdES pdfsign: %w", err)
	}
	signData.Appearance.LowerLeftX = rect[0]
	signData.Appearance.LowerLeftY = rect[1]
	signData.Appearance.UpperRightX = rect[2]
	signData.Appearance.UpperRightY = rect[3]
	img, err := componerImagenSelloParaFirma(signData.Signature.Info, options, w, h)
	if err != nil {
		return err
	}
	signData.Appearance.Image = img
	signData.Appearance.ImageAsWatermark = false
	return nil
}

// componerImagenSello genera la imagen final del sello visible para un
// rectángulo de w x h puntos: diseño, emblema o imagen propia, QR y giro. La
// firma y la vista previa del escritorio usan esta misma función.
func componerImagenSello(info pdfsign.SignDataSignatureInfo, options map[string]string, w, h float64) ([]byte, error) {
	return componerImagenSelloModo(info, options, w, h, capaSelloCompleto)
}

// La firma conserva el PNG compuesto opaco y aplica la opacidad una sola vez
// en PDF; así las zonas donde se superponen logo y fondo no se oscurecen.
func componerImagenSelloParaFirma(info pdfsign.SignDataSignatureInfo, options map[string]string, w, h float64) ([]byte, error) {
	fullOpacity := make(map[string]string, len(options)+1)
	for key, value := range options {
		if strings.EqualFold(strings.TrimSpace(key), "visibleSealLogoOpacityPercent") {
			continue
		}
		fullOpacity[key] = value
	}
	fullOpacity["visibleSealLogoOpacityPercent"] = "100"
	return componerImagenSello(info, fullOpacity, w, h)
}

func componerImagenSelloModo(info pdfsign.SignDataSignatureInfo, options map[string]string, w, h float64, capa capaSello) ([]byte, error) {
	opacidad, err := opacidadSelloDesdeOpciones(options)
	if err != nil {
		return nil, err
	}
	grados := math.Mod(valorFloatOpcionPAdES(options, "rotation", 0), 360)
	if !numeroFinitoSello(grados) {
		return nil, fmt.Errorf("PAdES pdfsign: giro del sello no válido")
	}
	if grados < 0 {
		grados += 360
	}
	rotation := normalizarRotacionPAdES(int(grados))
	giroLibre := math.Mod(grados, 90) != 0
	keepText := valorBoolOpcionPAdES(options, "visibleSealKeepText", true)
	customImage, _, err := imagenSelloDesdeOpciones(options)
	if err != nil {
		return nil, fmt.Errorf("PAdES pdfsign: imagen del sello inválida: %w", err)
	}
	if len(customImage) == 0 && strings.EqualFold(strings.TrimSpace(valorOpcion(options, "visibleSealLogo")), "institucional") {
		customImage = emblemaFirmaPNG
	}
	qrContent := codigoQRSelloDesdeOpciones(options)
	signerSummary := resumenFirmantesSelloDesdeOpciones(options)
	img, err := generarImagenSelloPAdESModo(info, w, h, keepText, qrContent, signerSummary, customImage, estiloTextoDesdeOpciones(options), capa, 255)
	if err != nil {
		return nil, fmt.Errorf("PAdES pdfsign: no se pudo generar el sello visible: %w", err)
	}
	if len(img) > 0 {
		if giroLibre {
			if img, err = girarImagenPNGLibre(img, grados); err != nil {
				return nil, fmt.Errorf("PAdES pdfsign: girando sello visible: %w", err)
			}
		} else {
			img, err = rotarImagenPNGSiProcede(img, rotation)
			if err != nil {
				return nil, fmt.Errorf("PAdES pdfsign: rotando sello visible: %w", err)
			}
		}
		// El PNG ocupa exactamente la caja girada. El escalado PDF solo
		// cambia resolución; nunca la proporción ni la disposición del sello.
		box, boxErr := cajaSelloGirado(0, 0, w, h, grados, 0, 0, 0, 0)
		if boxErr != nil {
			return nil, boxErr
		}
		img, err = ajustarImagenSelloAlRectanguloSinRecorte(img, box[2]-box[0], box[3]-box[1])
		if err != nil {
			return nil, fmt.Errorf("PAdES pdfsign: ajustando sello visible: %w", err)
		}
		if capa == capaSelloCompleto && opacidad < 100 {
			img, err = aplicarOpacidadVistaPreviaSello(img, opacidad)
			if err != nil {
				return nil, err
			}
		}
	}
	return img, nil
}

// La vista previa compone antes todas las capas y reduce después su alfa.
// Así el fondo, el texto, el QR y el logo dejan ver la página por igual.
func aplicarOpacidadVistaPreviaSello(raw []byte, porcentaje int) ([]byte, error) {
	decoded, err := decodificarImagenSello(raw)
	if err != nil {
		return nil, err
	}
	img := image.NewRGBA(decoded.Bounds())
	draw.Draw(img, img.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			pixel := img.RGBAAt(x, y)
			pixel.R = uint8(uint32(pixel.R) * uint32(porcentaje) / 100)
			pixel.G = uint8(uint32(pixel.G) * uint32(porcentaje) / 100)
			pixel.B = uint8(uint32(pixel.B) * uint32(porcentaje) / 100)
			pixel.A = uint8(uint32(pixel.A) * uint32(porcentaje) / 100)
			img.SetRGBA(x, y, pixel)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// El logo y el resto del sello comparten las mismas coordenadas en el XObject.
// Recortar los bordes transparentes de una capa desplazaría el emblema.
func ajustarImagenSelloAlRectanguloSinRecorte(raw []byte, widthPt, heightPt float64) ([]byte, error) {
	widthPx, heightPx, err := dimensionesRasterSello(widthPt, heightPt, 1, 1)
	if err != nil {
		return nil, err
	}
	source, err := decodificarImagenSello(raw)
	if err != nil {
		return nil, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, widthPx, heightPx))
	xdraw.CatmullRom.Scale(canvas, canvas.Bounds(), source, source.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func ajustarImagenSelloAlRectangulo(raw []byte, widthPt, heightPt float64) ([]byte, error) {
	widthPx, heightPx, err := dimensionesRasterSello(widthPt, heightPt, 1, 1)
	if err != nil {
		return nil, err
	}
	source, err := decodificarImagenSello(raw)
	if err != nil {
		return nil, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, widthPx, heightPx))
	dibujarImagenAjustada(canvas, source, canvas.Bounds(), 255)
	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func generarImagenSelloPAdES(info pdfsign.SignDataSignatureInfo, widthPt, heightPt float64, keepText bool, qrContent, signerSummary string, backgroundImage []byte, estilo estiloTextoSello) ([]byte, error) {
	return generarImagenSelloPAdESModo(info, widthPt, heightPt, keepText, qrContent, signerSummary, backgroundImage, estilo, capaSelloCompleto, 255)
}

func generarImagenSelloPAdESModo(info pdfsign.SignDataSignatureInfo, widthPt, heightPt float64, keepText bool, qrContent, signerSummary string, backgroundImage []byte, estilo estiloTextoSello, capa capaSello, opacidadLogo uint8) ([]byte, error) {
	// Resolución de trabajo alta para que texto y QR queden nítidos al
	// imprimir; el diseño se adapta a la proporción real del sello.
	escala := math.Max(3, 300/math.Max(1, math.Min(widthPt, heightPt)))
	widthPx, heightPx, err := dimensionesRasterSello(widthPt, heightPt, int(widthPt*escala), int(heightPt*escala))
	if err != nil {
		return nil, err
	}
	estilo.escala = float64(widthPx) / math.Max(widthPt, 1)
	rendered, err := renderizarImagenSelloPAdESModo(info, widthPx, heightPx, keepText, qrContent, signerSummary, backgroundImage, estilo, capa, opacidadLogo)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, rendered); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderizarImagenSelloPAdES(info pdfsign.SignDataSignatureInfo, widthPx, heightPx int, keepText bool, qrContent, signerSummary string, backgroundImage []byte, estilo estiloTextoSello) (*image.RGBA, error) {
	return renderizarImagenSelloPAdESModo(info, widthPx, heightPx, keepText, qrContent, signerSummary, backgroundImage, estilo, capaSelloCompleto, 255)
}

func renderizarImagenSelloPAdESModo(info pdfsign.SignDataSignatureInfo, widthPx, heightPx int, keepText bool, qrContent, signerSummary string, backgroundImage []byte, estilo estiloTextoSello, capa capaSello, opacidadLogo uint8) (*image.RGBA, error) {
	var logo image.Image
	if len(backgroundImage) > 0 {
		decoded, err := decodificarImagenSello(backgroundImage)
		if err != nil {
			return nil, err
		}
		logo = recortarBordesTransparentes(decoded)
	}
	return renderizarSelloModerno(info, widthPx, heightPx, keepText, qrContent, signerSummary, logo, estilo, capa, opacidadLogo)
}

func opacidadSelloDesdeOpciones(options map[string]string) (int, error) {
	raw := strings.TrimSpace(valorOpcion(options, "visibleSealLogoOpacityPercent"))
	if raw == "" {
		return 100, nil
	}
	percent, err := strconv.Atoi(raw)
	if err != nil || percent < 0 || percent > 100 {
		return 0, fmt.Errorf("PAdES pdfsign: opacidad del sello fuera del rango 0–100 %%")
	}
	return percent, nil
}

func generarQRSeccionSello(content string, heightPx int) image.Image {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	size := heightPx - 12
	if size < 42 {
		size = 42
	}
	if size > 1024 {
		size = 1024
	}
	pngData, err := qrcode.Encode(content, qrcode.Medium, size)
	if err != nil || len(pngData) == 0 {
		return nil
	}
	qrImg, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil
	}
	return qrImg
}

func dibujarTexto(img *image.RGBA, x, y int, s string, c color.Color, face font.Face) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

func nuevaFuenteSelloPAdES(size float64) (font.Face, error) {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingFull,
	})
}

func partirLineaSello(s string, face font.Face, maxWidth int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	palabras := strings.Fields(s)
	if len(palabras) == 0 {
		return nil
	}
	var lineas []string
	actual := palabras[0]
	for _, palabra := range palabras[1:] {
		candidato := actual + " " + palabra
		if medirTexto(face, candidato) <= maxWidth {
			actual = candidato
			continue
		}
		lineas = append(lineas, recortarTexto(face, actual, maxWidth))
		actual = palabra
		if len(lineas) >= 3 {
			break
		}
	}
	if len(lineas) < 3 && actual != "" {
		lineas = append(lineas, recortarTexto(face, actual, maxWidth))
	}
	if len(lineas) > 3 {
		lineas = lineas[:3]
	}
	return lineas
}

func recortarTexto(face font.Face, s string, maxWidth int) string {
	s = strings.TrimSpace(s)
	if s == "" || medirTexto(face, s) <= maxWidth {
		return s
	}
	ellipsis := "..."
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		candidato := strings.TrimSpace(string(runes)) + ellipsis
		if medirTexto(face, candidato) <= maxWidth {
			return candidato
		}
	}
	return ellipsis
}

func medirTexto(face font.Face, s string) int {
	d := &font.Drawer{Face: face}
	return d.MeasureString(s).Ceil()
}

func normalizarLineaSello(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func recortarBordesTransparentes(src image.Image) image.Image {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			if a <= 0x0200 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return src
	}
	cropped := image.NewRGBA(image.Rect(0, 0, maxX-minX+1, maxY-minY+1))
	draw.Draw(cropped, cropped.Bounds(), src, image.Point{X: minX, Y: minY}, draw.Src)
	return cropped
}

func dibujarImagenAjustada(dst *image.RGBA, src image.Image, target image.Rectangle, alpha uint8) {
	if dst == nil || src == nil || target.Dx() <= 0 || target.Dy() <= 0 {
		return
	}
	src = recortarBordesTransparentes(src)
	sb := src.Bounds()
	if sb.Dx() <= 0 || sb.Dy() <= 0 {
		return
	}
	scaleX := float64(target.Dx()) / float64(sb.Dx())
	scaleY := float64(target.Dy()) / float64(sb.Dy())
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if scale <= 0 {
		return
	}
	dstW := maxInt(1, int(float64(sb.Dx())*scale))
	dstH := maxInt(1, int(float64(sb.Dy())*scale))
	offsetX := target.Min.X + (target.Dx()-dstW)/2
	offsetY := target.Min.Y + (target.Dy()-dstH)/2
	scaled := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, sb, draw.Over, nil)
	if alpha < 255 {
		for y := 0; y < dstH; y++ {
			for x := 0; x < dstW; x++ {
				pixel := scaled.RGBAAt(x, y)
				if pixel.A == 0 {
					continue
				}
				// RGBA almacena canales premultiplicados: atenuar también RGB
				// conserva el color de los logotipos y evita halos saturados.
				pixel.R = componenteRGBA8(uint32(pixel.R) * uint32(alpha) / math.MaxUint8)
				pixel.G = componenteRGBA8(uint32(pixel.G) * uint32(alpha) / math.MaxUint8)
				pixel.B = componenteRGBA8(uint32(pixel.B) * uint32(alpha) / math.MaxUint8)
				pixel.A = componenteRGBA8(uint32(pixel.A) * uint32(alpha) / math.MaxUint8)
				scaled.SetRGBA(x, y, pixel)
			}
		}
	}
	draw.Draw(dst, image.Rect(offsetX, offsetY, offsetX+dstW, offsetY+dstH), scaled, image.Point{}, draw.Over)
}

func primeraOpcionNoVacia(options map[string]string, claves ...string) string {
	for _, clave := range claves {
		if valor := strings.TrimSpace(valorOpcion(options, clave)); valor != "" {
			return valor
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func componenteRGBA8(value uint32) uint8 {
	if value > math.MaxUint8 {
		return math.MaxUint8
	}
	return uint8(value)
}

func normalizarRotacionPAdES(rotation int) int {
	rotation %= 360
	if rotation < 0 {
		rotation += 360
	}
	switch rotation {
	case 0, 90, 180, 270:
		return rotation
	default:
		return 0
	}
}

func rotarImagenPNGSiProcede(data []byte, rotation int) ([]byte, error) {
	rotation = normalizarRotacionPAdES(rotation)
	if rotation == 0 || len(data) == 0 {
		return data, nil
	}

	src, err := decodificarImagenSello(data)
	if err != nil {
		return nil, err
	}
	dst := rotarImagen(src, rotation)

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func rotarImagen(src image.Image, rotation int) *image.RGBA {
	b := src.Bounds()
	width := b.Dx()
	height := b.Dy()

	switch normalizarRotacionPAdES(rotation) {
	case 90:
		dst := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dst.Set(height-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 180:
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dst.Set(width-1-x, height-1-y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 270:
		dst := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dst.Set(y, width-1-x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	default:
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}
}

func construirCadenaPdfsign(clave *ClaveLocal) [][]*x509.Certificate {
	if clave == nil || clave.cert == nil {
		return nil
	}
	cadena := []*x509.Certificate{clave.cert}
	restantes := make([]*x509.Certificate, 0, len(clave.chain))
	seen := map[string]struct{}{string(clave.cert.Raw): {}}
	for _, cert := range clave.chain {
		if cert == nil || len(cert.Raw) == 0 {
			continue
		}
		key := string(cert.Raw)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		restantes = append(restantes, cert)
	}

	actual := clave.cert
	for len(restantes) > 0 {
		idx := -1
		for i, candidato := range restantes {
			if actual.CheckSignatureFrom(candidato) == nil {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		cadena = append(cadena, restantes[idx])
		actual = restantes[idx]
		restantes = append(restantes[:idx], restantes[idx+1:]...)
	}

	if len(cadena) <= 1 {
		return nil
	}
	return [][]*x509.Certificate{cadena}
}

func nombreFirmantePAdES(clave *ClaveLocal) string {
	if clave == nil || clave.cert == nil {
		return ""
	}
	if cn := strings.TrimSpace(clave.cert.Subject.CommonName); cn != "" {
		return cn
	}
	return strings.TrimSpace(clave.cert.Subject.String())
}

func descripcionCertificadoPAdES(clave *ClaveLocal) string {
	if clave == nil || clave.cert == nil {
		return "Certificado digital"
	}
	if org := strings.Join(clave.cert.Issuer.Organization, " "); strings.TrimSpace(org) != "" {
		return "Certificado: " + strings.TrimSpace(org)
	}
	if issuer := strings.TrimSpace(clave.cert.Issuer.CommonName); issuer != "" {
		return "Certificado: " + issuer
	}
	return "Certificado digital"
}

func solicitaSelloVisiblePAdES(options map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(valorOpcion(options, "visibleSeal")), "true")
}

func imagenSelloDesdeOpciones(options map[string]string) ([]byte, bool, error) {
	raw := strings.TrimSpace(valorOpcion(options, "visibleSealImageBase64"))
	if raw != "" {
		if comma := strings.Index(raw, ","); comma >= 0 && strings.Contains(strings.ToLower(raw[:comma]), "base64") {
			raw = raw[comma+1:]
		}
		if base64.StdEncoding.DecodedLen(len(raw)) > 10*1024*1024 {
			return nil, false, fmt.Errorf("la imagen de sello supera el tamaño máximo de 10 MiB")
		}
		data, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, false, err
		}
		if len(data) == 0 {
			return nil, false, nil
		}
		return data, true, nil
	}
	return nil, false, nil
}

func codigoQRSelloDesdeOpciones(options map[string]string) string {
	return primeraOpcionNoVacia(options, "qrContent", "visibleSealQRContent")
}

func resumenFirmantesSelloDesdeOpciones(options map[string]string) string {
	return strings.TrimSpace(valorOpcion(options, "visibleSealSignerSummary"))
}

func aplicarPaginasAparienciaPdfsign(appearance *pdfsign.Appearance, options map[string]string) error {
	if appearance == nil {
		return nil
	}

	allPages, pages, err := parseVisibleSealPageSelection(valorOpcion(options, "page"))
	if err != nil {
		return fmt.Errorf("PAdES pdfsign: selección de páginas inválida: %w", err)
	}

	appearance.AllPages = allPages
	appearance.Pages = nil
	appearance.Page = 1

	switch {
	case allPages:
		return nil
	case len(pages) > 1:
		appearance.Pages = pages
		appearance.Page = pages[0]
		return nil
	case len(pages) == 1:
		appearance.Page = pages[0]
		return nil
	default:
		appearance.Page = 1
		return nil
	}
}

const (
	maxVisibleSealPageSelectionLength = 64 * 1024
	maxVisibleSealPageSelectionParts  = 1024
	maxVisibleSealSelectedPages       = 10_000
	maxVisibleSealExpandedPages       = 20_000
)

func parseVisibleSealPageSelection(raw string) (bool, []uint32, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > maxVisibleSealPageSelectionLength {
		return false, nil, fmt.Errorf("la selección supera el tamaño máximo de %d bytes", maxVisibleSealPageSelectionLength)
	}
	raw = strings.ToLower(raw)
	if raw == "" {
		return false, []uint32{1}, nil
	}
	switch raw {
	case "all", "*", "todas", "todos":
		return true, nil, nil
	}
	if parts := strings.Count(raw, ",") + 1; parts > maxVisibleSealPageSelectionParts {
		return false, nil, fmt.Errorf("la selección supera el máximo de %d partes", maxVisibleSealPageSelectionParts)
	}

	seen := make(map[uint32]struct{})
	pages := make([]uint32, 0, 4)
	var expandedPages uint64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			if len(bounds) != 2 {
				return false, nil, fmt.Errorf("rango mal formado %q", part)
			}
			start, ok := parsePositivePageNumber(bounds[0])
			if !ok {
				return false, nil, fmt.Errorf("página inicial inválida en %q", part)
			}
			end, ok := parsePositivePageNumber(bounds[1])
			if !ok {
				return false, nil, fmt.Errorf("página final inválida en %q", part)
			}
			if end < start {
				return false, nil, fmt.Errorf("rango descendente no permitido en %q", part)
			}
			rangeLength := uint64(end) - uint64(start) + 1
			if rangeLength > maxVisibleSealSelectedPages {
				return false, nil, fmt.Errorf("el rango %q supera el máximo de %d páginas", part, maxVisibleSealSelectedPages)
			}
			if expandedPages+rangeLength > maxVisibleSealExpandedPages {
				return false, nil, fmt.Errorf("la expansión supera el máximo de %d páginas procesadas", maxVisibleSealExpandedPages)
			}
			expandedPages += rangeLength
			for page := start; ; page++ {
				if _, exists := seen[page]; !exists {
					if len(pages) == maxVisibleSealSelectedPages {
						return false, nil, fmt.Errorf("la selección supera el máximo de %d páginas", maxVisibleSealSelectedPages)
					}
					seen[page] = struct{}{}
					pages = append(pages, page)
				}
				if page == end {
					break
				}
			}
			continue
		}
		page, ok := parsePositivePageNumber(part)
		if !ok {
			return false, nil, fmt.Errorf("página inválida %q", part)
		}
		if _, exists := seen[page]; exists {
			continue
		}
		expandedPages++
		if len(pages) == maxVisibleSealSelectedPages {
			return false, nil, fmt.Errorf("la selección supera el máximo de %d páginas", maxVisibleSealSelectedPages)
		}
		seen[page] = struct{}{}
		pages = append(pages, page)
	}
	if len(pages) == 0 {
		return false, []uint32{1}, nil
	}
	return false, pages, nil
}

func parsePositivePageNumber(raw string) (uint32, bool) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil || value == 0 || value > math.MaxUint32 {
		return 0, false
	}
	return uint32(value), true
}

func valorFloatOpcionPAdES(options map[string]string, key string, fallback float64) float64 {
	raw := strings.TrimSpace(valorOpcion(options, key))
	if raw == "" {
		return fallback
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		return v
	}
	return fallback
}

func valorBoolOpcionPAdES(options map[string]string, key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(valorOpcion(options, key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "si", "sí", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// ErrPDFCifrado indica que el PDF usa un cifrado que no se puede conservar.
var ErrPDFCifrado = errors.New("el PDF usa un cifrado que no se puede conservar al firmarlo (por ejemplo, cifrado con certificado); " +
	"quite la protección desde su lector de PDF y vuelva a firmarlo")

// ErrPDFContrasena indica que falta la contraseña de apertura del PDF.
var ErrPDFContrasena = errors.New("el PDF está protegido con contraseña de apertura; la web debe indicarla (userPassword u ownerPassword) " +
	"o puede quitar la protección desde su lector de PDF")

// ErrPDFPermisos indica que el autor del PDF no permite añadir firmas.
var ErrPDFPermisos = errors.New("el autor del PDF no permite añadir anotaciones ni rellenar formularios, que es lo que hace una firma; " +
	"hace falta su contraseña de propietario (ownerPassword)")

// contrasenaPDF devuelve la contraseña indicada por la web, como en
// AutoFirma Java: la de propietario da todos los permisos.
func contrasenaPDF(options map[string]string) string {
	if p := valorOpcion(options, "ownerPassword"); p != "" {
		return p
	}
	return valorOpcion(options, "userPassword")
}

// abrirPDF abre el documento con la contraseña indicada, si la hay. Los
// pánicos del lector ante documentos malformados se convierten en error.
func abrirPDF(data []byte, options map[string]string) (r *pdf.Reader, err error) {
	defer func() {
		if p := recover(); p != nil {
			r, err = nil, fmt.Errorf("PDF no procesable: %v", p)
		}
	}()
	if pw := contrasenaPDF(options); pw != "" {
		return pdf.NewReaderConContrasena(bytes.NewReader(data), int64(len(data)), pw)
	}
	return pdf.NewReader(bytes.NewReader(data), int64(len(data)))
}

// comprobarPDFCifrado decide si un PDF cifrado puede firmarse conservando su
// protección y explica al usuario lo que se hace.
func comprobarPDFCifrado(data []byte, options map[string]string) error {
	r, err := abrirPDF(data, options)
	if err != nil {
		switch {
		case errors.Is(err, pdf.ErrInvalidPassword):
			return ErrPDFContrasena
		case strings.Contains(strings.ToLower(err.Error()), "encrypt"):
			return ErrPDFCifrado
		}
		return nil // otros defectos se notifican al firmar
	}
	c := r.Cifrado()
	if !c.Presente {
		return nil
	}
	permisos := uint32(c.P) // #nosec G115 -- bits de permisos.
	anotar := permisos&(1<<5) != 0
	rellenar := c.R >= 3 && permisos&(1<<8) != 0
	if !anotar && !rellenar && !c.Propietario {
		return ErrPDFPermisos
	}
	avisos.Registrar("PDF cifrado", "el PDF está cifrado; la firma se añade conservando su cifrado, su contraseña y sus permisos")
	return nil
}

// firmarConLector firma con el lector abierto con la contraseña del PDF, de
// modo que los objetos nuevos se cifran con la clave del documento.
func firmarConLector(inputPath, outputPath string, data []byte, options map[string]string, signData pdfsign.SignData) error {
	rdr, err := abrirPDF(data, options)
	if err != nil {
		return err
	}
	entrada, err := os.Open(inputPath) // #nosec G304 -- fichero temporal propio.
	if err != nil {
		return err
	}
	defer entrada.Close()
	salida, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- fichero temporal propio.
	if err != nil {
		return err
	}
	if err := pdfsign.Sign(entrada, salida, rdr, int64(len(data)), signData); err != nil {
		_ = salida.Close()
		return err
	}
	return salida.Close()
}

// girarImagenPNGLibre conserva la escala de la tarjeta y añade las esquinas
// transparentes de su caja envolvente.
func girarImagenPNGLibre(raw []byte, grados float64) ([]byte, error) {
	src, err := decodificarImagenSello(raw)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	r := grados * math.Pi / 180
	ancho := int(math.Ceil(float64(b.Dx())*math.Abs(math.Cos(r)) + float64(b.Dy())*math.Abs(math.Sin(r))))
	alto := int(math.Ceil(float64(b.Dx())*math.Abs(math.Sin(r)) + float64(b.Dy())*math.Abs(math.Cos(r))))
	if err := validarPixelesSello(ancho, alto, maxSealImageEdge, maxSealImagePixels); err != nil {
		return nil, err
	}
	girada := rotarImagenLibre(src, grados, ancho, alto)
	var out bytes.Buffer
	if err := png.Encode(&out, girada); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// PrevisualizarSello genera la imagen del sello tal como quedará en el PDF,
// para la vista previa del escritorio. Usa las mismas opciones que la firma
// (visibleSealRectW/H en puntos, rotation, visibleSealLogo, imagen, QR...).
func PrevisualizarSello(options map[string]string, firmante, emisor string, fecha time.Time) ([]byte, error) {
	if err := normalizarQROpciones(options); err != nil {
		return nil, err
	}
	w := valorFloatOpcionPAdES(options, "visibleSealRectW", 200)
	h := valorFloatOpcionPAdES(options, "visibleSealRectH", 70)
	if !numeroFinitoSello(w) || !numeroFinitoSello(h) || w <= 0 || h <= 0 || w > 14400 || h > 14400 {
		return nil, errors.New("el tamaño del sello no es válido")
	}
	info := pdfsign.SignDataSignatureInfo{Name: firmante, Reason: motivoPorDefectoPAdES, Date: fecha}
	if emisor = strings.TrimSpace(emisor); emisor != "" {
		info.Location = "Certificado: " + emisor
	}
	aplicarMetadatosFirmaPdfsign(&info, options)
	// La imagen propia llega ya en visibleSealImageBase64 (el IPC la lee
	// de disco con sus comprobaciones de ruta).
	return componerImagenSello(info, options, w, h)
}
