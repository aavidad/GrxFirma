// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2

// Package excepciones centraliza las rebajas de seguridad expresas. Ninguna
// se activa al migrar una configuración ni mediante variables de entorno.
package excepciones

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"grxfirma/internal/adapters/outbound/common/settingsfile"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
	"grxfirma/internal/security/machinepolicy"
)

const UserField = settingsfile.UserField

type Exception struct {
	ID                string `json:"id"`
	UserKey           string `json:"user_key"`
	PolicyKey         string `json:"policy_key"`
	WarningKey        string `json:"warning_key"`
	BlockKey          string `json:"block_key"`
	SettingKey        string `json:"setting_key"`
	TitleKey          string `json:"title_key"`
	RiskKey           string `json:"risk_key"`
	AcceptanceKey     string `json:"acceptance_key"`
	HelpKey           string `json:"help_key"`
	OrganizationKey   string `json:"organization_key"`
	AcceptanceVersion string `json:"acceptance_version"`
}

var catalog = buildCatalog()

func buildCatalog() []Exception {
	ids := []string{"cms_cifrado_heredado", "cms_cbc", "cms_rsa_v15_crear", "cms_rsa_v15_leer", "cms_aes_ecb", "afp_aad_ambigua", "alto_clave_fichero_crear", "alto_clave_fichero_leer", "ca_clave_fichero", "sha1_crear", "sha1_leer", "sha1_conservar", "xml_c14n_antigua", "pdf_cifrado_heredado", "rsa_corta", "des_intercambio", "tsa_http", "proxy_http", "trifasica_http", "ca_instalacion_automatica", "cofirma_sin_original", "origen_vacio", "rutas_directas", "aprobacion_automatica", "llamante_desarrollo", "pkcs11_experimental"}
	legacy := map[string]string{
		"des_intercambio":       machinepolicy.PermitirDESLegacy,
		"sha1_crear":            machinepolicy.PermitirSHA1Legacy,
		"sha1_conservar":        machinepolicy.PermitirSHA1Legacy,
		"cms_aes_ecb":           machinepolicy.PermitirCMSAESECBLegacy,
		"origen_vacio":          machinepolicy.PermitirOrigenVacio,
		"rutas_directas":        machinepolicy.PermitirRutasDirectas,
		"aprobacion_automatica": machinepolicy.AprobacionAutomaticaHost,
	}
	out := make([]Exception, 0, len(ids))
	for _, id := range ids {
		policy := "permitir_" + id
		if name, ok := legacy[id]; ok {
			policy = name
		}
		prefix := "excepciones." + id
		out = append(out, Exception{ID: id, UserKey: UserField + "." + id, PolicyKey: policy, WarningKey: prefix + ".aviso", BlockKey: prefix + ".bloqueo", SettingKey: prefix + ".ajuste", TitleKey: prefix + ".titulo", RiskKey: prefix + ".riesgo", AcceptanceKey: prefix + ".aceptacion", HelpKey: prefix + ".ayuda", OrganizationKey: prefix + ".organizacion", AcceptanceVersion: AcceptanceVersion})
	}
	return out
}

func Catalog() []Exception { return append([]Exception(nil), catalog...) }
func Find(id string) (Exception, bool) {
	for _, e := range catalog {
		if e.ID == id {
			return e, true
		}
	}
	return Exception{}, false
}
func Warning(id string) string { e, _ := Find(id); return e.WarningKey }

// Block conserva ID y claves incluso cuando un adaptador envuelve el error.
type Block struct {
	ID                string `json:"id"`
	BlockKey          string `json:"block_key"`
	SettingKey        string `json:"setting_key"`
	PolicyKey         string `json:"policy_key"`
	TitleKey          string `json:"title_key"`
	RiskKey           string `json:"risk_key"`
	AcceptanceKey     string `json:"acceptance_key"`
	HelpKey           string `json:"help_key"`
	OrganizationKey   string `json:"organization_key"`
	AcceptanceVersion string `json:"acceptance_version"`
	Activable         bool   `json:"activable"`
	ConfigUnavailable bool   `json:"config_unavailable,omitempty"`
}

