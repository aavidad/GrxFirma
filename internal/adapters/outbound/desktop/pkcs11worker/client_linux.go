// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
)

var (
	ErrHelperUnavailable = errors.New("auxiliar PKCS#11 no disponible")
	ErrOperationFailed   = errors.New("operación aislada PKCS#11 fallida")
)

// OperationError exposes a fixed protocol code, never an error from exec or
// from a native driver. The UI translates codes using its own local messages.
type OperationError struct{ Code string }

func (e *OperationError) Error() string { return "PKCS#11: " + e.Code }

// Client starts one process per operation, with private anonymous pipes. Both
// paths must come from trusted local settings, not browser request options.
// PINSource must honor context cancellation; late buffers are still cleared.
type Client struct {
	Executable string
	ModulePath string
	PINSource  PINSource
	Timeout    time.Duration
	// Resources are explicit local driver data grants, never request options,
	// environment variables or browser-provided configuration. Zero means the
	// fixed PC/SC profile (plus the built-in SoftHSM file-store profile).
	Resources SandboxResources
	pinBuffer pinBufferAllocator // per-instance test seam; never configuration
}

func (c Client) Execute(ctx context.Context, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	modulePath, err := ValidateModulePath(c.ModulePath)
	if err != nil {
		return Response{}, ErrHelperUnavailable
	}
	executable, err := ValidateModulePath(c.Executable)
	if err != nil {
		return Response{}, ErrHelperUnavailable
	}
	info, err := os.Stat(executable)
	if err != nil || info.Mode().Perm()&0111 == 0 {
		return Response{}, ErrHelperUnavailable
	}
	// The caller cannot redirect the configured native module via Request.
	if request.ModulePath != "" && request.ModulePath != modulePath {
		return Response{}, ErrProtocol
	}
	request.ModulePath = modulePath
	request.Version = ProtocolVersion
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Response{}, ErrOperationFailed
	}
	request.ID = hex.EncodeToString(id[:])
	if err := request.Validate(); err != nil {
		return Response{}, err
	}
	timeout := c.Timeout
	if timeout <= 0 || timeout > 2*time.Minute {
		timeout = 2 * time.Minute
	}
	operationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resources, err := moduleSandboxResources(modulePath, c.Resources)
	if err != nil {
		return Response{}, err
	}
	launch, err := NewSandboxCommand(operationCtx, executable, modulePath, resources)
	if err != nil {
		return Response{}, err
	}
	defer launch.Close()
	if launch.Module != modulePath || launch.Executable != executable {
		return Response{}, ErrSandboxPolicy
	}
	command := launch.Command
	// Each operation requires its own filesystem/network/PID boundary. There
	// is no unsandboxed fallback when the local tool or kernel cannot create it.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		// A driver may create children. This is the fresh group created above,
		// never the process group of the parent application.
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = time.Second
	input, err := command.StdinPipe()
	if err != nil {
		return Response{}, ErrHelperUnavailable
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		return Response{}, ErrHelperUnavailable
	}
	defer output.Close()
	// A nil stderr is /dev/null: even startup failures cannot leak driver logs.
	if err := command.Start(); err != nil {
		return Response{}, ErrSandboxUnavailable
	}
	_ = launch.Close() // the child inherited only descriptors consumed by bwrap
	// A driver descendant could retain a pipe even after its group is killed.
	// Closing our ends on the deadline also bounds blocked Read/Write calls.
	stopIO := context.AfterFunc(operationCtx, func() {
		_ = input.Close()
		_ = output.Close()
	})
	defer stopIO()
	waited := false
	defer func() {
		if !waited {
			_ = command.Cancel()
			_ = input.Close()
			_ = output.Close()
			_ = command.Wait()
		}
	}()
	if err := WriteRequest(input, request); err != nil {
		return Response{}, operationError(operationCtx, ErrOperationFailed)
	}
	prompts := 0
	for {
		kind, payload, err := ReadFrame(output, MaxResponseBytes)
		if err != nil {
			return Response{}, operationError(operationCtx, ErrProtocol)
		}
		if kind == FramePINChallenge {
			var challenge PINChallenge
			err := decodeJSON(payload, &challenge)
			clear(payload)
			prompts++
			if err != nil || len(payload) > 256 || request.Operation != "sign" || prompts > 2 ||
				challenge.Version != ProtocolVersion || challenge.ID != request.ID {
				return Response{}, ErrProtocol
			}
			if err := c.replyToPINChallenge(operationCtx, input, challenge); err != nil {
				return Response{}, operationError(operationCtx, err)
			}
			continue
		}
		var response Response
		err = decodeJSON(payload, &response)
		clear(payload)
		if kind != FrameResponse || err != nil || validateResponse(request, response) != nil {
			return Response{}, ErrProtocol
		}
		_ = input.Close()
		// Require exactly one final response and EOF, not a forged early success
		// followed by a second response, extra output, crash or hung Finalize.
		var extra [1]byte
		if n, err := output.Read(extra[:]); n != 0 || err != io.EOF {
			return Response{}, operationError(operationCtx, ErrProtocol)
		}
		err = command.Wait()
		waited = true
		if err != nil {
			return Response{}, operationError(operationCtx, ErrOperationFailed)
		}
		if err := operationCtx.Err(); err != nil {
			return Response{}, err
		}
		if response.Code != "ok" {
			if response.Code == "pin_memory_unavailable" {
				return Response{}, ErrPINMemoryUnavailable
			}
			return Response{}, &OperationError{Code: response.Code}
		}
		return response, nil
	}
}

