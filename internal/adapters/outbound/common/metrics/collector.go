// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
)

const (
	defaultDirName  = "grxfirma"
	defaultFileName = "metrics.json"
	defaultMaxBytes = 1 << 20
)

// DefaultPath devuelve dónde se guardan las métricas por defecto:
// <carpeta privada de ejecución>/grxfirma/metrics.json. Nunca está en la
// carpeta temporal compartida; ver appdirs.RuntimeUsuario. Si no hay carpeta
// privada devuelve el error y no hay ruta por defecto.
func DefaultPath() (string, error) {
	dir, err := appdirs.RuntimeUsuario()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, defaultDirName, defaultFileName), nil
}

type signKey struct {
	Format string `json:"format"`
	Status string `json:"status"`
}

type protocolKey struct {
	Op     string `json:"op"`
	Status string `json:"status"`
}

type durationAggregate struct {
	Count   int64 `json:"count"`
	TotalMS int64 `json:"total_ms"`
	MaxMS   int64 `json:"max_ms"`
	Average int64 `json:"average_ms"`
}

type signTotalRow struct {
	Format string `json:"format"`
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type signDurationRow struct {
	Format string `json:"format"`
	Count  int64  `json:"count"`
	Total  int64  `json:"total_ms"`
	Max    int64  `json:"max_ms"`
	Avg    int64  `json:"avg_ms"`
}

type certificateSourceRow struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type protocolRequestRow struct {
	Op     string `json:"op"`
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type snapshot struct {
	GeneratedAt           time.Time              `json:"generated_at"`
	SignTotal             []signTotalRow         `json:"sign_total"`
	SignDurationMS        []signDurationRow      `json:"sign_duration_ms"`
	CertificateSource     []certificateSourceRow `json:"certificate_source"`
	ProtocolRequestsTotal []protocolRequestRow   `json:"protocol_requests_total"`
}

// Collector mantiene métricas agregadas en memoria y las exporta a JSON.
// Si no tiene ruta (no hay carpeta privada del usuario) sigue contando en
// memoria pero no escribe nada: falla cerrado sin romper la operación.
type Collector struct {
	mu                sync.Mutex
	path              string
	dirReady          bool // la carpeta ya se comprobó privada en este proceso
	maxBytes          int64
	signTotal         map[signKey]int64
	signDuration      map[string]durationAggregate
	certificateSource map[string]int64
	protocolRequests  map[protocolKey]int64
}

// New crea un recolector que escribe en path. Con path vacío usa DefaultPath;
// si tampoco hay ruta por defecto, el recolector no escribe a disco.
func New(path string, maxBytes int64) *Collector {
	if path == "" {
		if def, err := DefaultPath(); err == nil {
			path = def
		}
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	return &Collector{
		path:              path,
		maxBytes:          maxBytes,
		signTotal:         make(map[signKey]int64),
		signDuration:      make(map[string]durationAggregate),
		certificateSource: make(map[string]int64),
		protocolRequests:  make(map[protocolKey]int64),
	}
}

// NewDefault crea un recolector en la ruta de DefaultPath.
func NewDefault() *Collector {
	return New("", defaultMaxBytes)
}

func (c *Collector) RecordSign(_ context.Context, format string, status string, duration time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	k := signKey{Format: normalizeLabel(format), Status: normalizeLabel(status)}
	c.signTotal[k]++

	ms := duration.Milliseconds()
	agg := c.signDuration[normalizeLabel(format)]
	agg.Count++
	agg.TotalMS += ms
	if ms > agg.MaxMS {
		agg.MaxMS = ms
	}
	agg.Average = agg.TotalMS / agg.Count
	c.signDuration[normalizeLabel(format)] = agg

	c.exportLocked()
}

func (c *Collector) RecordCertificateSource(_ context.Context, sourceType string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.certificateSource[normalizeLabel(sourceType)]++
	c.exportLocked()
}

func (c *Collector) RecordProtocolRequest(_ context.Context, op string, status string, _ time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	k := protocolKey{Op: normalizeLabel(op), Status: normalizeLabel(status)}
	c.protocolRequests[k]++
	c.exportLocked()
}

// exportLocked escribe la instantánea solo dentro de una carpeta privada del
// usuario. Si la carpeta no se puede crear o no es privada (enlace, de otro
// usuario, con permisos para otros) no escribe nada.
func (c *Collector) exportLocked() {
	if c.path == "" {
		return
	}
	// La carpeta se protege una vez por proceso (en Windows eso reescribe la
	// DACL); si una escritura falla, se vuelve a comprobar en la siguiente.
	if !c.dirReady {
		if err := securefile.EnsurePrivateDir(filepath.Dir(c.path)); err != nil {
			return
		}
		c.dirReady = true
	}
	data, err := json.MarshalIndent(c.snapshotLocked(), "", "  ")
	if err != nil {
		return
	}
	if !c.rotateLocked(int64(len(data))) {
		return
	}
	if err := securefile.WriteFileAtomic(c.path, append(data, '\n'), 0o600); err != nil {
		c.dirReady = false
	}
}

// rotateLocked pasa el fichero actual a .1 si con lo nuevo supera el límite.
// Devuelve false si el fichero actual no es un fichero normal (por ejemplo,
// un enlace): entonces no se escribe.
func (c *Collector) rotateLocked(incoming int64) bool {
	info, err := os.Lstat(c.path)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	if !info.Mode().IsRegular() {
		return false
	}
	if info.Size()+incoming <= c.maxBytes {
		return true
	}
	old := c.path + ".1"
	if err := securefile.RemoveFile(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false
	}
	return os.Rename(c.path, old) == nil
}

func (c *Collector) snapshotLocked() snapshot {
	out := snapshot{
		GeneratedAt:           time.Now().UTC(),
		SignTotal:             make([]signTotalRow, 0, len(c.signTotal)),
		SignDurationMS:        make([]signDurationRow, 0, len(c.signDuration)),
		CertificateSource:     make([]certificateSourceRow, 0, len(c.certificateSource)),
		ProtocolRequestsTotal: make([]protocolRequestRow, 0, len(c.protocolRequests)),
	}

	for k, count := range c.signTotal {
		out.SignTotal = append(out.SignTotal, signTotalRow{Format: k.Format, Status: k.Status, Count: count})
	}
	for format, agg := range c.signDuration {
		out.SignDurationMS = append(out.SignDurationMS, signDurationRow{
			Format: format,
			Count:  agg.Count,
			Total:  agg.TotalMS,
			Max:    agg.MaxMS,
			Avg:    agg.Average,
		})
	}
	for source, count := range c.certificateSource {
		out.CertificateSource = append(out.CertificateSource, certificateSourceRow{Type: source, Count: count})
	}
	for k, count := range c.protocolRequests {
		out.ProtocolRequestsTotal = append(out.ProtocolRequestsTotal, protocolRequestRow{Op: k.Op, Status: k.Status, Count: count})
	}
	return out
}

func normalizeLabel(value string) string {
	if value == "" {
		return "desconocido"
	}
	return value
}
