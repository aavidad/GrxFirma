// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// Confianza que el usuario puede forzar tras un aviso (orden de Alberto,
// 13/11/2026: «que sea el usuario el que fuerce tras mensaje de warning»).
//
// Al verificar, cinco obstáculos dejan hoy la firma no válida aunque el
// documento no haya cambiado: una CA que el sistema no conoce, una clave RSA
// corta, un certificado de servidor usado para firmar, una revocación que no
// se pudo comprobar y SHA-1. El usuario, y solo él en su equipo (CLI o
// escritorio), puede aceptar cada uno de forma explícita. Lo aceptado se
// guarda en su config.json y la firma sale válida, siempre con aviso,
// evidencia y registro en la auditoría.
//
// Nada más se puede forzar: integridad, firma criptográfica, revocado,
// caducado, MD5, cobertura o envolturas siguen decidiendo por su cuenta.

// TipoConfianzaForzada identifica cada obstáculo que el usuario puede aceptar.
type TipoConfianzaForzada string

const (
	// ConfianzaCADesconocida: la cadena no llega a una raíz de confianza.
	// Se acepta por la huella de la raíz o del último certificado de la
	// cadena construida.
	ConfianzaCADesconocida TipoConfianzaForzada = "ca_desconocida"
	// ConfianzaClaveRSACorta: clave RSA de menos de 2048 bits. Se acepta por
	// la huella del certificado con la clave corta.
	ConfianzaClaveRSACorta TipoConfianzaForzada = "clave_rsa_corta"
	// ConfianzaCertificadoComponente: certificado de servidor (EKU solo
	// serverAuth o serverAuth+clientAuth) usado para firmar. Se acepta por la
	// huella del certificado firmante.
	ConfianzaCertificadoComponente TipoConfianzaForzada = "certificado_componente"
	// ConfianzaRevocacionNoComprobable: interruptor global para la revocación
	// no concluyente (sin red, CRL no aplicable, cadena incompleta).
	ConfianzaRevocacionNoComprobable TipoConfianzaForzada = "revocacion_no_comprobable"
	// ConfianzaAlgoritmoSHA1: interruptor global para firmas con SHA-1 cuando
	// es el único obstáculo.
	ConfianzaAlgoritmoSHA1 TipoConfianzaForzada = "algoritmo_sha1"
)

// EvidenciaConfianzaForzada marca en el resultado que la firma se da por
// válida porque el usuario lo aceptó. Summary lleva el tipo.
const EvidenciaConfianzaForzada = "trust.user_forced"

// Claves de catálogo de los textos de aviso. Los textos viven en los
// catálogos de idioma; aquí solo se guardan las claves.
const (
	prefijoClaveAvisoConfianza    = "verify.forced_trust.notice."
	prefijoClaveAplicadaConfianza = "verify.forced_trust.applied."
	// ClavePistaConfianzaForzable se añade al motivo cuando el usuario puede
	// aceptar el obstáculo en la configuración.
	ClavePistaConfianzaForzable = "verify.forced_trust.hint"
	// ClaveConfianzaForzadaProhibida se añade cuando la política de máquina
	// impide usar lo aceptado (también es el texto de ErrConfianzaProhibida).
	ClaveConfianzaForzadaProhibida = "verify.forced_trust.forbidden"
)

var tiposConfianzaForzada = []TipoConfianzaForzada{
	ConfianzaCADesconocida,
	ConfianzaClaveRSACorta,
	ConfianzaCertificadoComponente,
	ConfianzaRevocacionNoComprobable,
	ConfianzaAlgoritmoSHA1,
}

// TiposConfianzaForzada devuelve los tipos en orden estable.
func TiposConfianzaForzada() []TipoConfianzaForzada {
	return append([]TipoConfianzaForzada(nil), tiposConfianzaForzada...)
}

// Valido indica si el tipo es uno de los cinco que se pueden aceptar.
func (t TipoConfianzaForzada) Valido() bool {
	for _, conocido := range tiposConfianzaForzada {
		if t == conocido {
			return true
		}
	}
	return false
}

// PorCertificado indica si el tipo se acepta para un certificado concreto
// (con huella) y no como interruptor global.
func (t TipoConfianzaForzada) PorCertificado() bool {
	switch t {
	case ConfianzaCADesconocida, ConfianzaClaveRSACorta, ConfianzaCertificadoComponente:
		return true
	default:
		return false
	}
}

// ClaveAviso es la clave del texto que se muestra antes de aceptar.
func (t TipoConfianzaForzada) ClaveAviso() string { return prefijoClaveAvisoConfianza + string(t) }

