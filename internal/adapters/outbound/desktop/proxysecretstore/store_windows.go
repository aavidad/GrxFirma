// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package proxysecretstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/ports"
)

const (
	maxProxySecretRealmBytes    = 1024
	maxProxySecretUsernameBytes = 1024
	maxProxySecretPasswordBytes = 64 * 1024
	maxProxySecretEnvelopeBytes = 128 * 1024
	maxProxySecretBlobBytes     = 256 * 1024
)

type fileReader func(string, int64) ([]byte, error)
type fileWriter func(string, []byte, os.FileMode) error
type mkdirAllFunc func(string, os.FileMode) error
type fileProtector func(string, os.FileMode) error
type removeFileFunc func(string) error
type configDirFunc func() (string, error)

// dpapiCodec protege y desprotege con DPAPI de usuario. La implementación de
// producción llama directamente a CryptProtectData/CryptUnprotectData: antes
// se lanzaba powershell.exe, que muchas Administraciones bloquean y que los
// EDR señalan, y el secreto viajaba por la tubería de un proceso hijo.
type dpapiCodec interface {
	protect(plaintext []byte) ([]byte, error)
	unprotect(ciphertext []byte) ([]byte, error)
}

type Store struct {
	dpapi       dpapiCodec
	idgen       func() (string, error)
	configDir   configDirFunc
	readFile    fileReader
	writeFile   fileWriter
	mkdirAll    mkdirAllFunc
	protectDir  fileProtector
	protectFile fileProtector
	removeFile  removeFileFunc
}

func New() *Store {
	return &Store{
		dpapi:       nativeDPAPI{},
		idgen:       randomID,
		configDir:   os.UserConfigDir,
		readFile:    securefile.ReadFileLimit,
		writeFile:   securefile.WriteFileAtomic,
		mkdirAll:    os.MkdirAll,
		protectDir:  securefile.ProtectDirectory,
		protectFile: securefile.ProtectFile,
		removeFile:  securefile.RemoveFile,
	}
}

func (s *Store) secretsDir() (string, error) {
	if s == nil || s.configDir == nil {
		return "", errors.New("proxysecretstore: backend no configurado")
	}
	base, err := s.configDir()
	if err != nil {
		return "", fmt.Errorf("proxysecretstore: resolviendo directorio de configuracion: %w", err)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("proxysecretstore: directorio de configuracion vacio")
	}
	return filepath.Join(base, "GrxFirma", "proxy-secrets"), nil
}

func (s *Store) secretPath(id string) (string, error) {
	var err error
	id, err = normalizeSecretID(id)
	if err != nil {
		return "", err
	}
	dir, err := s.secretsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".dpapi"), nil
}

func protectWithDPAPI(codec dpapiCodec, plaintext []byte) ([]byte, error) {
	if codec == nil {
		return nil, errors.New("proxysecretstore: backend no configurado")
	}
	if len(plaintext) == 0 || len(plaintext) > maxProxySecretEnvelopeBytes {
		return nil, fmt.Errorf(
			"proxysecretstore: payload plano fuera del limite de %d bytes",
			maxProxySecretEnvelopeBytes,
		)
	}
	protected, err := codec.protect(plaintext)
	if err != nil {
		return nil, err
	}
	if len(protected) == 0 || len(protected) > maxProxySecretBlobBytes {
		zeroSecretBytes(protected)
		return nil, fmt.Errorf("proxysecretstore: payload DPAPI fuera del limite de %d bytes", maxProxySecretBlobBytes)
	}
	return protected, nil
}

func unprotectWithDPAPI(codec dpapiCodec, ciphertext []byte) ([]byte, error) {
	if codec == nil {
		return nil, errors.New("proxysecretstore: backend no configurado")
	}
	if len(ciphertext) == 0 || len(ciphertext) > maxProxySecretBlobBytes {
		return nil, fmt.Errorf(
			"proxysecretstore: blob DPAPI fuera del limite de %d bytes",
			maxProxySecretBlobBytes,
		)
	}
	plain, err := codec.unprotect(ciphertext)
	if err != nil {
		return nil, err
	}
	if len(plain) == 0 || len(plain) > maxProxySecretEnvelopeBytes {
		zeroSecretBytes(plain)
		return nil, fmt.Errorf("proxysecretstore: payload plano DPAPI fuera del limite de %d bytes", maxProxySecretEnvelopeBytes)
	}
	return plain, nil
}

