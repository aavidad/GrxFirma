// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	mobileinbound "grxfirma/internal/adapters/inbound/mobile"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/updatecheck"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

const (
	mobilePKCS12DecodeMessage               = "No se pudo abrir el PKCS#12. Compruebe la contraseña; si usa un formato antiguo, reexpórtelo como PKCS#12 moderno (AES y SHA-256)."
	mobileCertificateNotCurrentMessage      = "El certificado no está vigente. Use un certificado vigente; si ha caducado, renuévelo."
	mobileSigningIdentityUnsupportedMessage = "El certificado o su clave no son aptos para firmar en Android. Use un certificado de firma con RSA de al menos 2048 bits o ECDSA de al menos 256 bits."
	mobileCertificateImportFallbackMessage  = "No se pudo importar el certificado. Compruebe que sea un PKCS#12 válido e inténtelo de nuevo."
)

type signService interface {
	Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error)
}

type verifyService interface {
	Execute(ctx context.Context, cmd application.VerifyCommand) (application.VerifyResult, error)
}

type selectCertificateService interface {
	Execute(ctx context.Context, cmd application.SelectCertificateCommand) (application.SelectCertificateResult, error)
}

type processBatchService interface {
	Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error)
}

type retrieveRequestService interface {
	Execute(ctx context.Context, cmd application.RetrieveRequestCommand) (application.RetrieveRequestResult, error)
}

type uploadResultService interface {
	Execute(ctx context.Context, cmd application.UploadResultCommand) error
}

type importCertificateService interface {
	Execute(ctx context.Context, cmd application.ImportCertificateCommand) (application.ImportCertificateResult, error)
}

type resolvePlatformProfileService interface {
	Execute(ctx context.Context, cmd application.ResolvePlatformProfileCommand) (application.ResolvePlatformProfileResult, error)
}

type androidIntentHandler interface {
	HandleShared(ctx context.Context, action, descriptor string, payload []byte) (mobileinbound.Solicitud, error)
}

// Facade es la entrada publica y estable para integraciones mobile.
type Facade struct {
	operationMu          sync.Mutex
	signService          signService
	verifyService        verifyService
	selectService        selectCertificateService
	batchService         processBatchService
	retrieveService      retrieveRequestService
	uploadService        uploadResultService
	importService        importCertificateService
	profileService       resolvePlatformProfileService
	androidIntentAdapter androidIntentHandler
	contractJSON         string
	session              *sessionIdentityStore
	temporaryDirectory   string
	systemTrustAnchors   bool
	revocationMode       string
	timeout              time.Duration
	regionMu             sync.RWMutex
	language             string
	timeZone             *time.Location
	// Dependencias de red sustituibles en pruebas; nil usa el motor real.
	clock              func() time.Time
	revocationCheck    func(context.Context, [][]byte) (commonsigner.CertificateOnlineRevocationResult, error)
	timestampProbe     func(context.Context, string, []byte) ([]byte, error)
	veriFactuQuery     func(context.Context, string) (json.RawMessage, error)
	veriFactuImageRead func(context.Context, []byte) (commonsigner.VeriFactuQR, error)
	updateCheck        func(context.Context, string) (updatecheck.Resultado, error)
}

func newFacade(signService signService, verifyService verifyService, selectService selectCertificateService) *Facade {
	return &Facade{
		signService:   signService,
		verifyService: verifyService,
		selectService: selectService,
		timeout:       30 * time.Second,
	}
}

func (f *Facade) withBatchService(service processBatchService) *Facade {
	if f == nil {
		return nil
	}
	f.batchService = service
	return f
}

func (f *Facade) withRetrieveService(service retrieveRequestService) *Facade {
	if f == nil {
		return nil
	}
	f.retrieveService = service
	return f
}

func (f *Facade) withUploadService(service uploadResultService) *Facade {
	if f == nil {
		return nil
	}
	f.uploadService = service
	return f
}

func (f *Facade) withImportService(service importCertificateService) *Facade {
	if f == nil {
		return nil
	}
	f.importService = service
	return f
}

func (f *Facade) withProfileService(service resolvePlatformProfileService) *Facade {
	if f == nil {
		return nil
	}
	f.profileService = service
	return f
}

func (f *Facade) withAndroidIntentAdapter(adapter androidIntentHandler) *Facade {
	if f == nil {
		return nil
	}
	f.androidIntentAdapter = adapter
	return f
}

type signRequest struct {
	Name          string            `json:"name"`
	ContentBase64 string            `json:"content_base64"`
	MIMEType      string            `json:"mime_type"`
	Format        string            `json:"format"`
	Action        string            `json:"action"`
	CertificateID string            `json:"certificate_id"`
	Options       map[string]string `json:"options"`
}

