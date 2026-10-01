// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/application"
)

type protocolDiagnosticEvent struct {
	Timestamp string `json:"timestamp"`
	Phase     string `json:"phase,omitempty"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

type protocolDiagnosticPhaseState struct {
	Current       string `json:"current"`
	LastCompleted string `json:"lastCompleted,omitempty"`
}

type protocolDiagnosticTimings struct {
	TotalElapsedMs            int64 `json:"totalElapsedMs,omitempty"`
	StartupToHandshakeMs      int64 `json:"startupToHandshakeMs,omitempty"`
	CertificateCatalogLoadMs  int64 `json:"certificateCatalogLoadMs,omitempty"`
	DocumentPickMs            int64 `json:"documentPickMs,omitempty"`
	CertificateSelectionMs    int64 `json:"certificateSelectionMs,omitempty"`
	HandshakeToFirstMessageMs int64 `json:"handshakeToFirstMessageMs,omitempty"`
}

type protocolDiagnosticApp struct {
	Component string `json:"component"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

type protocolDiagnosticSessionInfo struct {
	Mode              string `json:"mode,omitempty"`
	ProtocolVersion   int    `json:"protocolVersion,omitempty"`
	SessionIDMasked   string `json:"sessionIdMasked,omitempty"`
	RequestIDMasked   string `json:"requestIdMasked,omitempty"`
	Origin            string `json:"origin,omitempty"`
	OriginHost        string `json:"originHost,omitempty"`
	OriginCategory    string `json:"originCategory,omitempty"`
	ListenAddr        string `json:"listenAddr,omitempty"`
	RawURISummary     string `json:"rawUriSummary,omitempty"`
	LastVisibleStatus string `json:"lastVisibleStatus,omitempty"`
	LastVisibleDetail string `json:"lastVisibleDetail,omitempty"`
}

type protocolDiagnosticOperation struct {
	Type  string                       `json:"type,omitempty"`
	Phase protocolDiagnosticPhaseState `json:"phase"`
}

type protocolIncident struct {
	Schema     string                        `json:"schema"`
	Timestamp  string                        `json:"timestamp"`
	App        protocolDiagnosticApp         `json:"app"`
	Session    protocolDiagnosticSessionInfo `json:"session"`
	Operation  protocolDiagnosticOperation   `json:"operation"`
	Diagnostic application.GuidedDiagnostic  `json:"diagnostic"`
	Timings    protocolDiagnosticTimings     `json:"timings,omitempty"`
	EventsTail []protocolDiagnosticEvent     `json:"eventsTail,omitempty"`
}

type protocolDiagnosticSession struct {
	mu sync.Mutex

	startedAt time.Time
	app       protocolDiagnosticApp
	session   protocolDiagnosticSessionInfo
	operation protocolDiagnosticOperation
	timings   protocolDiagnosticTimings
	events    []protocolDiagnosticEvent

	phaseStartedAt    map[string]time.Time
	handshakeAt       time.Time
	firstMessageAt    time.Time
	incidentPersisted bool
}

const (
	protocolIncidentRetentionDays = 30
	protocolIncidentMaxGroups     = 50
)

var protocolIncidentRetainedFileName = regexp.MustCompile(
	`^(protocol-incident-\d{8}-\d{6}-[A-Za-z0-9_-]{1,32}-[0-9]+)(?:\.json|\.txt|\.logtail\.txt)$`,
)

var currentProtocolDiagnostic struct {
	mu   sync.Mutex
	data *protocolDiagnosticSession
}

func protocolDiagnosticReset(rawURI string) {
	currentProtocolDiagnostic.mu.Lock()
	defer currentProtocolDiagnostic.mu.Unlock()
	currentProtocolDiagnostic.data = &protocolDiagnosticSession{
		startedAt: time.Now(),
		app: protocolDiagnosticApp{
			Component: "grxfirma-afirmauri",
			Version:   version,
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
		},
		session: protocolDiagnosticSessionInfo{
			RawURISummary: protocolRawURISummary(rawURI),
		},
		phaseStartedAt: make(map[string]time.Time),
	}
}

func withProtocolDiagnostic(fn func(*protocolDiagnosticSession)) {
	currentProtocolDiagnostic.mu.Lock()
	session := currentProtocolDiagnostic.data
	currentProtocolDiagnostic.mu.Unlock()
	if session == nil {
		return
	}
	fn(session)
}

func protocolDiagnosticSetLaunchMode(mode string, protocolVersion int, sessionID, listenAddr string) {
	withProtocolDiagnostic(func(s *protocolDiagnosticSession) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.session.Mode = sanitizeProtocolMode(mode)
		if protocolVersion >= 0 && protocolVersion <= 4 {
			s.session.ProtocolVersion = protocolVersion
		}
		s.session.SessionIDMasked = maskSessionForLog(sessionID)
		if strings.TrimSpace(listenAddr) != "" {
			s.session.ListenAddr = "local_listener"
		}
		msg := tl("Canal %s preparado", s.session.Mode)
		if s.session.ProtocolVersion > 0 {
			msg += fmt.Sprintf(" (v%d)", s.session.ProtocolVersion)
		}
		if s.session.ListenAddr != "" {
			msg += " en " + s.session.ListenAddr
		}
		s.events = appendTailEvent(s.events, protocolDiagnosticEvent{
			Timestamp: time.Now().Format(time.RFC3339),
			Phase:     "launch_ready",
			Level:     "info",
			Message:   msg,
		})
	})
}

