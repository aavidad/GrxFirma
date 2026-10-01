// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package legacycrypto centraliza la política de primitivas criptográficas
// heredadas que todavía pueden ser necesarias para interoperar con V1.9.
package legacycrypto

import (
	"errors"
	"net/url"
	"strings"

	"grxfirma/internal/security/avisos"
	"grxfirma/internal/security/machinepolicy"
)

const EnvEnableLegacyDES = "GRXFIRMA_ENABLE_LEGACY_DES"

var ErrDESDisabled = errors.New(
	"el fallback DES del protocolo heredado esta desactivado; " +
		"si el portal aun exige el formato de sesion de V1.9, un administrador " +
		"debe habilitar la politica de maquina " + machinepolicy.PermitirDESLegacy)

// DESEnabled solo permite DES mediante una decisión explícita del operador.
func DESEnabled() bool {
	// Solo la política de máquina (administrador) puede habilitar DES; la
	// variable de entorno EnvEnableLegacyDES ya no rebaja la seguridad.
	return machinepolicy.OptIn(machinepolicy.PermitirDESLegacy)
}

func RequireDES() error {
	if !DESEnabled() {
		return ErrDESDisabled
	}
	return nil
}

// RequireDESEnIntercambio autoriza DES solo para el paquete de sesión del
// servidor intermedio de AutoFirma 1.x (StorageService/RetrieveService, que
// usan FIRe y AutoScript 1.9). Además de la política de máquina, se admite
// cuando todos los servidores del intercambio son HTTPS: la confidencialidad
// real la da TLS y la clave del protocolo (8 dígitos) no es más débil con DES
// que con otro cifrado. Nunca se usa DES para firmar ni fuera de este
// intercambio, y el uso se explica al usuario.
func RequireDESEnIntercambio(endpoints ...string) error {
	if DESEnabled() {
		return nil
	}
	hosts, ok := endpointsHTTPS(endpoints)
	if !ok {
		return ErrDESDisabled
	}
	avisos.Registrar(
		"Compatibilidad con AutoFirma 1.x",
		"el portal usa el servidor intermedio heredado ("+strings.Join(hosts, ", ")+
			"), que cifra el intercambio con DES y una clave de 8 dígitos. Se ha permitido solo para ese "+
			"intercambio porque va cifrado por HTTPS; la firma se genera con algoritmos seguros",
	)
	return nil
}

func endpointsHTTPS(endpoints []string) ([]string, bool) {
	var hosts []string
	for _, raw := range endpoints {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
			return nil, false
		}
		hosts = append(hosts, u.Hostname())
	}
	return hosts, len(hosts) > 0
}