type nativeDPAPI struct{}

func (nativeDPAPI) protect(plaintext []byte) ([]byte, error) {
	return callDPAPI(plaintext, true)
}

func (nativeDPAPI) unprotect(ciphertext []byte) ([]byte, error) {
	return callDPAPI(ciphertext, false)
}

func callDPAPI(input []byte, protect bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("proxysecretstore: entrada DPAPI vacia")
	}
	if len(input) > maxProxySecretBlobBytes {
		return nil, errors.New("proxysecretstore: entrada DPAPI demasiado grande")
	}
	in := windows.DataBlob{Size: uint32(len(input)), Data: &input[0]} // #nosec G115 -- acotado por maxProxySecretBlobBytes.
	var out windows.DataBlob
	var err error
	if protect {
		description, descErr := windows.UTF16PtrFromString("GrxFirma proxy")
		if descErr != nil {
			return nil, descErr
		}
		err = windows.CryptProtectData(&in, description, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, fmt.Errorf("proxysecretstore: DPAPI: %w", err)
	}
	if out.Data == nil {
		return nil, errors.New("proxysecretstore: DPAPI devolvio un resultado vacio")
	}
	buffer := unsafe.Slice(out.Data, out.Size)
	result := append([]byte(nil), buffer...)
	zeroSecretBytes(buffer)
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return result, nil
}

func decodeBase64Limited(encoded []byte, maxDecodedBytes int, label string) ([]byte, error) {
	if len(encoded) == 0 ||
		maxDecodedBytes <= 0 ||
		len(encoded) > base64.StdEncoding.EncodedLen(maxDecodedBytes) {
		return nil, fmt.Errorf(
			"proxysecretstore: %s fuera del limite de %d bytes",
			label,
			maxDecodedBytes,
		)
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	n, err := base64.StdEncoding.Decode(decoded, encoded)
	if err != nil {
		zeroSecretBytes(decoded)
		return nil, fmt.Errorf("proxysecretstore: decodificando %s: %w", label, err)
	}
	if n == 0 || n > maxDecodedBytes {
		zeroSecretBytes(decoded)
		return nil, fmt.Errorf(
			"proxysecretstore: %s fuera del limite de %d bytes",
			label,
			maxDecodedBytes,
		)
	}
	return decoded[:n], nil
}

func validateProxySecretMaterial(realm, username string, password []byte) error {
	switch {
	case strings.TrimSpace(realm) == "":
		return errors.New("proxysecretstore: realm vacio")
	case len(realm) > maxProxySecretRealmBytes:
		return fmt.Errorf(
			"proxysecretstore: realm supera el limite de %d bytes",
			maxProxySecretRealmBytes,
		)
	case len(username) > maxProxySecretUsernameBytes:
		return fmt.Errorf(
			"proxysecretstore: username supera el limite de %d bytes",
			maxProxySecretUsernameBytes,
		)
	case len(password) == 0:
		return errors.New("proxysecretstore: password vacio")
	case len(password) > maxProxySecretPasswordBytes:
		return fmt.Errorf(
			"proxysecretstore: password supera el limite de %d bytes",
			maxProxySecretPasswordBytes,
		)
	default:
		return nil
	}
}

func (s *Store) Store(ctx context.Context, realm string, material ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	if s == nil ||
		s.dpapi == nil ||
		s.idgen == nil ||
		s.mkdirAll == nil ||
		s.protectDir == nil ||
		s.writeFile == nil ||
		s.protectFile == nil ||
		s.removeFile == nil {
		return ports.ProxySecretDescriptor{}, errors.New("proxysecretstore: backend no configurado")
	}
	realm = strings.TrimSpace(realm)
	if err := validateProxySecretMaterial(realm, material.Username, material.Password); err != nil {
		return ports.ProxySecretDescriptor{}, err
	}
	id, err := s.idgen()
	if err != nil {
		return ports.ProxySecretDescriptor{}, err
	}
	id, err = normalizeSecretID(id)
	if err != nil {
		return ports.ProxySecretDescriptor{}, err
	}
	dir, err := s.secretsDir()
	if err != nil {
		return ports.ProxySecretDescriptor{}, err
	}
	if err := s.mkdirAll(dir, 0o700); err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: creando directorio seguro: %w", err)
	}
	if err := s.protectDir(dir, 0o700); err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: protegiendo directorio seguro: %w", err)
	}
	env := secretEnvelope{
		Realm:       realm,
		Username:    material.Username,
		PasswordB64: base64.StdEncoding.EncodeToString(material.Password),
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: serializando secreto: %w", err)
	}
	defer zeroSecretBytes(raw)
	protected, err := protectWithDPAPI(s.dpapi, raw)
	if err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: protegiendo secreto con DPAPI: %w", err)
	}
	defer zeroSecretBytes(protected)
	path, err := s.secretPath(id)
	if err != nil {
		return ports.ProxySecretDescriptor{}, err
	}
	if err := s.writeFile(path, protected, 0o600); err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: escribiendo secreto protegido: %w", err)
	}
	if err := s.protectFile(path, 0o600); err != nil {
		protectErr := fmt.Errorf("proxysecretstore: protegiendo secreto almacenado: %w", err)
		if cleanupErr := s.removeFile(path); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			return ports.ProxySecretDescriptor{}, errors.Join(
				protectErr,
				fmt.Errorf("proxysecretstore: limpiando secreto sin proteccion: %w", cleanupErr),
			)
		}
		return ports.ProxySecretDescriptor{}, protectErr
	}
	return ports.ProxySecretDescriptor{
		ID:       id,
		Realm:    realm,
		Username: material.Username,
	}, nil
}

