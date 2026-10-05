// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/security/machinepolicy"
)

const (
	ficheroConfig      = "config.json"
	ficheroPolicy      = "policy.json"
	maxConfigFileBytes = 4 * 1024 * 1024

	// DirPolicyDefecto es el directorio de política de organización en el sistema.
	DirPolicyDefecto = "/etc/grxfirma"
)

// Config contiene la configuración completa de GrxFirma.
// Los valores por defecto garantizan el modelo Zero Server: sin puertos abiertos al arrancar.
type Config struct {
	// Red — desactivados por defecto (modelo Zero Server)
	WebsocketHabilitado bool `json:"websocket_habilitado"`
	// WebsocketPermitido corta tanto el servicio residente como las sesiones
	// temporales abiertas por afirma://. Por defecto se conserva la compatibilidad.
	WebsocketPermitido *bool `json:"websocket_permitido,omitempty"`
	RestHabilitado     bool  `json:"rest_habilitado"`

	// Confianza
	TofuHabilitado bool `json:"tofu_habilitado"`

	// Certificados
	DirectorioP12 string `json:"directorio_p12"`

	// Dominios administrados/compatibilidad. No se evalúan directamente:
	// deben entrar siempre en truststore.NewWithOptions(... ExtraAllowed).
	DominiosDeConfianza []string `json:"dominios_de_confianza"`

	// Logging
	NivelLog string `json:"nivel_log"`

	// Límites operativos
	TimeoutOperacionSegundos int   `json:"timeout_operacion_segundos"`
	MaxTamanoDocumentoBytes  int64 `json:"max_tamano_documento_bytes"`

	// FirmaRemotaCSC activa el prototipo de firma con certificados remotos
	// (CSC API) cuando no hay política. Desactivado por defecto; no admite
	// variable de entorno.
	FirmaRemotaCSC bool `json:"firma_remota_csc"`
	// FirmaRemotaCSCOAuth lista pares "servicio=oauth" (host[:puerto]) que
	// autorizan un servidor OAuth en otro host que el servicio CSC.
	FirmaRemotaCSCOAuth []string `json:"firma_remota_csc_oauth_permitidos"`
}

// Policy representa únicamente los overrides definidos explícitamente en
// policy.json. Se usa cuando una capa necesita distinguir valores fijados por
// organización de la configuración agregada del usuario.
type Policy struct {
	WebsocketHabilitado      *bool    `json:"websocket_habilitado"`
	WebsocketPermitido       *bool    `json:"websocket_permitido"`
	RestHabilitado           *bool    `json:"rest_habilitado"`
	TofuHabilitado           *bool    `json:"tofu_habilitado"`
	DirectorioP12            *string  `json:"directorio_p12"`
	DominiosDeConfianza      []string `json:"dominios_de_confianza"`
	NivelLog                 *string  `json:"nivel_log"`
	TimeoutOperacionSegundos *int     `json:"timeout_operacion_segundos"`
	MaxTamanoDocumentoBytes  *int64   `json:"max_tamano_documento_bytes"`
	// FirmaRemotaCSC decide la firma remota CSC por encima del usuario:
	// true la permite y false la prohíbe.
	FirmaRemotaCSC *bool `json:"firma_remota_csc"`
	// FirmaRemotaCSCOAuth sustituye la lista de pares del usuario.
	FirmaRemotaCSCOAuth []string `json:"firma_remota_csc_oauth_permitidos"`
}

// Default devuelve la configuración con valores seguros por defecto.
// Estos valores están codificados en Go y no dependen de ningún fichero.
func Default() Config {
	permitido := true
	return Config{
		WebsocketHabilitado:      false,
		WebsocketPermitido:       &permitido,
		RestHabilitado:           false,
		TofuHabilitado:           true,
		DirectorioP12:            "",
		DominiosDeConfianza:      []string{},
		NivelLog:                 "info",
		TimeoutOperacionSegundos: 30,
		MaxTamanoDocumentoBytes:  52428800, // 50 MB
		FirmaRemotaCSCOAuth:      []string{},
	}
}

// PermiteWebSocket conserva el comportamiento anterior en configuraciones
// construidas en memoria que no incluyen aún este ajuste.
func (cfg Config) PermiteWebSocket() bool {
	return cfg.WebsocketPermitido == nil || *cfg.WebsocketPermitido
}

// Load carga la configuración aplicando la jerarquía completa de precedencia,
// usando /etc/grxfirma/policy.json como directorio de política del sistema.
// Si el fichero de usuario no existe, se crea con los valores por defecto.
func Load(userConfigDir string) (Config, error) {
	return LoadWithPolicyDir(userConfigDir, DirPolicyDefecto)
}

