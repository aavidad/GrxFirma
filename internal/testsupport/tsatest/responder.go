// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package tsatest ofrece una TSA RFC 3161 firmada para pruebas de integración.
package tsatest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
)

const maxRequestBytes = 64 * 1024

// Responder construye respuestas RFC 3161 ligadas al nonce y al messageImprint
// de cada petición.
type Responder struct {
	certificate *x509.Certificate
	privateKey  *rsa.PrivateKey
}

// NewResponder crea una TSA efímera con certificado de uso exclusivo para
// timestamping.
func NewResponder(t testing.TB) *Responder {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("tsatest: no se pudo generar la clave TSA: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject: pkix.Name{
			CommonName: "GrxFirma Test TSA",
		},
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
	}
	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		t.Fatalf("tsatest: no se pudo crear el certificado TSA: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("tsatest: no se pudo parsear el certificado TSA: %v", err)
	}
	return &Responder{certificate: certificate, privateKey: privateKey}
}

// ResponseFor crea una respuesta válida para requestDER. mutate permite a los
// tests negativos alterar de forma explícita los campos que se quieren probar.
func (r *Responder) ResponseFor(
	requestDER []byte,
	mutate func(*timestamp.Timestamp),
) ([]byte, error) {
	request, err := timestamp.ParseRequest(requestDER)
	if err != nil {
		return nil, fmt.Errorf("peticion TSA invalida: %w", err)
	}
	response := &timestamp.Timestamp{
		HashAlgorithm:     request.HashAlgorithm,
		HashedMessage:     append([]byte(nil), request.HashedMessage...),
		Time:              time.Now().UTC(),
		Policy:            asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1},
		AddTSACertificate: true,
	}
	if request.Nonce != nil {
		response.Nonce = new(big.Int).Set(request.Nonce)
	}
	if mutate != nil {
		mutate(response)
	}
	return response.CreateResponseWithOpts(r.certificate, r.privateKey, crypto.SHA256)
}

// ServeHTTP responde a una petición timestamp-query con timestamp-reply.
func (r *Responder) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxRequestBytes+1))
	if err != nil || len(body) > maxRequestBytes {
		http.Error(w, "invalid timestamp request", http.StatusBadRequest)
		return
	}
	response, err := r.ResponseFor(body, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/timestamp-reply")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}
