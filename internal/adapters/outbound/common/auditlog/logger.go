// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package auditlog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
	"grxfirma/internal/ports"
)

const (
	defaultMaxBytes = 10 * 1024 * 1024
	defaultMaxAge   = 90 * 24 * time.Hour
)

// Logger persiste evidencias saneadas en un fichero JSONL con rotacion simple.
type Logger struct {
	path     string
	maxBytes int64
	maxAge   time.Duration
	now      func() time.Time
	mu       sync.Mutex
	// ultimaHuella es la huella del último registro escrito, que encadena el
	// siguiente; se recupera del fichero la primera vez.
	ultimaHuella string
	cadenaLista  bool
}

// New crea un logger con la ruta y limite indicados.
func New(path string, maxBytes int64) *Logger {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	return &Logger{
		path:     path,
		maxBytes: maxBytes,
		maxAge:   defaultMaxAge,
		now:      time.Now,
	}
}

// NewDefault construye el logger en la ruta por defecto del usuario.
// RutaPorDefecto devuelve la ruta del registro de auditoría del usuario.
func RutaPorDefecto() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver HOME para auditlog: %w", err)
	}
	return filepath.Join(appdirs.Data(home), "audit.jsonl"), nil
}

func NewDefault() (*Logger, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("no se pudo resolver HOME para auditlog: %w", err)
	}
	return New(filepath.Join(appdirs.Data(home), "audit.jsonl"), defaultMaxBytes), nil
}

// Log implementa ports.EvidenceLogger.
func (l *Logger) Log(ctx context.Context, evidence ports.Evidence) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || l.path == "" {
		return nil
	}

	if int64(len(evidence.Payload))+int64(len(`{"cadena":"","evidencia":}`))+64+1 > l.maxBytes {
		return fmt.Errorf(
			"la evidencia de auditoria supera el limite de %d bytes",
			l.maxBytes,
		)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	auditDir := filepath.Dir(l.path)
	if err := os.MkdirAll(auditDir, 0o700); err != nil {
		return fmt.Errorf("no se pudo crear el directorio de auditoria: %w", err)
	}
	if err := securefile.ProtectDirectory(auditDir, 0o700); err != nil {
		return fmt.Errorf("no se pudo proteger el directorio de auditoria: %w", err)
	}

	if err := l.pruneExpired(); err != nil {
		return err
	}
	if !l.cadenaLista {
		huella, err := huellaUltimoRegistro(l.path, l.path+".1")
		if err != nil {
			return err
		}
		l.ultimaHuella, l.cadenaLista = huella, true
	}
	registro := Encadenar(evidence.Payload, l.ultimaHuella)
	line := append(append([]byte(nil), registro...), '\n')
	if err := l.rotateIfNeeded(int64(len(line))); err != nil {
		return err
	}

	f, err := securefile.OpenAppend(l.path, 0o600)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el fichero de auditoria: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("no se pudo escribir la evidencia de auditoria: %w", err)
	}
	l.ultimaHuella = Huella(registro)
	return nil
}

func (l *Logger) pruneExpired() error {
	cutoff := l.now().UTC().Add(-l.maxAge)
	for _, path := range []string{l.path, l.path + ".1"} {
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("no se pudo inspeccionar la retencion de auditoria: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf(
				"el artefacto de auditoria retenido no es un fichero regular: %s",
				filepath.Base(path),
			)
		}
		if !info.ModTime().UTC().Before(cutoff) {
			continue
		}
		if err := securefile.RemoveFile(path); err != nil {
			return fmt.Errorf("no se pudo retirar la auditoria fuera de retencion: %w", err)
		}
	}
	return nil
}

func (l *Logger) rotateIfNeeded(incoming int64) error {
	info, err := os.Lstat(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("no se pudo inspeccionar el fichero de auditoria: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("el fichero de auditoria no es regular")
	}
	if info.Size()+incoming <= l.maxBytes {
		return nil
	}

	rotated := l.path + ".1"
	if rotatedInfo, statErr := os.Lstat(rotated); statErr == nil {
		if rotatedInfo.Mode()&os.ModeSymlink != 0 ||
			!rotatedInfo.Mode().IsRegular() {
			return fmt.Errorf("la rotacion de auditoria no es un fichero regular")
		}
		if err := securefile.RemoveFile(rotated); err != nil {
			return fmt.Errorf("no se pudo retirar la rotacion de auditoria: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("no se pudo inspeccionar la rotacion de auditoria: %w", statErr)
	}
	if err := os.Rename(l.path, rotated); err != nil {
		return fmt.Errorf("no se pudo rotar el fichero de auditoria: %w", err)
	}
	if err := securefile.ProtectFile(rotated, 0o600); err != nil {
		return fmt.Errorf("no se pudo proteger la rotacion de auditoria: %w", err)
	}
	return nil
}

var _ ports.EvidenceLogger = (*Logger)(nil)
