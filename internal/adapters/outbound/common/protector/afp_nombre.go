// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"mime"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// nombreDocumentoPorDefecto sustituye un nombre vacío o reducido a «.»/«..».
const nombreDocumentoPorDefecto = "documento"

// maxBytesNombreDesprotegido limita el nombre devuelto, holgado frente a los
// 255 bytes que admiten los sistemas de ficheros habituales.
const maxBytesNombreDesprotegido = 200

// extensionesDocumento son las extensiones que se conservan al desproteger.
// Cualquier otra (.hta, .lnk, .desktop, .exe...) se cambia por .bin para que
// el resultado no se ejecute con un doble clic.
var extensionesDocumento = map[string]bool{
	".pdf": true, ".xml": true, ".txt": true, ".csv": true, ".json": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	".odt": true, ".ods": true, ".odp": true, ".odg": true, ".rtf": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".tif": true, ".tiff": true,
	".p7m": true, ".p7s": true, ".csig": true, ".xsig": true, ".asice": true, ".asics": true,
	".zip": true, ".bin": true,
}

// nombreDesprotegidoSeguro sanea el nombre que trae un .afp. El sobre no
// autentica al emisor (F-08): se queda solo el último componente, sin
// caracteres de control ni de formato (como U+202E), sin los reservados de
// Windows y con una extensión de documento conocida o .bin.
func nombreDesprotegidoSeguro(nombre string) string {
	nombre = path.Base(strings.ReplaceAll(nombre, `\`, "/"))
	var b strings.Builder
	for _, r := range nombre {
		switch {
		case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			continue
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	// Windows ignora los puntos y espacios finales: «x.hta.» abriría x.hta.
	nombre = strings.TrimRight(strings.TrimSpace(b.String()), ". ")
	if nombre == "" || nombre == "." || nombre == ".." {
		nombre = nombreDocumentoPorDefecto
	}
	ext := path.Ext(nombre)
	if !extensionesDocumento[strings.ToLower(ext)] {
		nombre = strings.TrimSuffix(nombre, ext) + ".bin"
		ext = ".bin"
	}
	if len(nombre) > maxBytesNombreDesprotegido {
		base := strings.TrimSuffix(nombre, ext)
		limite := maxBytesNombreDesprotegido - len(ext)
		for limite > 0 && !utf8.RuneStart(base[limite]) {
			limite--
		}
		nombre = base[:limite] + ext
	}
	return nombre
}

// mimeDesprotegidoSeguro valida el tipo MIME que trae un .afp y lo devuelve
// en forma canónica, o application/octet-stream si no es válido.
func mimeDesprotegidoSeguro(tipo string) string {
	const porDefecto = "application/octet-stream"
	mediaType, params, err := mime.ParseMediaType(tipo)
	if err != nil {
		return porDefecto
	}
	if canonico := mime.FormatMediaType(mediaType, params); canonico != "" {
		return canonico
	}
	return porDefecto
}
