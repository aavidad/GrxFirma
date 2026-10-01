// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package sign

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" // #nosec G501 -- derivación de la clave de objeto que exige ISO 32000-1 §7.6.2 (algoritmo 1).
	"crypto/rand"
	"crypto/rc4" // #nosec G503 -- solo para documentos que ya usan RC4: se conserva su protección.
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/digitorus/pdf"
)

// Cifrado de los objetos que añade la firma a un PDF cifrado. En una
// actualización incremental los objetos nuevos deben cifrarse con la clave
// del documento (ISO 32000-1 §7.6): cadenas y flujos, con la clave de cada
// objeto. No se cifran el valor /Contents de la firma (§7.6.1, se rellena
// después con la firma CMS) ni el flujo de referencias cruzadas (§7.5.8.2).

type cifradorObjetos struct {
	p pdf.ParametrosCifrado
}

func nuevoCifrador(r *pdf.Reader) *cifradorObjetos {
	if r == nil {
		return nil
	}
	p := r.Cifrado()
	if !p.Presente || len(p.Clave) == 0 {
		return nil
	}
	return &cifradorObjetos{p: p}
}

// claveObjeto es el algoritmo 1 de §7.6.2 (en AES-256 se usa la del fichero).
func (c *cifradorObjetos) claveObjeto(id uint32) []byte {
	if c.p.AES256 {
		return c.p.Clave
	}
	h := md5.New() // #nosec G401 -- algoritmo de la norma PDF, no de integridad.
	h.Write(c.p.Clave)
	h.Write(binary.LittleEndian.AppendUint32(nil, id)[:3])
	h.Write([]byte{0, 0}) // generación 0: los objetos nuevos siempre la tienen
	if c.p.AES {
		h.Write([]byte("sAlT"))
	}
	clave := h.Sum(nil)
	if n := len(c.p.Clave) + 5; n < len(clave) {
		clave = clave[:n]
	}
	return clave
}

func (c *cifradorObjetos) cifrar(id uint32, datos []byte) ([]byte, error) {
	clave := c.claveObjeto(id)
	if !c.p.AES {
		s, err := rc4.NewCipher(clave) // #nosec G405 -- mantiene el cifrado RC4 del documento original.
		if err != nil {
			return nil, err
		}
		out := make([]byte, len(datos))
		s.XORKeyStream(out, datos)
		return out, nil
	}
	b, err := aes.NewCipher(clave)
	if err != nil {
		return nil, err
	}
	relleno := aes.BlockSize - len(datos)%aes.BlockSize
	plano := append(append([]byte{}, datos...), bytes.Repeat([]byte{byte(relleno)}, relleno)...)
	// Vector inicial aleatorio por cadena o flujo (§7.6.2), delante del cifrado.
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	out := make([]byte, len(plano))
	cipher.NewCBCEncrypter(b, iv).CryptBlocks(out, plano)
	return append(iv, out...), nil
}

// cifrarObjeto cifra las cadenas y el flujo de un objeto serializado.
func (c *cifradorObjetos) cifrarObjeto(id uint32, obj []byte) ([]byte, error) {
	if bytes.Contains(obj, []byte("/Type /XRef")) {
		return obj, nil
	}
	esFirma := bytes.Contains(obj, []byte("/Type /Sig\n")) || bytes.Contains(obj, []byte("/Type /DocTimeStamp"))
	var (
		out                         bytes.Buffer
		profundidad                 int
		ultimoNombre                string
		inicioLongitud, finLongitud = -1, -1
		esperaLongitud              bool
	)
	for i := 0; i < len(obj); {
		ch := obj[i]
		switch {
		case ch == '<' && i+1 < len(obj) && obj[i+1] == '<':
			profundidad++
			out.WriteString("<<")
			i += 2
		case ch == '>' && i+1 < len(obj) && obj[i+1] == '>':
			profundidad--
			out.WriteString(">>")
			i += 2
		case ch == '<':
			fin := bytes.IndexByte(obj[i:], '>')
			if fin < 0 {
				return nil, errors.New("cadena hexadecimal sin cerrar")
			}
			crudo := obj[i : i+fin+1]
			i += fin + 1
			if esFirma && ultimoNombre == "Contents" {
				out.Write(crudo) // el hueco de la firma CMS no se cifra
				continue
			}
			datos, err := decodificarHex(crudo[1 : len(crudo)-1])
			if err != nil {
				return nil, err
			}
			if err := c.escribirCadena(&out, id, datos); err != nil {
				return nil, err
			}
			esperaLongitud = false
		case ch == '(':
			datos, n, err := decodificarLiteral(obj[i:])
			if err != nil {
				return nil, err
			}
			i += n
			if err := c.escribirCadena(&out, id, datos); err != nil {
				return nil, err
			}
			esperaLongitud = false
		case ch == '/':
			j := i + 1
			for j < len(obj) && !esDelimitadorPDF(obj[j]) {
				j++
			}
			ultimoNombre = string(obj[i+1 : j])
			esperaLongitud = profundidad == 1 && ultimoNombre == "Length"
			out.Write(obj[i:j])
			i = j
		case profundidad == 0 && bytes.HasPrefix(obj[i:], []byte("stream")) && (i == 0 || esBlancoPDF(obj[i-1]) || obj[i-1] == '>'):
			return c.cifrarFlujo(id, obj, i, out.Bytes(), inicioLongitud, finLongitud)
		case esperaLongitud && ch >= '0' && ch <= '9':
			j := i
			for j < len(obj) && obj[j] >= '0' && obj[j] <= '9' {
				j++
			}
			inicioLongitud, finLongitud = out.Len(), out.Len()+(j-i)
			out.Write(obj[i:j])
			i = j
			esperaLongitud = false
		default:
			if !esBlancoPDF(ch) {
				esperaLongitud = esperaLongitud && ch == ' '
			}
			out.WriteByte(ch)
			i++
		}
	}
	return out.Bytes(), nil
}

