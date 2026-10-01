// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "errors"

// BatchJob representa una coleccion de trabajos de firma que comparten una misma sesion de intercambio.
type BatchJob struct {
	Jobs    []SignatureJob
	Session ExchangeSession
}

func (b BatchJob) Validate() error {
	if len(b.Jobs) == 0 {
		return errors.New("el lote de firma debe contener al menos un trabajo")
	}
	for i, job := range b.Jobs {
		if err := job.Validate(); err != nil {
			return errors.New("trabajo " + itoa(i) + " del lote no valido: " + err.Error())
		}
	}
	return nil
}

// itoa es una conversion minima para evitar importar fmt o strconv en el dominio.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
