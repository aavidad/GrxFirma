// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"crypto/des" // #nosec G502 -- DES is required for V1.9 session responses and is gated by explicit operator opt-in.
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	"grxfirma/internal/adapters/inbound/legacy/legacycrypto"
	legacyws "grxfirma/internal/adapters/inbound/legacy/websocket"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/adapters/outbound/common/securefile"
	desktopdocumentpicker "grxfirma/internal/adapters/outbound/desktop/documentpicker"
	desktopfilesystem "grxfirma/internal/adapters/outbound/desktop/filesystem"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/presentation/desktop/certpicker"
)

const (
	legacyLoadMaxBytes        = 5 * 1024 * 1024
	legacySHA1BlockedTitleID  = "Firma bloqueada: SHA-1 no es seguro"
	legacySHA1BlockedDetailID = "El portal solicita una firma SHA-1. SHA-1 es un algoritmo obsoleto e inseguro y GrxFirma no ha generado ninguna firma. La entidad responsable del portal debe actualizarlo a SHA-256 o superior."
	legacySHA1BlockedResponse = "SAF_27: Firma bloqueada: el portal solicita SHA-1, un algoritmo obsoleto e inseguro. No se ha generado ninguna firma; la entidad responsable del portal debe actualizarlo a SHA-256 o superior."
)

type signDocumentExecutor interface {
	Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error)
}

type processBatchExecutor interface {
	Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error)
}

type remoteBatchExecutor interface {
	ExecuteLegacyRemoteResult(ctx context.Context, req afirmauri.RemoteBatchCommand) (string, error)
}

type legacyLoadPicker func(ctx context.Context, initialPath, extensions string, multi bool) ([]string, error)
type legacySavePicker func(ctx context.Context, defaultPath, extensions string) (string, error)

type legacyWebSocketHandler struct {
	signUC        signDocumentExecutor
	batchUC       processBatchExecutor
	approval      ports.UserApproval
	catalogo      ports.CertificateCatalog
	selector      certpicker.CertSelector
	documentos    ports.DocumentPicker
	keys          ports.SigningKeyProvider
	retrieve      *triphase.Executor
	batchRemote   remoteBatchExecutor
	loadPicker    legacyLoadPicker
	savePicker    legacySavePicker
	afterSave     func(savedPath string, size int)
	afterTerminal func(op afirmauri.TipoOperacion, result string)
	stickyMu      sync.Mutex
	stickyID      string
	cacheMu       sync.RWMutex
	cacheCerts    []domain.CertificateRef
	cacheErr      error
	cacheLoaded   bool
}

func (h *legacyWebSocketHandler) withApproval(approval ports.UserApproval) *legacyWebSocketHandler {
	if h != nil {
		h.approval = approval
	}
	return h
}

type operationProbe struct {
	logger   *slog.Logger
	ctx      context.Context
	name     string
	start    time.Time
	lastWall time.Time
	lastCPU  time.Duration
	stopOnce sync.Once
	done     chan struct{}
}

func newOperationProbe(ctx context.Context, logger *slog.Logger, name string) *operationProbe {
	if !protocolDebugEnabled() {
		return nil
	}
	now := time.Now()
	return &operationProbe{
		logger:   logger,
		ctx:      ctx,
		name:     name,
		start:    now,
		lastWall: now,
		lastCPU:  readProcessCPUTime(),
		done:     make(chan struct{}),
	}
}

func (p *operationProbe) Start() {
	if p == nil || p.logger == nil {
		return
	}
	p.logger.InfoContext(p.ctx, "afirmauri_operation_probe_begin", "pid", os.Getpid(), "operation_probe", p.name)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-p.done:
				return
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.sample("tick")
			}
		}
	}()
}

func (p *operationProbe) Mark(reason string, kv ...any) {
	p.sample(reason, kv...)
}

func (p *operationProbe) Stop(reason string, kv ...any) {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		p.sample(reason, kv...)
		close(p.done)
	})
}

