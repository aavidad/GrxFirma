// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package auditlog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

// Cadena de huellas del registro de auditoría: cada registro lleva en
// "cadena" la huella SHA-256 (hexadecimal) del registro anterior, también a
// través de la rotación. Alterar, insertar o borrar un registro intermedio
// rompe la cadena. Para detectar además que se ha recortado el final, el
// registro puede firmarse con el certificado del usuario (firma CAdES del
// fichero) en el momento de entregarlo.

// InicioCadena es el valor de "cadena" del primer registro.
var InicioCadena = strings.Repeat("0", 64)

const maxLecturaAuditoria = 32 * 1024 * 1024

// Huella devuelve la huella de un registro (sin el salto de línea).
func Huella(registro []byte) string {
	h := sha256.Sum256(registro)
	return hex.EncodeToString(h[:])
}

// Encadenar añade la huella del registro anterior a la evidencia. Si la
// evidencia es un objeto JSON se conserva tal cual, con "cadena" como primer
// campo; en otro caso se guarda como texto en "evidencia".
func Encadenar(payload []byte, anterior string) []byte {
	if anterior == "" {
		anterior = InicioCadena
	}
	cabecera := `{"cadena":"` + anterior + `"`
	p := bytes.TrimSpace(payload)
	var obj map[string]json.RawMessage
	if len(p) > 1 && p[0] == '{' && json.Unmarshal(p, &obj) == nil && !bytes.ContainsAny(p, "\r\n") {
		if _, repetido := obj["cadena"]; !repetido {
			resto := bytes.TrimSpace(p[1:])
			if len(resto) > 0 && resto[0] == '}' {
				return []byte(cabecera + "}")
			}
			return append([]byte(cabecera+","), resto...)
		}
	}
	texto, _ := json.Marshal(string(p))
	return []byte(cabecera + `,"evidencia":` + string(texto) + "}")
}

// huellaUltimoRegistro recupera la huella del último registro escrito.
func huellaUltimoRegistro(rutas ...string) (string, error) {
	for _, ruta := range rutas {
		data, err := leerRegistro(ruta)
		if err != nil {
			return "", err
		}
		lineas := lineasRegistro(data)
		if len(lineas) > 0 {
			return Huella(lineas[len(lineas)-1]), nil
		}
	}
	return "", nil
}

func leerRegistro(ruta string) ([]byte, error) {
	info, err := os.Lstat(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo inspeccionar el registro de auditoria: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("el registro de auditoria no es un fichero regular")
	}
	data, err := securefile.ReadFileLimit(ruta, maxLecturaAuditoria)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el registro de auditoria: %w", err)
	}
	return data, nil
}

func lineasRegistro(data []byte) [][]byte {
	var out [][]byte
	for _, l := range bytes.Split(data, []byte("\n")) {
		if l = bytes.TrimRight(l, "\r"); len(l) > 0 {
			out = append(out, l)
		}
	}
	return out
}

// InformeCadena resume la verificación de la cadena.
type InformeCadena struct {
	Registros int
	// InicioRetirado indica que el primer registro disponible encadena con
	// otros ya retirados por la política de retención.
	InicioRetirado bool
	// SinCadena cuenta los registros antiguos, anteriores a la cadena de
	// huellas, que se encuentran al principio del registro.
	SinCadena int
}

// ErrCadenaRota indica que el registro de auditoría se ha alterado.
var ErrCadenaRota = errors.New("la cadena de huellas del registro de auditoria está rota")

// VerificarCadena comprueba la cadena de huellas de los ficheros indicados,
// en orden cronológico (primero la rotación, después el activo).
func VerificarCadena(rutas ...string) (InformeCadena, error) {
	var (
		inf      InformeCadena
		anterior string
		enCadena bool
	)
	for _, ruta := range rutas {
		data, err := leerRegistro(ruta)
		if err != nil {
			return inf, err
		}
		for _, linea := range lineasRegistro(data) {
			var r struct {
				Cadena *string `json:"cadena"`
			}
			if err := json.Unmarshal(linea, &r); err != nil {
				return inf, fmt.Errorf("%w: el registro %d no es JSON válido", ErrCadenaRota, inf.Registros+1)
			}
			inf.Registros++
			if r.Cadena == nil {
				if enCadena {
					return inf, fmt.Errorf("%w: el registro %d no lleva la huella del anterior", ErrCadenaRota, inf.Registros)
				}
				inf.SinCadena++
				anterior = Huella(linea)
				continue
			}
			switch {
			case !enCadena && inf.SinCadena == 0:
				// Primer registro encadenado disponible.
				inf.InicioRetirado = *r.Cadena != InicioCadena
			case *r.Cadena != anterior:
				return inf, fmt.Errorf("%w: el registro %d no encadena con el %d (uno de los dos se ha alterado, o se han borrado o insertado registros entre ellos)", ErrCadenaRota, inf.Registros, inf.Registros-1)
			}
			enCadena = true
			anterior = Huella(linea)
		}
	}
	return inf, nil
}
