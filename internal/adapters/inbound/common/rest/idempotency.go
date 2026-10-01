// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/observability/correlation"
)

const (
	defaultSignRequestTTL       = 24 * time.Hour
	maxSignRequestEntries       = 4096
	maxSignRequestRegistryBytes = 1 << 20
	signRequestRegistryVersion  = 1
	signRequestRegistryFilename = "sign-request-ids.json"
)

var (
	errInvalidSignRequestID = errors.New("identificador de petición de firma no válido")
	errSignRequestReplay    = errors.New("petición de firma repetida")
	errSignRequestCapacity  = errors.New("registro de peticiones de firma lleno")
)

type persistedSignRequests struct {
	Version int                  `json:"version"`
	Entries map[string]time.Time `json:"entries"`
}

func (a *Adaptador) acceptSignRequestID(w http.ResponseWriter, raw string) bool {
	err := a.reserveSignRequestID(raw)
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, errInvalidSignRequestID):
		writeError(w, http.StatusBadRequest, "request_id no es válido")
	case errors.Is(err, errSignRequestReplay):
		writeError(w, http.StatusConflict, "request_id ya procesado")
	case errors.Is(err, errSignRequestCapacity):
		writeError(w, http.StatusTooManyRequests, "registro de idempotencia temporalmente lleno")
	default:
		writeError(w, http.StatusServiceUnavailable, "no se pudo asegurar la idempotencia de la firma")
	}
	return false
}

func (a *Adaptador) reserveSignRequestID(raw string) error {
	if raw == "" {
		return nil
	}
	requestID := strings.TrimSpace(raw)
	if requestID != raw || !validSignRequestID(requestID) {
		return errInvalidSignRequestID
	}
	digest := sha256.Sum256([]byte(requestID))
	key := hex.EncodeToString(digest[:])
	now := time.Now().UTC()

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.loadSignRequestsLocked(now); err != nil {
		return err
	}
	for existing, expiresAt := range a.signRequests {
		if !expiresAt.After(now) {
			delete(a.signRequests, existing)
		}
	}
	if _, exists := a.signRequests[key]; exists {
		return errSignRequestReplay
	}
	if len(a.signRequests) >= maxSignRequestEntries {
		return errSignRequestCapacity
	}
	ttl := a.signRequestTTL
	if ttl <= 0 {
		ttl = defaultSignRequestTTL
	}
	a.signRequests[key] = now.Add(ttl)
	if err := a.persistSignRequestsLocked(); err != nil {
		delete(a.signRequests, key)
		return fmt.Errorf("persistir registro de idempotencia: %w", err)
	}
	return nil
}

func validSignRequestID(value string) bool {
	return correlation.ValidateID(value) == nil
}

func (a *Adaptador) loadSignRequestsLocked(now time.Time) error {
	if a.signRequestsLoaded {
		return nil
	}
	a.signRequests = map[string]time.Time{}
	path, ok := a.signRequestRegistryPath()
	if !ok {
		a.signRequestsLoaded = true
		return nil
	}
	file, err := securefile.OpenRead(path)
	if errors.Is(err, os.ErrNotExist) {
		a.signRequestsLoaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("abrir registro de idempotencia: %w", err)
	}
	defer file.Close()

	limited := io.LimitReader(file, maxSignRequestRegistryBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("leer registro de idempotencia: %w", err)
	}
	if len(data) > maxSignRequestRegistryBytes {
		return errors.New("registro de idempotencia demasiado grande")
	}
	var persisted persistedSignRequests
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&persisted); err != nil {
		return fmt.Errorf("decodificar registro de idempotencia: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	if persisted.Version != signRequestRegistryVersion {
		return fmt.Errorf("versión de registro de idempotencia no soportada: %d", persisted.Version)
	}
	if len(persisted.Entries) > maxSignRequestEntries {
		return errSignRequestCapacity
	}
	for key, expiresAt := range persisted.Entries {
		if !validSignRequestDigest(key) {
			return errors.New("registro de idempotencia contiene una clave no válida")
		}
		if expiresAt.After(now) {
			a.signRequests[key] = expiresAt.UTC()
		}
	}
	a.signRequestsLoaded = true
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decodificar final del registro de idempotencia: %w", err)
	}
	return errors.New("registro de idempotencia contiene datos adicionales")
}

func validSignRequestDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (a *Adaptador) persistSignRequestsLocked() error {
	path, ok := a.signRequestRegistryPath()
	if !ok {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := protectIdempotencyDirectory(dir); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".sign-request-ids-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	keepTemp := true
	defer func() {
		_ = temp.Close()
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	persisted := persistedSignRequests{
		Version: signRequestRegistryVersion,
		Entries: a.signRequests,
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(persisted); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := replaceIdempotencyFile(tempPath, path); err != nil {
		return err
	}
	keepTemp = false
	return nil
}

func protectIdempotencyDirectory(path string) (err error) {
	return securefile.ProtectDirectory(path, 0o700)
}

func (a *Adaptador) signRequestRegistryPath() (string, bool) {
	dir := strings.TrimSpace(a.ConfigDir)
	if dir == "" {
		return "", false
	}
	return filepath.Join(dir, signRequestRegistryFilename), true
}