func (p *operationProbe) sample(reason string, kv ...any) {
	if p == nil || p.logger == nil {
		return
	}
	now := time.Now()
	cpuNow := readProcessCPUTime()
	deltaWall := now.Sub(p.lastWall)
	deltaCPU := cpuNow - p.lastCPU
	cpuPct := 0.0
	if deltaWall > 0 {
		cpuPct = 100 * deltaCPU.Seconds() / deltaWall.Seconds()
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fields := []any{
		"pid", os.Getpid(),
		"operation_probe", p.name,
		"reason", reason,
		"elapsed_ms_total", now.Sub(p.start).Milliseconds(),
		"cpu_pct_interval", cpuPct,
		"cpu_ms_total", cpuNow.Milliseconds(),
		"goroutines", runtime.NumGoroutine(),
		"heap_alloc_mb", bytesToMB(ms.Alloc),
		"heap_sys_mb", bytesToMB(ms.HeapSys),
		"rss_mb", bytesToMB(readProcessRSSBytes()),
	}
	fields = append(fields, kv...)
	p.logger.InfoContext(p.ctx, "afirmauri_operation_probe", fields...)
	p.lastWall = now
	p.lastCPU = cpuNow
}

type legacySignManifest struct {
	XMLName xml.Name                  `xml:"sign"`
	Entries []legacySignManifestEntry `xml:"e"`
}

type legacySignManifestEntry struct {
	Key   string `xml:"k,attr"`
	Value string `xml:"v,attr"`
}

type legacyBatchResponse struct {
	Signs []legacyBatchSingleResult `json:"signs"`
}

type legacyBatchSingleResult struct {
	ID          string `json:"id"`
	Result      string `json:"result"`
	Description string `json:"description,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

type legacyXMLBatchResponse struct {
	XMLName xml.Name               `xml:"signs"`
	Signs   []legacyXMLBatchResult `xml:"signresult"`
}

type legacyXMLBatchResult struct {
	ID          string `xml:"id,attr"`
	Result      string `xml:"result,attr"`
	Description string `xml:"description,attr"`
}

func newLegacyWebSocketHandler(
	signUC signDocumentExecutor,
	batchUC processBatchExecutor,
	catalogo ports.CertificateCatalog,
	selector certpicker.CertSelector,
	documentos ports.DocumentPicker,
	keys ports.SigningKeyProvider,
	retrieve *triphase.Executor,
	batchRemote remoteBatchExecutor,
) *legacyWebSocketHandler {
	h := &legacyWebSocketHandler{
		signUC:      signUC,
		batchUC:     batchUC,
		catalogo:    catalogo,
		selector:    selector,
		documentos:  documentos,
		keys:        keys,
		retrieve:    retrieve,
		batchRemote: batchRemote,
		loadPicker:  selectLegacyLoadPaths,
		savePicker:  selectLegacySaveTargetPath,
	}
	if !certpicker.SupportsCredentialLoading(selector) {
		go h.warmCertificates(context.Background())
	}
	return h
}

func (h *legacyWebSocketHandler) warmCertificates(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	_, _ = h.loadCertificatesCached(ctx)
}

func (h *legacyWebSocketHandler) loadCertificatesCached(ctx context.Context) ([]domain.CertificateRef, error) {
	h.cacheMu.RLock()
	if h.cacheLoaded {
		certs := append([]domain.CertificateRef(nil), h.cacheCerts...)
		err := h.cacheErr
		h.cacheMu.RUnlock()
		return certs, err
	}
	h.cacheMu.RUnlock()

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.cacheLoaded {
		return append([]domain.CertificateRef(nil), h.cacheCerts...), h.cacheErr
	}
	certs, err := h.cargarCertificadosLegacySinCache(ctx)
	if err == nil {
		h.cacheCerts = append([]domain.CertificateRef(nil), certs...)
	} else {
		h.cacheCerts = nil
	}
	h.cacheErr = err
	h.cacheLoaded = true
	return append([]domain.CertificateRef(nil), h.cacheCerts...), h.cacheErr
}

func (h *legacyWebSocketHandler) HandleLegacy(ctx context.Context, raw string, solicitud afirmauri.Solicitud) (resultado legacyws.Resultado, handled bool, err error) {
	release, err := certpicker.BeginCredentialOperation(ctx, h.selector)
	if err != nil {
		return legacyws.Resultado{}, true, err
	}
	defer release()
	defer func() {
		if err != nil || solicitud.Operacion == afirmauri.OperacionFirma || solicitud.Operacion == afirmauri.OperacionLote || solicitud.Operacion == afirmauri.OperacionSignSave {
			certpicker.ClearCredentials(h.selector)
		}
	}()
	slog.Info("legacy_handle_entry", "operation", solicitud.Operacion, "has_sign", solicitud.SignCommand != nil, "has_retrieve", solicitud.RetrieveCommand != nil, "has_batch", solicitud.BatchCommand != nil, "has_remote_batch", solicitud.RemoteBatch != nil)
	defer func() {
		if err != nil && !isLegacyCancellation(err) {
			reportNativeProtocolFailure(
				"legacy_"+string(solicitud.Operacion),
				err,
			)
		}
	}()
	if err := validateNativeProtocolOperation(solicitud.Operacion); err != nil {
		return legacyws.Resultado{}, true, err
	}
	switch solicitud.Operacion {
	case afirmauri.OperacionFirma:
		resp, err := h.handleSign(ctx, solicitud)
		if err != nil {
			return legacyws.Resultado{}, true, err
		}
		return legacyws.Resultado{
			Tipo:      "firma",
			Texto:     resp,
			Operacion: solicitud.Operacion,
			Solicitud: &solicitud,
		}, true, nil
	case afirmauri.OperacionSelectCert:
		resp, err := h.handleSelectCert(ctx, solicitud)
		if err != nil {
			return legacyws.Resultado{}, true, err
		}
		return legacyws.Resultado{
			Tipo:      "selectcert",
			Texto:     resp,
			Operacion: solicitud.Operacion,
			Solicitud: &solicitud,
		}, true, nil
	case afirmauri.OperacionLote:
		resp, err := h.handleBatch(ctx, solicitud)
		if err != nil {
			return legacyws.Resultado{}, true, err
		}
		return legacyws.Resultado{
			Tipo:      "batch",
			Texto:     resp,
			Operacion: solicitud.Operacion,
			Solicitud: &solicitud,
		}, true, nil
	case afirmauri.OperacionSave:
		return legacyws.Resultado{
			Tipo:      "save",
			Texto:     h.handleSave(ctx, solicitud),
			Operacion: solicitud.Operacion,
			Solicitud: &solicitud,
		}, true, nil
	case afirmauri.OperacionLoad:
		return legacyws.Resultado{
			Tipo:      "load",
			Texto:     h.handleLoad(ctx, solicitud),
			Operacion: solicitud.Operacion,
			Solicitud: &solicitud,
		}, true, nil
	case afirmauri.OperacionSignSave:
		resp, err := h.handleSignAndSave(ctx, solicitud)
		if err != nil {
			return legacyws.Resultado{}, true, err
		}
		return legacyws.Resultado{
			Tipo:      "signandsave",
			Texto:     resp,
			Operacion: solicitud.Operacion,
			Solicitud: &solicitud,
		}, true, nil
	default:
		return legacyws.Resultado{}, false, nil
	}
}

func (h *legacyWebSocketHandler) handleSign(ctx context.Context, solicitud afirmauri.Solicitud) (string, error) {
	start := time.Now()
	probe := newOperationProbe(ctx, slog.Default(), "legacy_sign")
	probe.Start()
	defer probe.Stop("return")
	slog.Info("legacy_handle_sign_start", "has_sign", solicitud.SignCommand != nil, "has_retrieve", solicitud.RetrieveCommand != nil, "upload_endpoint", strings.TrimSpace(solicitud.Sesion.UploadEndpoint) != "", "retrieve_endpoint", strings.TrimSpace(solicitud.Sesion.RetrieveEndpoint) != "")
	slog.Info("legacy_handle_sign_build_command")
	cmd, solicitudResuelta, err := h.buildSignCommand(ctx, solicitud)
	if err != nil {
		probe.Stop("build_command_error", "error", err)
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	probe.Mark("build_command_done", "format", cmd.Format, "action", cmd.Action)
	formatoWeb := strings.TrimSpace(legacyQueryParam(solicitud.LegacyParams, "format", "signFormat"))
	servidorTrifasico := strings.TrimSpace(cmd.Options["serverUrl"])
	if triphase.EsFormatoTrifasico(formatoWeb) && servidorTrifasico != "" && h.retrieve != nil {
		return h.firmarTrifasicoServidor(ctx, cmd, solicitudResuelta, formatoWeb, servidorTrifasico)
	}
	if formato := formatoWeb; solicitud.SignCommand != nil &&
		strings.HasSuffix(strings.ToLower(formato), "tri") {
		avisos.Registrar(
			"Firma trifásica calculada en este equipo",
			"el portal pidió "+formato+" y envió el documento; se firma aquí con el formato "+string(cmd.Format)+
				" equivalente, sin mandar el documento a otro servidor",
		)
	}
	slog.Info("legacy_handle_sign_build_command_done", "elapsed_ms_total", time.Since(start).Milliseconds())
	cmd.Options = expandLegacyProtocolSignOptions(cmd.Options, solicitudResuelta.LegacyParams, string(cmd.Format))
	cmd.Options = aplicarFormatoXAdESPorDefectoJava(cmd.Options, cmd.Format)

	slog.Info("legacy_handle_sign_select_certificate")
	selectStart := time.Now()
	cert, certDER, err := h.selectCertificate(ctx, solicitudResuelta)
	if err != nil {
		probe.Stop("select_certificate_error", "error", err)
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	probe.Mark("select_certificate_done", "certificate_id", cert.ID)
	slog.Info("legacy_handle_sign_select_certificate_done", "elapsed_ms_total", time.Since(start).Milliseconds(), "elapsed_ms_step", time.Since(selectStart).Milliseconds())
	cmd.CertificateID = cert.ID
	// La web puede pedir que el usuario sitúe la firma visible (Java:
	// visibleSignature=want).
	nombreCertificado := cert.Subject
	if parsed, parseErr := x509.ParseCertificate(certDER); parseErr == nil && strings.TrimSpace(parsed.Subject.CommonName) != "" {
		nombreCertificado = strings.TrimSpace(parsed.Subject.CommonName)
	}
	if cmd.Options, err = certpicker.ResolverSelloVisible(ctx, h.selector, cmd.Format, cmd.Options, cmd.Document, nombreCertificado); err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}

	setProtocolPhase("sign_execute", tl("Procesando firma..."), tl("La firma se está generando con el certificado seleccionado"))
	slog.Info("legacy_handle_sign_execute", "format", cmd.Format, "action", cmd.Action)
	resultado, err := h.signUC.Execute(ctx, cmd)
	if err != nil {
		probe.Stop("sign_execute_error", "certificate_id", cert.ID, "error", err)
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	probe.Mark("sign_execute_done", "certificate_id", cert.ID, "result_bytes", len(resultado.Result.Data))
	slog.Info("legacy_handle_sign_execute_done", "elapsed_ms_total", time.Since(start).Milliseconds())
	extraInfo := buildLegacyExtraInfo(cmd.Document.Name, solicitudResuelta.Version)
	probe.Stop("legacy_sign_response_ready", "certificate_id", cert.ID, "response_extra_len", len(extraInfo))
	return encodeLegacySignResponse(certDER, resultado.Result.Data, solicitudResuelta.Sesion.SessionKey, extraInfo, solicitudResuelta.Sesion.RetrieveEndpoint, solicitudResuelta.Sesion.UploadEndpoint)
}

// firmarTrifasicoServidor completa una firma "tri" contra el servidor de la
// web (como AutoFirma Java): el usuario elige certificado y confirma igual que
// en una firma local, y la clave solo firma el resumen que envía el servidor.
func (h *legacyWebSocketHandler) firmarTrifasicoServidor(ctx context.Context, cmd application.SignCommand, solicitud afirmauri.Solicitud, formato, servidor string) (string, error) {
	cert, certDER, err := h.selectCertificate(ctx, solicitud)
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	nombreCertificado := cert.Subject
	if parsed, parseErr := x509.ParseCertificate(certDER); parseErr == nil && strings.TrimSpace(parsed.Subject.CommonName) != "" {
		nombreCertificado = strings.TrimSpace(parsed.Subject.CommonName)
	}
	// En firma trifásica el dato local puede ser un identificador del servidor;
	// no se presenta como si fuera el PDF que se va a firmar.
	cmd.Options, err = certpicker.ResolverSelloVisible(ctx, h.selector, cmd.Format, cmd.Options, domain.Document{}, nombreCertificado)
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	if h.approval == nil {
		return "", errors.New("aprobador de firma no configurado")
	}
	aprobado, err := h.approval.Request(ctx, application.MensajeAprobacionFirma(cmd, cert))
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", fmt.Errorf("error al solicitar aprobación de la firma: %w", err)
	}
	if !aprobado {
		return "CANCEL", nil
	}
	key, err := h.keys.KeyFor(ctx, cert)
	if err != nil {
		ports.CloseSigningKey(key)
		return "", fmt.Errorf("clave de firma no disponible: %w", err)
	}
	defer ports.CloseSigningKey(key)
	avisos.Registrar("Firma trifásica con el servidor de la web",
		"el portal pidió "+formato+": el documento se procesa en su servidor de firma y este equipo solo firma el resumen con tu certificado, como AutoFirma")
	setProtocolPhase("sign_execute", tl("Procesando firma..."), tl("La firma se está generando con el certificado seleccionado"))
	firma, err := h.retrieve.FirmarConServidor(triphase.ContextWithSigningKey(ctx, key), triphase.SolicitudFirmaServidor{
		ServerURL: servidor, FormatoLegacy: formato, Accion: cmd.Action,
		Algoritmo: cmd.Options["algorithm"], Datos: cmd.Document.Content, ExtraParams: cmd.Options,
	})
	if err != nil {
		return "", err
	}
	return encodeLegacySignResponse(certDER, firma, solicitud.Sesion.SessionKey, buildLegacyExtraInfo(cmd.Document.Name, solicitud.Version), solicitud.Sesion.RetrieveEndpoint, solicitud.Sesion.UploadEndpoint)
}

// aplicarFormatoXAdESPorDefectoJava replica AutoFirma Java: una firma XAdES
// pedida por una web sin propiedad "format" se genera como XAdES Enveloping.
func aplicarFormatoXAdESPorDefectoJava(options map[string]string, format domain.SignatureFormat) map[string]string {
	if format != domain.FormatXAdES {
		return options
	}
	for k, v := range options {
		if strings.EqualFold(k, "format") && strings.TrimSpace(v) != "" {
			return options
		}
	}
	out := make(map[string]string, len(options)+1)
	for k, v := range options {
		out[k] = v
	}
	out["format"] = "XAdES Enveloping"
	return out
}

func (h *legacyWebSocketHandler) handleSelectCert(ctx context.Context, solicitud afirmauri.Solicitud) (string, error) {
	_, certDER, err := h.selectCertificate(ctx, solicitud)
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	return encodeLegacyCertificateResponse(certDER, solicitud.Sesion.SessionKey, solicitud.Sesion.RetrieveEndpoint, solicitud.Sesion.UploadEndpoint)
}

func (h *legacyWebSocketHandler) handleBatch(ctx context.Context, solicitud afirmauri.Solicitud) (string, error) {
	if solicitud.RemoteBatch != nil {
		return h.handleRemoteBatch(ctx, *solicitud.RemoteBatch)
	}
	if solicitud.BatchCommand != nil {
		meta, err := h.resolveBatchMetadataFromLegacyParams(solicitud)
		if err != nil {
			return "", err
		}
		return h.handleLocalBatch(ctx, *solicitud.BatchCommand, solicitud, meta)
	}
	if solicitud.RetrieveCommand == nil || h.retrieve == nil {
		return "", fmt.Errorf("solicitud batch legacy sin datos recuperables")
	}

	raw, err := h.retrieve.RetrieveRaw(ctx, solicitud.Sesion)
	if err != nil {
		return "", err
	}
	raw, err = triphase.DecodeRetrievePayload(raw, solicitud.Sesion.SessionKey, solicitud.Sesion.RetrieveEndpoint, solicitud.Sesion.UploadEndpoint)
	if err != nil {
		return "", err
	}
	remote, cmd, _, err := afirmauri.ResolveRetrievedBatch(raw, solicitud.Sesion)
	if err != nil {
		return "", err
	}
	if remote != nil {
		return h.handleRemoteBatch(ctx, *remote)
	}
	if cmd == nil {
		return "", fmt.Errorf("lote recuperado sin comando procesable")
	}
	meta, err := afirmauri.ResolveRetrievedBatchMetadata(raw, solicitud.Sesion)
	if err != nil {
		return "", err
	}
	return h.handleLocalBatch(ctx, *cmd, solicitud, meta)
}

func (h *legacyWebSocketHandler) buildSignCommand(ctx context.Context, solicitud afirmauri.Solicitud) (application.SignCommand, afirmauri.Solicitud, error) {
	slog.Info("legacy_build_sign_command_start", "has_sign", solicitud.SignCommand != nil, "has_retrieve", solicitud.RetrieveCommand != nil)
	if solicitud.SignCommand != nil {
		slog.Info("legacy_build_sign_command_direct")
		return *solicitud.SignCommand, solicitud, nil
	}
	if solicitud.RetrieveCommand == nil || h.retrieve == nil {
		slog.Info("legacy_build_sign_command_local_interactive")
		return h.buildLocalInteractiveSignCommand(ctx, solicitud)
	}

	slog.Info("legacy_build_sign_command_remote_retrieve_start")
	raw, err := h.retrieve.RetrieveRaw(ctx, solicitud.Sesion)
	if err != nil {
		return application.SignCommand{}, solicitud, err
	}
	slog.Info("legacy_build_sign_command_remote_retrieve_done", "payload_len", len(raw))
	raw, err = triphase.DecodeRetrievePayload(raw, solicitud.Sesion.SessionKey, solicitud.Sesion.RetrieveEndpoint, solicitud.Sesion.UploadEndpoint)
	if err != nil {
		return application.SignCommand{}, solicitud, err
	}
	if len(raw) == 0 {
		return application.SignCommand{}, solicitud, fmt.Errorf("payload remoto vacío")
	}

	slog.Info("legacy_build_sign_command_resolve_payload")
	payload, solicitudResuelta, err := resolveLegacySignPayload(raw, solicitud)
	if err != nil {
		return application.SignCommand{}, solicitud, err
	}

	name := inferLegacyDocumentName(solicitudResuelta)
	mime := inferLegacyMimeType(solicitudResuelta.Formato)
	doc, err := domain.NewDocument(name, payload, mime)
	if err != nil {
		return application.SignCommand{}, solicitud, err
	}
	return application.SignCommand{
		Document: doc,
		Format:   solicitudResuelta.Formato,
		Action:   solicitudResuelta.AccionFirma,
		Options:  clonarOpcionesLegacy(solicitudResuelta.Options),
	}, solicitudResuelta, nil
}

func (h *legacyWebSocketHandler) buildLocalInteractiveSignCommand(ctx context.Context, solicitud afirmauri.Solicitud) (application.SignCommand, afirmauri.Solicitud, error) {
	if h.documentos == nil {
		slog.Error("legacy_document_pick_missing_picker")
		return application.SignCommand{}, solicitud, fmt.Errorf("solicitud de firma legacy sin datos recuperables ni selector documental")
	}

	setProtocolPhase("document_pick", tl("Preparando documento..."), tl("Abriendo el selector de fichero solicitado por la web"))
	documentPickStart := time.Now()
	slog.Info("legacy_document_pick_start", "operation", solicitud.Operacion)
	documento, err := h.documentos.Pick(ctx)
	protocolDiagnosticMeasurePhase("document_pick", documentPickStart)
	if err != nil {
		slog.Error("legacy_document_pick_error", "error", err)
		return application.SignCommand{}, solicitud, err
	}
	slog.Info("legacy_document_pick_selected", "name", documento.Name, "size", len(documento.Content))
	documento = normalizarDocumentoLegacy(documento, solicitud)
	formato := resolverFormatoLocalLegacy(solicitud, documento.Name)

	return application.SignCommand{
		Document: documento,
		Format:   formato,
		Action:   solicitud.AccionFirma,
		Options:  clonarOpcionesLegacy(solicitud.Options),
	}, solicitud, nil
}

func (h *legacyWebSocketHandler) selectCertificate(ctx context.Context, solicitud afirmauri.Solicitud) (domain.CertificateRef, []byte, error) {
	if h.catalogo == nil {
		return domain.CertificateRef{}, nil, fmt.Errorf("catálogo de certificados no configurado")
	}
	if h.selector == nil {
		return domain.CertificateRef{}, nil, fmt.Errorf("selector de certificado no configurado")
	}

	trace := logging.New("legacy/selectcert", os.Stderr)
	start := time.Now()
	setProtocolPhase("certificate_catalog_load", tl("Cargando certificados..."), tl("Preparando el selector de certificados"))
	slog.Info("legacy_select_certificate_start", "operation", solicitud.Operacion)
	trace.InfoContext(ctx, "legacy_select_certificate_start", "operation", string(solicitud.Operacion), "origins", strings.Join(solicitud.Origenes, ","), "sticky_param", legacyQueryParam(solicitud.LegacyParams, "sticky"), "has_request_id", strings.TrimSpace(solicitud.Sesion.RequestID) != "")
	certLoadStart := time.Now()
	var candidatos []domain.CertificateRef
	var err error
	if certpicker.SupportsCredentialLoading(h.selector) {
		// El selector puede cargar o refrescar identidades durante la sesión.
		// No reutilizar la instantánea anterior a esa acción.
		candidatos, err = h.catalogo.List(ctx)
	} else {
		candidatos, err = h.loadCertificatesCached(ctx)
	}
	protocolDiagnosticMeasurePhase("certificate_catalog_load", certLoadStart)
	if err != nil {
		setProtocolPhase("certificate_catalog_load", tl("Error cargando certificados"), tl("No se pudo preparar el catálogo de certificados"))
		slog.Error("legacy_select_certificate_catalog_error", "elapsed_ms", time.Since(start).Milliseconds(), "error", err)
		trace.ErrorContext(ctx, "legacy_select_certificate_catalog_error", "elapsed_ms", time.Since(start).Milliseconds(), "error", err)
		return domain.CertificateRef{}, nil, err
	}
	// Filtros de la web (filters=...), como AutoFirma Java: si ninguno
	// cumple, no se ofrecen los demás y se explica el motivo.
	filtro := application.FiltrarCertificados(candidatos, solicitud.Options, time.Now())
	if len(filtro.Ignorados) > 0 {
		trace.WarnContext(ctx, "legacy_select_certificate_filtros_desconocidos", "filtros", strings.Join(filtro.Ignorados, ","))
	}
	if filtro.Aplicado {
		avisos.Registrar("Certificados acotados por la web", application.DescribirFiltro(filtro))
		if len(filtro.Certificados) == 0 && len(candidatos) > 0 && !certpicker.SupportsCredentialLoading(h.selector) {
			setProtocolPhase("certificate_catalog_load", tl("Ningún certificado cumple los requisitos"), tl("La web solo admite certificados con unas condiciones que no cumple ninguno de este equipo"))
			return domain.CertificateRef{}, nil, fmt.Errorf("no hay certificados compatibles: ninguno cumple los requisitos de la web (%s)", filtro.Descripcion)
		}
	}
	candidatos = filtro.Certificados
	slog.Info("legacy_select_certificate_catalog_ready", "elapsed_ms", time.Since(start).Milliseconds(), "count", len(candidatos))
	trace.InfoContext(ctx, "legacy_select_certificate_catalog_ready", "elapsed_ms", time.Since(start).Milliseconds(), "count", len(candidatos))
	if len(candidatos) == 0 && !certpicker.SupportsCredentialLoading(h.selector) {
		setProtocolPhase("certificate_catalog_load", tl("Sin certificados disponibles"), tl("No se han encontrado certificados compatibles"))
		trace.WarnContext(ctx, "legacy_select_certificate_no_candidates")
		return domain.CertificateRef{}, nil, fmt.Errorf("no hay certificados compatibles disponibles")
	}

	sticky := parseLegacyBool(legacyQueryParam(solicitud.LegacyParams, "sticky"))
	resetSticky := parseLegacyBool(legacyQueryParam(solicitud.LegacyParams, "resetsticky", "resetSticky"))
	if resetSticky {
		h.clearStickyCertificate()
	}
	requireExplicitSelection := nativeProtocolUIEnabled() || certpicker.SupportsCredentialLoading(h.selector)
	if sticky && !requireExplicitSelection {
		if ref, ok := h.findStickyCertificate(candidatos); ok {
			trace.InfoContext(ctx, "legacy_select_certificate_sticky_hit", "certificate_id", ref.ID, "count", len(candidatos))
			return h.materializarCertificado(ctx, ref)
		}
		trace.InfoContext(ctx, "legacy_select_certificate_sticky_miss", "count", len(candidatos))
	}
	// Con un único certificado ya no se selecciona en silencio: el selector
	// es el consentimiento del usuario. Sin él, cualquier página de un
	// origen de confianza (o con XSS en él) podría obtener firmas o el
	// certificado sin ninguna interacción si la clave no pide PIN.

	slog.Info("legacy_select_certificate_show_picker", "count", len(candidatos))
	trace.InfoContext(ctx, "legacy_select_certificate_show_picker", "count", len(candidatos))
	setProtocolPhase("certificate_select", tl("Selecciona un certificado"), tl("Elige el certificado con el que quieres continuar para firmar"))
	selectStart := time.Now()
	seleccion, err := h.selector.Select(certpicker.ConContextoSolicitud(ctx, primerOrigenLegacy(solicitud.Origenes), string(solicitud.Operacion)), candidatos)
	protocolDiagnosticMeasurePhase("certificate_select", selectStart)
	if err != nil {
		setProtocolPhase("certificate_select", tl("Selección cancelada"), tl("No se seleccionó ningún certificado"))
		slog.Error("legacy_select_certificate_picker_error", "error", err)
		trace.ErrorContext(ctx, "legacy_select_certificate_picker_error", "elapsed_ms", time.Since(selectStart).Milliseconds(), "error", err)
		return domain.CertificateRef{}, nil, err
	}
	ref := seleccion.Certificado
	slog.Info("legacy_select_certificate_picker_selected")
	if aviso := application.AvisoCaducidadCertificado(ref, time.Now()); aviso != "" {
		avisos.Registrar("Certificado próximo a caducar", aviso)
	}
	trace.InfoContext(ctx, "legacy_select_certificate_picker_selected", "elapsed_ms", time.Since(selectStart).Milliseconds(), "certificate_id", ref.ID, "remember_mode", string(seleccion.Recuerdo))
	if sticky && strings.TrimSpace(ref.ID) != "" {
		h.setStickyCertificate(ref.ID)
	}
	return h.materializarCertificado(ctx, ref)
}

func (h *legacyWebSocketHandler) cargarCertificadosLegacySinCache(ctx context.Context) ([]domain.CertificateRef, error) {
	start := time.Now()
	slog.Info("legacy_catalog_list_start")
	certs, err := h.catalogo.List(ctx)
	if err != nil {
		slog.Error("legacy_catalog_list_error", "elapsed_ms", time.Since(start).Milliseconds(), "error", err)
		return nil, fmt.Errorf("no se pudo obtener el catálogo de certificados: %w", err)
	}
	slog.Info("legacy_catalog_list_done", "elapsed_ms", time.Since(start).Milliseconds(), "count", len(certs))
	if len(certs) == 0 {
		return nil, nil
	}

	ahora := time.Now()
	candidatos := make([]domain.CertificateRef, 0, len(certs))
	for _, cert := range certs {
		if cert.IsExpired(ahora) {
			continue
		}
		candidatos = append(candidatos, cert)
	}
	if len(candidatos) == 0 {
		return certs, nil
	}
	return candidatos, nil
}

func (h *legacyWebSocketHandler) materializarCertificado(ctx context.Context, ref domain.CertificateRef) (domain.CertificateRef, []byte, error) {
	if ref.IsExpired(time.Now()) {
		return domain.CertificateRef{}, nil, errors.New("el certificado seleccionado está caducado; utilice otro certificado vigente")
	}
	trace := logging.New("legacy/selectcert", os.Stderr)
	if h.keys == nil {
		trace.ErrorContext(ctx, "legacy_select_certificate_materialize_missing_provider", "certificate_id", ref.ID)
		return domain.CertificateRef{}, nil, fmt.Errorf("proveedor de claves no configurado")
	}
	key, err := h.keys.KeyFor(ctx, ref)
	if err != nil {
		ports.CloseSigningKey(key)
		trace.ErrorContext(ctx, "legacy_select_certificate_materialize_key_error", "certificate_id", ref.ID, "error", err)
		return domain.CertificateRef{}, nil, fmt.Errorf("clave de firma no disponible para el certificado seleccionado: %w", err)
	}
	defer ports.CloseSigningKey(key)
	chain := key.CertificateChainDER()
	if len(chain) == 0 || len(chain[0]) == 0 {
		trace.ErrorContext(ctx, "legacy_select_certificate_materialize_empty_der", "certificate_id", ref.ID)
		return domain.CertificateRef{}, nil, fmt.Errorf("el certificado seleccionado no expone DER")
	}
	trace.InfoContext(ctx, "legacy_select_certificate_materialize_ok", "certificate_id", ref.ID, "der_len", len(chain[0]))
	return ref, append([]byte(nil), chain[0]...), nil
}

func inferLegacyDocumentName(solicitud afirmauri.Solicitud) string {
	ext := ".bin"
	switch strings.ToLower(strings.TrimSpace(string(solicitud.Formato))) {
	case "pades":
		ext = ".pdf"
	case "xades":
		ext = ".xml"
	case "cades":
		ext = ".csig"
	}
	base := strings.TrimSpace(solicitud.Sesion.RequestID)
	if base == "" {
		base = "documento"
	}
	base = filepath.Base(base)
	if filepath.Ext(base) == "" {
		base += ext
	}
	return base
}

func inferLegacyMimeType(format domain.SignatureFormat) string {
	switch strings.ToLower(strings.TrimSpace(string(format))) {
	case "pades":
		return "application/pdf"
	case "xades":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

func resolverFormatoLocalLegacy(solicitud afirmauri.Solicitud, nombre string) domain.SignatureFormat {
	raw := strings.ToLower(strings.TrimSpace(legacyQueryParam(solicitud.LegacyParams, "format", "signFormat")))
	if raw != "" && raw != "auto" {
		return solicitud.Formato
	}

	switch strings.ToLower(strings.TrimSpace(filepath.Ext(nombre))) {
	case ".pdf":
		return domain.FormatPAdES
	case ".xml":
		return domain.FormatXAdES
	default:
		return domain.FormatCAdES
	}
}

func normalizarDocumentoLegacy(documento domain.Document, solicitud afirmauri.Solicitud) domain.Document {
	nombre := strings.TrimSpace(filepath.Base(documento.Name))
	if nombre == "" || nombre == "." || nombre == "/" {
		nombre = "documento"
	}

	mimeType := strings.TrimSpace(documento.MIMEType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(nombre)))
	}
	if mimeType == "" {
		mimeType = inferLegacyMimeType(resolverFormatoLocalLegacy(solicitud, nombre))
	}

	documento.Name = nombre
	documento.MIMEType = mimeType
	return documento
}

func buildLegacyExtraInfo(name string, version int) []byte {
	if version < 3 {
		return nil
	}
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." || name == "/" {
		return nil
	}
	raw, err := json.Marshal(map[string]string{"filename": name})
	if err != nil {
		return nil
	}
	return raw
}

func encodeLegacyCertificateResponse(certDER []byte, sessionKey string, endpoints ...string) (string, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return base64.URLEncoding.EncodeToString(certDER), nil
	}
	return encryptAndFormatLegacyProtocol(certDER, []byte(sessionKey), endpoints...)
}

func encodeLegacySignResponse(certDER, signature []byte, sessionKey string, extraInfo []byte, endpoints ...string) (string, error) {
	if strings.TrimSpace(sessionKey) == "" {
		out := base64.URLEncoding.EncodeToString(certDER) + "|" + base64.URLEncoding.EncodeToString(signature)
		if len(extraInfo) == 0 {
			return out, nil
		}
		return out + "|" + base64.URLEncoding.EncodeToString(extraInfo), nil
	}
	encCert, err := encryptAndFormatLegacyProtocol(certDER, []byte(sessionKey), endpoints...)
	if err != nil {
		return "", err
	}
	encSig, err := encryptAndFormatLegacyProtocol(signature, []byte(sessionKey), endpoints...)
	if err != nil {
		return "", err
	}
	if len(extraInfo) == 0 {
		return encCert + "|" + encSig, nil
	}
	encExtra, err := encryptAndFormatLegacyProtocol(extraInfo, []byte(sessionKey), endpoints...)
	if err != nil {
		return "", err
	}
	return encCert + "|" + encSig + "|" + encExtra, nil
}

func encryptAndFormatLegacyProtocol(data []byte, keyBytes []byte, endpoints ...string) (string, error) {
	padLen := (8 - (len(data) % 8)) % 8
	if padLen > 0 {
		data = append(data, make([]byte, padLen)...)
	}
	encBytes, err := encryptDES(data, keyBytes, endpoints...)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%s", padLen, base64.URLEncoding.EncodeToString(encBytes)), nil
}

func encryptDES(plaintext []byte, key []byte, endpoints ...string) ([]byte, error) {
	if err := legacycrypto.RequireDESEnIntercambio(endpoints...); err != nil {
		return nil, err
	}
	if len(key) < 8 {
		paddedKey := make([]byte, 8)
		copy(paddedKey, key)
		key = paddedKey
	} else if len(key) > 8 {
		key = key[:8]
	}
	block, err := des.NewCipher(key) // #nosec G405 -- DES is required only for V1.9 interoperability and is gated by explicit operator opt-in above.
	if err != nil {
		return nil, err
	}
	if len(plaintext)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("plaintext is not a multiple of the block size")
	}
	ciphertext := make([]byte, len(plaintext))
	bs := block.BlockSize()
	for i := 0; i < len(plaintext); i += bs {
		block.Encrypt(ciphertext[i:i+bs], plaintext[i:i+bs])
	}
	return ciphertext, nil
}

func clonarOpcionesLegacy(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func primerOrigenLegacy(origenes []string) string {
	for _, origen := range origenes {
		if origen = strings.TrimSpace(origen); origen != "" {
			return origen
		}
	}
	return ""
}

func (h *legacyWebSocketHandler) pickLegacyLoadPaths(ctx context.Context, initialPath, extensions string, multi bool) ([]string, error) {
	picker := legacyLoadPicker(selectLegacyLoadPaths)
	if h != nil && h.loadPicker != nil {
		picker = h.loadPicker
	}
	return picker(ctx, initialPath, extensions, multi)
}

func (h *legacyWebSocketHandler) pickLegacySaveTarget(ctx context.Context, defaultPath, extensions string) (string, error) {
	picker := legacySavePicker(selectLegacySaveTargetPath)
	if h != nil && h.savePicker != nil {
		picker = h.savePicker
	}
	return picker(ctx, defaultPath, extensions)
}

func (h *legacyWebSocketHandler) handleSave(ctx context.Context, solicitud afirmauri.Solicitud) string {
	raw := strings.TrimSpace(legacyQueryParam(solicitud.LegacyParams, "dat", "data"))
	if raw == "" {
		return "SAF_05: No se han proporcionado datos para guardar"
	}
	data, err := decodeLegacyProtocolBase64(raw)
	if err != nil {
		data = []byte(raw)
	}
	targetPath, err := buildSafeLegacyTargetPath(
		solicitud.LegacyParams,
		legacyQueryParam(solicitud.LegacyParams, "filename", "fileName"),
		".bin",
	)
	if err != nil {
		return "SAF_05: No se pudo determinar ruta de guardado"
	}
	selectedPath, err := h.pickLegacySaveTarget(ctx, targetPath, effectiveLegacySaveExtensions(solicitud.LegacyParams, ".bin"))
	if err != nil {
		if isLegacySaveCancellation(err) {
			return "CANCEL"
		}
		return "SAF_05: No se pudo guardar el fichero"
	}
	selectedPath = strings.TrimSpace(selectedPath)
	if selectedPath == "" {
		return "CANCEL"
	}
	targetPath, ok := resolveUserChosenWritablePath(selectedPath)
	if ok && destinoEjecutable(targetPath) {
		return "SAF_05: Tipo de fichero no permitido por seguridad"
	}
	if !ok {
		return "SAF_05: Ruta fuera del ámbito permitido"
	}
	savedPath, err := writeLegacySavedFile(targetPath, data)
	if err != nil {
		return "SAF_05: No se pudo guardar el fichero"
	}
	if h.afterSave != nil {
		h.afterSave(savedPath, len(data))
	}
	if h.afterTerminal != nil {
		h.afterTerminal(afirmauri.OperacionSave, "SAVE_OK")
	}
	return "SAVE_OK"
}

func (h *legacyWebSocketHandler) handleRemoteBatch(ctx context.Context, req afirmauri.RemoteBatchCommand) (string, error) {
	if h.batchRemote == nil {
		return "", fmt.Errorf("ejecutor batch remoto no configurado")
	}
	if h.keys == nil {
		return "", fmt.Errorf("proveedor de claves no configurado")
	}
	ref, _, err := h.selectCertificate(ctx, afirmauri.Solicitud{LegacyParams: req.LegacyParams})
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	if h.approval == nil {
		return "", errors.New("aprobador del lote remoto no configurado")
	}
	approved, err := h.approval.Request(ctx,
		fmt.Sprintf("¿Desea procesar el lote remoto con el certificado '%s'?", ref.Subject))
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", fmt.Errorf("error al solicitar aprobación del lote remoto: %w", err)
	}
	if !approved {
		return "CANCEL", nil
	}
	key, err := h.keys.KeyFor(ctx, ref)
	if err != nil {
		ports.CloseSigningKey(key)
		return "", err
	}
	defer ports.CloseSigningKey(key)
	ctx = triphase.ContextWithSigningKey(ctx, key)
	result, err := h.batchRemote.ExecuteLegacyRemoteResult(ctx, req)
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		presentLegacySHA1Failure("batch", err)
		return legacyRemoteBatchErrorResult(err), nil
	}
	return result, nil
}

func (h *legacyWebSocketHandler) handleLocalBatch(ctx context.Context, cmd application.ProcessBatchCommand, solicitud afirmauri.Solicitud, meta afirmauri.BatchMetadata) (string, error) {
	if h.batchUC == nil {
		return "", fmt.Errorf("caso de uso batch no configurado")
	}
	// El payload es la fuente canónica de esta bandera. Esto cubre también los
	// lotes recuperados de servidor que fueron materializados antes de obtener
	// sus metadatos.
	cmd.StopOnError = meta.StopOnError
	resultado, err := h.batchUC.Execute(ctx, cmd)
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "SAF_20: Error en el proceso local del lote de firma", nil
	}
	resumen := buildLegacyBatchResultList(meta, resultado)
	raw, err := serializeLegacyBatchResponse(resumen, meta.IsJSON)
	if err != nil {
		return "", err
	}
	needCert := parseLegacyBool(legacyQueryParam(solicitud.LegacyParams, "needcert", "needCert"))
	var certDER []byte
	if needCert {
		if len(resultado.Results) > 0 {
			key, err := h.keys.KeyFor(ctx, resultado.Results[0].CertificateUsed)
			if err != nil {
				ports.CloseSigningKey(key)
				return "", err
			}
			defer ports.CloseSigningKey(key)
			chain := key.CertificateChainDER()
			if len(chain) > 0 {
				certDER = append([]byte(nil), chain[0]...)
			}
		}
		if len(certDER) == 0 {
			_, der, err := h.selectCertificate(ctx, solicitud)
			if err != nil {
				if isLegacyCancellation(err) {
					return "CANCEL", nil
				}
				return "", err
			}
			certDER = der
		}
	}
	return encodeLegacyBatchResult(raw, certDER, needCert, solicitud.Sesion.SessionKey, solicitud.Sesion.RetrieveEndpoint, solicitud.Sesion.UploadEndpoint)
}

func legacyRemoteBatchErrorResult(err error) string {
	if err == nil {
		return "SAF_27: Error en la firma por lotes"
	}
	if isLegacySHA1Failure(err) {
		return legacySHA1BlockedResponse
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "prefirma"),
		strings.Contains(lower, "postfirma"),
		strings.Contains(lower, "retrieve"),
		strings.Contains(lower, "storage"),
		strings.Contains(lower, "upload"),
		strings.Contains(lower, "servicio"),
		strings.Contains(lower, "timeout"),
		strings.Contains(lower, "connection"),
		strings.Contains(lower, "broken pipe"),
		strings.Contains(lower, "eof"),
		strings.Contains(lower, "http"):
		return "SAF_26: Error en la comunicación con el servicio de firma de lotes"
	default:
		return "SAF_27: Error en la firma por lotes"
	}
}

func isLegacySHA1Failure(err error) bool {
	return errors.Is(err, cryptopolicy.ErrSHA1Disabled)
}

func presentLegacySHA1Failure(action string, err error) bool {
	if !isLegacySHA1Failure(err) {
		return false
	}
	setProtocolPhase(
		"sign_execute",
		tl(legacySHA1BlockedTitleID),
		tl(legacySHA1BlockedDetailID),
	)
	protocolDiagnosticPersistFailure(action, err)
	return true
}

func (h *legacyWebSocketHandler) handleLoad(ctx context.Context, solicitud afirmauri.Solicitud) string {
	paths := splitLegacyLoadPaths(legacyQueryParam(solicitud.LegacyParams, "filePath", "filepath", "file", "fileName"))
	elegidasPorUsuario := false
	if len(paths) > 0 && !legacyDirectPathsEnabled() {
		return "SAF_25: Las rutas directas del protocolo están deshabilitadas"
	}
	if len(paths) == 0 {
		multiload := parseLegacyBool(legacyQueryParam(solicitud.LegacyParams, "multiload", "multiLoad", "multiple", "multi"))
		selectedPaths, err := h.pickLegacyLoadPaths(ctx, "", legacyQueryParam(solicitud.LegacyParams, "exts", "extensions"), multiload)
		if err != nil {
			if isLegacyLoadCancellation(err) {
				return "CANCEL"
			}
			if !isLegacyLoadPickerUnavailable(err) {
				return "SAF_25: No se pudo cargar el fichero"
			}
		} else {
			if len(selectedPaths) == 0 {
				return "CANCEL"
			}
			paths = selectedPaths
			elegidasPorUsuario = true
		}
	}
	if len(paths) == 0 && h.documentos != nil {
		documento, err := h.documentos.Pick(ctx)
		if err != nil {
			if isLegacyCancellation(err) {
				return "CANCEL"
			}
			return "SAF_25: No se pudo cargar el fichero"
		}
		displayName := filepath.Base(strings.TrimSpace(documento.Name))
		if !isLegacyLoadResponseNameAllowed(displayName) {
			return "SAF_25: Nombre de fichero no representable en el protocolo"
		}
		items := []string{
			displayName + ":" + base64.StdEncoding.EncodeToString(documento.Content),
		}
		return strings.Join(items, "|")
	}
	if len(paths) == 0 {
		return "SAF_25: No se ha indicado la ruta del fichero"
	}
	items := make([]string, 0, len(paths))
	for _, rawPath := range paths {
		displayName := filepath.Base(filepath.Clean(strings.TrimSpace(rawPath)))
		if !isLegacyLoadResponseNameAllowed(displayName) {
			return "SAF_25: Nombre de fichero no representable en el protocolo"
		}
		resolve := resolveLegacyReadablePath
		if elegidasPorUsuario {
			resolve = resolveUserChosenReadablePath
		}
		resolved, ok := resolve(rawPath)
		if !ok {
			return "SAF_25: Ruta fuera del ámbito permitido"
		}
		data, err := securefile.ReadFileLimit(resolved, legacyLoadMaxBytes)
		if err != nil {
			return "SAF_25: No se pudo cargar el fichero"
		}
		items = append(items, displayName+":"+base64.StdEncoding.EncodeToString(data))
	}
	return strings.Join(items, "|")
}

func isLegacyLoadResponseNameAllowed(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.ContainsAny(name, "|:")
}

func (h *legacyWebSocketHandler) handleSignAndSave(ctx context.Context, solicitud afirmauri.Solicitud) (string, error) {
	if solicitud.SignCommand == nil {
		return "SAF_44: Operacion de firma sin datos", nil
	}

	cmd := *solicitud.SignCommand
	cert, _, err := h.selectCertificate(ctx, solicitud)
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}
	cmd.CertificateID = cert.ID

	resultado, err := h.signUC.Execute(ctx, cmd)
	if err != nil {
		if isLegacyCancellation(err) {
			return "CANCEL", nil
		}
		return "", err
	}

	defaultName := strings.TrimSpace(filepath.Base(cmd.Document.Name))
	if defaultName == "" || defaultName == "." || defaultName == "/" {
		defaultName = "documento_firmado" + defaultSignedExtension(cmd.Format)
	}
	targetPath, err := buildSafeLegacyTargetPath(solicitud.LegacyParams, defaultName, defaultSignedExtension(cmd.Format))
	if err != nil {
		return "SAF_05: No se pudo determinar ruta de guardado", nil
	}
	selectedPath, err := h.pickLegacySaveTarget(ctx, targetPath, effectiveLegacySaveExtensions(solicitud.LegacyParams, defaultSignedExtension(cmd.Format)))
	if err != nil {
		if isLegacySaveCancellation(err) {
			return "CANCEL", nil
		}
		return "SAF_05: No se pudo guardar el fichero", nil
	}
	selectedPath = strings.TrimSpace(selectedPath)
	if selectedPath == "" {
		return "CANCEL", nil
	}
	targetPath, ok := resolveUserChosenWritablePath(selectedPath)
	if ok && destinoEjecutable(targetPath) {
		return "SAF_05: Tipo de fichero no permitido por seguridad", nil
	}
	if !ok {
		return "SAF_05: Ruta fuera del ámbito permitido", nil
	}
	savedPath, err := writeLegacySavedFile(targetPath, resultado.Result.Data)
	if err != nil {
		return "SAF_05: No se pudo guardar el fichero firmado", nil
	}
	if h.afterSave != nil {
		h.afterSave(savedPath, len(resultado.Result.Data))
	}
	if h.afterTerminal != nil {
		h.afterTerminal(afirmauri.OperacionSignSave, "SAVE_OK")
	}
	return "SAVE_OK", nil
}

func resolveLegacySignPayload(raw []byte, solicitud afirmauri.Solicitud) ([]byte, afirmauri.Solicitud, error) {
	trimmed := bytes.TrimSpace(raw)
	if !bytes.HasPrefix(trimmed, []byte("<sign>")) && !bytes.HasPrefix(trimmed, []byte("<sign ")) {
		return append([]byte(nil), raw...), solicitud, nil
	}

	var manifest legacySignManifest
	if err := xml.Unmarshal(trimmed, &manifest); err != nil {
		return nil, solicitud, fmt.Errorf("el manifiesto XML legado es inválido: %w", err)
	}
	if len(manifest.Entries) == 0 {
		return nil, solicitud, fmt.Errorf("el manifiesto XML legado no contiene entradas")
	}

	params := make(url.Values, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		key := strings.TrimSpace(strings.ToLower(entry.Key))
		value := strings.TrimSpace(entry.Value)
		if key == "" || value == "" {
			continue
		}
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		params.Set(key, value)
	}
	if len(params) == 0 {
		return nil, solicitud, fmt.Errorf("el manifiesto XML legado no contiene valores utilizables")
	}

	payloadRaw := strings.TrimSpace(legacyParam(params, "dat", "data"))
	if payloadRaw == "" {
		return nil, solicitud, fmt.Errorf("el manifiesto XML legado no contiene 'dat'")
	}
	payload, err := decodeLegacyProtocolBase64(payloadRaw)
	if err != nil {
		return nil, solicitud, fmt.Errorf("el payload del manifiesto XML legado no es base64 válido: %w", err)
	}

	resuelta := solicitud
	if rawFormat := strings.TrimSpace(legacyParam(params, "format", "signformat")); rawFormat != "" {
		format, err := application.ParseSignatureFormat(rawFormat)
		if err != nil {
			return nil, solicitud, fmt.Errorf("formato de firma inválido en manifiesto XML legado: %w", err)
		}
		resuelta.Formato = format
	}
	if key := strings.TrimSpace(legacyParam(params, "key", "cipherkey")); key != "" {
		resuelta.Sesion.SessionKey = key
	}
	if requestID := strings.TrimSpace(legacyParam(params, "fileid", "id", "fileid", "requestid")); requestID != "" {
		resuelta.Sesion.RequestID = requestID
	}
	for _, field := range []string{"params", "properties", "extraparams"} {
		rawValue := strings.TrimSpace(params.Get(field))
		if rawValue == "" {
			continue
		}
		body := rawValue
		if decoded, err := decodeLegacyProtocolBase64(rawValue); err == nil {
			body = string(decoded)
		}
		if resuelta.Options == nil {
			resuelta.Options = make(map[string]string)
		}
		for k, v := range decodeLegacyProperties(body) {
			resuelta.Options[k] = v
		}
	}
	return payload, resuelta, nil
}

func legacyParam(values url.Values, keys ...string) string {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func decodeLegacyRequestProperties(values url.Values) map[string]string {
	props := make(map[string]string)
	for _, field := range []string{"properties", "params", "extraParams", "extraparams"} {
		rawValue := strings.TrimSpace(legacyQueryParam(values, field))
		if rawValue == "" {
			continue
		}
		body := rawValue
		if decoded, err := decodeLegacyProtocolBase64(rawValue); err == nil {
			body = string(decoded)
		}
		for k, v := range decodeLegacyProperties(body) {
			props[k] = v
		}
	}
	return props
}

func decodeLegacyProperties(raw string) map[string]string {
	props := make(map[string]string)
	body := strings.TrimSpace(raw)
	if body == "" {
		return props
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, `\n`, "\n"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			props[line] = ""
			continue
		}
		key := strings.TrimSpace(line[:sep])
		value := strings.TrimSpace(line[sep+1:])
		if key != "" {
			props[key] = value
		}
	}
	return props
}

func expandLegacyProtocolSignOptions(base map[string]string, params url.Values, format string) map[string]string {
	out := clonarOpcionesLegacy(base)
	if out == nil {
		out = make(map[string]string)
	}

	props := decodeLegacyRequestProperties(params)
	for k, v := range props {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		out[k] = v
	}
	for k, values := range params {
		if len(values) == 0 {
			continue
		}
		v := strings.TrimSpace(values[len(values)-1])
		if v == "" {
			continue
		}
		out[k] = v
	}

	exp := strings.ToLower(strings.TrimSpace(out["expPolicy"]))
	switch exp {
	case "firmaage", "firmaage19":
		if strings.TrimSpace(out["policyIdentifier"]) == "" {
			out["policyIdentifier"] = "urn:oid:2.16.724.1.3.1.1.2.1.9"
		}
		if strings.TrimSpace(out["policyQualifier"]) == "" {
			out["policyQualifier"] = "https://sede.administracion.gob.es/politica_de_firma_anexo_1.pdf"
		}
		if strings.TrimSpace(out["policyIdentifierHashAlgorithm"]) == "" {
			out["policyIdentifierHashAlgorithm"] = "http://www.w3.org/2000/09/xmldsig#sha1"
		}
		if strings.TrimSpace(out["policyIdentifierHash"]) == "" {
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "xades":
				out["policyIdentifierHash"] = "G7roucf600+f03r/o0bAOQ6WAs0="
			default:
				out["policyIdentifierHash"] = "G7roucf600+f03r/o0bAOQ6WAs0="
			}
		}
	case "firmaage18":
		if strings.TrimSpace(out["policyIdentifier"]) == "" {
			out["policyIdentifier"] = "urn:oid:2.16.724.1.3.1.1.2.1.8"
		}
		if strings.TrimSpace(out["policyQualifier"]) == "" {
			out["policyQualifier"] = "https://sede.administracion.gob.es/PAG_Sede/dam/jcr:b0de3f91-5171-48e2-81f3-5c2407d9c091/politica_firma_AGE_v1_8.pdf"
		}
		if strings.TrimSpace(out["policyIdentifierHashAlgorithm"]) == "" {
			out["policyIdentifierHashAlgorithm"] = "http://www.w3.org/2000/09/xmldsig#sha1"
		}
		if strings.TrimSpace(out["policyIdentifierHash"]) == "" {
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "xades":
				out["policyIdentifierHash"] = "V8lVVNGDCPen6VELRD1Ja8HARFk="
				if strings.TrimSpace(out["xadesNamespace"]) == "" {
					out["xadesNamespace"] = "http://uri.etsi.org/01903/v1.2.2#"
				}
				if strings.TrimSpace(out["signedPropertiesTypeUrl"]) == "" {
					out["signedPropertiesTypeUrl"] = "http://uri.etsi.org/01903/v1.2.2#SignedProperties"
				}
			default:
				out["policyIdentifierHash"] = "7SxX3erFuH31TvAw9LZ70N7p1vA="
			}
		}
	}
	return out
}

func decodeLegacyProtocolBase64(raw string) ([]byte, error) {
	s := strings.ReplaceAll(strings.TrimSpace(raw), " ", "+")
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func legacyQueryParam(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func buildSafeLegacyTargetPath(values url.Values, defaultName, fallbackExt string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home no disponible")
	}
	downloads := filepath.Join(home, "Descargas")
	fileName := strings.TrimSpace(legacyQueryParam(values, "filename", "fileName"))
	explicit := strings.TrimSpace(legacyQueryParam(values, "filePath", "filepath", "file"))
	if fileName == "" {
		fileName = defaultName
	}
	fileName = sanitizeLegacyFilename(fileName, fallbackExt)
	if explicit == "" {
		return filepath.Join(downloads, fileName), nil
	}

	cleanExplicit := filepath.Clean(explicit)
	if !filepath.IsAbs(cleanExplicit) {
		return filepath.Join(downloads, sanitizeLegacyFilename(cleanExplicit, fallbackExt)), nil
	}
	if !legacyDirectPathsEnabled() {
		return "", fmt.Errorf("las rutas directas del protocolo están deshabilitadas")
	}
	if isPathWithinAllowedRoots(cleanExplicit, home, os.TempDir()) {
		if filepath.Ext(cleanExplicit) == "" && fallbackExt != "" {
			cleanExplicit += fallbackExt
		}
		return cleanExplicit, nil
	}
	return filepath.Join(downloads, sanitizeLegacyFilename(filepath.Base(cleanExplicit), fallbackExt)), nil
}

// legacyDirectPathsEnabled permite que el portal indique rutas locales
// concretas. Solo un administrador puede habilitarlo mediante la política de
// máquina; GRXFIRMA_ENABLE_LEGACY_DIRECT_PATHS no basta.
func legacyDirectPathsEnabled() bool {
	return machinepolicy.OptIn(machinepolicy.PermitirRutasDirectas)
}

func sanitizeLegacyFilename(name, fallbackExt string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, `\`, "/")))
	name = strings.ReplaceAll(name, "..", "_")
	// El nombre procede del portal: se descartan controles y caracteres
	// reservados de Windows para que nunca se interprete fuera de un nombre.
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if len(name) > 200 {
		name = strings.ToValidUTF8(name[:200], "")
	}
	if name == "" || name == "." || name == "/" {
		name = "grxfirma_guardado"
	}
	if filepath.Ext(name) == "" && fallbackExt != "" {
		name += fallbackExt
	}
	return name
}

func effectiveLegacySaveExtensions(values url.Values, fallbackExt string) string {
	raw := strings.TrimSpace(legacyQueryParam(values, "exts", "extensions"))
	if raw != "" {
		return raw
	}
	fallbackExt = strings.TrimSpace(strings.TrimPrefix(fallbackExt, "."))
	if fallbackExt == "" {
		return ""
	}
	return fallbackExt
}

func resolveLegacyReadablePath(rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", false
	}
	cleanPath := filepath.Clean(rawPath)
	if !filepath.IsAbs(cleanPath) {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return "", false
		}
		cleanPath = filepath.Join(home, cleanPath)
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false
	}
	if !isPathWithinAllowedRoots(cleanPath, home, os.TempDir()) {
		return "", false
	}
	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return "", false
	}
	resolvedRoots := make([]string, 0, 2)
	for _, root := range []string{home, os.TempDir()} {
		resolvedRoot, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr == nil {
			resolvedRoots = append(resolvedRoots, resolvedRoot)
		}
	}
	if !isPathWithinAllowedRoots(resolvedPath, resolvedRoots...) {
		return "", false
	}
	return resolvedPath, true
}

func resolveLegacyWritablePath(rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || strings.IndexByte(rawPath, 0) >= 0 {
		return "", false
	}
	cleanPath := filepath.Clean(rawPath)
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false
	}
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(home, "Descargas", sanitizeLegacyFilename(cleanPath, ""))
	}
	if !isPathWithinAllowedRoots(cleanPath, home, os.TempDir()) {
		return "", false
	}
	return cleanPath, true
}

func writeLegacySavedFile(targetPath string, data []byte) (string, error) {
	writer := desktopfilesystem.NuevoEscritorResultado(desktopfilesystem.PoliticaForzar)
	return writer.Escribir(targetPath, data)
}

// Las rutas que el usuario elige en el diálogo nativo son decisión suya, como
// en AutoFirma Java: pueden estar en otra unidad o en una carpeta de red. Solo
// las rutas propuestas por el portal quedan limitadas a su carpeta personal.
// Se rechazan rutas relativas, espacios de nombres de dispositivo de Windows
// (\\.\ y \\?\) y destinos que no sean ficheros regulares.
func resolveUserChosenReadablePath(rawPath string) (string, bool) {
	cleanPath, ok := cleanUserChosenPath(rawPath)
	if !ok {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return resolved, true
}

func resolveUserChosenWritablePath(rawPath string) (string, bool) {
	cleanPath, ok := cleanUserChosenPath(rawPath)
	if !ok {
		return "", false
	}
	if info, err := os.Lstat(cleanPath); err == nil && !info.Mode().IsRegular() {
		return "", false
	}
	// No se resuelven enlaces simbólicos del directorio: el escritor rechaza
	// seguirlos, igual que antes.
	return cleanPath, true
}

// extensionesEjecutables son tipos que Windows (o el escritorio en Linux)
// ejecuta o interpreta al abrirlos. Una web no debe poder dejar uno en disco
// a través de GrxFirma aunque el usuario pulse "Guardar" sin fijarse.
var extensionesEjecutables = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".scr": true, ".pif": true,
	".msi": true, ".msp": true, ".msc": true, ".lnk": true, ".url": true, ".ps1": true,
	".psm1": true, ".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true,
	".wsh": true, ".hta": true, ".cpl": true, ".dll": true, ".jar": true, ".reg": true,
	".scf": true, ".inf": true, ".appref-ms": true, ".application": true,
	".settingcontent-ms": true, ".gadget": true, ".sh": true, ".desktop": true,
}

// destinoEjecutable rechaza guardar datos de la web con una extensión
// ejecutable y lo explica al usuario.
func destinoEjecutable(path string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimRight(path, ". ")))
	if !extensionesEjecutables[ext] {
		return false
	}
	avisos.Registrar(
		"Guardado bloqueado por seguridad",
		"la web intentó guardar un fichero de tipo "+ext+", que el sistema podría ejecutar; GrxFirma nunca guarda ficheros ejecutables. Guarda la firma con su extensión habitual (.csig, .xsig, .pdf...)",
	)
	return true
}

func cleanUserChosenPath(rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || strings.IndexByte(rawPath, 0) >= 0 {
		return "", false
	}
	if strings.HasPrefix(rawPath, `\\.\`) || strings.HasPrefix(rawPath, `\\?\`) ||
		strings.HasPrefix(rawPath, "//./") || strings.HasPrefix(rawPath, "//?/") {
		return "", false
	}
	cleanPath := filepath.Clean(rawPath)
	if !filepath.IsAbs(cleanPath) {
		return "", false
	}
	return cleanPath, true
}

func isPathWithinAllowedRoots(target string, roots ...string) bool {
	target = filepath.Clean(target)
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, target)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func splitLegacyLoadPaths(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (h *legacyWebSocketHandler) clearStickyCertificate() {
	if h == nil {
		return
	}
	h.stickyMu.Lock()
	defer h.stickyMu.Unlock()
	h.stickyID = ""
}

func (h *legacyWebSocketHandler) setStickyCertificate(id string) {
	if h == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	h.stickyMu.Lock()
	defer h.stickyMu.Unlock()
	h.stickyID = id
}

func (h *legacyWebSocketHandler) findStickyCertificate(certs []domain.CertificateRef) (domain.CertificateRef, bool) {
	if h == nil {
		return domain.CertificateRef{}, false
	}
	h.stickyMu.Lock()
	defer h.stickyMu.Unlock()
	if strings.TrimSpace(h.stickyID) == "" {
		return domain.CertificateRef{}, false
	}
	for _, cert := range certs {
		if cert.ID == h.stickyID {
			return cert, true
		}
	}
	return domain.CertificateRef{}, false
}

func isLegacyCancellation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, certpicker.ErrSeleccionCancelada) ||
		errors.Is(err, desktopdocumentpicker.ErrSeleccionCancelada) {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "cancel")
}

func isLegacyLoadCancellation(err error) bool {
	return errors.Is(err, errLegacyLoadCanceled) || isLegacyCancellation(err)
}

func isLegacySaveCancellation(err error) bool {
	return errors.Is(err, errLegacySaveCanceled) || isLegacyCancellation(err)
}

func isLegacyLoadPickerUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errLegacyLoadPickerUnavailable) {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "selector legacy de carga no disponible") ||
		strings.Contains(message, "legacy load picker not available")
}

func defaultSignedExtension(format domain.SignatureFormat) string {
	switch strings.ToLower(strings.TrimSpace(string(format))) {
	case "pades":
		return ".pdf"
	case "xades":
		return ".xml"
	default:
		return ".csig"
	}
}

func parseLegacyBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "si", "on":
		return true
	default:
		return false
	}
}

func (h *legacyWebSocketHandler) resolveBatchMetadataFromLegacyParams(solicitud afirmauri.Solicitud) (afirmauri.BatchMetadata, error) {
	raw := strings.TrimSpace(legacyQueryParam(solicitud.LegacyParams, "dat", "data"))
	if raw == "" {
		return afirmauri.BatchMetadata{}, fmt.Errorf("payload batch legacy no disponible")
	}
	decoded, err := decodeLegacyProtocolBase64(raw)
	if err != nil {
		return afirmauri.BatchMetadata{}, err
	}
	return afirmauri.ParseBatchMetadata(decoded)
}

func buildLegacyBatchResultList(meta afirmauri.BatchMetadata, resultado application.BatchResult) []legacyBatchSingleResult {
	const (
		batchResultDone    = "DONE_AND_SAVED"
		batchResultSkipped = "SKIPPED"
		batchResultError   = "ERROR_PRE"
	)
	count := len(meta.IDs)
	if count == 0 {
		count = len(resultado.Results) + len(resultado.Errores)
	}
	items := make([]legacyBatchSingleResult, 0, count)
	firstError := count
	if meta.StopOnError {
		for index := range resultado.Errores {
			if index >= 0 && index < firstError {
				firstError = index
			}
		}
	}
	resultIndex := 0
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("item-%d", i+1)
		if i < len(meta.IDs) && strings.TrimSpace(meta.IDs[i]) != "" {
			id = strings.TrimSpace(meta.IDs[i])
		}
		item := legacyBatchSingleResult{ID: id, Result: batchResultDone}
		if err, ok := resultado.Errores[i]; ok {
			item.Result = batchResultError
			item.Description = strings.TrimSpace(err.Error())
		} else if meta.StopOnError && firstError < count {
			// LocalBatchSigner V1.9 revoca también los éxitos anteriores al
			// primer error y elimina sus firmas cuando stoponerror está activo.
			item.Result = batchResultSkipped
		} else if resultIndex < len(resultado.Results) {
			item.Signature = base64.StdEncoding.EncodeToString(resultado.Results[resultIndex].Result.Data)
			resultIndex++
		} else {
			// Un trabajo sin resultado ni error explícito no puede anunciarse
			// como guardado: ocurre, por ejemplo, tras cancelar el contexto.
			item.Result = batchResultSkipped
		}
		items = append(items, item)
	}
	return items
}

func serializeLegacyBatchResponse(results []legacyBatchSingleResult, isJSON bool) ([]byte, error) {
	if isJSON {
		return json.Marshal(legacyBatchResponse{Signs: results})
	}
	xmlResp := legacyXMLBatchResponse{
		Signs: make([]legacyXMLBatchResult, 0, len(results)),
	}
	for _, item := range results {
		xmlResp.Signs = append(xmlResp.Signs, legacyXMLBatchResult{
			ID:          strings.TrimSpace(item.ID),
			Result:      strings.TrimSpace(item.Result),
			Description: strings.TrimSpace(item.Description),
		})
	}
	out, err := xml.Marshal(xmlResp)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func encodeLegacyBatchResult(result []byte, certDER []byte, needCert bool, sessionKey string, endpoints ...string) (string, error) {
	if strings.TrimSpace(sessionKey) != "" {
		encBatch, err := encryptAndFormatLegacyProtocol(result, []byte(sessionKey), endpoints...)
		if err != nil {
			return "", err
		}
		if !needCert || len(certDER) == 0 {
			return encBatch, nil
		}
		encCert, err := encryptAndFormatLegacyProtocol(certDER, []byte(sessionKey), endpoints...)
		if err != nil {
			return "", err
		}
		return encBatch + "|" + encCert, nil
	}
	batchB64 := base64.StdEncoding.EncodeToString(result)
	if !needCert || len(certDER) == 0 {
		return batchB64, nil
	}
	return batchB64 + "|" + base64.StdEncoding.EncodeToString(certDER), nil
}