func protocolDiagnosticSetOrigin(origin string) {
	withProtocolDiagnostic(func(s *protocolDiagnosticSession) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.session.Origin, s.session.OriginHost, s.session.OriginCategory = sanitizeProtocolOrigin(origin)
		if s.session.Origin != "" {
			msg := tl("Origen conectado: ") + s.session.Origin
			if s.session.OriginCategory != "" {
				msg += " [" + s.session.OriginCategory + "]"
			}
			s.events = appendTailEvent(s.events, protocolDiagnosticEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Phase:     "browser_connected",
				Level:     "info",
				Message:   msg,
			})
		}
	})
}

func protocolDiagnosticSetOperation(op string) {
	op = sanitizeProtocolOperation(op)
	if op == "unknown" {
		return
	}
	withProtocolDiagnostic(func(s *protocolDiagnosticSession) {
		s.mu.Lock()
		defer s.mu.Unlock()
		changed := s.operation.Type != op
		s.operation.Type = op
		if s.firstMessageAt.IsZero() {
			s.firstMessageAt = time.Now()
			if !s.handshakeAt.IsZero() {
				s.timings.HandshakeToFirstMessageMs = s.firstMessageAt.Sub(s.handshakeAt).Milliseconds()
			}
		}
		if changed {
			s.events = appendTailEvent(s.events, protocolDiagnosticEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Phase:     "message_received",
				Level:     "info",
				Message:   tl("Operación recibida desde la web: ") + op,
			})
		}
	})
}

func protocolDiagnosticMarkPhase(phase, status, detail string) {
	phase = sanitizeProtocolEventLabel(phase)
	status = sanitizeProtocolIncidentText(status, 1024)
	detail = sanitizeProtocolIncidentText(detail, 1024)
	withProtocolDiagnostic(func(s *protocolDiagnosticSession) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if phase != "" {
			if current := s.operation.Phase.Current; current != "" && current != phase {
				s.operation.Phase.LastCompleted = current
			}
			s.operation.Phase.Current = phase
			s.phaseStartedAt[phase] = time.Now()
			if phase == "browser_connected" {
				s.handshakeAt = time.Now()
				s.timings.StartupToHandshakeMs = s.handshakeAt.Sub(s.startedAt).Milliseconds()
			}
		}
		if strings.TrimSpace(status) != "" {
			s.session.LastVisibleStatus = strings.TrimSpace(status)
		}
		if strings.TrimSpace(detail) != "" {
			s.session.LastVisibleDetail = strings.TrimSpace(detail)
		}
		if phase != "" || status != "" || detail != "" {
			msg := strings.TrimSpace(strings.Join([]string{status, detail}, " — "))
			if msg == "" {
				msg = phase
			}
			s.events = appendTailEvent(s.events, protocolDiagnosticEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Phase:     phase,
				Level:     "info",
				Message:   msg,
			})
		}
	})
}

