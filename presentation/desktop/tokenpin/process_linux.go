// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenpin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
)

func requestLocal(ctx context.Context, locale, fingerprint string, mode pkcs11worker.PINMode) ([]byte, error) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, ErrUnavailable
	}
	// Fixed system paths: no PATH lookup, shell, gpg-agent or browser-supplied
	// executable. Curses fallback is deliberately not enabled without a TTY UI.
	for _, candidate := range []string{"/usr/bin/pinentry-qt", "/usr/bin/pinentry-gnome3", "/usr/bin/pinentry-gtk-2"} {
		path, err := pkcs11worker.ValidateModulePath(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0111 == 0 {
			continue
		}
		return runDialog(ctx, path, dialogEnvironment(os.Environ()), dialogCommands(locale, fingerprint, mode), mode.ProtectedAuthenticationPath)
	}
	return nil, ErrUnavailable
}

func dialogEnvironment(environ []string) []string {
	result := []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	allowed := map[string]bool{"DISPLAY": true, "WAYLAND_DISPLAY": true, "XAUTHORITY": true, "XDG_RUNTIME_DIR": true, "DBUS_SESSION_BUS_ADDRESS": true, "LANGUAGE": true, "LC_ALL": true, "LC_CTYPE": true, "QT_QPA_PLATFORM": true}
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if ok && allowed[name] && len(value) <= 4096 && !strings.ContainsRune(value, 0) {
			// Do not accept a Qt plugin path or custom platform name.
			if name == "QT_QPA_PLATFORM" && value != "xcb" && value != "wayland" {
				continue
			}
			result = append(result, entry)
			delete(allowed, name)
		}
	}
	return result
}

func runDialog(ctx context.Context, executable string, environ, commands []string, protected bool) (pin []byte, err error) {
	return runDialogWithAllocator(ctx, executable, environ, commands, protected, nil)
}

func runDialogWithAllocator(ctx context.Context, executable string, environ, commands []string, protected bool, allocate pinMemoryAllocator) (pin []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Fail before starting a dialog or requesting a secret. The owner remains
	// live through process termination; callback/toolkit buffers are not ours.
	r, err := newProtocolReader(nil, protected, allocate)
	if err != nil {
		return nil, err
	}
	defer r.close()
	operationCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(operationCtx, executable)
	cmd.Env, cmd.Dir = environ, "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	defer output.Close()
	// nil Stderr goes to /dev/null; no dialog text or secret enters app logs.
	if err := cmd.Start(); err != nil {
		return nil, ErrUnavailable
	}
	stopIO := context.AfterFunc(operationCtx, func() { _ = input.Close(); _ = output.Close() })
	defer stopIO()
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Cancel()
			_ = input.Close()
			_ = output.Close()
			_ = cmd.Wait()
		}
		if operationCtx.Err() != nil {
			err = operationCtx.Err()
		}
		if err != nil {
			clear(pin)
			pin = nil
		}
	}()
	r.input = output
	pin, err = r.exchange(input, commands, protected)
	if err != nil {
		return pin, err
	}
	_ = input.Close()
	err = cmd.Wait()
	waited = true
	if err != nil {
		return pin, ErrFailed
	}
	return append([]byte(nil), pin...), nil
}
