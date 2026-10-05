// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/digitorus/timestamp"

	"grxfirma/internal/adapters/outbound/common/certutil"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/adapters/outbound/common/updatecheck"
)

// Servicios de la tercera oleada: detalle y validación en línea del
// certificado, diagnóstico, prueba de la TSA, QR tributario y versiones.
// Ninguno se ejecuta sin una acción explícita de la persona y ninguna
// respuesta incluye textos para mostrar: solo estados y datos.

const (
	certificateExpiringSoonDays = 30
	maxOnlineJSONBytes          = 8 << 10
	onlineRevocationTimeout     = 30 * time.Second
	timestampProbeTimeout       = 20 * time.Second
	updateCheckTimeout          = 10 * time.Second
	maxVersionBytes             = 32
)

func (f *Facade) now() time.Time {
	if f != nil && f.clock != nil {
		return f.clock()
	}
	return time.Now()
}

type certificateDetail struct {
	CertificateID string `json:"certificate_id"`
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	Fingerprint   string `json:"fingerprint"`
	NIF           string `json:"nif,omitempty"`
	Organization  string `json:"organization,omitempty"`
	Kind          string `json:"kind"`
	KeyType       string `json:"key_type"`
	KeyBits       int    `json:"key_bits"`
	NotBefore     string `json:"not_before"`
	NotAfter      string `json:"not_after"`
	DaysLeft      int    `json:"days_left"`
	Status        string `json:"status"`
	External      bool   `json:"external"`
	CanEncrypt    bool   `json:"can_encrypt"`
	HasOCSP       bool   `json:"has_ocsp"`
	HasCRL        bool   `json:"has_crl"`
}

type certificateDetailsResponse struct {
	ExpiringSoonDays int                 `json:"expiring_soon_days"`
	Certificates     []certificateDetail `json:"certificates"`
}

// sessionSnapshot copia lo público de la identidad: certificado, cadena y si
// la clave es externa. Nunca devuelve la clave privada.
func (s *sessionIdentityStore) sessionSnapshot() (*x509.Certificate, []*x509.Certificate, bool, string, bool) {
	if s == nil {
		return nil, nil, false, "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.identity == nil || s.identity.certificate == nil {
		return nil, nil, false, "", false
	}
	_, external := s.identity.signer.(*externalRSASigner)
	chain := append([]*x509.Certificate(nil), s.identity.chain...)
	return s.identity.certificate, chain, external, s.identity.reference.ID, true
}

// CertificateDetailsJSON describe los certificados de la sesión con su
// caducidad, tipo, NIF y organización. Hoy hay como máximo uno; la lista
// permite filtrar en la interfaz si el almacén admite más en el futuro.
func (f *Facade) CertificateDetailsJSON() (string, error) {
	if f == nil || f.session == nil {
		return "", errNoConfigurado("detalle del certificado")
	}
	response := certificateDetailsResponse{ExpiringSoonDays: certificateExpiringSoonDays, Certificates: []certificateDetail{}}
	certificate, _, external, id, ok := f.session.sessionSnapshot()
	if ok {
		response.Certificates = append(response.Certificates, describeCertificate(certificate, id, external, f.now()))
	}
	return marshal(response)
}

func describeCertificate(certificate *x509.Certificate, id string, external bool, now time.Time) certificateDetail {
	reference := certificateReference(certificate)
	kind, organization, nif := certutil.ClasificarCertificado(certificate)
	keyType, keyBits := "", 0
	switch key := certificate.PublicKey.(type) {
	case *rsa.PublicKey:
		keyType, keyBits = "RSA", key.N.BitLen()
	case *ecdsa.PublicKey:
		keyType, keyBits = "ECDSA", key.Curve.Params().BitSize
	}
	daysLeft := int(certificate.NotAfter.Sub(now).Hours() / 24)
	status := "valid"
	switch {
	case now.Before(certificate.NotBefore):
		status = "not_yet_valid"
	case now.After(certificate.NotAfter):
		status = "expired"
	case daysLeft <= certificateExpiringSoonDays:
		status = "expiring_soon"
	}
	return certificateDetail{
		CertificateID: id,
		Subject:       sanitizeOutputText(reference.Subject, 256),
		Issuer:        sanitizeOutputText(reference.Issuer, 256),
		Fingerprint:   reference.Fingerprint,
		NIF:           sanitizeOutputText(nif, 64),
		Organization:  sanitizeOutputText(organization, 256),
		Kind:          string(kind),
		KeyType:       keyType,
		KeyBits:       keyBits,
		NotBefore:     certificate.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:      certificate.NotAfter.UTC().Format(time.RFC3339),
		DaysLeft:      daysLeft,
		Status:        status,
		External:      external,
		CanEncrypt:    !external && certutil.PuedeCifrar(certificate),
		HasOCSP:       len(certificate.OCSPServer) > 0,
		HasCRL:        len(certificate.CRLDistributionPoints) > 0,
	}
}