func protocolDiagnosticMeasurePhase(phase string, startedAt time.Time) {
	if startedAt.IsZero() {
		return
	}
	withProtocolDiagnostic(func(s *protocolDiagnosticSession) {
		s.mu.Lock()
		defer s.mu.Unlock()
		elapsed := time.Since(startedAt).Milliseconds()
		switch strings.TrimSpace(phase) {
		case "certificate_catalog_load":
			s.timings.CertificateCatalogLoadMs = elapsed
		case "document_pick":
			s.timings.DocumentPickMs = elapsed
		case "certificate_select":
			s.timings.CertificateSelectionMs = elapsed
		}
	})
}

func protocolDiagnosticPersistFailure(action string, err error) {
	if err == nil {
		return
	}
	withProtocolDiagnostic(func(s *protocolDiagnosticSession) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.incidentPersisted {
			return
		}
		s.incidentPersisted = true
		if strings.TrimSpace(action) != "" && s.operation.Type == "" {
			s.operation.Type = sanitizeProtocolEventLabel(action)
		}
		if s.operation.Phase.Current == "" {
			s.operation.Phase.Current = inferProtocolFailurePhase(s.session.Mode, s.operation.Type)
		}
		s.timings.TotalElapsedMs = time.Since(s.startedAt).Milliseconds()
		rawError := strings.TrimSpace(err.Error())
		diag := application.BuildGuidedDiagnostic(action, rawError)
		diag.ExpertMessage = safeProtocolExpertMessage(diag)
		safeFailure := safeProtocolFailureSummary(diag)
		s.events = appendTailEvent(s.events, protocolDiagnosticEvent{
			Timestamp: time.Now().Format(time.RFC3339),
			Phase:     s.operation.Phase.Current,
			Level:     "error",
			Message:   safeFailure,
		})
		incident := protocolIncident{
			Schema:     "grxfirma-protocol-incident-v1",
			Timestamp:  time.Now().Format(time.RFC3339),
			App:        s.app,
			Session:    s.session,
			Operation:  s.operation,
			Diagnostic: diag,
			Timings:    s.timings,
			EventsTail: append([]protocolDiagnosticEvent(nil), s.events...),
		}
		basePath, writeErr := writeProtocolIncidentFiles(incident)
		if writeErr == nil {
			s.events = appendTailEvent(s.events, protocolDiagnosticEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Phase:     s.operation.Phase.Current,
				Level:     "info",
				Message:   tl("Incidencia persistida en ") + basePath,
			})
		}
	})
}

func appendTailEvent(events []protocolDiagnosticEvent, event protocolDiagnosticEvent) []protocolDiagnosticEvent {
	events = append(events, event)
	if len(events) > 40 {
		events = append([]protocolDiagnosticEvent(nil), events[len(events)-40:]...)
	}
	return events
}

func protocolIncidentDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "grxfirma-incidents")
	}
	return filepath.Join(appdirs.State(home), "incidents")
}

func ensurePrivateProtocolIncidentDir(path string) (err error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return securefile.ProtectDirectory(path, 0o700)
}