// ClaveAplicada es la clave del aviso que acompaña a una firma dada por
// válida gracias a lo aceptado.
func (t TipoConfianzaForzada) ClaveAplicada() string {
	return prefijoClaveAplicadaConfianza + string(t)
}

// NormalizarHuellaConfianza deja la huella SHA-256 en hexadecimal en
// minúsculas, sin separadores. Devuelve "" si no es una huella SHA-256.
func NormalizarHuellaConfianza(raw string) string {
	limpia := strings.ToLower(strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(raw)))
	if len(limpia) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(limpia); err != nil {
		return ""
	}
	return limpia
}

// ObstaculoConfianza es un obstáculo forzable hallado al verificar.
type ObstaculoConfianza struct {
	Tipo   TipoConfianzaForzada
	Huella string
	Sujeto string
}

// clave identifica el obstáculo: tipo y, si aplica, huella.
func (o ObstaculoConfianza) clave() string {
	if o.Tipo.PorCertificado() {
		return string(o.Tipo) + "|" + o.Huella
	}
	return string(o.Tipo)
}

// AceptacionConfianza es una entrada aceptada por el usuario.
type AceptacionConfianza struct {
	Tipo       TipoConfianzaForzada `json:"tipo"`
	Huella     string               `json:"huella,omitempty"`
	Sujeto     string               `json:"sujeto,omitempty"`
	AceptadaEn time.Time            `json:"aceptada_en"`
	// Aviso es la clave del texto de aviso que se mostró al aceptar.
	Aviso string `json:"aviso"`
}

// Errores de validación de una aceptación. El texto es la clave del
// catálogo: los adaptadores la traducen.
var (
	ErrConfianzaTipoDesconocido = errors.New("verify.forced_trust.error.unknown_type")
	ErrConfianzaHuellaInvalida  = errors.New("verify.forced_trust.error.fingerprint")
	ErrConfianzaHuellaSobrante  = errors.New("verify.forced_trust.error.global_type")
	ErrConfianzaProhibida       = errors.New("verify.forced_trust.forbidden")
)

// Normalizar valida la entrada y la deja en su forma canónica.
func (a AceptacionConfianza) Normalizar() (AceptacionConfianza, error) {
	if !a.Tipo.Valido() {
		return AceptacionConfianza{}, ErrConfianzaTipoDesconocido
	}
	if a.Tipo.PorCertificado() {
		huella := NormalizarHuellaConfianza(a.Huella)
		if huella == "" {
			return AceptacionConfianza{}, ErrConfianzaHuellaInvalida
		}
		a.Huella = huella
	} else {
		if strings.TrimSpace(a.Huella) != "" {
			return AceptacionConfianza{}, ErrConfianzaHuellaSobrante
		}
		a.Huella = ""
	}
	a.Sujeto = limitarTextoConfianza(a.Sujeto, 512)
	a.Aviso = a.Tipo.ClaveAviso()
	a.AceptadaEn = a.AceptadaEn.UTC().Truncate(time.Second)
	return a, nil
}

