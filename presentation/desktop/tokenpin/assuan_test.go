// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenpin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
)

func TestResponse(t *testing.T) {
	for _, tc := range []struct {
		name, wire, want string
		pin              bool
		err              error
	}{
		{"ok", "OK greeting\n", "", false, nil},
		{"pin", "D 1234\nOK\n", "1234", true, nil},
		{"escape", "D a%25%0A%C3%B1\nOK\n", "a%\nñ", true, nil},
		{"split", "D 12\nD 34\nOK\n", "1234", true, nil},
		{"empty", "D\nOK\n", "", true, nil},
		{"crlf", "D 1234\r\nOK\r\n", "1234", true, nil},
		{"cancel", "D 1234\nERR 83886179 arbitrary-secret\n", "", true, pkcs11worker.ErrPINCancelled},
		{"timeout", "S ERROR qt.getpin 83886142 synthetic\nERR 83886142 timeout\n", "", true, context.DeadlineExceeded},
		{"full_cancel", "S ERROR qt.getpin 83886278 synthetic\nERR 83886278 cancelled\n", "", true, pkcs11worker.ErrPINCancelled},
		{"status_then_success", "S ERROR qt.getpin 62 synthetic\nOK\n", "", true, ErrProtocol},
		{"status_then_data", "S ERROR qt.getpin 62 synthetic\nD 1234\nERR 62 timeout\n", "", true, ErrProtocol},
		{"not_confirmed", "ERR 114 no\n", "", false, pkcs11worker.ErrPINCancelled},
		{"assuan_cancel", "ERR 277 no\n", "", false, pkcs11worker.ErrPINCancelled},
		{"failure", "ERR 1 arbitrary-secret\n", "", true, ErrFailed},
		{"cache", "S PASSWORD_FROM_CACHE\nD 1234\nOK\n", "", true, ErrProtocol},
		{"inquire", "INQUIRE QUALITY 1234\n", "", true, ErrProtocol},
		{"data_outside_pin", "D 1234\nOK\n", "", false, ErrProtocol},
		{"invalid_escape", "D 1234%xx\nOK\n", "", true, ErrProtocol},
		{"short_escape", "D 1234%0\nOK\n", "", true, ErrProtocol},
		{"max_pin", "D " + strings.Repeat("1", pkcs11worker.MaxPINBytes) + "\nOK\n", strings.Repeat("1", pkcs11worker.MaxPINBytes), true, nil},
		{"oversize_pin", "D " + strings.Repeat("1", pkcs11worker.MaxPINBytes+1) + "\nOK\n", "", true, ErrProtocol},
		{"oversize_line", strings.Repeat("X", maxLine) + "\n", "", true, ErrProtocol},
		{"too_many_status", strings.Repeat("# status\n", 64) + "OK\n", "", true, ErrProtocol},
		{"truncated", "D 1234\n", "", true, ErrProtocol},
		{"invalid_error", "ERR -1 x\n", "", true, ErrProtocol},
		{"overflow_error", "ERR 4294967395 x\n", "", true, ErrProtocol},
		{"empty_error", "ERR  x\n", "", true, ErrProtocol},
		{"bad_error", "ERR 99x x\n", "", true, ErrProtocol},
		{"empty", "", "", true, ErrProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := newProtocolReader(strings.NewReader(tc.wire), !tc.pin, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer r.close()
			pin, err := r.response(tc.pin)
			defer clear(pin)
			if !errors.Is(err, tc.err) || string(pin) != tc.want {
				t.Fatalf("unexpected response: err=%v, length=%d", err, len(pin))
			}
			if err != nil && pin != nil {
				t.Fatal("error retains PIN")
			}
			if !bytes.Equal(r.line[:], make([]byte, len(r.line))) {
				t.Fatal("line not erased")
			}
			r.close()
			if !bytes.Equal(r.buffer[:], make([]byte, len(r.buffer))) {
				t.Fatal("receive buffer not erased")
			}
		})
	}
}

func TestExchange(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		protected   bool
		wantErr     error
	}{
		{"pin", "OK greeting\nOK\nD 1234\nOK\nOK bye\n", false, nil},
		{"pinpad", "OK greeting\nOK\nOK\nOK bye\n", true, nil},
		{"pinpad_cannot_return_pin", "OK\nOK\nD 1234\nOK\nOK\n", true, ErrProtocol},
		{"bye_failure", "OK\nOK\nD 1234\nOK\nERR 1 error\n", false, ErrFailed},
		{"extra", "OK\nOK\nD 1234\nOK\nOK\nD extra\n", false, ErrProtocol},
		{"early_data", "OK\nD 1234\nOK\n", false, ErrProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input bytes.Buffer
			pin, err := exchange(&input, strings.NewReader(tc.reply), []string{"SETTITLE Test"}, tc.protected)
			defer clear(pin)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
			if err != nil && pin != nil {
				t.Fatal("PIN retained on error")
			}
			if err == nil {
				command := "GETPIN"
				if tc.protected {
					command = "CONFIRM"
					if len(pin) != 0 {
						t.Fatal("pinpad produced PIN")
					}
				} else if string(pin) != "1234" {
					t.Fatal("unexpected PIN")
				}
				if input.String() != "SETTITLE Test\n"+command+"\nBYE\n" {
					t.Fatal("unexpected command sequence")
				}
			}
			if strings.Contains(input.String(), "1234") {
				t.Fatal("PIN echoed")
			}
		})
	}
}

func TestDialogLabelsAndValidation(t *testing.T) {
	for _, locale := range []string{"es", "en-GB"} {
		for _, mode := range []pkcs11worker.PINMode{{}, {ContextSpecific: true}, {ProtectedAuthenticationPath: true}} {
			commands := dialogCommands(locale, strings.Repeat("A", 64), mode)
			joined := strings.Join(commands, "\n")
			if len(commands) != 6 || !strings.Contains(joined, "SETCANCEL ") || strings.Contains(joined, "allow-external-password-cache") || strings.Contains(joined, "SETKEYINFO") {
				t.Fatal("dialog policy")
			}
			if !strings.Contains(joined, "SHA-256: "+strings.Repeat("A", 64)) {
				t.Fatal("missing bound fingerprint")
			}
			if mode.ProtectedAuthenticationPath && !strings.Contains(joined, "teclado") && !strings.Contains(joined, "keypad") {
				t.Fatal("missing pinpad instructions")
			}
		}
	}
	if escapeText("x%\nGETPIN\r") != "x%25%0AGETPIN%0D" {
		t.Fatal("text not escaped")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New("es").Request(ctx, domain.CertificateRef{}, pkcs11worker.PINMode{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
	for _, fp := range []string{"", strings.Repeat("x", 64), strings.Repeat("a", 63) + "\n"} {
		if _, err := New("es").Request(context.Background(), domain.CertificateRef{Fingerprint: fp}, pkcs11worker.PINMode{}); !errors.Is(err, ErrProtocol) {
			t.Fatal("invalid fingerprint accepted")
		}
	}
}