func writeProtocolIncidentFiles(incident protocolIncident) (string, error) {
	dir := protocolIncidentDir()
	if err := ensurePrivateProtocolIncidentDir(dir); err != nil {
		return "", err
	}
	if _, err := pruneProtocolIncidentFiles(
		dir,
		time.Now().UTC(),
		protocolIncidentRetentionDays,
		protocolIncidentMaxGroups,
	); err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	mode := incident.Session.Mode
	if mode == "" {
		mode = "protocol"
	}
	mode = protocolIncidentFileComponent(mode)
	rawJSON, err := json.MarshalIndent(incident, "", "  ")
	if err != nil {
		return "", err
	}
	jsonFile, err := os.CreateTemp(dir, fmt.Sprintf("protocol-incident-%s-%s-*.json", stamp, mode))
	if err != nil {
		return "", err
	}
	jsonPath := jsonFile.Name()
	if _, err := jsonFile.Write(rawJSON); err != nil {
		_ = jsonFile.Close()
		_ = os.Remove(jsonPath)
		return "", err
	}
	if err := jsonFile.Close(); err != nil {
		_ = os.Remove(jsonPath)
		return "", err
	}
	if err := os.Chmod(jsonPath, 0o600); err != nil {
		_ = os.Remove(jsonPath)
		return "", err
	}
	base := strings.TrimSuffix(jsonPath, ".json")
	text := buildProtocolIncidentText(incident)
	if err := securefile.WriteFileAtomic(base+".txt", []byte(text), 0o600); err != nil {
		return "", err
	}
	if logTail := buildProtocolIncidentLogTail(); strings.TrimSpace(logTail) != "" {
		if err := securefile.WriteFileAtomic(base+".logtail.txt", []byte(logTail), 0o600); err != nil {
			return "", err
		}
	}
	return base, nil
}

type protocolIncidentRetentionGroup struct {
	baseName string
	newest   time.Time
	files    []string
}

func pruneProtocolIncidentFiles(dir string, now time.Time, maxAgeDays, maxGroups int) (int, error) {
	if strings.TrimSpace(dir) == "" || now.IsZero() || maxAgeDays < 1 || maxGroups < 1 {
		return 0, errors.New("invalid protocol incident retention policy")
	}
	if err := ensurePrivateProtocolIncidentDir(dir); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	groups := make(map[string]*protocolIncidentRetentionGroup)
	for _, entry := range entries {
		match := protocolIncidentRetainedFileName.FindStringSubmatch(entry.Name())
		if len(match) != 2 || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return 0, infoErr
		}
		if !info.Mode().IsRegular() {
			continue
		}
		group := groups[match[1]]
		if group == nil {
			group = &protocolIncidentRetentionGroup{baseName: match[1]}
			groups[match[1]] = group
		}
		group.files = append(group.files, entry.Name())
		modified := info.ModTime().UTC()
		if group.newest.IsZero() || modified.After(group.newest) {
			group.newest = modified
		}
	}
	ordered := make([]*protocolIncidentRetentionGroup, 0, len(groups))
	for _, group := range groups {
		sort.Strings(group.files)
		ordered = append(ordered, group)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].newest.Equal(ordered[j].newest) {
			return ordered[i].baseName > ordered[j].baseName
		}
		return ordered[i].newest.After(ordered[j].newest)
	})

	cutoff := now.UTC().AddDate(0, 0, -maxAgeDays)
	removed := 0
	for index, group := range ordered {
		if !group.newest.Before(cutoff) && index < maxGroups {
			continue
		}
		for _, fileName := range group.files {
			path := filepath.Join(dir, fileName)
			info, infoErr := os.Lstat(path)
			if errors.Is(infoErr, os.ErrNotExist) {
				continue
			}
			if infoErr != nil {
				return removed, infoErr
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return removed, errors.New("protocol incident retention target is not a regular file")
			}
			if filepath.Dir(path) != filepath.Clean(dir) {
				return removed, errors.New("protocol incident retention target escaped its directory")
			}
			if removeErr := os.Remove(path); removeErr != nil {
				return removed, removeErr
			}
			removed++
		}
	}
	return removed, nil
}

func protocolIncidentFileComponent(value string) string {
	var out strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
		}
		if out.Len() >= 32 {
			break
		}
	}
	if out.Len() == 0 {
		return "protocol"
	}
	return out.String()
}

