// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package proxysecretstore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"grxfirma/internal/ports"
)

const secretToolService = "grxfirma-proxy" // #nosec G101 -- etiqueta pública de servicio para Secret Service, no una credencial.

var ErrSecretToolUnavailable = errors.New("proxysecretstore: secret-tool no disponible")

type Store struct {
	runner commandRunner
	tool   string
	idgen  func() (string, error)
}

type commandRunner interface {
	Run(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	// #nosec G204 -- executable comes from the fixed platform allowlist.
	cmd := exec.CommandContext(ctx, name, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil, err
	}
	return out, nil
}

func New() *Store {
	return &Store{
		runner: execCommandRunner{},
		idgen:  randomID,
	}
}

func (s *Store) toolPath() (string, error) {
	if s != nil && strings.TrimSpace(s.tool) != "" {
		return s.tool, nil
	}
	path, err := exec.LookPath("secret-tool")
	if err != nil {
		return "", ErrSecretToolUnavailable
	}
	return path, nil
}

func (s *Store) Store(ctx context.Context, realm string, material ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	if s == nil || s.runner == nil {
		return ports.ProxySecretDescriptor{}, errors.New("proxysecretstore: backend no configurado")
	}
	realm = strings.TrimSpace(realm)
	if realm == "" {
		return ports.ProxySecretDescriptor{}, errors.New("proxysecretstore: realm vacio")
	}
	if len(material.Password) == 0 {
		return ports.ProxySecretDescriptor{}, errors.New("proxysecretstore: password vacio")
	}
	tool, err := s.toolPath()
	if err != nil {
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
	env := secretEnvelope{
		Realm:       realm,
		Username:    material.Username,
		PasswordB64: base64.StdEncoding.EncodeToString(material.Password),
	}
	stdin, err := json.Marshal(env)
	if err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: serializando secreto: %w", err)
	}
	input := append(stdin, '\n')
	defer zeroSecretBytes(input)
	if _, err := s.runner.Run(ctx, input, tool,
		"store",
		"--label=GrxFirma Proxy",
		"service", secretToolService,
		"id", id,
	); err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: guardando secreto: %w", err)
	}
	return ports.ProxySecretDescriptor{
		ID:       id,
		Realm:    realm,
		Username: material.Username,
	}, nil
}

func (s *Store) Load(ctx context.Context, id string) (ports.ProxySecretMaterial, error) {
	if s == nil || s.runner == nil {
		return ports.ProxySecretMaterial{}, errors.New("proxysecretstore: backend no configurado")
	}
	id, err := normalizeSecretID(id)
	if err != nil {
		return ports.ProxySecretMaterial{}, err
	}
	tool, err := s.toolPath()
	if err != nil {
		return ports.ProxySecretMaterial{}, err
	}
	out, err := s.runner.Run(ctx, nil, tool, "lookup", "service", secretToolService, "id", id)
	if err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: cargando secreto: %w", err)
	}
	defer zeroSecretBytes(out)
	var env secretEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(out), &env); err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: parseando secreto: %w", err)
	}
	password, err := base64.StdEncoding.DecodeString(env.PasswordB64)
	if err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: decodificando password: %w", err)
	}
	return ports.ProxySecretMaterial{
		Realm:    env.Realm,
		Username: env.Username,
		Password: password,
	}, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if s == nil || s.runner == nil {
		return errors.New("proxysecretstore: backend no configurado")
	}
	id, err := normalizeSecretID(id)
	if err != nil {
		return err
	}
	tool, err := s.toolPath()
	if err != nil {
		return err
	}
	if _, err := s.runner.Run(ctx, nil, tool, "clear", "service", secretToolService, "id", id); err != nil {
		return fmt.Errorf("proxysecretstore: eliminando secreto: %w", err)
	}
	return nil
}

func classifyLinuxStatusReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrSecretToolUnavailable) {
		return ErrSecretToolUnavailable.Error()
	}
	raw := strings.TrimSpace(err.Error())
	if raw == "" {
		return "proxysecretstore: estado del backend desconocido"
	}
	lower := strings.ToLower(raw)
	for _, marker := range []string{
		"dbus",
		"d-bus",
		"cannot autolaunch",
		"secret service",
		"org.freedesktop.secrets",
		"secret collection",
		"no such interface",
		"no se pudo iniciar la sesion",
	} {
		if strings.Contains(lower, marker) {
			return "proxysecretstore: sesion Secret Service/DBus no disponible"
		}
	}
	return raw
}

func (s *Store) Status(ctx context.Context) (ports.ProxySecretStoreStatus, error) {
	tool, err := s.toolPath()
	if err != nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   "secret-service",
			Reason:    classifyLinuxStatusReason(err),
		}, nil
	}
	if s == nil || s.runner == nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   "secret-service",
			Reason:    "proxysecretstore: backend no configurado",
		}, nil
	}
	if s.idgen == nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   "secret-service",
			Reason:    "proxysecretstore: generador de identificadores no configurado",
		}, nil
	}
	probeID, err := s.idgen()
	if err != nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   "secret-service",
			Reason:    "proxysecretstore: no se pudo generar la sonda de estado",
		}, nil
	}
	// Una búsqueda --all vuelca también los secretos encontrados a stdout.
	// La sonda consulta un identificador aleatorio inexistente y solo usa el
	// error para detectar que Secret Service/DBus no está disponible.
	if _, err := s.runner.Run(ctx, nil, tool, "lookup", "service", secretToolService, "id", probeID); err != nil {
		reason := classifyLinuxStatusReason(err)
		if reason == ErrSecretToolUnavailable.Error() ||
			reason == "proxysecretstore: sesion Secret Service/DBus no disponible" {
			return ports.ProxySecretStoreStatus{
				Available: false,
				Platform:  runtime.GOOS,
				Backend:   "secret-service",
				Reason:    reason,
			}, nil
		}
	}
	return ports.ProxySecretStoreStatus{
		Available: true,
		Platform:  runtime.GOOS,
		Backend:   "secret-service",
	}, nil
}

var _ ports.ProxySecretStore = (*Store)(nil)