type signResponse struct {
	Format              string `json:"format"`
	Algorithm           string `json:"algorithm"`
	SignedContentBase64 string `json:"signed_content_base64"`
	CertificateID       string `json:"certificate_id"`
}

type sealPreviewRequest struct {
	CertificateID string            `json:"certificate_id"`
	Options       map[string]string `json:"options"`
}

// SealPreviewJSON usa el mismo compositor que la firma PAdES. La identidad
// procede exclusivamente de la sesión, nunca de un nombre suministrado por UI.
func (f *Facade) SealPreviewJSON(payload string) (string, error) {
	if f == nil || f.session == nil {
		return "", errNoConfigurado("vista previa del sello")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	var req sealPreviewRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "vista previa del sello", &req); err != nil {
		return "", err
	}
	if err := validateBoundedText("certificate_id", req.CertificateID, 128, false); err != nil {
		return "", err
	}
	if err := validateOptions(req.Options); err != nil {
		return "", err
	}
	if req.Options["visibleSeal"] != "true" {
		return "", newFacadeError("la vista previa exige un sello visible")
	}
	ref, ok := f.session.reference(req.CertificateID)
	if !ok {
		return "", newFacadeError("certificado de sesión no disponible")
	}
	options := f.withRegionOptions(req.Options)
	image, err := desktopsigner.PrevisualizarSello(options, ref.Subject, ref.Issuer, f.now())
	if err != nil {
		return "", safeOperationError("vista previa del sello")
	}
	return marshal(struct {
		ImageBase64 string `json:"image_base64"`
	}{base64.StdEncoding.EncodeToString(image)})
}

type verifyRequest struct {
	Name           string `json:"name"`
	ContentBase64  string `json:"content_base64"`
	MIMEType       string `json:"mime_type"`
	OriginalBase64 string `json:"original_content_base64"`
	// IncludeHTMLReport pide además el informe imprimible de escritorio.
	IncludeHTMLReport bool `json:"include_html_report,omitempty"`
}

type verifyResponse struct {
	Valid             bool                `json:"valid"`
	Reason            string              `json:"reason"`
	Details           []string            `json:"details"`
	Signers           []string            `json:"signers"`
	Format            string              `json:"format,omitempty"`
	Coverage          string              `json:"coverage,omitempty"`
	IntegrityStatus   string              `json:"integrity_status,omitempty"`
	CertificateStatus string              `json:"certificate_status,omitempty"`
	TrustStatus       string              `json:"trust_status,omitempty"`
	RevocationMode    string              `json:"revocation_mode,omitempty"`
	SignerSummaries   []verifySignerBrief `json:"signer_summaries,omitempty"`
	Warnings          []string            `json:"warnings,omitempty"`
	Errors            []string            `json:"errors,omitempty"`
	ReportHTMLBase64  string              `json:"report_html_base64,omitempty"`
}

