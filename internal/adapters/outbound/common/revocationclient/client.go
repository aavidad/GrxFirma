// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/ocsp"
	"grxfirma/internal/ports"
)

type Client struct {
	http *http.Client
	now  func() time.Time
}

func New() *Client {
	return newClient(&http.Client{Timeout: 15 * time.Second}, defaultEndpointPolicy())
}

func NewWithHTTPClient(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return newClient(client, defaultEndpointPolicy())
}

func newClient(client *http.Client, policy endpointPolicy) *Client {
	return &Client{
		http: hardenHTTPClient(client, policy),
		now:  time.Now,
	}
}

// CertificateStatus es el resultado autenticado de una consulta de revocación.
type CertificateStatus uint8

const (
	CertificateStatusGood CertificateStatus = iota
	CertificateStatusRevoked
	CertificateStatusUnknown
)

// CheckResult conserva el estado y la fuente de la evidencia autenticada.
type CheckResult struct {
	Status    CertificateStatus
	Method    string
	Evidence  ports.RevocationEvidence
	RevokedAt time.Time
	Reason    string
}

var (
	ErrCertificateRevoked       = errors.New("certificado revocado")
	ErrCertificateStatusUnknown = errors.New("estado de revocación desconocido")
)

// Check descarga y autentica evidencia de revocación. Un estado OCSP explícito
// Revoked o Unknown es terminal: nunca se oculta consultando otra fuente.
func (c *Client) Check(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (CheckResult, error) {
	if cert == nil || issuer == nil {
		return CheckResult{Status: CertificateStatusUnknown}, errors.New("certificado o emisor nulo")
	}
	if c == nil || c.http == nil || c.now == nil {
		return CheckResult{Status: CertificateStatusUnknown}, errors.New("cliente de revocación no configurado")
	}
	if err := validateCertificateIssuer(cert, issuer); err != nil {
		return CheckResult{Status: CertificateStatusUnknown}, err
	}

	var (
		result CheckResult
		errs   []error
	)
	ocspResult, ocspDER, ocspErr := c.fetchOCSP(ctx, cert, issuer)
	if ocspErr == nil {
		switch ocspResult.Status {
		case CertificateStatusRevoked, CertificateStatusUnknown:
			return CheckResult{
				Status:    ocspResult.Status,
				Method:    "ocsp",
				RevokedAt: ocspResult.RevokedAt,
			}, nil
		case CertificateStatusGood:
			result.Status = CertificateStatusGood
			result.Method = "ocsp"
			result.Evidence.OCSPResponses = append(result.Evidence.OCSPResponses, ocspDER)
		}
	} else {
		errs = append(errs, fmt.Errorf("OCSP: %w", ocspErr))
	}

	crlResult, crlDER, crlErr := c.fetchCRL(ctx, cert, issuer)
	if crlErr == nil {
		if crlResult.Status == CertificateStatusRevoked {
			return CheckResult{
				Status:    CertificateStatusRevoked,
				Method:    "crl",
				RevokedAt: crlResult.RevokedAt,
			}, nil
		}
		if crlResult.Status == CertificateStatusGood {
			if result.Method == "" {
				result.Status = CertificateStatusGood
				result.Method = "crl"
			}
			result.Evidence.CRLs = append(result.Evidence.CRLs, crlDER)
		}
	} else {
		errs = append(errs, fmt.Errorf("CRL: %w", crlErr))
	}

	if result.Method == "" {
		joined := errors.Join(errs...)
		reason := "sin evidencias OCSP o CRL autenticadas disponibles"
		if joined != nil {
			reason = joined.Error()
		}
		return CheckResult{
				Status: CertificateStatusUnknown,
				Reason: reason,
			}, errors.Join(
				errors.New("sin evidencias OCSP o CRL autenticadas disponibles"),
				joined,
			)
	}
	return result, nil
}

func (c *Client) Fetch(ctx context.Context, cert *x509.Certificate, issuer *x509.Certificate) (ports.RevocationEvidence, error) {
	result, err := c.Check(ctx, cert, issuer)
	if err != nil {
		return ports.RevocationEvidence{}, err
	}
	switch result.Status {
	case CertificateStatusGood:
		return result.Evidence, nil
	case CertificateStatusRevoked:
		return ports.RevocationEvidence{}, fmt.Errorf("%w según %s", ErrCertificateRevoked, result.Method)
	default:
		return ports.RevocationEvidence{}, fmt.Errorf("%w según %s", ErrCertificateStatusUnknown, result.Method)
	}
}

// ComprobarIdentidad adapta el resultado autenticado al contrato trivalente genérico.
func (c *Client) ComprobarIdentidad(ctx context.Context, cert, issuer *x509.Certificate) (ports.ResultadoRevocacionIdentidad, error) {
	resultado, err := c.Check(ctx, cert, issuer)
	salida := ports.ResultadoRevocacionIdentidad{Fuente: resultado.Method}
	if c != nil && c.now != nil {
		salida.ComprobadoEn = c.now().UTC()
	}
	switch resultado.Status {
	case CertificateStatusGood:
		salida.Estado = ports.EstadoRevocacionConforme
	case CertificateStatusRevoked:
		salida.Estado = ports.EstadoRevocacionRevocada
	case CertificateStatusUnknown:
		salida.Estado = ports.EstadoRevocacionIndeterminada
	}
	return salida, err
}

const (
	maxOCSPResponseBytes int64 = 1 << 20
	maxCRLResponseBytes  int64 = 16 << 20
)

func (c *Client) fetchOCSP(
	ctx context.Context,
	cert *x509.Certificate,
	issuer *x509.Certificate,
) (ValidationResult, []byte, error) {
	if len(cert.OCSPServer) == 0 {
		return ValidationResult{}, nil, errors.New("sin servidor OCSP")
	}

	reqDER, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return ValidationResult{}, nil, fmt.Errorf("construyendo petición OCSP: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cert.OCSPServer[0], bytesReader(reqDER))
	if err != nil {
		return ValidationResult{}, nil, err
	}
	req.Header.Set("Content-Type", "application/ocsp-request")

	resp, err := c.http.Do(req)
	if err != nil {
		return ValidationResult{}, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ValidationResult{}, nil, fmt.Errorf("ocsp HTTP %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxOCSPResponseBytes, "OCSP")
	if err != nil {
		return ValidationResult{}, nil, err
	}
	return ValidateOCSPResponse(body, cert, issuer, c.now())
}

func (c *Client) fetchCRL(
	ctx context.Context,
	cert *x509.Certificate,
	issuer *x509.Certificate,
) (ValidationResult, []byte, error) {
	if len(cert.CRLDistributionPoints) == 0 {
		return ValidationResult{}, nil, errors.New("sin punto de distribución CRL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cert.CRLDistributionPoints[0], nil)
	if err != nil {
		return ValidationResult{}, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ValidationResult{}, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ValidationResult{}, nil, fmt.Errorf("crl HTTP %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxCRLResponseBytes, "CRL")
	if err != nil {
		return ValidationResult{}, nil, err
	}
	result, err := ValidateCRLResponse(body, cert, issuer, c.now())
	return result, body, err
}

func readBoundedBody(body io.Reader, limit int64, label string) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("límite de respuesta no positivo")
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s supera el máximo de %d bytes", label, limit)
	}
	return data, nil
}

type bodyReader struct {
	data []byte
	pos  int
}

func bytesReader(data []byte) io.Reader {
	return &bodyReader{data: data}
}

func (r *bodyReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

var _ ports.RevocationProvider = (*Client)(nil)
var _ ports.ComprobadorRevocacionIdentidad = (*Client)(nil)

const maxIssuerCertificateBytes = 64 * 1024
const maxIssuerAIAURLs = 4

// FetchIssuer descarga el certificado emisor indicado en la extensión AIA
// (CA Issuers) con el cliente HTTP endurecido. Solo se acepta si su nombre es
// el emisor del certificado y su clave verifica la firma de éste: la
// descarga nunca aporta confianza por sí misma, solo completa la cadena para
// poder consultar la revocación.
func (c *Client) FetchIssuer(ctx context.Context, cert *x509.Certificate) (*x509.Certificate, error) {
	if c == nil || c.http == nil || cert == nil {
		return nil, errors.New("cliente AIA no configurado")
	}
	// Un certificado puede publicar varias URL. El presupuesto total limita
	// toda la búsqueda, no solo cada petición individual del cliente HTTP.
	ctx, cancel := context.WithTimeout(ctx, defaultHTTPTimeout)
	defer cancel()
	var lastErr error = errors.New("el certificado no indica la URL del emisor (AIA)")
	for i, url := range cert.IssuingCertificateURL {
		if i >= maxIssuerAIAURLs {
			break
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := readBoundedBody(resp.Body, maxIssuerCertificateBytes, "certificado emisor")
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("AIA HTTP %d", resp.StatusCode)
			continue
		}
		issuer, err := parseIssuerCertificate(body)
		if err != nil {
			lastErr = err
			continue
		}
		if !bytes.Equal(issuer.RawSubject, cert.RawIssuer) || cert.CheckSignatureFrom(issuer) != nil {
			lastErr = errors.New("el certificado descargado por AIA no es el emisor")
			continue
		}
		return issuer, nil
	}
	return nil, lastErr
}

func parseIssuerCertificate(body []byte) (*x509.Certificate, error) {
	if block, _ := pem.Decode(body); block != nil && block.Type == "CERTIFICATE" {
		body = block.Bytes
	}
	return x509.ParseCertificate(body)
}