type certificateRequest struct {
	CertificateID string `json:"certificate_id"`
}

type revocationResponse struct {
	Status    string `json:"status"`
	Method    string `json:"method,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
	RevokedAt string `json:"revoked_at,omitempty"`
	HasOCSP   bool   `json:"has_ocsp"`
	HasCRL    bool   `json:"has_crl"`
}

// CheckCertificateRevocationJSON consulta OCSP o CRL del certificado de la
// sesión con el mismo comprobador que escritorio. Solo envía al prestador lo
// que exige OCSP/CRL (número de serie y emisor) y no guarda nada.
func (f *Facade) CheckCertificateRevocationJSON(payload string) (string, error) {
	if f == nil || f.session == nil {
		return "", errNoConfigurado("validación en línea")
	}
	var req certificateRequest
	if err := decodeJSONStrict(payload, maxOnlineJSONBytes, "validación en línea", &req); err != nil {
		return "", err
	}
	if err := validateBoundedText("certificate_id", req.CertificateID, 128, false); err != nil {
		return "", err
	}
	certificate, chain, _, id, ok := f.session.sessionSnapshot()
	if !ok || id != req.CertificateID {
		return "", newFacadeError("certificado de sesión no disponible")
	}
	chainDER := make([][]byte, 0, len(chain)+1)
	chainDER = append(chainDER, certificate.Raw)
	for _, issuer := range chain {
		chainDER = append(chainDER, issuer.Raw)
	}
	check := f.revocationCheck
	if check == nil {
		check = commonsigner.CheckCertificateOnlineRevocation
	}
	ctx, cancel := context.WithTimeout(context.Background(), onlineRevocationTimeout)
	defer cancel()
	result, err := check(ctx, chainDER)
	response := revocationResponse{
		Status:  "unavailable",
		HasOCSP: len(certificate.OCSPServer) > 0,
		HasCRL:  len(certificate.CRLDistributionPoints) > 0,
	}
	if err == nil {
		switch result.Status {
		case commonsigner.CertificateOnlineRevocationValid:
			response.Status = "valid"
		case commonsigner.CertificateOnlineRevocationRevoked:
			response.Status = "revoked"
		case commonsigner.CertificateOnlineRevocationInconclusive:
			response.Status = "inconclusive"
		}
		switch method := strings.ToUpper(strings.TrimSpace(result.Method)); method {
		case "OCSP", "CRL":
			response.Method = method
		}
		if !result.CheckedAt.IsZero() {
			response.CheckedAt = result.CheckedAt.UTC().Format(time.RFC3339)
		}
		if !result.RevokedAt.IsZero() {
			response.RevokedAt = result.RevokedAt.UTC().Format(time.RFC3339)
		}
	}
	return marshal(response)
}

type diagnosticsResponse struct {
	EngineVersion   string `json:"engine_version"`
	ContractVersion int    `json:"contract_version"`
	Platform        string `json:"platform"`
	GoVersion       string `json:"go_version"`
	Architecture    string `json:"architecture"`
	EngineTimeUTC   string `json:"engine_time_utc"`
	SessionIdentity bool   `json:"session_identity"`
}

// DiagnosticsJSON informa del motor sin datos personales: versión, contrato,
// arquitectura y hora del motor. No revela el titular del certificado.
func (f *Facade) DiagnosticsJSON() string {
	response := diagnosticsResponse{
		EngineVersion:   engineVersion,
		ContractVersion: mobileContractVersion,
		Platform:        "unknown",
		GoVersion:       goruntime.Version(),
		Architecture:    goruntime.GOOS + "/" + goruntime.GOARCH,
		EngineTimeUTC:   f.now().UTC().Format(time.RFC3339),
	}
	if f != nil && f.contractJSON != "" {
		var contract struct {
			Platform string `json:"platform"`
		}
		if json.Unmarshal([]byte(f.contractJSON), &contract) == nil && contract.Platform != "" {
			response.Platform = contract.Platform
		}
	}
	if f != nil && f.session != nil {
		_, _, _, _, ok := f.session.sessionSnapshot()
		response.SessionIdentity = ok
	}
	raw, err := marshal(response)
	if err != nil {
		return "{}"
	}
	return raw
}

type timestampProbeRequest struct {
	URL string `json:"url"`
}

type timestampProbeResponse struct {
	Status        string `json:"status"`
	HTTPS         bool   `json:"https"`
	TSATime       string `json:"tsa_time,omitempty"`
	LocalTime     string `json:"local_time"`
	SkewSeconds   int64  `json:"skew_seconds"`
	ElapsedMillis int64  `json:"elapsed_ms"`
}

// ProbeTimestampAuthorityJSON pide un sello de tiempo de prueba sobre un
// resumen aleatorio (sin documento) y compara su hora con la del dispositivo.
// El cliente RFC 3161 es el mismo que usa la firma y valida nonce y firma.
func (f *Facade) ProbeTimestampAuthorityJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("prueba de TSA")
	}
	var req timestampProbeRequest
	if err := decodeJSONStrict(payload, maxOnlineJSONBytes, "prueba de TSA", &req); err != nil {
		return "", err
	}
	response := timestampProbeResponse{Status: "invalid_url"}
	if err := validateTSAURL(req.URL); err != nil {
		response.LocalTime = f.now().UTC().Format(time.RFC3339)
		return marshal(response)
	}
	response.HTTPS = strings.HasPrefix(req.URL, "https://")
	digest := make([]byte, sha256.Size)
	if _, err := rand.Read(digest); err != nil {
		return "", safeOperationError("prueba de TSA")
	}
	probe := f.timestampProbe
	if probe == nil {
		probe = func(ctx context.Context, endpoint string, hash []byte) ([]byte, error) {
			return tsaclient.New(endpoint).RequestTimestamp(ctx, hash, crypto.SHA256)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timestampProbeTimeout)
	defer cancel()
	started := f.now()
	token, err := probe(ctx, req.URL, digest)
	finished := f.now()
	response.LocalTime = finished.UTC().Format(time.RFC3339)
	response.ElapsedMillis = finished.Sub(started).Milliseconds()
	if err != nil {
		response.Status = timestampProbeFailure(ctx, err)
		return marshal(response)
	}
	parsed, err := timestamp.Parse(token)
	if err != nil || parsed.Time.IsZero() {
		response.Status = "bad_response"
		return marshal(response)
	}
	// La hora local de referencia es el punto medio de la petición.
	middle := started.Add(finished.Sub(started) / 2)
	response.Status = "ok"
	response.TSATime = parsed.Time.UTC().Format(time.RFC3339)
	response.SkewSeconds = int64(middle.Sub(parsed.Time).Round(time.Second) / time.Second)
	return marshal(response)
}

func timestampProbeFailure(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "petición HTTP"), strings.Contains(message, "creando petición"):
		return "unreachable"
	case strings.Contains(message, "respondio con HTTP"), strings.Contains(message, "rechazo"):
		return "rejected"
	default:
		return "bad_response"
	}
}

type veriFactuQRRequest struct {
	URL string `json:"url"`
}

// ReadVeriFactuQRJSON valida la URL del QR tributario con el motor de
// escritorio. No hace ninguna petición de red.
func (f *Facade) ReadVeriFactuQRJSON(payload string) (string, error) {
	var req veriFactuQRRequest
	if err := decodeJSONStrict(payload, maxOnlineJSONBytes, "QR Veri*Factu", &req); err != nil {
		return "", newFacadeError(verifactuKeyPrefix + "qr_url")
	}
	qr, err := commonsigner.LeerQRVeriFactu(req.URL)
	if err != nil {
		return "", veriFactuQRError(err)
	}
	return marshal(qr)
}

// QueryVeriFactuQRJSON consulta el servicio público de cotejo de la AEAT.
// Solo se llama cuando la persona pulsa «Cotejar con la AEAT». El motor
// exige HTTPS, host y ruta oficiales, sin redirecciones ni proxy y con
// tiempo máximo; se envían únicamente los cuatro datos del QR.
func (f *Facade) QueryVeriFactuQRJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("cotejo AEAT")
	}
	var req veriFactuQRRequest
	if err := decodeJSONStrict(payload, maxOnlineJSONBytes, "QR Veri*Factu", &req); err != nil {
		return "", newFacadeError(verifactuKeyPrefix + "qr_url")
	}
	if _, err := commonsigner.LeerQRVeriFactu(req.URL); err != nil {
		return "", veriFactuQRError(err)
	}
	query := f.veriFactuQuery
	if query == nil {
		query = commonsigner.ConsultarQRVeriFactu
	}
	data, err := query(context.Background(), req.URL)
	if err != nil {
		return "", veriFactuQRError(err)
	}
	return marshal(struct {
		Response json.RawMessage `json:"response"`
	}{data})
}

func veriFactuQRError(err error) error {
	var keyed interface{ LocalizationKey() string }
	if errors.As(err, &keyed) {
		switch key := keyed.LocalizationKey(); key {
		case verifactuKeyPrefix + "qr_url", verifactuKeyPrefix + "qr_params", verifactuKeyPrefix + "qr_service":
			return newFacadeError(key)
		}
	}
	return newFacadeError(verifactuKeyPrefix + "qr_service")
}

type updateCheckRequest struct {
	CurrentVersion string `json:"current_version"`
}

type updateCheckResponse struct {
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
}

// CheckUpdateJSON consulta la última publicación en la API pública de
// GitHub del repositorio oficial. Nunca descarga ni instala nada.
func (f *Facade) CheckUpdateJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("comprobación de versión")
	}
	var req updateCheckRequest
	if err := decodeJSONStrict(payload, maxOnlineJSONBytes, "comprobación de versión", &req); err != nil {
		return "", err
	}
	if err := validateVersionText(req.CurrentVersion); err != nil {
		return "", err
	}
	check := f.updateCheck
	if check == nil {
		check = defaultUpdateCheck
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()
	result, err := check(ctx, req.CurrentVersion)
	response := updateCheckResponse{Current: req.CurrentVersion}
	switch {
	case err != nil:
		response.Status = "error"
		response.ErrorCode = updatecheck.ErrorCode(err)
	case result.Estado == updatecheck.EstadoSinPublicaciones:
		response.Status = "no_releases"
	case result.HayNueva:
		response.Status = "newer"
	case !result.Comparable:
		response.Status = "not_comparable"
	default:
		response.Status = "current"
	}
	if err == nil {
		response.Latest = sanitizeOutputText(result.UltimaVersion, maxVersionBytes)
		response.URL = sanitizeOutputText(result.URL, 512)
	}
	return marshal(response)
}

func validateVersionText(version string) error {
	if version == "" || len(version) > maxVersionBytes {
		return newFacadeError("current_version no es valida")
	}
	for _, r := range version {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '.' || r == '-' || r == '+') {
			return newFacadeError("current_version no es valida")
		}
	}
	return nil
}

// defaultUpdateCheck usa un transporte propio: TLS 1.2 o superior, sin proxy
// del entorno y con tiempos acotados. El cliente comprueba la URL oficial,
// limita las redirecciones al mismo origen y el tamaño de la respuesta.
func defaultUpdateCheck(ctx context.Context, version string) (updatecheck.Resultado, error) {
	transport := &http.Transport{
		Proxy:                  nil,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  8 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
		DisableKeepAlives:      true,
	}
	defer transport.CloseIdleConnections()
	client := updatecheck.NewWithHTTPClient(&http.Client{Transport: transport})
	return client.Comprobar(ctx, version)
}