func (c *cifradorObjetos) escribirCadena(out *bytes.Buffer, id uint32, datos []byte) error {
	cifrada, err := c.cifrar(id, datos)
	if err != nil {
		return err
	}
	out.WriteByte('<')
	out.WriteString(hex.EncodeToString(cifrada))
	out.WriteByte('>')
	return nil
}

// cifrarFlujo cifra los datos entre "stream" y "endstream" y corrige /Length.
func (c *cifradorObjetos) cifrarFlujo(id uint32, obj []byte, pos int, cabecera []byte, iniLong, finLong int) ([]byte, error) {
	inicio := pos + len("stream")
	switch {
	case bytes.HasPrefix(obj[inicio:], []byte("\r\n")):
		inicio += 2
	case bytes.HasPrefix(obj[inicio:], []byte("\n")):
		inicio++
	default:
		return nil, errors.New("flujo sin salto de línea tras stream")
	}
	fin := bytes.LastIndex(obj, []byte("endstream"))
	if fin < inicio {
		return nil, errors.New("flujo sin endstream")
	}
	datosFin := fin
	if datosFin > inicio && obj[datosFin-1] == '\n' {
		datosFin--
		if datosFin > inicio && obj[datosFin-1] == '\r' {
			datosFin--
		}
	}
	if iniLong < 0 {
		return nil, errors.New("flujo sin /Length directo")
	}
	cifrado, err := c.cifrar(id, obj[inicio:datosFin])
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Write(cabecera[:iniLong])
	out.WriteString(strconv.Itoa(len(cifrado)))
	out.Write(cabecera[finLong:])
	out.WriteString("stream\n")
	out.Write(cifrado)
	out.WriteString("\n")
	out.Write(obj[fin:])
	return out.Bytes(), nil
}

func esBlancoPDF(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func esDelimitadorPDF(c byte) bool {
	return esBlancoPDF(c) || bytes.IndexByte([]byte("()<>[]{}/%"), c) >= 0
}

func decodificarHex(h []byte) ([]byte, error) {
	limpio := make([]byte, 0, len(h))
	for _, c := range h {
		if !esBlancoPDF(c) {
			limpio = append(limpio, c)
		}
	}
	if len(limpio)%2 == 1 {
		limpio = append(limpio, '0')
	}
	out := make([]byte, len(limpio)/2)
	if _, err := hex.Decode(out, limpio); err != nil {
		return nil, fmt.Errorf("cadena hexadecimal no válida: %w", err)
	}
	return out, nil
}

// decodificarLiteral interpreta una cadena literal "(...)" (§7.3.4.2) y
// devuelve sus bytes y la longitud consumida.
func decodificarLiteral(b []byte) ([]byte, int, error) {
	var out []byte
	nivel := 0
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch c {
		case '(':
			if nivel > 0 {
				out = append(out, c)
			}
			nivel++
		case ')':
			nivel--
			if nivel == 0 {
				return out, i + 1, nil
			}
			out = append(out, c)
		case '\\':
			i++
			if i >= len(b) {
				return nil, 0, errors.New("cadena literal sin cerrar")
			}
			switch e := b[i]; e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if i+1 < len(b) && b[i+1] == '\n' {
					i++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && i+1 < len(b) && b[i+1] >= '0' && b[i+1] <= '7'; k++ {
						i++
						v = v*8 + int(b[i]-'0')
					}
					out = append(out, byte(v&0xff))
				} else {
					out = append(out, e)
				}
			}
		case '\r':
			// Un fin de línea sin escapar equivale a un salto de línea.
			if i+1 < len(b) && b[i+1] == '\n' {
				i++
			}
			out = append(out, '\n')
		default:
			out = append(out, c)
		}
	}
	return nil, 0, errors.New("cadena literal sin cerrar")
}