type verifySignerBrief struct {
	ID          string `json:"id,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type selectCertificateRequest struct {
	SubjectFilter   string `json:"subject_filter"`
	IssuerFilter    string `json:"issuer_filter"`
	SoloNoCaducados bool   `json:"solo_no_caducados"`
}

type selectCertificateResponse struct {
	CertificateID string `json:"certificate_id"`
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	Fingerprint   string `json:"fingerprint"`
	Confirmed     bool   `json:"confirmed"`
}

type batchItemRequest struct {
	Name          string            `json:"name"`
	ContentBase64 string            `json:"content_base64"`
	MIMEType      string            `json:"mime_type"`
	Format        string            `json:"format"`
	Action        string            `json:"action"`
	Options       map[string]string `json:"options"`
}

type exchangeSessionRequest struct {
	RequestID        string `json:"request_id"`
	SessionKey       string `json:"session_key"`
	UploadEndpoint   string `json:"upload_endpoint"`
	RetrieveEndpoint string `json:"retrieve_endpoint"`
	State            string `json:"state"`
}

type processBatchRequest struct {
	Items         []batchItemRequest `json:"items"`
	CertificateID string             `json:"certificate_id,omitempty"`
	// Options es la plantilla global; las opciones de cada item prevalecen por clave.
	Options map[string]string       `json:"options,omitempty"`
	Session *exchangeSessionRequest `json:"session,omitempty"`
}

type batchItemResponse struct {
	Format              string `json:"format"`
	Algorithm           string `json:"algorithm"`
	SignedContentBase64 string `json:"signed_content_base64"`
	CertificateID       string `json:"certificate_id"`
}

type batchErrorResponse struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

type processBatchResponse struct {
	OK      bool                 `json:"ok"`
	Results []batchItemResponse  `json:"results"`
	Errors  []batchErrorResponse `json:"errors,omitempty"`
}

type retrieveRequest struct {
	Session exchangeSessionRequest `json:"session"`
}

type retrieveResponse struct {
	RequestID        string `json:"request_id"`
	SessionKey       string `json:"session_key,omitempty"`
	UploadEndpoint   string `json:"upload_endpoint"`
	RetrieveEndpoint string `json:"retrieve_endpoint"`
	State            string `json:"state"`
	DataBase64       string `json:"data_base64"`
}

type uploadRequest struct {
	Session    exchangeSessionRequest `json:"session"`
	DataBase64 string                 `json:"data_base64"`
}

type uploadResponse struct {
	OK bool `json:"ok"`
}

type importCertificateRequest struct {
	DataBase64 string `json:"data_base64"`
	Password   string `json:"password"`
}

type importCertificateResponse struct {
	CertificateID string `json:"certificate_id"`
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	Fingerprint   string `json:"fingerprint"`
}

type externalIdentityRequest struct {
	CertificateBase64 string   `json:"certificate_base64"`
	ChainBase64       []string `json:"chain_base64"`
}

// InstallExternalIdentityJSON enlaza un certificado con un firmador del DNIe.
// El callback recibe solo resúmenes; la clave privada permanece en la tarjeta.
func (f *Facade) InstallExternalIdentityJSON(payload string, signer ExternalDigestSigner) (string, error) {
	if f == nil || f.session == nil || signer == nil {
		return "", errNoConfigurado("identidad externa")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	var req externalIdentityRequest
	if err := decodeJSONStrict(payload, maxCertificateBytes*2, "identidad externa", &req); err != nil {
		return "", err
	}
	if len(req.ChainBase64) > maxCertificateChainLength {
		return "", newFacadeError(mobileSigningIdentityUnsupportedMessage)
	}
	leaf, err := decodeBase64Limited(req.CertificateBase64, "certificate_base64", maxCertificateBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(leaf)
	chain := make([][]byte, 0, len(req.ChainBase64))
	for _, encoded := range req.ChainBase64 {
		der, err := decodeBase64Limited(encoded, "chain_base64", maxCertificateBytes)
		if err != nil {
			return "", err
		}
		chain = append(chain, der)
		defer zeroBytes(der)
	}
	ref, err := f.session.installExternalIdentity(leaf, chain, signer)
	if err != nil {
		return "", newFacadeError(mobileCertificateImportErrorMessage(err))
	}
	return marshal(importCertificateResponse{
		CertificateID: sanitizeOutputText(ref.ID, 128),
		Subject:       sanitizeOutputText(ref.Subject, 500),
		Issuer:        sanitizeOutputText(ref.Issuer, 500),
		Fingerprint:   sanitizeOutputText(ref.Fingerprint, 128),
	})
}

type platformProfileResponse struct {
	HasSecureStorage    bool `json:"has_secure_storage"`
	HasBiometricPrompt  bool `json:"has_biometric_prompt"`
	HasSmartCardAccess  bool `json:"has_smart_card_access"`
	HasDocumentPicker   bool `json:"has_document_picker"`
	HasLocalServer      bool `json:"has_local_server"`
	HasNativeMessaging  bool `json:"has_native_messaging"`
	HasLegacyAfirmaURI  bool `json:"has_legacy_afirma_uri"`
	HasMobileDeepLink   bool `json:"has_mobile_deep_link"`
	HasLocalTLSTrust    bool `json:"has_local_tls_trust"`
	HasPDFPreview       bool `json:"has_pdf_preview"`
	HasTemporaryStorage bool `json:"has_temporary_storage"`
}

type androidIntentRequest struct {
	Action        string                     `json:"action"`
	Descriptor    string                     `json:"descriptor,omitempty"`
	PayloadBase64 string                     `json:"payload_base64,omitempty"`
	Text          string                     `json:"text,omitempty"`
	Items         []androidIntentItemRequest `json:"items,omitempty"`
}

type androidIntentItemRequest struct {
	Descriptor    string `json:"descriptor,omitempty"`
	PayloadBase64 string `json:"payload_base64,omitempty"`
	Text          string `json:"text,omitempty"`
}

type androidIntentResponse struct {
	Type            string `json:"type"`
	Origin          string `json:"origin"`
	SignatureAction string `json:"signature_action,omitempty"`
	Format          string `json:"format,omitempty"`
	DocumentName    string `json:"document_name,omitempty"`
	DocumentMIME    string `json:"document_mime,omitempty"`
	JobCount        int    `json:"job_count,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	ViewTitle       string `json:"view_title"`
	ViewDetail      string `json:"view_detail"`
	ViewState       string `json:"view_state"`
	ViewActionID    string `json:"view_action_id"`
}