func (b *Block) Error() string {
	loc := localizador.Detectar()
	key := b.BlockKey
	if !b.Activable && b.OrganizationKey != "" {
		key = b.OrganizationKey
	}
	message := loc.T(key)
	if b.ConfigUnavailable {
		return loc.T("excepciones.directorio_no_disponible") + " [" + b.ID + "; " + b.BlockKey + "]"
	}
	if !b.Activable && b.OrganizationKey != "" {
		message = loc.T(b.RiskKey) + "\n" + message + " [" + b.ID + "; " + b.OrganizationKey + "]"
	}
	return message + " [" + b.ID + "; " + b.BlockKey + "]"
}
func blockForState(e Exception, s State) *Block {
	return &Block{ID: e.ID, BlockKey: e.BlockKey, SettingKey: e.SettingKey, PolicyKey: e.PolicyKey, TitleKey: e.TitleKey, RiskKey: e.RiskKey, AcceptanceKey: e.AcceptanceKey, HelpKey: e.HelpKey, OrganizationKey: e.OrganizationKey, AcceptanceVersion: AcceptanceVersion, Activable: s.Activable, ConfigUnavailable: s.ConfigUnavailable}
}
func blocked(e Exception) error { return Describe(e.ID) }

// Describe devuelve metadatos sin permitir activar una excepción por sí solo.
func Describe(id string) *Block {
	dir, err := configDir()
	e, ok := Find(id)
	if !ok {
		return nil
	}
	return blockForState(e, state(e, dir, err))
}
func DescribeAt(dir, id string) *Block {
	e, ok := Find(id)
	if !ok {
		return nil
	}
	return blockForState(e, state(e, dir, nil))
}

// FromText recupera el contrato en adaptadores antiguos que guardan Error().
// Solo reconoce la marca completa emitida por Block, nunca texto libre.
func FromText(message string, contexts ...context.Context) *Block {
	for _, e := range catalog {
		if strings.Contains(message, "["+e.ID+"; "+e.BlockKey+"]") {
			if len(contexts) > 0 && contexts[0] != nil {
				dir, err := directoryFor(contexts...)
				return blockForState(e, state(e, dir, err))
			}
			return blocked(e).(*Block)
		}
	}
	return nil
}

type State struct {
	Exception
	Activable         bool        `json:"activable"`
	LastAcceptance    *Acceptance `json:"last_acceptance,omitempty"`
	Enabled           bool        `json:"enabled"`
	Fixed             bool        `json:"fixed"`
	UserEnabled       bool        `json:"user_enabled"`
	PolicyUnreadable  bool        `json:"policy_unreadable"`
	ConfigUnavailable bool        `json:"config_unavailable,omitempty"`
}

var ErrConfigDirectory = errors.New("excepciones.directorio_no_disponible")

func validDirectory(dir string) bool {
	return dir != "" && filepath.IsAbs(dir) && filepath.Clean(dir) == dir && filepath.Dir(dir) != dir
}

func configDir() (string, error) { return configDirForOS(runtime.GOOS) }
func configDirForOS(goos string) (string, error) {
	if goos == "android" {
		return "", ErrConfigDirectory
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return "", ErrConfigDirectory
	}
	return appdirs.Config(home), nil
}

// List lee el estado efectivo, sin escribir ni crear preferencias.
func List() []State {
	dir, err := configDir()
	return listAt(dir, err)
}

func ListAt(dir string) []State { return listAt(dir, nil) }

func listAt(dir string, err error) []State {
	out := make([]State, 0, len(catalog))
	for _, e := range catalog {
		out = append(out, state(e, dir, err))
	}
	return out
}
func state(e Exception, dir string, dirErr error) State {
	s := State{Exception: e, Activable: true}
	if dirErr != nil || !validDirectory(dir) {
		s.Activable = false
		s.ConfigUnavailable = true
		return s
	}
	if dirErr == nil && dir != "" {
		s.LastAcceptance = readAcceptance(dir, e.ID)
	}
	value, present, err := machinepolicy.ReadOptIn(e.PolicyKey)
	if err != nil {
		s.Fixed = true
		s.PolicyUnreadable = true
		s.Activable = false
		return s
	}
	if e.ID == "cms_cifrado_heredado" || e.ID == "cms_cbc" || e.ID == "cms_aes_ecb" {
		prohibited, _, perr := machinepolicy.ReadOptIn(machinepolicy.ProhibirCifradoCMSHeredado)
		if perr != nil || prohibited {
			s.Fixed = true
			s.PolicyUnreadable = perr != nil
			s.Activable = false
			return s
		}
	}
	user, userErr := readUser(dir, e.ID)
	s.UserEnabled = user
	if present {
		s.Enabled = value
		s.Activable = value
		s.Fixed = true
		return s
	}
	s.Enabled = dirErr == nil && userErr == nil && user
	return s
}