// LoadWithPolicyDir es igual que Load pero permite inyectar el directorio de política
// del sistema (útil para tests sin acceso a /etc).
func LoadWithPolicyDir(userConfigDir, policyDir string) (Config, error) {
	cfg := Default()

	// Capa 3: fichero de usuario (~/.config/grxfirma/config.json)
	if err := cargarFicheroUsuario(filepath.Join(userConfigDir, ficheroConfig), &cfg); err != nil {
		return cfg, fmt.Errorf("config: fichero de usuario: %w", err)
	}

	// Capa 2: variables de entorno GRXFIRMA_* (sobreescriben el fichero)
	aplicarEntorno(&cfg)

	// Capa 1: política de organización — máxima precedencia
	aplicarPolicyDesde(&cfg, policyDir)

	return cfg, nil
}

// LoadPolicy carga el contenido bruto de policy.json sin mezclarlo con config
// del usuario ni variables de entorno. Si el fichero no existe, devuelve una
// política vacía y nil.
func LoadPolicy(policyDir string) (Policy, error) {
	return loadSystemPolicy(policyDir)
}

// loadSystemPolicy lee la política de organización. En Windows, la ruta por
// defecto (/etc/grxfirma) se resolvería como C:\etc, que puede crear
// cualquier usuario: allí la política procede exclusivamente del registro
// HKLM\SOFTWARE\Policies\GrxFirma, escribible solo por administradores.
func loadSystemPolicy(policyDir string) (Policy, error) {
	if policyDir == DirPolicyDefecto && machinepolicy.Native() {
		return loadMachinePolicy()
	}
	return loadPolicyFile(filepath.Join(policyDir, ficheroPolicy))
}

// lectorPoliticaMaquina abstrae el registro para poder probar la lectura
// fuera de Windows.
type lectorPoliticaMaquina struct {
	boolean func(string) (bool, bool, error)
	texto   func(string) (string, bool, error)
	textos  func(string) ([]string, bool, error)
	entero  func(string) (int64, bool, error)
}

var lectorRegistro = lectorPoliticaMaquina{
	boolean: machinepolicy.Bool,
	texto:   machinepolicy.String,
	textos:  machinepolicy.Strings,
	entero:  machinepolicy.Int64,
}

func loadMachinePolicy() (Policy, error) {
	return loadMachinePolicyDesde(lectorRegistro)
}

func loadMachinePolicyDesde(lector lectorPoliticaMaquina) (Policy, error) {
	var p Policy
	var errs []error
	leerBool := func(nombre string, destino **bool) {
		v, ok, err := lector.boolean(nombre)
		if err != nil {
			errs = append(errs, err)
		} else if ok {
			*destino = &v
		}
	}
	leerTexto := func(nombre string, destino **string) {
		v, ok, err := lector.texto(nombre)
		if err != nil {
			errs = append(errs, err)
		} else if ok {
			*destino = &v
		}
	}
	leerBool(machinepolicy.WebsocketHabilitado, &p.WebsocketHabilitado)
	leerBool(machinepolicy.WebsocketPermitido, &p.WebsocketPermitido)
	leerBool(machinepolicy.RestHabilitado, &p.RestHabilitado)
	leerBool(machinepolicy.TofuHabilitado, &p.TofuHabilitado)
	leerTexto(machinepolicy.DirectorioP12, &p.DirectorioP12)
	leerTexto(machinepolicy.NivelLog, &p.NivelLog)
	if v, ok, err := lector.textos(machinepolicy.DominiosDeConfianza); err != nil {
		errs = append(errs, err)
	} else if ok {
		p.DominiosDeConfianza = v
	}
	if v, ok, err := lector.entero(machinepolicy.TimeoutOperacionSegundos); err != nil {
		errs = append(errs, err)
	} else if ok {
		n := int(v)
		p.TimeoutOperacionSegundos = &n
	}
	leerBool(machinepolicy.FirmaRemotaCSC, &p.FirmaRemotaCSC)
	if v, ok, err := lector.textos(machinepolicy.FirmaRemotaCSCOAuth); err != nil {
		errs = append(errs, err)
	} else if ok {
		p.FirmaRemotaCSCOAuth = v
	}
	if v, ok, err := lector.entero(machinepolicy.MaxTamanoDocumentoBytes); err != nil {
		errs = append(errs, err)
	} else if ok {
		p.MaxTamanoDocumentoBytes = &v
	}
	return p, errors.Join(errs...)
}

// cargarFicheroUsuario carga el fichero JSON en dst.
// Si no existe, crea el fichero con el contenido actual de dst (primer arranque).
func cargarFicheroUsuario(path string, dst *Config) error {
	data, err := securefile.ReadFileLimit(path, maxConfigFileBytes)
	if os.IsNotExist(err) {
		return crearFichero(path, *dst)
	}
	if err != nil {
		return fmt.Errorf("leer %s: %w", path, err)
	}
	if err := json.Unmarshal(sinBOM(data), dst); err != nil {
		return fmt.Errorf("parsear %s: %w", path, err)
	}
	return nil
}