// SignJSON firma un documento a partir de un payload JSON bind-friendly.
func (f *Facade) SignJSON(payload string) (string, error) {
	if f == nil || f.signService == nil {
		return "", errNoConfigurado("firma")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()

	var req signRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "firma", &req); err != nil {
		return "", err
	}
	if err := validateDocumentMetadata(req.Name, req.MIMEType); err != nil {
		return "", err
	}
	if err := validateBoundedText("format", req.Format, 32, true); err != nil {
		return "", err
	}
	if err := validateBoundedText("action", req.Action, 32, true); err != nil {
		return "", err
	}
	if err := validateSignatureAction(req.Action); err != nil {
		return "", err
	}
	if err := validateBoundedText("certificate_id", req.CertificateID, 128, false); err != nil {
		return "", err
	}
	if err := validateOptions(req.Options); err != nil {
		return "", err
	}
	content, err := decodeBase64Limited(req.ContentBase64, "content_base64", maxDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(content)

	format, err := resolveSignatureFormat(req.Format, req.Name, req.MIMEType, content)
	if err != nil {
		return "", err
	}
	if err := f.checkFormatPrerequisites(format, req.CertificateID, content); err != nil {
		return "", err
	}
	if req.Options == nil {
		req.Options = make(map[string]string)
	}
	if strings.TrimSpace(req.Options["profile"]) == "" {
		req.Options["profile"] = "baseline"
	}
	req.Options = f.withRegionOptions(req.Options)
	if err := validateSigningOptions(format, req.Action, req.Options); err != nil {
		return "", err
	}
	cmd, err := application.NewSignCommand(
		req.Name,
		content,
		req.MIMEType,
		format,
		req.Action,
		req.CertificateID,
		req.Options,
	)
	if err != nil {
		return "", newFacadeError("solicitud de firma no valida")
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	result, err := f.signService.Execute(ctx, cmd)
	if err != nil {
		return "", signingError(err)
	}
	if len(result.Result.Data) == 0 || len(result.Result.Data) > maxSignedDocumentBytes {
		zeroBytes(result.Result.Data)
		return "", newFacadeError("el resultado de firma tiene un tamano no permitido")
	}
	defer zeroBytes(result.Result.Data)
	return marshal(signResponse{
		Format:              sanitizeOutputText(string(result.Result.Format), 64),
		Algorithm:           sanitizeOutputText(result.Result.Algorithm, 128),
		SignedContentBase64: base64.StdEncoding.EncodeToString(result.Result.Data),
		CertificateID:       sanitizeOutputText(result.CertificateUsed.ID, 128),
	})
}

// InspectSignatureJSON detecta firmas existentes sin confundirlas con firmas
// verificadas. No necesita identidad, original ni acceso a la red.
func (f *Facade) InspectSignatureJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("inspeccion")
	}
	var req verifyRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "inspeccion", &req); err != nil {
		return "", err
	}
	if err := validateDocumentMetadata(req.Name, req.MIMEType); err != nil {
		return "", err
	}
	content, err := decodeBase64Limited(req.ContentBase64, "content_base64", maxDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(content)
	doc, err := domain.NewDocument(req.Name, content, req.MIMEType)
	if err != nil {
		return "", newFacadeError("documento no valido")
	}
	present, format := commonsigner.InspectSignature(doc)
	return marshal(struct {
		HasSignature bool   `json:"has_signature"`
		Format       string `json:"format"`
	}{present, format})
}

