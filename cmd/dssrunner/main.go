// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

const (
	defaultEndpoint     = "https://ec.europa.eu/digital-building-blocks/DSS/webapp-demo/services/rest/validation/validateSignature"
	maxDSSRedirects     = 3
	maxDSSResponseBytes = 16 << 20

	expectedTotalPassed               = "TOTAL_PASSED"
	expectedIntegrityFormatRecognized = "INTEGRITY_FORMAT_RECOGNIZED"
)

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type remoteDocument struct {
	Bytes           string `json:"bytes"`
	Name            string `json:"name"`
	DigestAlgorithm any    `json:"digestAlgorithm,omitempty"`
}

type validateRequest struct {
	SignedDocument          remoteDocument   `json:"signedDocument"`
	OriginalDocuments       []remoteDocument `json:"originalDocuments,omitempty"`
	TokenExtractionStrategy string           `json:"tokenExtractionStrategy"`
}

type dssDiagnosticSignature struct {
	BasicSignature struct {
		SignatureIntact bool `json:"SignatureIntact"`
		SignatureValid  bool `json:"SignatureValid"`
	} `json:"BasicSignature"`
	SignatureFormat string `json:"SignatureFormat"`
}

type dssSimpleReportSignature struct {
	Signature struct {
		Indication    string `json:"Indication"`
		SubIndication string `json:"SubIndication"`
	} `json:"Signature"`
}

type dssResponse struct {
	DiagnosticData struct {
		Signature []dssDiagnosticSignature `json:"Signature"`
	} `json:"DiagnosticData"`
	SimpleReport struct {
		Signatures []dssSimpleReportSignature `json:"signatureOrTimestampOrEvidenceRecord"`
	} `json:"SimpleReport"`
}

type dssResult string

const (
	dssResultTotalPassed               dssResult = expectedTotalPassed
	dssResultIntegrityFormatRecognized dssResult = expectedIntegrityFormatRecognized
	dssResultInvalid                   dssResult = "INVALID"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dssrunner", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var originals stringSliceFlag
	format := fs.String("format", "", "Formato esperado: cades, xades o pades")
	signedPath := fs.String("signed", "", "Ruta al documento firmado")
	endpoint := fs.String("endpoint", strings.TrimSpace(os.Getenv("GRXFIRMA_DSS_ENDPOINT")), "Endpoint REST DSS")
	expected := fs.String("expect", "", "Resultado exigido: TOTAL_PASSED o INTEGRITY_FORMAT_RECOGNIZED")
	fs.Var(&originals, "original", "Ruta al documento original (repetible)")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *format == "" || *signedPath == "" || *expected == "" {
		_, _ = fmt.Fprintln(stderr, "uso: dssrunner --format cades|xades|pades --signed fichero --expect TOTAL_PASSED|INTEGRITY_FORMAT_RECOGNIZED [--original fichero] [--endpoint url]")
		return 2
	}
	expectedResult, err := parseExpectedResult(*expected)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "resultado DSS esperado no válido: %v\n", err)
		return 2
	}
	if *endpoint == "" {
		*endpoint = defaultEndpoint
	}

	req, err := buildRequest(*signedPath, originals)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "preparando petición DSS: %v\n", err)
		return 1
	}
	resp, err := validate(*endpoint, req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "validando contra DSS: %v\n", err)
		return 1
	}

	summary, err := checkResponse(strings.ToLower(strings.TrimSpace(*format)), expectedResult, resp)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "respuesta DSS no conforme: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintln(stdout, summary)
	return 0
}

func buildRequest(signedPath string, originals []string) (validateRequest, error) {
	signed, err := toRemoteDocument(signedPath)
	if err != nil {
		return validateRequest{}, err
	}
	req := validateRequest{
		SignedDocument:          signed,
		TokenExtractionStrategy: "NONE",
	}
	for _, path := range originals {
		doc, err := toRemoteDocument(path)
		if err != nil {
			return validateRequest{}, err
		}
		req.OriginalDocuments = append(req.OriginalDocuments, doc)
	}
	return req, nil
}

func toRemoteDocument(path string) (remoteDocument, error) {
	data, err := securefile.ReadFileLimit(path, 100*1024*1024)
	if err != nil {
		return remoteDocument{}, fmt.Errorf("leer %s: %w", path, err)
	}
	return remoteDocument{
		Bytes: base64.StdEncoding.EncodeToString(data),
		Name:  filepathBase(path),
	}, nil
}