func (s *Store) Load(ctx context.Context, id string) (ports.ProxySecretMaterial, error) {
	if s == nil || s.dpapi == nil || s.readFile == nil {
		return ports.ProxySecretMaterial{}, errors.New("proxysecretstore: backend no configurado")
	}
	path, err := s.secretPath(id)
	if err != nil {
		return ports.ProxySecretMaterial{}, err
	}
	protected, err := s.readFile(path, maxProxySecretBlobBytes)
	if err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: leyendo secreto protegido: %w", err)
	}
	defer zeroSecretBytes(protected)
	raw, err := unprotectWithDPAPI(s.dpapi, protected)
	if err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: desprotegiendo secreto con DPAPI: %w", err)
	}
	defer zeroSecretBytes(raw)
	var env secretEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: parseando secreto: %w", err)
	}
	password, err := base64.StdEncoding.DecodeString(env.PasswordB64)
	if err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: decodificando password: %w", err)
	}
	if err := validateProxySecretMaterial(env.Realm, env.Username, password); err != nil {
		zeroSecretBytes(password)
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: secreto almacenado invalido: %w", err)
	}
	return ports.ProxySecretMaterial{
		Realm:    env.Realm,
		Username: env.Username,
		Password: password,
	}, nil
}

func (s *Store) Delete(_ context.Context, id string) error {
	if s == nil || s.removeFile == nil {
		return errors.New("proxysecretstore: backend no configurado")
	}
	path, err := s.secretPath(id)
	if err != nil {
		return err
	}
	if err := s.removeFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("proxysecretstore: eliminando secreto protegido: %w", err)
	}
	return nil
}

func classifyWindowsStatusReason(err error) string {
	if err == nil {
		return ""
	}
	raw := strings.TrimSpace(err.Error())
	if raw == "" {
		return "proxysecretstore: estado del backend DPAPI desconocido"
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "appdata"), strings.Contains(lower, "configuracion"), strings.Contains(lower, "configuración"):
		return "proxysecretstore: directorio de configuracion de usuario no disponible"
	default:
		return raw
	}
}

func (s *Store) Status(_ context.Context) (ports.ProxySecretStoreStatus, error) {
	if s == nil || s.dpapi == nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   windowsBackendName,
			Reason:    "proxysecretstore: backend no configurado",
		}, nil
	}
	if _, err := s.secretsDir(); err != nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   windowsBackendName,
			Reason:    classifyWindowsStatusReason(err),
		}, nil
	}
	if _, err := protectWithDPAPI(s.dpapi, []byte("ok")); err != nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   windowsBackendName,
			Reason:    classifyWindowsStatusReason(err),
		}, nil
	}
	return ports.ProxySecretStoreStatus{
		Available: true,
		Platform:  runtime.GOOS,
		Backend:   windowsBackendName,
	}, nil
}

var _ ports.ProxySecretStore = (*Store)(nil)
