// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenpin

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/domain"
)

func TestMain(m *testing.M) {
	if scenario := os.Getenv("GRXFIRMA_TEST_PINENTRY"); scenario != "" {
		if len(os.Args) != 1 {
			os.Exit(31)
		}
		if scenario == "hang" {
			time.Sleep(time.Minute)
			os.Exit(32)
		}
		fmt.Fprintln(os.Stdout, "OK synthetic helper")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			switch scanner.Text() {
			case "GETPIN":
				if scenario == "cancel" {
					fmt.Fprintln(os.Stdout, "ERR 99 cancelled")
					continue
				}
				fmt.Fprintln(os.Stderr, "synthetic-secret-not-for-logs")
				fmt.Fprintln(os.Stdout, "D 1234%25\nOK")
			case "BYE":
				fmt.Fprintln(os.Stdout, "OK bye")
				if scenario == "crash" {
					os.Exit(3)
				}
				if scenario == "hang_after_bye" {
					_ = os.Stdout.Close()
					time.Sleep(time.Minute)
					os.Exit(33)
				}
				os.Exit(0)
			default:
				fmt.Fprintln(os.Stdout, "OK")
			}
		}
		os.Exit(34)
	}
	os.Exit(m.Run())
}

func TestRunDialogProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		scenario  string
		protected bool
		want      error
	}{
		{"success", false, nil}, {"success", true, nil}, {"cancel", false, pkcs11worker.ErrPINCancelled}, {"crash", false, ErrFailed}, {"hang", false, context.DeadlineExceeded}, {"hang_after_bye", false, context.DeadlineExceeded},
	} {
		t.Run(fmt.Sprint(tc.scenario, tc.protected), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
			defer cancel()
			started := time.Now()
			var owner *secmem.Blob
			var memory []byte
			pin, err := runDialogWithAllocator(ctx, executable, []string{"GRXFIRMA_TEST_PINENTRY=" + tc.scenario, "GORACE=atexit_sleep_ms=0"}, []string{"SETTITLE Synthetic QA"}, tc.protected, func(size int) (*secmem.Blob, error) {
				if tc.protected {
					t.Fatal("protected dialog requested locked memory")
				}
				var err error
				owner, err = secmem.NewLockedSize(size)
				if owner != nil {
					memory = owner.Bytes()
				}
				return owner, err
			})
			defer clear(pin)
			if owner != nil && (owner.Locked() || !bytes.Equal(memory, make([]byte, len(memory)))) {
				t.Fatal("process path retained owned PIN memory")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v", err)
			}
			if time.Since(started) > 3*time.Second {
				t.Fatal("unbounded process")
			}
			if err != nil && pin != nil {
				t.Fatal("PIN retained after failure")
			}
			if err == nil && !tc.protected && string(pin) != "1234%" {
				t.Fatal("wrong synthetic PIN")
			}
			if tc.protected && len(pin) != 0 {
				t.Fatal("pinpad returned data")
			}
		})
	}
	if _, err := runDialog(context.Background(), "/no-such-pinentry", nil, nil, false); !errors.Is(err, ErrUnavailable) {
		t.Fatal("startup error not sanitized")
	}
}

func TestRunDialogMemoryFailurePrecedesProcessAndPrompt(t *testing.T) {
	calls := 0
	pin, err := runDialogWithAllocator(context.Background(), "/no-such-pinentry", nil, []string{"SETTITLE Synthetic QA"}, false, func(size int) (*secmem.Blob, error) {
		calls++
		if size != pinMemorySize {
			t.Fatal("unexpected reservation size")
		}
		return nil, secmem.ErrLockUnavailable
	})
	if calls != 1 || pin != nil || !errors.Is(err, pkcs11worker.ErrPINMemoryUnavailable) {
		t.Fatalf("memory failure did not precede process startup: %v", err)
	}
}

func TestEnvironmentAndHeadless(t *testing.T) {
	env := dialogEnvironment([]string{"LD_PRELOAD=evil", "GRXFIRMA_PASSWORD=secret", "HOME=/personal", "INSIDE_EMACS=yes", "QT_PLUGIN_PATH=/evil", "GTK_MODULES=evil", "DISPLAY=:77", "DISPLAY=:88", "WAYLAND_DISPLAY=wayland-0", "XAUTHORITY=/synthetic/auth", "QT_QPA_PLATFORM=evil:/tmp", "LC_ALL=C.UTF-8"})
	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{"evil", "secret", "personal", "EMACS", ":88"} {
		if strings.Contains(joined, forbidden) {
			t.Fatal("inherited unsafe environment")
		}
	}
	if !strings.Contains(joined, "DISPLAY=:77") || !strings.Contains(joined, "LC_ALL=C.UTF-8") {
		t.Fatal("missing graphical environment")
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	pin, err := New("es").Request(context.Background(), domain.CertificateRef{Fingerprint: strings.Repeat("a", 64)}, pkcs11worker.PINMode{})
	if !errors.Is(err, ErrUnavailable) || pin != nil {
		t.Fatal("headless starts UI")
	}
}