func validate(endpoint string, req validateRequest) (dssResponse, error) {
	validatedEndpoint, err := validateDSSEndpoint(endpoint)
	if err != nil {
		return dssResponse{}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return dssResponse{}, fmt.Errorf("serializar petición: %w", err)
	}
	// #nosec G704 -- dssrunner is an operator-facing conformance tool; its explicit
	// endpoint is validated as HTTPS (or loopback HTTP for local tests) before use.
	httpReq, err := http.NewRequest(http.MethodPost, validatedEndpoint.String(), bytes.NewReader(body))
	if err != nil {
		return dssResponse{}, fmt.Errorf("crear petición: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxDSSRedirects {
				return errors.New("demasiadas redirecciones del servicio DSS")
			}
			redirect, err := validateDSSEndpoint(req.URL.String())
			if err != nil {
				return err
			}
			if !sameDSSOrigin(validatedEndpoint, redirect) {
				return errors.New("redirección DSS a otro origen no permitida")
			}
			return nil
		},
	}
	// #nosec G704 -- the operator-selected URL was validated above and redirects
	// are restricted to the same origin without a TLS downgrade.
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return dssResponse{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 2048))
		return dssResponse{}, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}

	if httpResp.ContentLength > maxDSSResponseBytes {
		return dssResponse{}, errors.New("respuesta DSS demasiado grande")
	}
	responseBody, err := io.ReadAll(io.LimitReader(httpResp.Body, maxDSSResponseBytes+1))
	if err != nil {
		return dssResponse{}, fmt.Errorf("leer respuesta DSS: %w", err)
	}
	if len(responseBody) > maxDSSResponseBytes {
		return dssResponse{}, errors.New("respuesta DSS demasiado grande")
	}

	var resp dssResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		return dssResponse{}, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return resp, nil
}

func validateDSSEndpoint(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "#") {
		return nil, errors.New("el endpoint DSS no admite fragmentos")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return nil, fmt.Errorf("endpoint DSS no válido: %w", err)
	}
	if parsed.Host == "" {
		return nil, errors.New("el endpoint DSS debe incluir un host")
	}
	if parsed.User != nil {
		return nil, errors.New("el endpoint DSS no admite credenciales en la URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return parsed, nil
	case "http":
		if isLoopbackHost(parsed.Hostname()) {
			return parsed, nil
		}
		return nil, errors.New("HTTP sin TLS solo se admite para endpoints DSS de loopback")
	default:
		return nil, errors.New("el endpoint DSS debe usar HTTPS")
	}
}

func sameDSSOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(strings.TrimSuffix(a.Hostname(), "."), strings.TrimSuffix(b.Hostname(), ".")) &&
		effectiveDSSPort(a) == effectiveDSSPort(b)
}