// VerifyJSON verifica una firma a partir de un payload JSON bind-friendly.
func (f *Facade) VerifyJSON(payload string) (string, error) {
	if f == nil || f.verifyService == nil {
		return "", errNoConfigurado("verificacion")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()

	var req verifyRequest
	if err := decodeJSONStrict(payload, maxVerifyJSONBytes, "verificacion", &req); err != nil {
		return "", err
	}
	if err := validateDocumentMetadata(req.Name, req.MIMEType); err != nil {
		return "", err
	}
	content, err := decodeBase64Limited(
		req.ContentBase64,
		"content_base64",
		maxSignedDocumentBytes,
	)
	if err != nil {
		return "", err
	}
	defer zeroBytes(content)
	doc, err := domain.NewDocument(req.Name, content, req.MIMEType)
	if err != nil {
		return "", newFacadeError("documento firmado no valido")
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	verifyCmd := application.VerifyCommand{SignedDocument: doc}
	if req.OriginalBase64 != "" {
		originalContent, err := decodeBase64Limited(
			req.OriginalBase64,
			"original_content_base64",
			maxDocumentBytes,
		)
		if err != nil {
			return "", err
		}
		defer zeroBytes(originalContent)
		originalDoc, err := domain.NewDocument(req.Name, originalContent, inferOriginalMIMEType(req.MIMEType))
		if err != nil {
			return "", newFacadeError("documento original no valido")
		}
		verifyCmd.OriginalDocument = &originalDoc
	}
	result, err := f.verifyService.Execute(ctx, verifyCmd)
	if err != nil {
		return "", safeOperationError("verificacion")
	}
	signerCount := min(len(result.Firmantes), 64)
	signers := make([]string, 0, signerCount)
	for _, signer := range result.Firmantes[:signerCount] {
		signers = append(signers, sanitizeOutputText(signer.ID, 128))
	}
	summaryCount := min(len(result.Verification.SignerSummaries), 64)
	summaries := make([]verifySignerBrief, 0, summaryCount)
	for _, signer := range result.Verification.SignerSummaries[:summaryCount] {
		summaries = append(summaries, verifySignerBrief{
			ID:          sanitizeOutputText(signer.ID, 128),
			Subject:     sanitizeOutputText(signer.Subject, 500),
			Issuer:      sanitizeOutputText(signer.Issuer, 500),
			Fingerprint: sanitizeOutputText(signer.Fingerprint, 128),
		})
	}
	maximumSourceWarnings := 64
	if !f.systemTrustAnchors {
		maximumSourceWarnings--
	}
	warnings := sanitizeOutputStrings(result.Verification.Warnings, maximumSourceWarnings, 500)
	trustStatus := result.Verification.Trust.Status
	if !f.systemTrustAnchors {
		trustStatus = domain.VerificationStatusUnknown
		warnings = append(warnings, textoMotor("movil.verificacion.cadena_no_evaluada"))
	}
	reportHTML := ""
	if req.IncludeHTMLReport {
		html, err := f.verificationReportHTML(req.Name, content, result.Verification)
		if err != nil {
			return "", safeOperationError("informe de verificacion")
		}
		reportHTML = base64.StdEncoding.EncodeToString(html)
	}
	return marshal(verifyResponse{
		Valid:             result.Verification.Valid,
		Reason:            sanitizeOutputText(result.Verification.Reason, 500),
		Details:           sanitizeOutputStrings(result.Verification.Details, 64, 500),
		Signers:           signers,
		Format:            sanitizeOutputText(result.Verification.Format, 64),
		Coverage:          sanitizeOutputText(result.Verification.Coverage, 64),
		IntegrityStatus:   string(result.Verification.Integrity.Status),
		CertificateStatus: string(result.Verification.Certificate.Status),
		TrustStatus:       string(trustStatus),
		RevocationMode:    sanitizeOutputText(f.revocationMode, 64),
		SignerSummaries:   summaries,
		Warnings:          warnings,
		Errors:            sanitizeOutputStrings(result.Verification.Errors, 64, 500),
		ReportHTMLBase64:  reportHTML,
	})
}

// SelectCertificateJSON resuelve la seleccion de certificado con un payload bind-friendly.
func (f *Facade) SelectCertificateJSON(payload string) (string, error) {
	if f == nil || f.selectService == nil {
		return "", errNoConfigurado("seleccion de certificado")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()

	var req selectCertificateRequest
	if err := decodeJSONStrict(payload, maxSelectJSONBytes, "seleccion", &req); err != nil {
		return "", err
	}
	if err := validateBoundedText("subject_filter", req.SubjectFilter, 256, true); err != nil {
		return "", err
	}
	if err := validateBoundedText("issuer_filter", req.IssuerFilter, 256, true); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	result, err := f.selectService.Execute(ctx, application.SelectCertificateCommand{
		SubjectFilter:   req.SubjectFilter,
		IssuerFilter:    req.IssuerFilter,
		SoloNoCaducados: req.SoloNoCaducados,
	})
	if err != nil {
		return "", safeOperationError("seleccion de certificado")
	}
	return marshal(selectCertificateResponse{
		CertificateID: sanitizeOutputText(result.Selection.Certificate.ID, 128),
		Subject:       sanitizeOutputText(result.Selection.Certificate.Subject, 500),
		Issuer:        sanitizeOutputText(result.Selection.Certificate.Issuer, 500),
		Fingerprint:   sanitizeOutputText(result.Selection.Certificate.Fingerprint, 128),
		Confirmed:     result.Selection.Confirmed,
	})
}

func (f *Facade) processBatchJSON(payload string) (string, error) {
	if f == nil || f.batchService == nil {
		return "", errNoConfigurado("lote")
	}
	var req processBatchRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", errJSONInvalido("lote")
	}
	if len(req.Items) == 0 {
		return "", newFacadeError("el lote no puede estar vacio")
	}
	items := make([]application.BatchItemInput, 0, len(req.Items))
	for _, item := range req.Items {
		content, err := base64.StdEncoding.DecodeString(item.ContentBase64)
		if err != nil {
			return "", errCampoBase64("content_base64")
		}
		items = append(items, application.BatchItemInput{
			Nombre:    item.Name,
			Contenido: content,
			TipoMIME:  item.MIMEType,
			Formato:   item.Format,
			Accion:    item.Action,
			Opciones:  item.Options,
		})
	}
	cmd, err := application.NewProcessBatchCommandWithDefaults(items, req.Options)
	if err != nil {
		return "", err
	}
	cmd.CertificateID = strings.TrimSpace(req.CertificateID)
	if req.Session != nil {
		cmd.Session = exchangeSessionFromRequest(*req.Session)
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	result, err := f.batchService.Execute(ctx, cmd)
	if err != nil {
		return "", err
	}
	resp := processBatchResponse{
		OK:      len(result.Errores) == 0,
		Results: make([]batchItemResponse, 0, len(result.Results)),
	}
	for _, item := range result.Results {
		resp.Results = append(resp.Results, batchItemResponse{
			Format:              string(item.Result.Format),
			Algorithm:           item.Result.Algorithm,
			SignedContentBase64: base64.StdEncoding.EncodeToString(item.Result.Data),
			CertificateID:       item.CertificateUsed.ID,
		})
	}
	if len(result.Errores) > 0 {
		resp.Errors = make([]batchErrorResponse, 0, len(result.Errores))
		for index, err := range result.Errores {
			resp.Errors = append(resp.Errors, batchErrorResponse{Index: index, Message: err.Error()})
		}
	}
	return marshal(resp)
}

func (f *Facade) retrieveRequestJSON(payload string) (string, error) {
	if f == nil || f.retrieveService == nil {
		return "", errNoConfigurado("recuperacion remota")
	}
	var req retrieveRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", errJSONInvalido("recuperacion")
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	result, err := f.retrieveService.Execute(ctx, application.RetrieveRequestCommand{
		Session: exchangeSessionFromRequest(req.Session),
	})
	if err != nil {
		return "", err
	}
	return marshal(retrieveResponse{
		RequestID:        result.Session.RequestID,
		SessionKey:       result.Session.SessionKey,
		UploadEndpoint:   result.Session.UploadEndpoint,
		RetrieveEndpoint: result.Session.RetrieveEndpoint,
		State:            string(result.Session.State),
		DataBase64:       base64.StdEncoding.EncodeToString(result.Data),
	})
}

func (f *Facade) uploadResultJSON(payload string) (string, error) {
	if f == nil || f.uploadService == nil {
		return "", errNoConfigurado("subida remota")
	}
	var req uploadRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		return "", errJSONInvalido("subida")
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		return "", errCampoBase64("data_base64")
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	if err := f.uploadService.Execute(ctx, application.UploadResultCommand{
		Session: exchangeSessionFromRequest(req.Session),
		Data:    data,
	}); err != nil {
		return "", err
	}
	return marshal(uploadResponse{OK: true})
}

// ImportCertificateJSON importa un certificado externo a partir de un payload JSON bind-friendly.
func (f *Facade) ImportCertificateJSON(payload string) (string, error) {
	if f == nil || f.importService == nil {
		return "", errNoConfigurado("importacion de certificado")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()

	var req importCertificateRequest
	if err := decodeJSONStrict(payload, maxImportJSONBytes, "importacion", &req); err != nil {
		return "", err
	}
	if len(req.Password) > maxPasswordBytes {
		return "", newFacadeError("password supera el limite permitido")
	}
	data, err := decodeBase64Limited(req.DataBase64, "data_base64", maxCertificateBytes)
	if err != nil {
		return "", err
	}
	defer func() {
		zeroBytes(data)
		req.DataBase64 = ""
		req.Password = ""
	}()
	return f.importCertificateLocked(data, req.Password)
}

// ImportCertificateBytesJSON evita crear una copia Base64 del PKCS#12 en el
// borde Android. gomobile entrega un slice mutable que se limpia antes de
// devolver; la contraseña sigue limitada por el contrato String de gobind.
func (f *Facade) ImportCertificateBytesJSON(data []byte, password string) (string, error) {
	if f == nil || f.importService == nil {
		return "", errNoConfigurado("importacion de certificado")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	if len(data) == 0 || len(data) > maxCertificateBytes {
		return "", newFacadeError("tamano de certificado no permitido")
	}
	if len(password) > maxPasswordBytes {
		return "", newFacadeError("password supera el limite permitido")
	}
	defer zeroBytes(data)
	return f.importCertificateLocked(data, password)
}

// ImportCertificateSecretBytesJSON mantiene ambos buffers mutables en el borde
// gomobile. La conversión String queda limitada a la biblioteca PKCS#12 Go.
func (f *Facade) ImportCertificateSecretBytesJSON(data, password []byte) (string, error) {
	defer zeroBytes(data)
	defer zeroBytes(password)
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return "", newFacadeError("tamano de password no permitido")
	}
	return f.ImportCertificateBytesJSON(data, string(password))
}

func (f *Facade) importCertificateLocked(data []byte, password string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	result, err := f.importService.Execute(ctx, application.ImportCertificateCommand{
		Data:     data,
		Password: password,
	})
	if err != nil {
		return "", newFacadeError(mobileCertificateImportErrorMessage(err))
	}
	return marshal(importCertificateResponse{
		CertificateID: sanitizeOutputText(result.Certificate.ID, 128),
		Subject:       sanitizeOutputText(result.Certificate.Subject, 500),
		Issuer:        sanitizeOutputText(result.Certificate.Issuer, 500),
		Fingerprint:   sanitizeOutputText(result.Certificate.Fingerprint, 128),
	})
}

func mobileCertificateImportErrorMessage(err error) string {
	switch {
	case errors.Is(err, errMobilePKCS12Decode):
		return mobilePKCS12DecodeMessage
	case errors.Is(err, errMobileCertificateNotCurrent):
		return mobileCertificateNotCurrentMessage
	case errors.Is(err, errMobileSigningIdentityUnsupported):
		return mobileSigningIdentityUnsupportedMessage
	case errors.Is(err, errMobileSessionFull):
		return mobileSessionFullKey
	default:
		return mobileCertificateImportFallbackMessage
	}
}

// ResolvePlatformProfileJSON devuelve las capacidades observables de la plataforma mobile.
func (f *Facade) ResolvePlatformProfileJSON() (string, error) {
	if f == nil || f.profileService == nil {
		return "", errNoConfigurado("perfil de plataforma")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	result, err := f.profileService.Execute(ctx, application.ResolvePlatformProfileCommand{})
	if err != nil {
		return "", safeOperationError("perfil de plataforma")
	}
	return marshal(platformProfileResponse{
		HasSecureStorage:    result.Profile.HasSecureStorage,
		HasBiometricPrompt:  result.Profile.HasBiometricPrompt,
		HasSmartCardAccess:  result.Profile.HasSmartCardAccess,
		HasDocumentPicker:   result.Profile.HasDocumentPicker,
		HasLocalServer:      result.Profile.HasLocalServer,
		HasNativeMessaging:  result.Profile.HasNativeMessaging,
		HasLegacyAfirmaURI:  result.Profile.HasLegacyAfirmaURI,
		HasMobileDeepLink:   result.Profile.HasMobileDeepLink,
		HasLocalTLSTrust:    result.Profile.HasLocalTLSTrust,
		HasPDFPreview:       result.Profile.HasPDFPreview,
		HasTemporaryStorage: result.Profile.HasTemporaryStorage,
	})
}

// TranslateAndroidIntentJSON traduce un intent Android a una solicitud mobile
// bind-friendly, sin exponer tipos internos del núcleo.
func (f *Facade) TranslateAndroidIntentJSON(payload string) (string, error) {
	if f == nil || f.androidIntentAdapter == nil {
		return "", errNoConfigurado("android intent")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()

	var req androidIntentRequest
	if err := decodeJSONStrict(payload, maxIntentJSONBytes, "android intent", &req); err != nil {
		return "", err
	}
	if err := validateBoundedText("action", req.Action, 128, false); err != nil {
		return "", err
	}
	if req.Descriptor != "" {
		if err := validateBoundedText("descriptor", req.Descriptor, 160, false); err != nil {
			return "", err
		}
	}
	if isAndroidIntentSendMultiple(req.Action) {
		return f.translateAndroidIntentBatchJSON(req)
	}
	rawPayload, err := payloadBytesFromAndroidRequest(req)
	if err != nil {
		return "", err
	}
	defer zeroBytes(rawPayload)
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	solicitud, err := f.androidIntentAdapter.HandleShared(ctx, req.Action, req.Descriptor, rawPayload)
	if err != nil {
		return "", safeOperationError("traduccion de intent")
	}
	return marshal(androidIntentResponseFromSolicitud(solicitud))
}

func (f *Facade) translateAndroidIntentBatchJSON(req androidIntentRequest) (string, error) {
	if len(req.Items) == 0 {
		return "", newFacadeError("items es obligatorio para send_multiple")
	}
	if len(req.Items) > 16 {
		return "", newFacadeError("items supera el limite permitido")
	}
	entradas := make([]mobileinbound.DocumentoCompartidoEntrada, 0, len(req.Items))
	defer func() {
		for index := range entradas {
			zeroBytes(entradas[index].Payload)
		}
	}()
	totalBytes := 0
	for _, item := range req.Items {
		if err := validateBoundedText("descriptor", item.Descriptor, 160, false); err != nil {
			return "", err
		}
		payload, err := payloadBytesFromAndroidItemRequest(item)
		if err != nil {
			return "", err
		}
		totalBytes += len(payload)
		if totalBytes > maxDocumentBytes {
			zeroBytes(payload)
			for index := range entradas {
				zeroBytes(entradas[index].Payload)
			}
			return "", newFacadeError("items supera el limite total permitido")
		}
		entradas = append(entradas, mobileinbound.DocumentoCompartidoEntrada{
			Descriptor: item.Descriptor,
			Payload:    payload,
		})
	}
	solicitud, err := mobileinbound.BuildSolicitudDocumentosCompartidos("android-intent", entradas)
	if err != nil {
		return "", err
	}
	return marshal(androidIntentResponseFromSolicitud(solicitud))
}

func marshal(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func errNoConfigurado(operacion string) error {
	return newFacadeError("servicio de " + operacion + " no configurado")
}

func errJSONInvalido(operacion string) error {
	return newFacadeError("json de " + operacion + " invalido")
}

func errCampoBase64(field string) error {
	return newFacadeError(field + " no es base64 valido")
}

func payloadBytesFromAndroidRequest(req androidIntentRequest) ([]byte, error) {
	if req.Text != "" {
		if len(req.Text) > maxDocumentBytes {
			return nil, newFacadeError("text supera el limite permitido")
		}
		return []byte(req.Text), nil
	}
	if req.PayloadBase64 == "" {
		return nil, newFacadeError("payload_base64 o text es obligatorio")
	}
	return decodeBase64Limited(req.PayloadBase64, "payload_base64", maxDocumentBytes)
}

func payloadBytesFromAndroidItemRequest(req androidIntentItemRequest) ([]byte, error) {
	if req.Text != "" {
		if len(req.Text) > maxDocumentBytes {
			return nil, newFacadeError("text supera el limite permitido")
		}
		return []byte(req.Text), nil
	}
	if req.PayloadBase64 == "" {
		return nil, newFacadeError("payload_base64 o text es obligatorio")
	}
	return decodeBase64Limited(req.PayloadBase64, "payload_base64", maxDocumentBytes)
}

func isAndroidIntentSendMultiple(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "android.intent.action.send_multiple", "send_multiple":
		return true
	default:
		return false
	}
}

func androidIntentResponseFromSolicitud(s mobileinbound.Solicitud) androidIntentResponse {
	resp := androidIntentResponse{
		Type:         string(s.Tipo),
		Origin:       s.Origen,
		Format:       string(s.Formato),
		ViewTitle:    s.ViewModel.Titulo,
		ViewDetail:   s.ViewModel.Detalle,
		ViewState:    string(s.ViewModel.Estado),
		ViewActionID: s.ViewModel.AccionID,
	}
	if s.AccionFirma != "" {
		resp.SignatureAction = string(s.AccionFirma)
	}
	if s.SignCommand != nil {
		resp.DocumentName = s.SignCommand.Document.Name
		resp.DocumentMIME = s.SignCommand.Document.MIMEType
	}
	if s.BatchCommand != nil {
		resp.JobCount = len(s.BatchCommand.Jobs)
	}
	if s.RetrieveCommand != nil {
		resp.RequestID = s.RetrieveCommand.Session.RequestID
	}
	return resp
}

func inferOriginalMIMEType(signedMIME string) string {
	switch signedMIME {
	case "application/pkcs7-signature", "application/cms", "application/pkcs7-mime":
		return "application/octet-stream"
	default:
		return signedMIME
	}
}

func exchangeSessionFromRequest(req exchangeSessionRequest) domain.ExchangeSession {
	state := domain.ExchangeSessionState(req.State)
	if state == "" {
		state = domain.SessionActive
	}
	return domain.ExchangeSession{
		RequestID:        req.RequestID,
		SessionKey:       req.SessionKey,
		UploadEndpoint:   req.UploadEndpoint,
		RetrieveEndpoint: req.RetrieveEndpoint,
		State:            state,
	}
}

type facadeError struct {
	Message string
}

func (e *facadeError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func newFacadeError(message string) error {
	return &facadeError{Message: message}
}