func limitarTextoConfianza(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// ConfianzaForzada es lo que el usuario tiene aceptado y si la política de
// máquina lo prohíbe.
type ConfianzaForzada struct {
	Aceptaciones []AceptacionConfianza
	Prohibida    bool
}

// Acepta indica si el obstáculo está aceptado y la política lo permite.
func (c ConfianzaForzada) Acepta(o ObstaculoConfianza) bool {
	if c.Prohibida {
		return false
	}
	clave := o.clave()
	for _, a := range c.Aceptaciones {
		if (ObstaculoConfianza{Tipo: a.Tipo, Huella: a.Huella}).clave() == clave {
			return true
		}
	}
	return false
}

// RegistroConfianzaForzada viaja en el contexto de una verificación pedida
// por el usuario en su equipo. Los verificadores consultan en él lo aceptado
// y apuntan lo que han aceptado y lo que han encontrado. Sin registro en el
// contexto (REST, afirma://, host nativo, móvil) no se acepta nada.
type RegistroConfianzaForzada struct {
	mu         sync.Mutex
	confianza  ConfianzaForzada
	aplicadas  []ObstaculoConfianza
	obstaculos []ObstaculoConfianza
	memo       *memoVerificacion
}

type memoVerificacion struct {
	mu      sync.Mutex
	valores map[string]any
}

// NuevoRegistroConfianzaForzada crea el registro de una verificación.
func NuevoRegistroConfianzaForzada(c ConfianzaForzada) *RegistroConfianzaForzada {
	return &RegistroConfianzaForzada{
		confianza: c,
		memo:      &memoVerificacion{valores: map[string]any{}},
	}
}

// Derivar crea un registro para repetir la verificación con otra confianza,
// compartiendo la memoria de consultas de red (revocación) del original.
func (r *RegistroConfianzaForzada) Derivar(c ConfianzaForzada) *RegistroConfianzaForzada {
	nuevo := NuevoRegistroConfianzaForzada(c)
	if r != nil && r.memo != nil {
		nuevo.memo = r.memo
	}
	return nuevo
}

// Decidir apunta el obstáculo y devuelve true si el usuario lo tiene aceptado
// (y la política lo permite). Con registro nil devuelve false: no se fuerza
// nada fuera de la verificación local del usuario.
func (r *RegistroConfianzaForzada) Decidir(o ObstaculoConfianza) bool {
	if r == nil || !o.Tipo.Valido() {
		return false
	}
	o.Huella = strings.ToLower(o.Huella)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.confianza.Acepta(o) {
		r.aplicadas = anadirObstaculo(r.aplicadas, o)
		return true
	}
	r.obstaculos = anadirObstaculo(r.obstaculos, o)
	return false
}

func anadirObstaculo(lista []ObstaculoConfianza, o ObstaculoConfianza) []ObstaculoConfianza {
	for _, existente := range lista {
		if existente.clave() == o.clave() {
			return lista
		}
	}
	return append(lista, o)
}

// Aplicadas devuelve los obstáculos que se dieron por buenos por estar
// aceptados.
func (r *RegistroConfianzaForzada) Aplicadas() []ObstaculoConfianza {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ObstaculoConfianza(nil), r.aplicadas...)
}

// Obstaculos devuelve los obstáculos forzables hallados y no aceptados.
func (r *RegistroConfianzaForzada) Obstaculos() []ObstaculoConfianza {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ObstaculoConfianza(nil), r.obstaculos...)
}

// Confianza devuelve lo aceptado con lo que trabaja el registro.
func (r *RegistroConfianzaForzada) Confianza() ConfianzaForzada {
	if r == nil {
		return ConfianzaForzada{}
	}
	return r.confianza
}

// Recordar guarda el resultado de una consulta externa durante la
// verificación (y su repetición), para no repetirla. Sin registro, calcula.
func (r *RegistroConfianzaForzada) Recordar(clave string, calcular func() any) any {
	if r == nil || r.memo == nil {
		return calcular()
	}
	r.memo.mu.Lock()
	if v, ok := r.memo.valores[clave]; ok {
		r.memo.mu.Unlock()
		return v
	}
	r.memo.mu.Unlock()
	v := calcular()
	r.memo.mu.Lock()
	r.memo.valores[clave] = v
	r.memo.mu.Unlock()
	return v
}

// MarcarConfianzaForzada deja a la vista que la firma se da por válida porque
// el usuario aceptó el obstáculo: aviso, detalle y evidencia
// trust.user_forced=<tipo>. Es idempotente.
func (r VerificationResult) MarcarConfianzaForzada(o ObstaculoConfianza) VerificationResult {
	r.Warnings = anadirTextoUnico(r.Warnings, o.Tipo.ClaveAplicada())
	detalle := "confianza_forzada=" + string(o.Tipo)
	if o.Huella != "" {
		detalle += ":" + o.Huella
	}
	r.Details = anadirTextoUnico(r.Details, detalle)
	for _, evidencia := range r.Evidence {
		if evidencia.Type == EvidenciaConfianzaForzada && evidencia.Summary == string(o.Tipo) {
			return r
		}
	}
	r.Evidence = append(r.Evidence, VerificationEvidence{Type: EvidenciaConfianzaForzada, Summary: string(o.Tipo)})
	return r
}

func anadirTextoUnico(lista []string, valor string) []string {
	for _, existente := range lista {
		if existente == valor {
			return lista
		}
	}
	return append(lista, valor)
}

type registroConfianzaForzadaKey struct{}

// ConRegistroConfianzaForzada adjunta el registro al contexto. Solo lo usan
// los casos de uso de verificación local del usuario.
func ConRegistroConfianzaForzada(ctx context.Context, r *RegistroConfianzaForzada) context.Context {
	return context.WithValue(ctx, registroConfianzaForzadaKey{}, r)
}

// RegistroConfianzaForzadaDe recupera el registro del contexto (nil si no
// hay).
func RegistroConfianzaForzadaDe(ctx context.Context) *RegistroConfianzaForzada {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(registroConfianzaForzadaKey{}).(*RegistroConfianzaForzada)
	return r
}
