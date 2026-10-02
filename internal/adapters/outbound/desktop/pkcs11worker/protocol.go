// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package pkcs11worker defines the private, bounded pipe protocol for the
// isolated PKCS#11 helper. It does not import or load a native module.
package pkcs11worker

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/domain"
)

const (
	ProtocolVersion           = 1
	MaxRequestBytes           = 64 * 1024
	MaxResponseBytes          = 8 * 1024 * 1024
	MaxPINBytes               = 4096
	MaxCertificates           = 256
	MaxCertificateBytes       = 64 * 1024
	MaxChainCertificates      = 64
	MaxSignatureBytes         = 16 * 1024
	FrameRequest         byte = 1
	FrameResponse        byte = 2
	FramePINChallenge    byte = 3
	FramePINReply        byte = 4
)

var ErrProtocol = errors.New("protocolo privado PKCS#11 no valido")
var ErrPINCancelled = errors.New("entrada de PIN cancelada")

// These fixed local presentation errors are never decoded from driver text.
var ErrPINUnavailable = errors.New("diálogo PIN no disponible; instale pinentry-qt o pinentry-gnome3 en una sesión gráfica")
var ErrPINPromptFailed = errors.New("no se pudo completar el diálogo PIN")
var ErrPINMemoryUnavailable = errors.New("no se pudo proteger la memoria del PIN; cierre otras operaciones y revise el límite de memoria fijada del sistema")

