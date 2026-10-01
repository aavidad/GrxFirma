// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/signingpolicy"
)

type Backend interface {
	ports.CertificateCatalog
	ports.SigningKeyProvider
	Close()
}

// PINSource transfers a mutable PIN buffer to the backend. The server also
// clears each returned buffer on completion, including backend error paths.
type PINMode struct {
	ProtectedAuthenticationPath bool
	ContextSpecific             bool
}

type PINSource func(context.Context, PINMode) ([]byte, error)
type BackendFactory func(modulePath string, source PINSource) (Backend, error)

type Server struct {
	Factory BackendFactory
	// ErrorCode may classify driver errors; its result is allowlisted below.
	ErrorCode func(error) string
	Now       func() time.Time
	pinBuffer pinBufferAllocator // per-instance test seam; never configuration
}

// Serve processes exactly one operation. A supervising process must enforce
// its deadline by terminating the helper, since C drivers can ignore context.
func (s Server) Serve(ctx context.Context, input io.Reader, output io.Writer) (serveErr error) {
	var request Request
	var finalResponse *Response
	respond := func(code string) error {
		id := request.ID
		if len(id) != 32 || !isHex(id) {
			id = ""
		}
		finalResponse = &Response{Version: ProtocolVersion, ID: id, Code: code}
		return nil
	}
	defer func() {
		if recover() != nil {
			_ = respond("driver_failed")
		}
		// Final status is sent only after key/module cleanup and PIN zeroing.
		// A panicking cleanup must not follow an already reported success.
		if finalResponse != nil {
			serveErr = writeJSON(output, FrameResponse, *finalResponse, MaxResponseBytes)
		}
	}()
	kind, payload, err := ReadFrame(input, MaxRequestBytes)
	if err != nil {
		return err
	}
	defer clear(payload)
	if kind != FrameRequest || decodeJSON(payload, &request) != nil || request.Validate() != nil {
		return respond("invalid_request")
	}
	defer clear(request.Digest)
	if ctx.Err() != nil {
		return respond("cancelled")
	}
	if s.Factory == nil {
		return respond("driver_unavailable")
	}

	var pinBuffers []func()
	defer func() {
		for _, release := range pinBuffers {
			release()
		}
	}()
	prompts := 0
	var pinMemoryFailure error
	pinSource := func(pinContext context.Context, mode PINMode) ([]byte, error) {
		if err := pinContext.Err(); err != nil {
			return nil, err
		}
		if request.Operation != "sign" || prompts >= 2 {
			return nil, ErrProtocol
		}
		prompts++
		buffer, release, err := newPINReplyBuffer(mode.ProtectedAuthenticationPath, s.pinBuffer)
		if err != nil {
			pinMemoryFailure = ErrPINMemoryUnavailable
			return nil, err
		}
		// Keep the owner reachable until backend/key cleanup and zeroing finish.
		pinBuffers = append(pinBuffers, release)
		if err := writeJSON(output, FramePINChallenge, PINChallenge{Version: ProtocolVersion, ID: request.ID,
			ProtectedAuthenticationPath: mode.ProtectedAuthenticationPath, ContextSpecific: mode.ContextSpecific}, 256); err != nil {
			return nil, err
		}
		reply, err := readPINReplyInto(input, buffer)
		if err != nil {
			return nil, err
		}
		if err := pinContext.Err(); err != nil {
			return nil, err
		}
		if reply[0] == 0 && len(reply) == 1 {
			return nil, ErrPINCancelled
		}
		if reply[0] != 1 || (mode.ProtectedAuthenticationPath && len(reply) != 1) {
			return nil, ErrProtocol
		}
		return reply[1:], nil
	}
	backend, err := s.Factory(request.ModulePath, pinSource)
	if backend != nil {
		defer backend.Close()
	}
	if err != nil {
		return respond(s.errorCode(err))
	}
	if backend == nil {
		return respond("driver_unavailable")
	}
	refs, err := backend.List(ctx)
	if err != nil {
		return respond(s.errorCode(err))
	}
	if len(refs) > MaxCertificates {
		return respond("size_limit")
	}
	response := Response{Version: ProtocolVersion, ID: request.ID, Code: "ok"}
	for _, ref := range refs {
		if len(ref.ID) == 0 || len(ref.ID) > 2048 || len(ref.Fingerprint) != 64 || !isHex(ref.Fingerprint) ||
			len(ref.Subject) > 4096 || len(ref.Issuer) > 4096 || len(ref.Organizacion) > 4096 || len(ref.NIF) > 512 || len(ref.Tipo) > 128 {
			return respond("driver_failed")
		}
	}
	if request.Operation == "list" {
		response.Certificates = make([]CatalogEntry, len(refs))
		for i, ref := range refs {
			response.Certificates[i] = CatalogEntry{Certificate: ref, HasSigningKey: ref.HasSigningKey, SigningKeyNeedsUnlock: ref.SigningKeyNeedsUnlock}
		}
		finalResponse = &response
		return nil
	}
	var selected domain.CertificateRef
	matches := 0
	for _, ref := range refs {
		if ref.ID == request.CertificateID && strings.EqualFold(ref.Fingerprint, request.Fingerprint) {
			selected = ref
			matches++
		}
	}
	if matches != 1 {
		return respond("certificate_not_found")
	}
	key, err := backend.KeyFor(ctx, selected)
	if key != nil {
		defer ports.CloseSigningKey(key)
	}
	if err != nil {
		return respond(s.errorCode(err))
	}
	if key == nil {
		return respond("key_unavailable")
	}
	chain := key.CertificateChainDER()
	if len(chain) == 0 || len(chain) > MaxChainCertificates {
		return respond("key_unavailable")
	}
	for _, der := range chain {
		if len(der) == 0 || len(der) > MaxCertificateBytes {
			return respond("size_limit")
		}
		if _, err := x509.ParseCertificate(der); err != nil {
			return respond("key_unavailable")
		}
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return respond("key_unavailable")
	}
	fingerprint := sha256.Sum256(leaf.Raw)
	if !strings.EqualFold(hex.EncodeToString(fingerprint[:]), request.Fingerprint) {
		return respond("certificate_not_found")
	}
	if request.Operation == "describe" {
		response.ChainDER = make([][]byte, len(chain))
		for i, der := range chain {
			response.ChainDER[i] = append([]byte(nil), der...)
		}
		finalResponse = &response
		return nil
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	if err := signingpolicy.ValidateCertificateDER(leaf.Raw, now); err != nil {
		return respond("unsuitable_certificate")
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return respond("key_unavailable")
	}
	public, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil || !bytes.Equal(public, leaf.RawSubjectPublicKeyInfo) {
		return respond("key_unavailable")
	}
	hash := map[string]crypto.Hash{"sha256": crypto.SHA256, "sha384": crypto.SHA384, "sha512": crypto.SHA512}[request.Hash]
	if err := ctx.Err(); err != nil {
		return respond("cancelled")
	}
	signature, err := signer.Sign(rand.Reader, request.Digest, hash)
	// Native backends may normalize PINProvider errors to cancellation. Preserve
	// our fixed pre-challenge failure, even if a driver ignores the PIN error.
	if pinMemoryFailure != nil {
		return respond(s.errorCode(pinMemoryFailure))
	}
	if err != nil {
		return respond(s.errorCode(err))
	}
	if err := ctx.Err(); err != nil {
		return respond("cancelled")
	}
	if len(signature) == 0 || len(signature) > MaxSignatureBytes {
		return respond("size_limit")
	}
	response.Signature = append([]byte(nil), signature...)
	finalResponse = &response
	return nil
}

func (s Server) errorCode(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "cancelled"
	}
	if errors.Is(err, ErrPINCancelled) {
		return "pin_cancelled"
	}
	if errors.Is(err, ErrPINMemoryUnavailable) {
		return "pin_memory_unavailable"
	}
	if errors.Is(err, ErrProtocol) {
		return "invalid_request"
	}
	if s.ErrorCode != nil {
		switch code := s.ErrorCode(err); code {
		case "pin_incorrect", "pin_locked", "pin_expired", "pin_cancelled", "device_removed", "driver_unavailable", "unsupported_algorithm", "key_unavailable", "unsuitable_certificate":
			return code
		}
	}
	return "driver_failed"
}