func buildProtocolIncidentText(incident protocolIncident) string {
	lines := []string{
		tl("GrxFirma — incidencia protocolaria"),
		"",
		tl("Fecha: ") + incident.Timestamp,
		tl("Componente: ") + incident.App.Component,
		tl("Versión: ") + incident.App.Version,
		tl("SO: ") + incident.App.OS + "/" + incident.App.Arch,
	}
	if incident.Session.Mode != "" {
		lines = append(lines, tl("Modo: ")+incident.Session.Mode)
	}
	if incident.Session.Origin != "" {
		lines = append(lines, tl("Origen: ")+incident.Session.Origin)
	}
	if incident.Session.OriginHost != "" {
		lines = append(lines, tl("Host origen: ")+incident.Session.OriginHost)
	}
	if incident.Session.OriginCategory != "" {
		lines = append(lines, tl("Tipo de origen: ")+incident.Session.OriginCategory)
	}
	if incident.Session.ListenAddr != "" {
		lines = append(lines, tl("Canal local: ")+incident.Session.ListenAddr)
	}
	if incident.Operation.Type != "" {
		lines = append(lines, tl("Operación: ")+incident.Operation.Type)
	}
	if incident.Operation.Phase.Current != "" {
		lines = append(lines, tl("Fase actual: ")+incident.Operation.Phase.Current)
	}
	if incident.Operation.Phase.LastCompleted != "" {
		lines = append(lines, tl("Última fase completada: ")+incident.Operation.Phase.LastCompleted)
	}
	if incident.Timings.TotalElapsedMs > 0 {
		lines = append(lines, tl("Tiempo total hasta el fallo: %d ms", incident.Timings.TotalElapsedMs))
	}
	if incident.Timings.StartupToHandshakeMs > 0 {
		lines = append(lines, tl("Arranque hasta handshake: %d ms", incident.Timings.StartupToHandshakeMs))
	}
	if incident.Timings.HandshakeToFirstMessageMs > 0 {
		lines = append(lines, tl("Handshake hasta primer mensaje: %d ms", incident.Timings.HandshakeToFirstMessageMs))
	}
	if incident.Timings.CertificateCatalogLoadMs > 0 {
		lines = append(lines, tl("Carga de catálogo: %d ms", incident.Timings.CertificateCatalogLoadMs))
	}
	if incident.Timings.DocumentPickMs > 0 {
		lines = append(lines, tl("Selección de documento: %d ms", incident.Timings.DocumentPickMs))
	}
	if incident.Timings.CertificateSelectionMs > 0 {
		lines = append(lines, tl("Selección de certificado: %d ms", incident.Timings.CertificateSelectionMs))
	}
	lines = append(lines,
		"",
		tl("Diagnóstico usuario: ")+incident.Diagnostic.UserMessage,
		tl("Responsabilidad probable: ")+incident.Diagnostic.ResponsibilityMessage,
		tl("Siguiente acción sugerida: ")+incident.Diagnostic.SuggestedAction,
		"",
		tl("Diagnóstico experto: ")+incident.Diagnostic.ExpertMessage,
	)
	if incident.Diagnostic.FailureCode != "" {
		lines = append(lines, tl("Código estable: ")+incident.Diagnostic.FailureCode)
	}
	if len(incident.EventsTail) > 0 {
		lines = append(lines, "", tl("Últimos eventos:"))
		for _, event := range incident.EventsTail {
			line := fmt.Sprintf("- %s [%s]", event.Timestamp, event.Level)
			if event.Phase != "" {
				line += " {" + event.Phase + "}"
			}
			if event.Message != "" {
				line += " " + event.Message
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func buildProtocolIncidentLogTail() string {
	text, err := readProtocolDebugLogTail(resolveProtocolDebugLogPath(), 64*1024)
	if err != nil {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return text + "\n"
}

func resolveProtocolDebugLogPath() string {
	if path := strings.TrimSpace(os.Getenv("GRXFIRMA_LOG_FILE")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("GRXFIRMA_DEBUG_LOG_FILE")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "grxfirma-debug.log")
	}
	return filepath.Join(appdirs.State(home), "logs", "grxfirma-debug.log")
}

func readProtocolDebugLogTail(path string, maxBytes int64) (string, error) {
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return "", fmt.Errorf("el límite del log debe ser positivo")
	}
	file, err := securefile.OpenRead(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	truncated := info.Size() > maxBytes
	if truncated {
		if _, err := file.Seek(-maxBytes, io.SeekEnd); err != nil {
			return "", err
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxBytes {
		data = data[len(data)-int(maxBytes):]
		truncated = true
	}
	if truncated {
		if idx := strings.IndexByte(string(data), '\n'); idx >= 0 && idx+1 < len(data) {
			data = data[idx+1:]
		}
	}
	return redactProtocolLogText(string(data)), nil
}

func protocolRawURISummary(rawURI string) string {
	uri := strings.TrimSpace(rawURI)
	if uri == "" {
		return ""
	}
	if len(uri) > 16*1024 {
		return "scheme=unknown;operation=unknown"
	}
	u, err := url.Parse(uri)
	if err != nil || u == nil {
		return "scheme=unknown;operation=unknown"
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "afirma" {
		scheme = "unknown"
	}
	operation := sanitizeProtocolOperation(u.Host)
	if operation == "unknown" {
		operation = sanitizeProtocolOperation(strings.Trim(strings.TrimSpace(u.Path), "/"))
	}
	if operation == "unknown" {
		for _, key := range []string{"op", "operation", "action"} {
			if value := u.Query().Get(key); value != "" {
				operation = sanitizeProtocolOperation(value)
				break
			}
		}
	}
	summary := "scheme=" + scheme + ";operation=" + operation
	for _, key := range []string{"origin", "originUrl", "originurl"} {
		if origin := u.Query().Get(key); origin != "" {
			_, _, category := sanitizeProtocolOrigin(origin)
			if category != "" {
				summary += ";origin=" + category
			}
			break
		}
	}
	return summary
}

func sanitizeProtocolOperation(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sign", "firma":
		return "sign"
	case "cosign", "cofirma":
		return "cosign"
	case "countersign", "contrafirma":
		return "countersign"
	case "batch", "lote":
		return "batch"
	case "save", "guardar":
		return "save"
	case "load", "cargar":
		return "load"
	case "signandsave", "sign-and-save", "firmaryguardar":
		return "signandsave"
	case "selectcert", "select-cert", "seleccionarcertificado":
		return "selectcert"
	case "websocket":
		return "websocket"
	case "service":
		return "service"
	case "echo":
		return "echo"
	default:
		return "unknown"
	}
}

func sanitizeProtocolMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "direct", "desktop":
		return "direct"
	case "websocket":
		return "websocket"
	case "service":
		return "service"
	default:
		return "unknown"
	}
}

func sanitizeProtocolEventLabel(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var out strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-' {
			out.WriteRune(r)
		}
		if out.Len() >= 48 {
			break
		}
	}
	if out.Len() == 0 {
		return "unknown"
	}
	return out.String()
}

func sanitizeProtocolIncidentText(raw string, maxChars int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	safe := redactProtocolLogText(raw)
	runes := []rune(safe)
	if maxChars > 0 && len(runes) > maxChars {
		safe = string(runes[:maxChars]) + "…"
	}
	return safe
}

func inferProtocolFailurePhase(mode, operation string) string {
	operation = strings.TrimSpace(operation)
	mode = strings.TrimSpace(mode)
	switch {
	case operation != "":
		return "operation_" + operation
	case mode == "websocket":
		return "waiting_browser"
	case mode == "service":
		return "waiting_service_request"
	default:
		return "protocol_failure"
	}
}

func classifyProtocolOrigin(origin string) (string, string) {
	raw := strings.TrimSpace(origin)
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "unknown"
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return "", "unknown"
	}
	switch {
	case host == "localhost" || host == "127.0.0.1" || host == "::1":
		return host, "local"
	case strings.HasSuffix(host, ".gob.es"), strings.HasSuffix(host, ".redsara.es"), strings.Contains(host, "valide"):
		return host, "government_portal"
	case strings.HasSuffix(host, ".dipgra.es"), strings.HasSuffix(host, ".granada.org"), strings.HasSuffix(host, ".granada.es"):
		return host, "institutional_portal"
	case strings.Count(host, ".") == 0:
		return host, "private_network"
	default:
		return host, "external_portal"
	}
}

func sanitizeProtocolOrigin(origin string) (string, string, string) {
	if len(strings.TrimSpace(origin)) > 4096 {
		return "", "", "unknown"
	}
	host, category := classifyProtocolOrigin(origin)
	if host == "" {
		return "", "", category
	}
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u == nil {
		return "", "", "unknown"
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	switch scheme {
	case "http", "https", "chrome-extension", "moz-extension":
	default:
		return "", "", "unknown"
	}
	return scheme + "://" + host, host, category
}

func safeProtocolFailureSummary(diag application.GuidedDiagnostic) string {
	summary := "operation_failed"
	if diag.Category != "" {
		summary += " category=" + string(diag.Category)
	}
	if strings.TrimSpace(diag.FailureCode) != "" {
		summary += " code=" + strings.TrimSpace(diag.FailureCode)
	}
	return summary
}

func safeProtocolExpertMessage(diag application.GuidedDiagnostic) string {
	message := "Clasificación automática: " + string(diag.Category)
	if strings.TrimSpace(diag.FailureCode) != "" {
		message += ". Código estable: " + strings.TrimSpace(diag.FailureCode)
	}
	return message
}

func protocolRedactPathPrefixes(raw string) string {
	out := raw
	for _, prefix := range protocolSensitivePathPrefixes() {
		if prefix == "" {
			continue
		}
		out = redactProtocolPathPrefix(out, prefix)
	}
	return out
}

func redactProtocolPathPrefix(raw, prefix string) string {
	var out strings.Builder
	searchFrom := 0
	for searchFrom < len(raw) {
		idx := strings.Index(raw[searchFrom:], prefix)
		if idx < 0 {
			out.WriteString(raw[searchFrom:])
			break
		}
		idx += searchFrom
		prefixEnd := idx + len(prefix)
		if prefixEnd < len(raw) && raw[prefixEnd] != '/' && raw[prefixEnd] != '\\' {
			out.WriteString(raw[searchFrom : idx+1])
			searchFrom = idx + 1
			continue
		}
		end := prefixEnd
		for end < len(raw) {
			switch raw[end] {
			case ' ', '\t', '\r', '\n', '"', '\'', ')', ']', '}', ',', ';', ':':
				goto pathEnd
			default:
				end++
			}
		}
	pathEnd:
		out.WriteString(raw[searchFrom:idx])
		out.WriteString("[LOCAL_PATH_REDACTED]")
		searchFrom = end
	}
	return out.String()
}

func protocolSensitivePathPrefixes() []string {
	prefixes := make([]string, 0, 6)
	if home, err := os.UserHomeDir(); err == nil {
		home = strings.TrimSpace(home)
		if home != "" {
			prefixes = append(prefixes, home)
			if runtime.GOOS == "windows" {
				prefixes = append(prefixes, strings.ReplaceAll(home, `\`, `/`))
			}
		}
	}
	for _, envName := range []string{"HOME", "USERPROFILE"} {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			prefixes = append(prefixes, value)
			if runtime.GOOS == "windows" {
				prefixes = append(prefixes, strings.ReplaceAll(value, `\`, `/`))
			}
		}
	}
	return uniqueProtocolStrings(prefixes)
}

func uniqueProtocolStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
