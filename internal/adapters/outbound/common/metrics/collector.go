// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package metrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultPath     = "/tmp/grxfirma-metrics.json"
	defaultMaxBytes = 1 << 20
)

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
type Collector struct {
	mu                sync.Mutex
	path              string
	maxBytes          int64
	signTotal         map[signKey]int64
	signDuration      map[string]durationAggregate
	certificateSource map[string]int64
	protocolRequests  map[protocolKey]int64
}

func New(path string, maxBytes int64) *Collector {
	if path == "" {
		path = defaultPath
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

func NewDefault() *Collector {
	return New(defaultPath, defaultMaxBytes)
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

func (c *Collector) exportLocked() {
	data, err := json.MarshalIndent(c.snapshotLocked(), "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(c.path), 0o700)
	c.rotateLocked(int64(len(data)))
	_ = os.WriteFile(c.path, append(data, '\n'), 0o600)
}

func (c *Collector) rotateLocked(incoming int64) {
	info, err := os.Stat(c.path)
	if err != nil {
		return
	}
	if info.Size()+incoming <= c.maxBytes {
		return
	}
	_ = os.Remove(c.path + ".1")
	_ = os.Rename(c.path, c.path+".1")
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
