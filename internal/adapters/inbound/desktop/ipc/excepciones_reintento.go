// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/security/excepciones"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const unsafeRetryTTL = 5 * time.Minute
const unsafeRetryMaxBytes = 4 << 20
const unsafeRetryLimit = 4

type unsafeRetryOwnerKey struct{}
type unsafeRetryFilesKey struct{}
type unsafeRetry struct {
	params      peticion
	files       map[string][]byte
	owner       string
	exceptionID string
	expires     time.Time
	activated   bool
	stored      bool
	timer       *time.Timer
	preferences [32]byte
}

func withUnsafeRetryOwner(ctx context.Context) context.Context {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ctx
	}
	return context.WithValue(ctx, unsafeRetryOwnerKey{}, hex.EncodeToString(b[:]))
}
func unsafeRetryOwner(ctx context.Context) string {
	v, _ := ctx.Value(unsafeRetryOwnerKey{}).(string)
	return v
}
func (r *unsafeRetry) clear() {
	if r == nil {
		return
	}
	for _, b := range r.files {
		secmem.Zeroize(b)
	}
	secmem.Zeroize(r.params.Params)
	r.files = nil
	r.params.Params = nil
}
func (r *unsafeRetry) clearUnlessStored() {
	if r != nil && !r.stored {
		r.clear()
	}
}
func (m *Manejador) prepareUnsafeRetry(ctx context.Context, p peticion) *unsafeRetry {
	if ctx.Value(unsafeRetryFilesKey{}) != nil {
		return nil
	}
	if !esInterfazVerificada(ctx) || unsafeRetryOwner(ctx) == "" || len(p.Params) > 64<<10 {
		return nil
	}
	obj, err := strictTokenSettingsObject(p.Params)
	if err != nil {
		return nil
	}
	var paths []string
	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "verify":
		for key := range obj {
			switch key {
			case "inputPath", "originalPath", "reportLanguage":
			default:
				return nil
			}
		}
		var v paramsVerify
		if json.Unmarshal(p.Params, &v) != nil {
			return nil
		}
		paths = []string{v.InputPath}
		if v.OriginalPath != "" {
			paths = append(paths, v.OriginalPath)
		}
	case "sign":
		for key := range obj {
			switch key {
			case "inputPath", "certificateId", "certificateIndex", "format", "action", "returnSignatureB64", "saveToDisk", "outputPath", "extraOptions", "allowInvalidPDF", "strictCompat", "reason", "location", "contactInfo", "qrContent", "additionalCertificateIds", "originalPath", "overwrite", "overwriteConfirmed", "visibleSeal":
			default:
				return nil
			}
		}
		var v paramsFirma
		if json.Unmarshal(p.Params, &v) != nil {
			return nil
		}
		// Sin contraseñas, certificados por índice, firmas remotas ni opciones
		// opacas: esos recorridos deben reenviarse desde la interfaz.
		if v.SaveToDisk || v.OutputPath != "" || v.OverwriteConfirmed || !v.ReturnSignatureB64 {
			return nil
		}
		if v.CertificateID == "" || v.CertificateID != strings.TrimSpace(v.CertificateID) {
			return nil
		}
		if m.CSC != nil {
			if _, remote := m.CSC.Credencial(v.CertificateID); remote {
				return nil
			}
		}
		if v.CertificateID == "" || v.OriginalPath != "" || len(v.AdditionalCertificateIDs) > 0 || len(v.VisibleSeal) > 0 || (v.Action != "" && v.Action != "sign") {
			return nil
		}
		for key := range v.ExtraOptions {
			switch strings.ToLower(key) {
			case "algorithm", "mode", "profile", "container":
			default:
				return nil
			}
		}
		paths = []string{v.InputPath}
	default:
		return nil
	}
	r := &unsafeRetry{params: peticion{Action: strings.ToLower(strings.TrimSpace(p.Action)), Params: append(json.RawMessage(nil), p.Params...)}, files: map[string][]byte{}, owner: unsafeRetryOwner(ctx), expires: time.Now().Add(unsafeRetryTTL)}
	r.preferences, err = unsafePreferenceDigest(m.configDirSeguro())
	if err != nil {
		r.clear()
		return nil
	}
	total := 0
	for _, path := range paths {
		if validarRutaLectura(path) != nil {
			r.clear()
			return nil
		}
		b, err := securefile.ReadFileLimit(path, int64(unsafeRetryMaxBytes-total))
		if err != nil {
			r.clear()
			return nil
		}
		total += len(b)
		r.files[path] = b
	}
	return r
}
func leerFicheroReintentoIPC(ctx context.Context, path string) ([]byte, error) {
	if files, ok := ctx.Value(unsafeRetryFilesKey{}).(map[string][]byte); ok {
		if b, exists := files[path]; exists {
			return append([]byte(nil), b...), nil
		}
	}
	return leerFicheroSeguroIPC(path)
}
func (m *Manejador) storeUnsafeRetry(ctx context.Context, r *unsafeRetry, id string) string {
	if r == nil || !esInterfazVerificada(ctx) {
		return ""
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ""
	}
	token := hex.EncodeToString(random[:])
	m.unsafeRetryMu.Lock()
	defer m.unsafeRetryMu.Unlock()
	if m.unsafeRetries == nil {
		m.unsafeRetries = map[string]*unsafeRetry{}
	}
	if len(m.unsafeRetries) >= unsafeRetryLimit {
		return ""
	}
	r.exceptionID = id
	r.stored = true
	m.unsafeRetries[token] = r
	r.timer = time.AfterFunc(unsafeRetryTTL, func() {
		m.unsafeRetryMu.Lock()
		defer m.unsafeRetryMu.Unlock()
		if m.unsafeRetries[token] == r {
			delete(m.unsafeRetries, token)
			r.clear()
		}
	})
	return token
}
func (m *Manejador) retryMatches(ctx context.Context, token, id string) bool {
	m.unsafeRetryMu.Lock()
	defer m.unsafeRetryMu.Unlock()
	r := m.unsafeRetries[token]
	return esInterfazVerificada(ctx) && r != nil && r.owner == unsafeRetryOwner(ctx) && r.exceptionID == id && time.Now().Before(r.expires)
}
func (m *Manejador) activateUnsafeRetry(ctx context.Context, token string) {
	m.unsafeRetryMu.Lock()
	defer m.unsafeRetryMu.Unlock()
	if r := m.unsafeRetries[token]; r != nil && r.owner == unsafeRetryOwner(ctx) {
		r.activated = true
	}
}
func (m *Manejador) clearUnsafeRetries(ctx context.Context) {
	if m == nil {
		return
	}
	owner := unsafeRetryOwner(ctx)
	m.unsafeRetryMu.Lock()
	defer m.unsafeRetryMu.Unlock()
	for token, r := range m.unsafeRetries {
		if r.owner == owner {
			delete(m.unsafeRetries, token)
			r.timer.Stop()
			r.clear()
		}
	}
}
func (m *Manejador) handleUnsafeRetry(ctx context.Context, raw json.RawMessage) respuesta {
	const action = "unsafe_operation_retry"
	if !esInterfazVerificada(ctx) {
		return unsafeLocalRequired(action)
	}
	obj, err := strictTokenSettingsObject(raw)
	if err != nil || len(obj) != 1 {
		return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
	}
	var p struct {
		RetryID string `json:"retryId"`
	}
	if json.Unmarshal(raw, &p) != nil || p.RetryID == "" {
		return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
	}
	m.unsafeRetryMu.Lock()
	r := m.unsafeRetries[p.RetryID]
	if r == nil || r.owner != unsafeRetryOwner(ctx) || !r.activated || !time.Now().Before(r.expires) {
		m.unsafeRetryMu.Unlock()
		return respuesta{OK: false, Action: action, Error: m.t("excepciones.reintento_caducado")}
	}
	delete(m.unsafeRetries, p.RetryID)
	r.timer.Stop()
	m.unsafeRetryMu.Unlock()
	defer r.clear()
	if err := excepciones.Check(r.exceptionID, ctx); err != nil {
		return respuesta{OK: false, Action: action, Error: err.Error()}
	}
	for path, expected := range r.files {
		data, err := securefile.ReadFileLimit(path, unsafeRetryMaxBytes)
		same := err == nil && bytes.Equal(data, expected)
		secmem.Zeroize(data)
		if !same {
			return respuesta{OK: false, Action: action, Error: m.t("excepciones.reintento_no_disponible")}
		}
	}
	current, err := unsafePreferenceDigest(m.configDirSeguro())
	if err != nil || current != r.preferences {
		return respuesta{OK: false, Action: action, Error: m.t("excepciones.reintento_no_disponible")}
	}
	if r.params.Action == "sign" && m.CSC != nil {
		var params paramsFirma
		_ = json.Unmarshal(r.params.Params, &params)
		if _, remote := m.CSC.Credencial(params.CertificateID); remote {
			return respuesta{OK: false, Action: action, Error: m.t("excepciones.reintento_no_disponible")}
		}
	}
	ctx = context.WithValue(ctx, unsafeRetryFilesKey{}, r.files)
	result := m.despachar(ctx, r.params)
	result.Action = action
	return result
}

// El consentimiento cambia los mapas reservados; el resto de preferencias
// debe seguir siendo el mismo para no cambiar el sentido del intento original.
func unsafePreferenceDigest(dir string) ([32]byte, error) {
	all := map[string]map[string]json.RawMessage{}
	for _, name := range []string{"config.json", "settings.json"} {
		raw, err := securefile.ReadFileLimit(filepath.Join(dir, name), 4<<20)
		if err != nil && !os.IsNotExist(err) {
			return [32]byte{}, err
		}
		doc := map[string]json.RawMessage{}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &doc); err != nil {
				return [32]byte{}, err
			}
		}
		for key := range doc {
			if excepciones.ReservedSettings(key) {
				delete(doc, key)
			}
		}
		all[name] = doc
	}
	raw, err := json.Marshal(all)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}
