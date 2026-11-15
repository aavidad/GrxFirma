// SPDX-License-Identifier: EUPL-1.2
package triphase

import (
	"errors"
	"strings"
	"unicode/utf16"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

func javaPropertyWhitespace(b byte) bool { return b == ' ' || b == '\t' || b == '\f' }
func trimJavaPropertyStart(s string) string {
	i := 0
	for i < len(s) && javaPropertyWhitespace(s[i]) {
		i++
	}
	return s[i:]
}

// readJavaProperties solo inspecciona. Conserva pares repetidos para que
// ninguna variante de clave o firma oculte otra antes de enviar el original.
// Properties.load(InputStream) interpreta los bytes sin escape como Latin-1.
func readJavaProperties(body string) ([][2]string, error) {
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(body, "\n")
	var pairs [][2]string
	for n := 0; n < len(lines); n++ {
		line := trimJavaPropertyStart(lines[n])
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		var logical strings.Builder
		for {
			slashes := 0
			for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
				slashes++
			}
			continued := slashes%2 == 1
			if continued {
				line = line[:len(line)-1]
			}
			logical.WriteString(line)
			if !continued || n+1 == len(lines) {
				break
			}
			n++
			line = trimJavaPropertyStart(lines[n])
		}
		text := logical.String()
		// Una continuación vacía seguida de otra línea no crea una propiedad.
		if text == "" && n+1 < len(lines) {
			continue
		}
		keyEnd := len(text)
		escaped := false
		for i := 0; i < len(text); i++ {
			b := text[i]
			if !escaped && (b == '=' || b == ':' || javaPropertyWhitespace(b)) {
				keyEnd = i
				break
			}
			if b == '\\' {
				escaped = !escaped
			} else {
				escaped = false
			}
		}
		valueStart := keyEnd
		if valueStart < len(text) && javaPropertyWhitespace(text[valueStart]) {
			for valueStart < len(text) && javaPropertyWhitespace(text[valueStart]) {
				valueStart++
			}
		}
		if valueStart < len(text) && (text[valueStart] == '=' || text[valueStart] == ':') {
			valueStart++
		}
		for valueStart < len(text) && javaPropertyWhitespace(text[valueStart]) {
			valueStart++
		}
		key, err := decodeJavaProperty(text[:keyEnd])
		if err != nil {
			return nil, err
		}
		value, err := decodeJavaProperty(text[valueStart:])
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]string{key, value})
	}
	return pairs, nil
}

func decodeJavaProperty(raw string) (string, error) {
	units := make([]uint16, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		if b != '\\' {
			units = append(units, uint16(b))
			continue
		}
		i++
		if i == len(raw) {
			break
		}
		b = raw[i]
		switch b {
		case 't':
			units = append(units, '\t')
		case 'n':
			units = append(units, '\n')
		case 'r':
			units = append(units, '\r')
		case 'f':
			units = append(units, '\f')
		case 'u':
			if i+4 >= len(raw) {
				return "", javaPropertyUnicodeError()
			}
			var value uint16
			for j := 0; j < 4; j++ {
				i++
				c := raw[i]
				var digit byte
				switch {
				case c >= '0' && c <= '9':
					digit = c - '0'
				case c >= 'a' && c <= 'f':
					digit = c - 'a' + 10
				case c >= 'A' && c <= 'F':
					digit = c - 'A' + 10
				default:
					return "", javaPropertyUnicodeError()
				}
				value = value*16 + uint16(digit)
			}
			units = append(units, value)
		default:
			units = append(units, uint16(b))
		}
	}
	return string(utf16.Decode(units)), nil
}
func javaPropertyUnicodeError() error {
	return errors.New(localizador.Detectar().T("triphase.properties.unicode_invalido"))
}