func effectiveDSSPort(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}
	if strings.EqualFold(endpoint.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(endpoint.Scheme, "http") {
		return "80"
	}
	return ""
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func checkResponse(expectedFormat string, expectedResult dssResult, resp dssResponse) (string, error) {
	if len(resp.DiagnosticData.Signature) == 0 {
		return "", errors.New("DSS no devolvió firmas en DiagnosticData")
	}
	sig := resp.DiagnosticData.Signature[0]
	if !sig.BasicSignature.SignatureIntact || !sig.BasicSignature.SignatureValid {
		return "", fmt.Errorf("firma no íntegra o no válida según DSS: intact=%t valid=%t", sig.BasicSignature.SignatureIntact, sig.BasicSignature.SignatureValid)
	}
	wantPrefix := expectedFormatPrefix(expectedFormat)
	if wantPrefix == "" {
		return "", fmt.Errorf("formato no soportado: %s", expectedFormat)
	}
	if !strings.HasPrefix(sig.SignatureFormat, wantPrefix) {
		return "", fmt.Errorf("DSS devolvió formato %q, se esperaba prefijo %q", sig.SignatureFormat, wantPrefix)
	}

	if len(resp.SimpleReport.Signatures) == 0 {
		return "", errors.New("DSS no devolvió firmas en SimpleReport")
	}
	indication := resp.SimpleReport.Signatures[0].Signature.Indication
	subIndication := resp.SimpleReport.Signatures[0].Signature.SubIndication
	actualResult, err := classifyDSSResult(indication, subIndication)
	if err != nil {
		return "", err
	}

	switch expectedResult {
	case dssResultTotalPassed:
		if actualResult != dssResultTotalPassed {
			return "", fmt.Errorf(
				"DSS no alcanzó TOTAL_PASSED: resultado=%s indicacion=%s subindicacion=%s",
				actualResult,
				normalizedReportValue(indication),
				normalizedSubIndication(subIndication),
			)
		}
	case dssResultIntegrityFormatRecognized:
		if actualResult != dssResultTotalPassed && actualResult != dssResultIntegrityFormatRecognized {
			return "", fmt.Errorf(
				"DSS no confirmó integridad y formato: resultado=%s indicacion=%s subindicacion=%s",
				actualResult,
				normalizedReportValue(indication),
				normalizedSubIndication(subIndication),
			)
		}
	default:
		return "", fmt.Errorf("resultado esperado no soportado: %s", expectedResult)
	}

	return fmt.Sprintf(
		"DSS %s: formato=%s indicacion=%s subindicacion=%s",
		actualResult,
		sig.SignatureFormat,
		normalizedReportValue(indication),
		normalizedSubIndication(subIndication),
	), nil
}

func parseExpectedResult(raw string) (dssResult, error) {
	switch raw {
	case expectedTotalPassed:
		return dssResultTotalPassed, nil
	case expectedIntegrityFormatRecognized:
		return dssResultIntegrityFormatRecognized, nil
	default:
		return "", fmt.Errorf(
			"se exige uno de los valores exactos %s o %s",
			expectedTotalPassed,
			expectedIntegrityFormatRecognized,
		)
	}
}

func classifyDSSResult(indication, subIndication string) (dssResult, error) {
	normalizedIndication := normalizedReportValue(indication)
	normalizedSub := normalizedReportValue(subIndication)

	switch normalizedIndication {
	case expectedTotalPassed:
		if normalizedSub != "" {
			return "", fmt.Errorf(
				"respuesta DSS incoherente: TOTAL_PASSED incluye subindicacion=%s",
				normalizedSub,
			)
		}
		return dssResultTotalPassed, nil
	case "INDETERMINATE":
		switch normalizedSub {
		case "FORMAT_FAILURE", "HASH_FAILURE", "SIG_CRYPTO_FAILURE", "SIGNED_DATA_NOT_FOUND":
			return dssResultInvalid, nil
		case "CHAIN_CONSTRAINTS_FAILURE",
			"CERTIFICATE_CHAIN_GENERAL_FAILURE",
			"CRYPTO_CONSTRAINTS_FAILURE",
			"CRYPTO_CONSTRAINTS_FAILURE_NO_POE",
			"EXPIRED",
			"NOT_YET_VALID",
			"NO_CERTIFICATE_CHAIN_FOUND",
			"NO_POE",
			"NO_SIGNING_CERTIFICATE_FOUND",
			"OUT_OF_BOUNDS_NO_POE",
			"POLICY_PROCESSING_ERROR",
			"REVOKED",
			"REVOKED_CA_NO_POE",
			"REVOKED_NO_POE",
			"SIG_CONSTRAINTS_FAILURE",
			"SIGNATURE_POLICY_NOT_AVAILABLE",
			"TIMESTAMP_ORDER_FAILURE",
			"TRY_LATER":
			return dssResultIntegrityFormatRecognized, nil
		case "":
			return "", errors.New("DSS devolvió INDETERMINATE sin SubIndication")
		default:
			return "", fmt.Errorf(
				"DSS devolvió SubIndication no reconocida para INDETERMINATE: %s",
				normalizedSub,
			)
		}
	case "INVALID", "TOTAL_FAILED", "FAILED":
		return dssResultInvalid, nil
	case "":
		return "", errors.New("DSS devolvió Indication vacía")
	default:
		return "", fmt.Errorf(
			"DSS devolvió Indication no reconocida: indicacion=%s subindicacion=%s",
			normalizedIndication,
			normalizedSubIndication(subIndication),
		)
	}
}

func normalizedReportValue(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizedSubIndication(value string) string {
	if normalized := normalizedReportValue(value); normalized != "" {
		return normalized
	}
	return "N/A"
}

func expectedFormatPrefix(format string) string {
	switch format {
	case "cades":
		return "CAdES"
	case "xades":
		return "XAdES"
	case "pades":
		return "PAdES"
	default:
		return ""
	}
}

func filepathBase(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx < 0 {
		return path
	}
	return path[idx+1:]
}
