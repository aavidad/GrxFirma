// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"context"
	"strings"
	"sync"
)

// Claves de catálogo de los avisos que el motor da al firmar cuando sigue
// adelante por compatibilidad. Cada interfaz las traduce con su catálogo.
const (
	// AvisoCofirmaSinOriginal: se ha cofirmado o contrafirmado una CAdES
	// explícita sin ver su documento (KD-03).
	AvisoCofirmaSinOriginal = "firma.cofirma_sin_original.aviso"
	// AvisoCRLNoAplicable: en una firma LT se ha incrustado una CRL que no
	// cubre a un certificado de la firma (lote 14-5).
	AvisoCRLNoAplicable = "firma.lt.crl_no_aplicable.aviso"
)

const maxAvisosOperacion = 16

// AvisosOperacion recoge las claves de los avisos de una sola operación de
// firma. Va en el contexto, así que no se mezcla con otras operaciones que
// corran a la vez (a diferencia de internal/security/avisos, que es global
// del proceso). Es seguro usarlo desde varias goroutines.
type AvisosOperacion struct {
	mu     sync.Mutex
	claves []string
	// padre es el registro de la operación que contiene a esta (por ejemplo,
	// la petición IPC que llama al caso de uso de firma). Lo anotado aquí se
	// anota también allí, para que ninguna capa pierda un aviso.
	padre *AvisosOperacion
}

type avisosOperacionKey struct{}

// ConAvisosOperacion devuelve un contexto con un registro de avisos nuevo y
// vacío para la operación que empieza. Si el contexto ya traía uno, el nuevo
// le pasa también lo que se anote.
func ConAvisosOperacion(ctx context.Context) (context.Context, *AvisosOperacion) {
	if ctx == nil {
		ctx = context.Background()
	}
	padre, _ := ctx.Value(avisosOperacionKey{}).(*AvisosOperacion)
	avisos := &AvisosOperacion{padre: padre}
	return context.WithValue(ctx, avisosOperacionKey{}, avisos), avisos
}

// AnotarAviso añade la clave al registro de la operación en curso. Sin
// registro en el contexto no hace nada: quien llama debe dejar también su
// propia traza en el registro de la aplicación.
func AnotarAviso(ctx context.Context, clave string) {
	if ctx == nil {
		return
	}
	avisos, _ := ctx.Value(avisosOperacionKey{}).(*AvisosOperacion)
	avisos.anotar(clave)
}

func (a *AvisosOperacion) anotar(clave string) {
	clave = strings.TrimSpace(clave)
	if a == nil || clave == "" {
		return
	}
	a.mu.Lock()
	repetida := false
	for _, existente := range a.claves {
		if existente == clave {
			repetida = true
			break
		}
	}
	if !repetida && len(a.claves) < maxAvisosOperacion {
		a.claves = append(a.claves, clave)
	}
	padre := a.padre
	a.mu.Unlock()
	padre.anotar(clave)
}

// Claves devuelve una copia de las claves anotadas, en orden de llegada.
func (a *AvisosOperacion) Claves() []string {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.claves) == 0 {
		return nil
	}
	return append([]string(nil), a.claves...)
}

// UnirAvisos añade a destino las claves de origen que aún no tiene.
func UnirAvisos(destino []string, origen ...string) []string {
	for _, clave := range origen {
		repetida := false
		for _, existente := range destino {
			if existente == clave {
				repetida = true
				break
			}
		}
		if !repetida && strings.TrimSpace(clave) != "" {
			destino = append(destino, clave)
		}
	}
	return destino
}