// crearFichero escribe cfg como JSON en path, creando los directorios necesarios.
func crearFichero(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("crear directorio %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar config: %w", err)
	}
	if err := securefile.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("escribir %s: %w", path, err)
	}
	return nil
}

// aplicarEntorno sobreescribe campos de cfg con variables de entorno GRXFIRMA_*.
// Los valores inválidos (no parseables) se ignoran sin producir error.
func aplicarEntorno(cfg *Config) {
	if v := os.Getenv("GRXFIRMA_WEBSOCKET_HABILITADO"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.WebsocketHabilitado = b
		}
	}
	if v := os.Getenv("GRXFIRMA_WEBSOCKET_PERMITIDO"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.WebsocketPermitido = &b
		}
	}
	if v := os.Getenv("GRXFIRMA_REST_HABILITADO"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.RestHabilitado = b
		}
	}
	if v := os.Getenv("GRXFIRMA_TOFU_HABILITADO"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.TofuHabilitado = b
		}
	}
	if v := os.Getenv("GRXFIRMA_DIRECTORIO_P12"); v != "" {
		cfg.DirectorioP12 = v
	}
	if v := os.Getenv("GRXFIRMA_NIVEL_LOG"); v != "" {
		cfg.NivelLog = v
	}
	if v := os.Getenv("GRXFIRMA_TIMEOUT_OPERACION_SEGUNDOS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.TimeoutOperacionSegundos = n
		}
	}
	if v := os.Getenv("GRXFIRMA_MAX_TAMANO_DOCUMENTO_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.MaxTamanoDocumentoBytes = n
		}
	}
}

func loadPolicyFile(path string) (Policy, error) {
	var raw Policy
	data, err := securefile.ReadFileLimit(path, maxConfigFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return raw, nil
		}
		return raw, fmt.Errorf("leer %s: %w", path, err)
	}
	if err := json.Unmarshal(sinBOM(data), &raw); err != nil {
		return raw, fmt.Errorf("parsear %s: %w", path, err)
	}
	return raw, nil
}

// aplicarPolicy aplica los campos definidos en policy.json sobre cfg.
// Si el fichero no existe o no es legible, no modifica cfg (no bloquea el arranque).
func aplicarPolicyDesde(cfg *Config, policyDir string) {
	raw, err := loadSystemPolicy(policyDir)
	aplicarPolicyCargada(cfg, raw, err)
}

func aplicarPolicyCargada(cfg *Config, raw Policy, err error) {
	if err != nil {
		return // fichero ausente, ilegible o invalido: no aplicar
	}
	if raw.WebsocketHabilitado != nil {
		cfg.WebsocketHabilitado = *raw.WebsocketHabilitado
	}
	if raw.WebsocketPermitido != nil {
		cfg.WebsocketPermitido = raw.WebsocketPermitido
	}
	if raw.RestHabilitado != nil {
		cfg.RestHabilitado = *raw.RestHabilitado
	}
	if raw.TofuHabilitado != nil {
		cfg.TofuHabilitado = *raw.TofuHabilitado
	}
	if raw.DirectorioP12 != nil {
		cfg.DirectorioP12 = *raw.DirectorioP12
	}
	if raw.DominiosDeConfianza != nil {
		cfg.DominiosDeConfianza = raw.DominiosDeConfianza
	}
	if raw.NivelLog != nil {
		cfg.NivelLog = *raw.NivelLog
	}
	if raw.TimeoutOperacionSegundos != nil {
		cfg.TimeoutOperacionSegundos = *raw.TimeoutOperacionSegundos
	}
	if raw.MaxTamanoDocumentoBytes != nil {
		cfg.MaxTamanoDocumentoBytes = *raw.MaxTamanoDocumentoBytes
	}
	if raw.FirmaRemotaCSC != nil {
		cfg.FirmaRemotaCSC = *raw.FirmaRemotaCSC
	}
	if raw.FirmaRemotaCSCOAuth != nil {
		cfg.FirmaRemotaCSCOAuth = raw.FirmaRemotaCSCOAuth
	}
}

// FirmaRemotaCSCActiva decide si se puede usar la firma remota CSC. Si la
// política de la organización (policy.json o, en Windows, la política de
// máquina) fija el valor, manda ella. Sin política, solo config.json la
// activa: ninguna opción de la línea de órdenes ni variable de entorno.
func (cfg Config) FirmaRemotaCSCActiva(politica Policy) bool {
	if politica.FirmaRemotaCSC != nil {
		return *politica.FirmaRemotaCSC
	}
	return cfg.FirmaRemotaCSC
}

// sinBOM quita la marca UTF-8 inicial que añaden el Bloc de notas antiguo y
// PowerShell 5 (Set-Content -Encoding utf8); encoding/json la rechaza.
func sinBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
}
