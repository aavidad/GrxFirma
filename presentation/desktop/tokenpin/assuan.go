// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenpin

import (
	"bytes"
	"context"
	"io"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
)

const maxLine = 3*pkcs11worker.MaxPINBytes + 16
const receiveBufferSize = 4096
const pinMemorySize = receiveBufferSize + maxLine + pkcs11worker.MaxPINBytes

type pinMemoryAllocator func(int) (*secmem.Blob, error)

// Private Assuan pipes, per the upstream pinentry protocol:
// https://github.com/gpg/pinentry/blob/master/doc/pinentry.texi
// Unlike bufio.Scanner/string conversion, all receive buffers can be cleared.
type protocolReader struct {
	input          io.Reader
	buffer         []byte
	position, size int
	line           []byte
	pin            []byte
	owner          *secmem.Blob
}

func newProtocolReader(input io.Reader, protected bool, allocate pinMemoryAllocator) (*protocolReader, error) {
	r := &protocolReader{input: input}
	var memory []byte
	if protected {
		// CONFIRM does not request or accept PIN data and must remain usable
		// when memory locking is unavailable (e.g. hardware PIN pads).
		memory = make([]byte, receiveBufferSize+maxLine)
	} else {
		if allocate == nil {
			allocate = secmem.NewLockedSize
		}
		owner, err := allocate(pinMemorySize)
		if err != nil || owner == nil || !owner.Locked() || len(owner.Bytes()) != pinMemorySize {
			if owner != nil {
				owner.Destroy()
			}
			return nil, pkcs11worker.ErrPINMemoryUnavailable
		}
		r.owner = owner
		memory = owner.Bytes()
		r.pin = memory[receiveBufferSize+maxLine : pinMemorySize : pinMemorySize]
	}
	r.buffer = memory[:receiveBufferSize:receiveBufferSize]
	r.line = memory[receiveBufferSize : receiveBufferSize+maxLine : receiveBufferSize+maxLine]
	return r, nil
}

func (r *protocolReader) close() {
	clear(r.buffer)
	clear(r.line)
	clear(r.pin)
	if r.owner != nil {
		// Destroy also clears allocation padding before unlocking its pages.
		r.owner.Destroy()
	}
}

func (r *protocolReader) next() ([]byte, error) {
	clear(r.line[:])
	for count := 0; count < len(r.line); count++ {
		if r.position == r.size {
			clear(r.buffer[:])
			n, err := r.input.Read(r.buffer[:])
			if n == 0 {
				if err != nil {
					return nil, err
				}
				return nil, io.ErrNoProgress
			}
			r.position, r.size = 0, n
		}
		b := r.buffer[r.position]
		r.buffer[r.position] = 0
		r.position++
		if b == '\n' {
			if count > 0 && r.line[count-1] == '\r' {
				count--
			}
			return r.line[:count], nil
		}
		r.line[count] = b
	}
	return nil, ErrProtocol
}

