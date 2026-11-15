// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2

// Package excepciones centraliza las rebajas de seguridad expresas. Ninguna
// se activa al migrar una configuración ni mediante variables de entorno.
package excepciones

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

const UserField = "excepciones_inseguras"

type Exception struct {
	ID         string `json:"id"`
	UserKey    string `json:"user_key"`
	PolicyKey  string `json:"policy_key"`
	WarningKey string `json:"warning_key"`
	BlockKey   string `json:"block_key"`
	SettingKey string `json:"setting_key"`
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
		out = append(out, Exception{ID: id, UserKey: UserField + "." + id, PolicyKey: policy, WarningKey: prefix + ".aviso", BlockKey: prefix + ".bloqueo", SettingKey: prefix + ".ajuste"})
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
	ID         string
	BlockKey   string
	SettingKey string
	PolicyKey  string
}

func (b *Block) Error() string {
	return localizador.Detectar().T(b.BlockKey) + " [" + b.ID + "; " + b.BlockKey + "]"
}
func blocked(e Exception) error {
	return &Block{ID: e.ID, BlockKey: e.BlockKey, SettingKey: e.SettingKey, PolicyKey: e.PolicyKey}
}

// FromText recupera el contrato en adaptadores antiguos que guardan Error().
// Solo reconoce la marca completa emitida por Block, nunca texto libre.
func FromText(message string) *Block {
	for _, e := range catalog {
		if strings.Contains(message, "["+e.ID+"; "+e.BlockKey+"]") {
			return blocked(e).(*Block)
		}
	}
	return nil
}

type State struct {
	Exception
	Enabled          bool `json:"enabled"`
	Fixed            bool `json:"fixed"`
	UserEnabled      bool `json:"user_enabled"`
	PolicyUnreadable bool `json:"policy_unreadable"`
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
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
	s := State{Exception: e}
	value, present, err := machinepolicy.ReadOptIn(e.PolicyKey)
	if err != nil {
		s.Fixed = true
		s.PolicyUnreadable = true
		return s
	}
	if e.ID == "cms_cifrado_heredado" || e.ID == "cms_cbc" || e.ID == "cms_aes_ecb" {
		prohibited, _, perr := machinepolicy.ReadOptIn(machinepolicy.ProhibirCifradoCMSHeredado)
		if perr != nil || prohibited {
			s.Fixed = true
			s.PolicyUnreadable = perr != nil
			return s
		}
	}
	user, userErr := readUser(dir, e.ID)
	s.UserEnabled = user
	if present {
		s.Enabled = value
		s.Fixed = true
		return s
	}
	s.Enabled = dirErr == nil && userErr == nil && user
	return s
}

type directoryKey struct{}

// WithConfigDir vincula una operación al directorio privado de la aplicación.
func WithConfigDir(ctx context.Context, dir string) context.Context {
	if dir == "" {
		return ctx
	}
	return context.WithValue(ctx, directoryKey{}, dir)
}

func Check(id string, contexts ...context.Context) error {
	e, ok := Find(id)
	if !ok {
		return fmt.Errorf("excepciones.id_desconocido: %s", id)
	}
	dir, err := configDir()
	if len(contexts) > 0 && contexts[0] != nil {
		if value, ok := contexts[0].Value(directoryKey{}).(string); ok {
			dir = value
			err = nil
		}
	}
	if !state(e, dir, err).Enabled {
		return blocked(e)
	}
	return nil
}
func Enabled(id string) bool { return Check(id) == nil }
func Require(ctx context.Context, id string) error {
	if err := Check(id, ctx); err != nil {
		return err
	}
	domain.AnotarAviso(ctx, Warning(id))
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
		if err = json.Unmarshal(raw, &doc); err != nil {
			return false, err
		}
		if field, exists := doc[UserField]; exists {
			var values map[string]bool
			if err = json.Unmarshal(field, &values); err != nil {
				return false, err
			}
			if b, exists := values[id]; exists {
				value = b
			}
		}
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
	e, ok := Find(id)
	if !ok {
		return fmt.Errorf("excepciones.id_desconocido: %s", id)
	}
	s := state(e, dir, nil)
	if s.Fixed {
		return blocked(e)
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, file := range []string{"settings.json"} {
		path := filepath.Join(dir, file)
		doc := map[string]json.RawMessage{}
		raw, err := securefile.ReadFileLimit(path, 4<<20)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &doc); err != nil {
				return err
			}
		}
		values := map[string]bool{}
		if data, ok := doc[UserField]; ok {
			if err = json.Unmarshal(data, &values); err != nil {
				return err
			}
		}
		values[id] = enabled
		doc[UserField], err = json.Marshal(values)
		if err != nil {
			return err
		}
		raw, err = json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		if err = securefile.WriteFileAtomic(path, append(raw, '\n'), 0600); err != nil {
			return err
		}
	}
	return nil
}
