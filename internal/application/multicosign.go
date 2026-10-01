// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/domain"
)

// SignExecutor abstracta la firma simple ya existente para reutilizarla en una
// secuencia guiada sin acoplar la orquestación a un adaptador concreto.
type SignExecutor interface {
	Execute(ctx context.Context, cmd SignCommand) (SignResult, error)
}

// MultiCoSignUseCase orquesta una firma/cofirma secuencial sobre un único
// documento usando varios certificados y devolviendo una única salida final.
type MultiCoSignUseCase struct {
	firmador SignExecutor
}

func NuevoMultiCoSignUseCase(firmador SignExecutor) *MultiCoSignUseCase {
	return &MultiCoSignUseCase{firmador: firmador}
}

func (uc *MultiCoSignUseCase) Execute(ctx context.Context, cmd MultiCoSignCommand) (SignResult, error) {
	if uc == nil || uc.firmador == nil {
		return SignResult{}, errors.New("caso de uso de cofirma múltiple no configurado")
	}
	if err := cmd.Format.Validate(); err != nil {
		return SignResult{}, err
	}
	if !soportaCofirmaMultiple(cmd.Format) {
		return SignResult{}, fmt.Errorf(
			"la cofirma múltiple guiada no soporta el formato %s; use PAdES, ODF u OOXML",
			cmd.Format,
		)
	}
	if err := cmd.InitialAction.Validate(); err != nil {
		return SignResult{}, err
	}
	if cmd.InitialAction == domain.ActionCounterSign {
		return SignResult{}, errors.New("la cofirma múltiple guiada no soporta contrafirma")
	}
	if cmd.Document.Size() == 0 {
		return SignResult{}, errors.New("el documento no puede estar vacio")
	}
	if strings.TrimSpace(cmd.PrimaryCertificateID) == "" {
		return SignResult{}, errors.New("debe seleccionar un certificado principal")
	}

	ids := compactarCertificados(cmd.PrimaryCertificateID, cmd.AdditionalCertificateIDs)
	if len(ids) < 2 {
		return SignResult{}, errors.New("debe seleccionar al menos un cofirmante distinto del certificado principal")
	}
	currentDoc := cmd.Document
	currentAction := cmd.InitialAction
	currentOptions := cloneStringMap(cmd.Options)
	var last SignResult

	for idx, certID := range ids {
		signCmd := SignCommand{
			Document:      currentDoc,
			Format:        cmd.Format,
			Action:        currentAction,
			CertificateID: certID,
			Options:       cloneStringMap(currentOptions),
		}
		result, err := uc.firmador.Execute(ctx, signCmd)
		if err != nil {
			return SignResult{}, err
		}
		last = result
		if idx == len(ids)-1 {
			break
		}
		nextDoc, err := domain.NewDocument(currentDoc.Name, result.Result.Data, currentDoc.MIMEType)
		if err != nil {
			return SignResult{}, err
		}
		currentDoc = nextDoc
		currentAction = domain.ActionCoSign
		currentOptions = stripVisualOptions(currentOptions)
	}

	return last, nil
}

func soportaCofirmaMultiple(format domain.SignatureFormat) bool {
	switch format {
	case domain.FormatPAdES, domain.SignatureFormat("ODF"), domain.SignatureFormat("OOXML"):
		return true
	default:
		return false
	}
}

func compactarCertificados(primary string, additional []string) []string {
	out := make([]string, 0, 1+len(additional))
	seen := map[string]struct{}{}
	push := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	push(primary)
	for _, id := range additional {
		push(id)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// En las cofirmas posteriores evitamos duplicar sellos visibles o QR
// superpuestos. La primera operación conserva todas las opciones.
func stripVisualOptions(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := cloneStringMap(in)
	visual := map[string]struct{}{
		"visibleseal":              {},
		"visiblesealrectx":         {},
		"visiblesealrecty":         {},
		"visiblesealrectw":         {},
		"visiblesealrecth":         {},
		"page":                     {},
		"rotation":                 {},
		"visiblesealkeeptext":      {},
		"visiblesealimagepath":     {},
		"visiblesealimagebase64":   {},
		"visiblesealsignersummary": {},
		"qrcontent":                {},
		"visiblesealqrcontent":     {},
	}
	for key := range out {
		if _, ok := visual[strings.ToLower(strings.TrimSpace(key))]; ok {
			delete(out, key)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