func (r *protocolReader) response(pinAllowed bool) (pin []byte, err error) {
	if pinAllowed {
		if len(r.pin) != pkcs11worker.MaxPINBytes {
			return nil, ErrProtocol
		}
		clear(r.pin)
		pin = r.pin[:0]
	}
	defer func() {
		clear(r.line[:])
		if err != nil {
			clear(pin)
			pin = nil
		}
	}()
	errorStatus := false
	for count := 0; count < 64; count++ {
		line, readErr := r.next()
		if readErr != nil {
			return pin, ErrProtocol
		}
		switch {
		case bytes.Equal(line, []byte("OK")), bytes.HasPrefix(line, []byte("OK ")):
			if errorStatus {
				return pin, ErrProtocol
			}
			return pin, nil
		case bytes.HasPrefix(line, []byte("ERR ")):
			return pin, responseError(line[4:])
		case bytes.HasPrefix(line, []byte("#")), len(line) == 0:
			continue
		case bytes.HasPrefix(line, []byte("S BUTTON_INFO ")):
			continue
		case bytes.HasPrefix(line, []byte("S ERROR ")):
			// Some frontends emit sanitized status separately before ERR.
			// Discard all text and still require the terminating error code.
			errorStatus = true
			continue
		case bytes.Equal(line, []byte("D")), bytes.HasPrefix(line, []byte("D ")):
			if !pinAllowed || errorStatus {
				return pin, ErrProtocol
			}
			data := line[1:]
			if len(data) > 0 {
				data = data[1:]
			}
			for pos := 0; pos < len(data); pos++ {
				b := data[pos]
				if b == '%' {
					if pos+2 >= len(data) {
						return pin, ErrProtocol
					}
					hi, lo := unhex(data[pos+1]), unhex(data[pos+2])
					if hi < 0 || lo < 0 {
						return pin, ErrProtocol
					}
					b = byte(hi<<4 | lo) // #nosec G115 -- unhex returned two valid 4-bit nibbles.
					pos += 2
				}
				if len(pin) >= pkcs11worker.MaxPINBytes {
					return pin, ErrProtocol
				}
				pin = append(pin, b)
			}
		default:
			// In particular: no INQUIRE, PASSWORD_FROM_CACHE or PIN echo.
			return pin, ErrProtocol
		}
	}
	return pin, ErrProtocol
}

func unhex(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	default:
		return -1
	}
}

func responseError(line []byte) error {
	// libgpg-error encodes the source in higher bits; only inspect the fixed
	// code, never copy/return the arbitrary error text (which may contain PIN).
	// https://github.com/gpg/libgpg-error/blob/master/src/err-codes.h.in
	var code uint32
	digits := 0
	for _, b := range line {
		if b == ' ' {
			break
		}
		if b < '0' || b > '9' || digits >= 10 || uint64(code)*10+uint64(b-'0') > 0xffffffff {
			return ErrProtocol
		}
		code = code*10 + uint32(b-'0')
		digits++
	}
	if digits == 0 {
		return ErrProtocol
	}
	switch code & 0xffff {
	case 99, 114, 198, 277:
		return pkcs11worker.ErrPINCancelled
	case 62:
		return context.DeadlineExceeded
	default:
		return ErrFailed
	}
}

func exchange(input io.Writer, output io.Reader, commands []string, protected bool) (pin []byte, err error) {
	r, err := newProtocolReader(output, protected, nil)
	if err != nil {
		return nil, err
	}
	defer r.close()
	pin, err = r.exchange(input, commands, protected)
	if err != nil {
		return nil, err
	}
	// The external []byte contract is intentionally a caller-owned copy.
	// All protocol receive/decode storage remains locked until BYE and EOF.
	return append([]byte(nil), pin...), nil
}

// exchange returns a borrowed PIN view; the caller must retain r until done.
// The process caller delays the sole compatibility copy until successful Wait.
func (r *protocolReader) exchange(input io.Writer, commands []string, protected bool) (pin []byte, err error) {
	defer func() {
		if err != nil {
			clear(pin)
			pin = nil
		}
	}()
	if _, err = r.response(false); err != nil {
		return nil, err
	}
	for _, command := range commands {
		if _, err = io.WriteString(input, command+"\n"); err != nil {
			return nil, ErrFailed
		}
		if _, err = r.response(false); err != nil {
			return nil, err
		}
	}
	command := "GETPIN\n"
	if protected {
		command = "CONFIRM\n"
	}
	if _, err = io.WriteString(input, command); err != nil {
		return nil, ErrFailed
	}
	if pin, err = r.response(!protected); err != nil {
		return pin, err
	}
	if _, err = io.WriteString(input, "BYE\n"); err != nil {
		return pin, ErrFailed
	}
	if _, err = r.response(false); err != nil {
		return pin, err
	}
	// No unsolicited bytes, second result or lingering helper after success.
	if r.position != r.size {
		return pin, ErrProtocol
	}
	// Even unsolicited trailing bytes are received into the owned region.
	n, eofErr := r.input.Read(r.buffer[:1])
	clear(r.buffer)
	if n != 0 || eofErr != io.EOF {
		return pin, ErrProtocol
	}
	return pin, nil
}
