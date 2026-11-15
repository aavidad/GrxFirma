// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"errors"
	"strings"
)

// SignatureFormat representa el formato de firma electronica.
type SignatureFormat string

const (
	FormatCAdES SignatureFormat = "CAdES"
	FormatXAdES SignatureFormat = "XAdES"
	FormatPAdES SignatureFormat = "PAdES"
)

func (f SignatureFormat) Validate() error {
	if strings.TrimSpace(string(f)) == "" {
		return errors.New("formato de firma no puede estar vacio")
	}
	return nil
}

// SignatureAction representa la accion de firma a realizar.
type SignatureAction string

const (
	ActionSign        SignatureAction = "sign"
	ActionCoSign      SignatureAction = "cosign"
	ActionCounterSign SignatureAction = "countersign"
)

func (a SignatureAction) Validate() error {
	switch a {
	case ActionSign, ActionCoSign, ActionCounterSign:
		return nil
	default:
		return errors.New("accion de firma no soportada: " + string(a))
	}
}

// SignatureJob representa un trabajo de firma individual.
type SignatureJob struct {
	Document Document
	Format   SignatureFormat
	Action   SignatureAction
	// Options contiene parametros adicionales especificos del formato (nivel, politica, etc.).
	// Las claves son nombres de dominio neutros, no nombres de parametros de protocolo.
	Options map[string]string
	// OriginalContent es el documento que firmó una CAdES explícita (sin el
	// contenido dentro) al cofirmarla o contrafirmarla. Es opcional: si llega,
	// el motor lo coteja con la firma antes de añadir la suya y no firma a
	// ciegas (KD-03); sin él se sigue firmando con aviso, como AutoFirma.
	OriginalContent []byte
}

func (j SignatureJob) Validate() error {
	if err := j.Format.Validate(); err != nil {
		return err
	}
	if err := j.Action.Validate(); err != nil {
		return err
	}
	if j.Document.Size() == 0 {
		return errors.New("el trabajo de firma debe contener un documento no vacio")
	}
	return nil
}

// SignatureResult es el resultado de una operacion de firma exitosa.
type SignatureResult struct {
	// Warnings contiene claves de catálogo de los permisos usados en esta operación.
	Warnings  []string `json:"warnings,omitempty"`
	Format    SignatureFormat
	Data      []byte
	Algorithm string
}