func (c Client) replyToPINChallenge(ctx context.Context, input io.Writer, challenge PINChallenge) error {
	if c.PINSource == nil {
		return ErrPINCancelled
	}
	// Reserve the transport region before asking the user for a secret. Never
	// silently fall back to an unlocked buffer if the OS denies the reservation.
	buffer, release, err := newPINReplyBuffer(challenge.ProtectedAuthenticationPath, c.pinBuffer)
	if err != nil {
		return err
	}
	defer release()
	pin, err := requestPIN(ctx, c.PINSource, PINMode{
		ProtectedAuthenticationPath: challenge.ProtectedAuthenticationPath,
		ContextSpecific:             challenge.ContextSpecific,
	})
	defer clear(pin)
	if err != nil {
		// No cancellation reply: the caller terminates the helper immediately.
		// Only fixed local errors reach the UI, never arbitrary source text.
		for _, fixed := range []error{ErrPINUnavailable, ErrPINPromptFailed, ErrPINMemoryUnavailable, context.DeadlineExceeded} {
			if errors.Is(err, fixed) {
				return fixed
			}
		}
		return ErrPINCancelled
	}
	if len(pin) > MaxPINBytes || (challenge.ProtectedAuthenticationPath && len(pin) != 0) {
		return ErrProtocol
	}
	if err := writePINReplyInto(input, pin, false, buffer); err != nil {
		return ErrOperationFailed
	}
	return nil
}

func operationError(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fallback
}

// Unbuffered transfer makes ownership explicit when a prompt finishes after
// cancellation. The abandoned result is cleared by the prompt goroutine.
func requestPIN(ctx context.Context, source PINSource, mode PINMode) ([]byte, error) {
	if source == nil {
		return nil, ErrPINCancelled
	}
	type result struct {
		pin []byte
		err error
	}
	completed := make(chan result)
	go func() {
		var value result
		defer func() {
			if recover() != nil {
				value.err = ErrPINCancelled
			}
			select {
			case completed <- value:
			case <-ctx.Done():
				clear(value.pin)
			}
		}()
		value.pin, value.err = source(ctx, mode)
	}()
	select {
	case value := <-completed:
		if ctx.Err() != nil {
			clear(value.pin)
			return nil, ctx.Err()
		}
		return value.pin, value.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func validateResponse(request Request, response Response) error {
	if response.Version != ProtocolVersion || response.ID != request.ID {
		return ErrProtocol
	}
	if response.Code != "ok" {
		if len(response.Certificates) != 0 || len(response.ChainDER) != 0 || len(response.Signature) != 0 {
			return ErrProtocol
		}
		switch response.Code {
		case "cancelled", "pin_cancelled", "invalid_request", "driver_unavailable", "size_limit", "driver_failed",
			"certificate_not_found", "key_unavailable", "unsuitable_certificate", "pin_incorrect", "pin_locked",
			"pin_expired", "device_removed", "unsupported_algorithm", "pin_memory_unavailable":
			return nil
		default:
			return ErrProtocol
		}
	}
	switch request.Operation {
	case "list":
		if len(response.Certificates) > MaxCertificates || len(response.ChainDER) != 0 || len(response.Signature) != 0 {
			return ErrProtocol
		}
		seen := make(map[string]bool)
		for _, entry := range response.Certificates {
			ref := entry.Certificate
			if entry.HasSigningKey && entry.SigningKeyNeedsUnlock {
				return ErrProtocol
			}
			if len(ref.ID) == 0 || len(ref.ID) > 2048 || seen[ref.ID] || len(ref.Fingerprint) != 64 || !isHex(ref.Fingerprint) ||
				len(ref.Subject) > 4096 || len(ref.Issuer) > 4096 || len(ref.Organizacion) > 4096 || len(ref.NIF) > 512 || len(ref.Tipo) > 128 {
				return ErrProtocol
			}
			seen[ref.ID] = true
		}
	case "describe":
		if len(response.Certificates) != 0 || len(response.Signature) != 0 || len(response.ChainDER) == 0 || len(response.ChainDER) > MaxChainCertificates {
			return ErrProtocol
		}
		for _, der := range response.ChainDER {
			if len(der) == 0 || len(der) > MaxCertificateBytes {
				return ErrProtocol
			}
			if _, err := x509.ParseCertificate(der); err != nil {
				return ErrProtocol
			}
		}
		fingerprint := sha256.Sum256(response.ChainDER[0])
		if !strings.EqualFold(hex.EncodeToString(fingerprint[:]), request.Fingerprint) {
			return ErrProtocol
		}
	case "sign":
		if len(response.Certificates) != 0 || len(response.ChainDER) != 0 || len(response.Signature) == 0 || len(response.Signature) > MaxSignatureBytes {
			return ErrProtocol
		}
	default:
		return ErrProtocol
	}
	return nil
}