// Request contains no PIN. ModulePath is supplied exclusively by the local
// administrator/configuration, never from web signature options.
type Request struct {
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Operation     string `json:"operation"`
	ModulePath    string `json:"modulePath"`
	CertificateID string `json:"certificateId,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	Hash          string `json:"hash,omitempty"`
	Digest        []byte `json:"digest,omitempty"`
}

// CatalogEntry transports the explicit private-key evidence omitted by the
// public CertificateRef JSON contract. Unknown is not converted to true.
type CatalogEntry struct {
	Certificate           domain.CertificateRef `json:"certificate"`
	HasSigningKey         bool                  `json:"hasSigningKey"`
	SigningKeyNeedsUnlock bool                  `json:"signingKeyNeedsUnlock"`
}

type Response struct {
	Version      int            `json:"version"`
	ID           string         `json:"id"`
	Code         string         `json:"code"`
	Certificates []CatalogEntry `json:"certificates,omitempty"`
	ChainDER     [][]byte       `json:"chainDER,omitempty"`
	Signature    []byte         `json:"signature,omitempty"`
}

// PINChallenge deliberately contains no driver-provided subject, label or
// message. The parent displays its own selected certificate and operation.
type PINChallenge struct {
	Version                     int    `json:"version"`
	ID                          string `json:"id"`
	ProtectedAuthenticationPath bool   `json:"protectedAuthenticationPath"`
	ContextSpecific             bool   `json:"contextSpecific"`
}

func (r Request) Validate() error {
	if r.Version != ProtocolVersion || len(r.ID) != 32 || !isHex(r.ID) ||
		len(r.ModulePath) > 4096 || !filepath.IsAbs(r.ModulePath) ||
		strings.ContainsRune(r.ModulePath, 0) {
		return ErrProtocol
	}
	switch r.Operation {
	case "list":
		if r.CertificateID != "" || r.Fingerprint != "" || r.Hash != "" || len(r.Digest) != 0 {
			return ErrProtocol
		}
	case "describe", "sign":
		if len(r.CertificateID) == 0 || len(r.CertificateID) > 2048 ||
			len(r.Fingerprint) != 64 || !isHex(r.Fingerprint) {
			return ErrProtocol
		}
		if r.Operation == "describe" {
			if r.Hash != "" || len(r.Digest) != 0 {
				return ErrProtocol
			}
		} else {
			length := map[string]int{"sha256": 32, "sha384": 48, "sha512": 64}[r.Hash]
			if length == 0 || len(r.Digest) != length {
				return ErrProtocol
			}
		}
	default:
		return ErrProtocol
	}
	return nil
}

func isHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

// WriteFrame writes a length-prefixed frame without logging or converting its
// binary payload to a string. The caller owns and clears sensitive payloads.
func WriteFrame(w io.Writer, kind byte, payload []byte, limit int) error {
	if limit < 0 || len(payload) > limit || uint64(len(payload)) > uint64(^uint32(0)) {
		return ErrProtocol
	}
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload))) // #nosec G115 -- len(payload) was bounded by uint32 above.
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) != 0 {
		n, err := w.Write(data)
		if n < 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// ReadFrame checks the header before allocating its payload.
func ReadFrame(r io.Reader, limit int) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	length := uint64(binary.BigEndian.Uint32(header[1:]))
	if limit < 0 || length > uint64(limit) {
		return 0, nil, ErrProtocol
	}
	payload := make([]byte, int(length)) // #nosec G115 -- length <= non-negative int limit above.
	if _, err := io.ReadFull(r, payload); err != nil {
		clear(payload)
		return 0, nil, err
	}
	return header[0], payload, nil
}

func writeJSON(w io.Writer, kind byte, value any, limit int) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return ErrProtocol
	}
	defer clear(payload)
	return WriteFrame(w, kind, payload, limit)
}

func decodeJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrProtocol
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrProtocol
	}
	return nil
}

func WriteRequest(w io.Writer, request Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	return writeJSON(w, FrameRequest, request, MaxRequestBytes)
}

// WritePINReply sends a cancellation marker or raw PIN bytes in a separate
// frame. Neither JSON/base64, argv nor environment variables carry the PIN.
func WritePINReply(w io.Writer, pin []byte, cancelled bool) error {
	if len(pin) > MaxPINBytes || (cancelled && len(pin) != 0) {
		return ErrProtocol
	}
	payload, release, err := newPINReplyBuffer(len(pin) == 0, nil)
	if err != nil {
		return err
	}
	defer release()
	return writePINReplyInto(w, pin, cancelled, payload)
}

type pinBufferAllocator func(int) (*secmem.Blob, error)

// Allocate before prompting/reading a PIN. A protected reader exchanges only
// a one-byte consent marker, which contains no secret and needs no mlock.
func newPINReplyBuffer(markerOnly bool, allocate pinBufferAllocator) ([]byte, func(), error) {
	if markerOnly {
		marker := make([]byte, 1)
		return marker, func() { clear(marker) }, nil
	}
	if allocate == nil {
		allocate = secmem.NewLockedSize
	}
	owner, err := allocate(MaxPINBytes + 1)
	if err != nil || owner == nil || !owner.Locked() || owner.Len() != MaxPINBytes+1 {
		if owner != nil {
			owner.Destroy()
		}
		return nil, nil, ErrPINMemoryUnavailable
	}
	return owner.Bytes(), owner.Destroy, nil
}

func writePINReplyInto(w io.Writer, pin []byte, cancelled bool, buffer []byte) error {
	defer clear(buffer)
	if len(pin) > MaxPINBytes || (cancelled && len(pin) != 0) || len(buffer) < 1+len(pin) {
		return ErrProtocol
	}
	payload := buffer[:1+len(pin)]
	clear(payload)
	if !cancelled {
		payload[0] = 1
	}
	copy(payload[1:], pin)
	return WriteFrame(w, FramePINReply, payload, MaxPINBytes+1)
}

// Receive only a PIN frame directly into the caller-owned locked region.
// Its header is checked before reading a byte of the sensitive payload.
func readPINReplyInto(r io.Reader, buffer []byte) (reply []byte, err error) {
	defer func() {
		if err != nil {
			clear(buffer)
		}
	}()
	var header [5]byte
	if _, err = io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	length := uint64(binary.BigEndian.Uint32(header[1:]))
	if header[0] != FramePINReply || length < 1 || length > MaxPINBytes+1 || length > uint64(len(buffer)) {
		return nil, ErrProtocol
	}
	reply = buffer[:int(length):int(length)]
	if _, err = io.ReadFull(r, reply); err != nil {
		return nil, err
	}
	return reply, nil
}