type directoryKey struct{}

// WithConfigDir vincula una operación al directorio privado de la aplicación.
func WithConfigDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, directoryKey{}, dir)
}

func directoryFor(contexts ...context.Context) (string, error) {
	dir, err := configDir()
	if len(contexts) > 0 && contexts[0] != nil {
		if value, ok := contexts[0].Value(directoryKey{}).(string); ok {
			dir = value
			err = nil
		}
	}
	if !validDirectory(dir) {
		return "", ErrConfigDirectory
	}
	return dir, err
}
func Check(id string, contexts ...context.Context) error {
	e, ok := Find(id)
	if !ok {
		return fmt.Errorf("excepciones.id_desconocido: %s", id)
	}
	dir, err := directoryFor(contexts...)
	effective := state(e, dir, err)
	if !effective.Enabled {
		b := blockForState(e, effective)
		if len(contexts) > 0 {
			reportBlock(contexts[0], b)
		}
		return b
	}
	return nil
}
func Enabled(id string) bool { return Check(id) == nil }
func Require(ctx context.Context, id string) error {
	if err := Check(id, ctx); err != nil {
		return err
	}
	domain.AnotarAviso(ctx, Warning(id))
	avisos.Registrar(localizador.Detectar().T(Warning(id)), "")
	return nil
}

var testMu sync.RWMutex
var testUsers = map[string]bool{}

type testHandle interface {
	Helper()
	Cleanup(func())
}

// SetForTest sustituye solo el ajuste de usuario; la máquina sigue mandando.
func SetForTest(tb testHandle, id string, enabled bool) {
	tb.Helper()
	testMu.Lock()
	old, exists := testUsers[id]
	testUsers[id] = enabled
	testMu.Unlock()
	tb.Cleanup(func() {
		testMu.Lock()
		defer testMu.Unlock()
		if exists {
			testUsers[id] = old
		} else {
			delete(testUsers, id)
		}
	})
}

func readUser(dir, id string) (bool, error) {
	if !validDirectory(dir) {
		return false, ErrConfigDirectory
	}
	testMu.RLock()
	v, ok := testUsers[id]
	testMu.RUnlock()
	if ok {
		return v, nil
	}
	value := false
	for _, file := range []string{"config.json", "settings.json"} {
		raw, err := securefile.ReadFileLimit(filepath.Join(dir, file), 4<<20)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		var doc map[string]json.RawMessage
		if json.Unmarshal(raw, &doc) != nil || doc == nil {
			return false, ErrAcceptanceRequired
		}
		for key := range doc {
			if ReservedSettings(key) && key != UserField && key != AcceptanceField {
				return false, ErrAcceptanceRequired
			}
		}
		field, exists := doc[UserField]
		if !exists {
			continue
		}
		var values map[string]bool
		if err = json.Unmarshal(field, &values); err != nil {
			return false, err
		}
		enabled, present := values[id]
		if !present {
			continue
		}
		if !enabled {
			value = false
			continue
		}
		receipt := acceptanceFromDocument(doc, id)
		value = currentAcceptance(receipt, id)
	}
	return value, nil
}

var writeMu sync.Mutex

// SetUser persiste solo una elección expresa, conservando el resto del JSON.
// settings.json tiene precedencia por clave y recibe los cambios por IPC.
// Cada cambio se escribe atómicamente en un único fichero.
func SetUser(id string, enabled bool) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	return SetUserAt(dir, id, enabled)
}

func SetUserAt(dir, id string, enabled bool) error {
	if enabled {
		return ErrAcceptanceRequired
	}
	if !validDirectory(dir) {
		return ErrConfigDirectory
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	e, ok := Find(id)
	if !ok {
		return ErrAcceptanceRequired
	}
	return settingsfile.WithLock(context.Background(), dir, func() error {
		effective := state(e, dir, nil)
		if effective.Fixed {
			return blockForState(e, effective)
		}
		return writeUserSettings(context.Background(), dir, id, false, nil)
	})
}
