// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"encoding/json"
)

// campoSecreto es un campo JSON de texto cuyo valor no debe pasar por un
// string de Go (no se puede borrar). nombre y los identificadores son fijos
// del protocolo, nunca datos externos.
type campoSecreto struct {
	nombre string
	valor  []byte
}

// datoAuthSecreto es un elemento de authData (CSC 2.1) con valor secreto.
type datoAuthSecreto struct {
	id    string
	valor []byte
}

// cuerpoConSecretos serializa base (sin secretos) y añade antes de la llave
// final los campos secretos y, si los hay, el array authData. El resultado
// debe borrarse tras enviarlo.
func cuerpoConSecretos(base any, campos []campoSecreto, authData []datoAuthSecreto) ([]byte, error) {
	inicial, err := json.Marshal(base)
	if err != nil || len(inicial) < 2 || inicial[0] != '{' || inicial[len(inicial)-1] != '}' {
		return nil, nuevoError(CodigoParametroInvalido, "body", err)
	}
	tam := len(inicial) + 32
	for _, c := range campos {
		tam += len(c.nombre) + 6*len(c.valor) + 8
	}
	for _, d := range authData {
		tam += len(d.id) + 6*len(d.valor) + 24
	}
	// Se reserva de una vez para que append no deje copias intermedias.
	cuerpo := make([]byte, 0, tam+16)
	cuerpo = append(cuerpo, inicial[:len(inicial)-1]...)
	vacio := len(inicial) == 2
	separar := func() {
		if !vacio {
			cuerpo = append(cuerpo, ',')
		}
		vacio = false
	}
	for _, c := range campos {
		separar()
		cuerpo = anadirCadenaJSON(cuerpo, []byte(c.nombre))
		cuerpo = append(cuerpo, ':')
		cuerpo = anadirCadenaJSON(cuerpo, c.valor)
	}
	if len(authData) > 0 {
		separar()
		cuerpo = append(cuerpo, `"authData":[`...)
		for i, d := range authData {
			if i > 0 {
				cuerpo = append(cuerpo, ',')
			}
			cuerpo = append(cuerpo, `{"id":`...)
			cuerpo = anadirCadenaJSON(cuerpo, []byte(d.id))
			cuerpo = append(cuerpo, `,"value":`...)
			cuerpo = anadirCadenaJSON(cuerpo, d.valor)
			cuerpo = append(cuerpo, '}')
		}
		cuerpo = append(cuerpo, ']')
	}
	return append(cuerpo, '}'), nil
}

const hexadecimal = "0123456789abcdef"

// anadirCadenaJSON escribe v como cadena JSON escapando comillas, barra
// inversa y caracteres de control. Los bytes UTF-8 se copian tal cual.
func anadirCadenaJSON(dst, v []byte) []byte {
	dst = append(dst, '"')
	for _, b := range v {
		switch {
		case b == '"' || b == '\\':
			dst = append(dst, '\\', b)
		case b < 0x20:
			dst = append(dst, '\\', 'u', '0', '0', hexadecimal[b>>4], hexadecimal[b&0xf])
		default:
			dst = append(dst, b)
		}
	}
	return append(dst, '"')
}

// anadirPorcentaje codifica v para application/x-www-form-urlencoded sin
// crear strings intermedios.
func anadirPorcentaje(dst, v []byte) []byte {
	const hexMayus = "0123456789ABCDEF"
	for _, b := range v {
		switch {
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9', b == '-', b == '.', b == '_', b == '~':
			dst = append(dst, b)
		default:
			dst = append(dst, '%', hexMayus[b>>4], hexMayus[b&0xf])
		}
	}
	return dst
}
