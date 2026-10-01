// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tsaclient

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/subtle"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/digitorus/timestamp"
	"grxfirma/internal/ports"
)

var (
	oidDigestSHA256 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA512       = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
)

const maxTimestampResponseBytes = 1024 * 1024

type timeStampReq struct {
	Version        int
	MessageImprint messageImprint
	ReqPolicy      asn1.ObjectIdentifier `asn1:"optional"`
	Nonce          *big.Int              `asn1:"optional"`
	CertReq        bool
	Extensions     []pkix.Extension `asn1:"optional,tag:0"`
}

type messageImprint struct {
	HashAlgorithm pkix.AlgorithmIdentifier
	HashedMessage []byte
}

type timeStampResp struct {
	Status         pkiStatusInfo
	TimeStampToken asn1.RawValue `asn1:"optional"`
}

type pkiStatusInfo struct {
	Status       int
	StatusString []asn1.RawValue `asn1:"optional"`
	FailInfo     asn1.BitString  `asn1:"optional"`
}

// Client implementa ports.TimestampAuthority sobre HTTP/HTTPS.
type Client struct {
	URL        string
	HTTPClient *http.Client
}

// New crea un cliente TSA con timeout por defecto.
func New(url string) *Client {
	return &Client{
		URL: url,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// RequestTimestamp solicita un TimeStampToken para el hash indicado.
func (c *Client) RequestTimestamp(ctx context.Context, hash []byte, hashAlgo crypto.Hash) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timestampRequestTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(c.URL)
	if err != nil || endpoint == nil || endpoint.Hostname() == "" ||
		(endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, errors.New("URL HTTP(S) de TSA inválida")
	}
	hashOID, err := oidForHash(hashAlgo)
	if err != nil {
		return nil, err
	}
	if len(hash) != hashAlgo.Size() {
		return nil, fmt.Errorf(
			"longitud de hash invalida para TSA: %d, esperada %d",
			len(hash),
			hashAlgo.Size(),
		)
	}
	nonce, err := randomNonce()
	if err != nil {
		return nil, fmt.Errorf("error generando nonce para TSA: %w", err)
	}

	req := timeStampReq{
		Version: 1,
		MessageImprint: messageImprint{
			HashAlgorithm: pkix.AlgorithmIdentifier{
				Algorithm: hashOID,
			},
			HashedMessage: hash,
		},
		Nonce:   nonce,
		CertReq: true,
	}
	reqDER, err := asn1.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error serializando TimeStampReq: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(reqDER))
	if err != nil {
		return nil, errors.New("error creando petición HTTP a TSA")
	}
	httpReq.Header.Set("Content-Type", "application/timestamp-query")

	client := boundedHTTPClient(c.HTTPClient)
	resp, err := client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("error en petición HTTP a TSA: %w", ctx.Err())
		}
		return nil, errors.New("error en petición HTTP a TSA; comprueba el servicio y su URL final")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TSA respondio con HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxTimestampResponseBytes {
		return nil, fmt.Errorf("respuesta TSA demasiado grande: %d bytes", resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTimestampResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta TSA: %w", err)
	}
	if len(body) > maxTimestampResponseBytes {
		return nil, errors.New("respuesta TSA demasiado grande")
	}

	var tsResp timeStampResp
	rest, err := asn1.Unmarshal(body, &tsResp)
	if err != nil {
		return nil, fmt.Errorf("error parseando TimeStampResp: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("TimeStampResp contiene datos sobrantes")
	}
	if tsResp.Status.Status != 0 && tsResp.Status.Status != 1 {
		return nil, fmt.Errorf("TSA rechazo la solicitud con status %d", tsResp.Status.Status)
	}
	if len(tsResp.TimeStampToken.FullBytes) == 0 {
		return nil, errors.New("la respuesta TSA no contiene TimeStampToken")
	}
	if err := validateTimestampToken(
		tsResp.TimeStampToken.FullBytes,
		hash,
		hashAlgo,
		nonce,
	); err != nil {
		return nil, err
	}

	return tsResp.TimeStampToken.FullBytes, nil
}

func validateTimestampToken(
	token []byte,
	expectedHash []byte,
	expectedAlgorithm crypto.Hash,
	expectedNonce *big.Int,
) error {
	parsed, err := timestamp.Parse(token)
	if err != nil {
		return fmt.Errorf("TimeStampToken invalido: %w", err)
	}
	// CertReq=true exige que la TSA incluya su certificado. timestamp.Parse
	// verifica entonces la firma CMS antes de exponer el TSTInfo.
	if len(parsed.Certificates) == 0 {
		return errors.New("TimeStampToken sin certificado TSA para verificar la firma")
	}
	if parsed.HashAlgorithm != expectedAlgorithm {
		return fmt.Errorf(
			"TimeStampToken usa un algoritmo distinto al solicitado: %v",
			parsed.HashAlgorithm,
		)
	}
	if len(parsed.HashedMessage) != len(expectedHash) ||
		subtle.ConstantTimeCompare(parsed.HashedMessage, expectedHash) != 1 {
		return errors.New("messageImprint de TimeStampToken no coincide con la solicitud")
	}
	if expectedNonce == nil || parsed.Nonce == nil ||
		parsed.Nonce.Cmp(expectedNonce) != 0 {
		return errors.New("nonce de TimeStampToken no coincide con la solicitud")
	}
	return nil
}

func oidForHash(h crypto.Hash) (asn1.ObjectIdentifier, error) {
	switch h {
	case crypto.SHA256:
		return oidDigestSHA256, nil
	case crypto.SHA512:
		return oidSHA512, nil
	default:
		return nil, fmt.Errorf("algoritmo hash no soportado para TSA: %v", h)
	}
}

func randomNonce() (*big.Int, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(b), nil
}

var _ ports.TimestampAuthority = (*Client)(nil)
