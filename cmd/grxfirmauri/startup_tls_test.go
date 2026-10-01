// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPrepareStartupLocalTLSTrustReportsOnlyChanges(t *testing.T) {
	previousEnsure := ensureStartupLocalTLSTrust
	previousPresent := presentStartupTLSTrustFailure
	previousNotice := getStartupTLSTrustNotice()
	t.Cleanup(func() {
		ensureStartupLocalTLSTrust = previousEnsure
		presentStartupTLSTrustFailure = previousPresent
		setStartupTLSTrustNotice(previousNotice)
	})

	var called int
	ensureStartupLocalTLSTrust = func(context.Context, string) (bool, error) {
		called++
		return called == 1, nil
	}
	presentStartupTLSTrustFailure = func(string) { t.Fatal("unexpected failure UI") }
	setStartupTLSTrustNotice("")
	var first bytes.Buffer
	if err := prepareStartupLocalTLSTrust(context.Background(), t.TempDir(), &first); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.String(), "Firefox") || getStartupTLSTrustNotice() == "" {
		t.Fatalf("first launch did not explain Firefox restart: %q", first.String())
	}
	setStartupTLSTrustNotice("")
	var second bytes.Buffer
	if err := prepareStartupLocalTLSTrust(context.Background(), t.TempDir(), &second); err != nil {
		t.Fatal(err)
	}
	if second.Len() != 0 || getStartupTLSTrustNotice() != "" {
		t.Fatalf("idempotent launch should not repeat notice: %q", second.String())
	}
}

func TestPrepareStartupLocalTLSTrustExplainsFailure(t *testing.T) {
	previousEnsure := ensureStartupLocalTLSTrust
	previousPresent := presentStartupTLSTrustFailure
	previousNotice := getStartupTLSTrustNotice()
	t.Cleanup(func() {
		ensureStartupLocalTLSTrust = previousEnsure
		presentStartupTLSTrustFailure = previousPresent
		setStartupTLSTrustNotice(previousNotice)
	})
	ensureStartupLocalTLSTrust = func(context.Context, string) (bool, error) {
		return true, errors.New("SEC_ERROR_BUSY")
	}
	var displayed string
	presentStartupTLSTrustFailure = func(message string) { displayed = message }
	setStartupTLSTrustNotice("")
	var stderr bytes.Buffer
	if err := prepareStartupLocalTLSTrust(context.Background(), t.TempDir(), &stderr); err == nil {
		t.Fatal("expected trust installation error")
	}
	if !strings.Contains(displayed, "Firefox") || !strings.Contains(stderr.String(), "SEC_ERROR_BUSY") {
		t.Fatalf("missing actionable UI or technical error: displayed=%q stderr=%q", displayed, stderr.String())
	}
	if !strings.Contains(detailWithStartupTLSTrustNotice("estado"), displayed) {
		t.Fatal("failure notice not retained in the app state")
	}
}

func TestProtocolEntryPreparesTrustBeforeHandler(t *testing.T) {
	previousEnsure := ensureStartupLocalTLSTrust
	previousPresent := presentStartupTLSTrustFailure
	previousNotice := getStartupTLSTrustNotice()
	t.Cleanup(func() {
		ensureStartupLocalTLSTrust = previousEnsure
		presentStartupTLSTrustFailure = previousPresent
		setStartupTLSTrustNotice(previousNotice)
	})
	setTestUserHome(t, t.TempDir())
	prepared := false
	ensureStartupLocalTLSTrust = func(context.Context, string) (bool, error) {
		prepared = true
		return false, nil
	}
	presentStartupTLSTrustFailure = func(string) { t.Fatal("unexpected failure UI") }
	var stderr bytes.Buffer
	code := runConDependencias(context.Background(), []string{"afirma://sign?op=sign"}, &stderr,
		func(context.Context, io.Writer, string) int {
			if !prepared {
				t.Fatal("handler ran before per-user TLS trust reconciliation")
			}
			return 0
		})
	if code != 0 {
		t.Fatalf("protocol launch failed: code=%d stderr=%q", code, stderr.String())
	}
}

func TestProtocolEntryStopsWhenTrustCannotBeInstalled(t *testing.T) {
	previousEnsure := ensureStartupLocalTLSTrust
	previousPresent := presentStartupTLSTrustFailure
	previousNotice := getStartupTLSTrustNotice()
	t.Cleanup(func() {
		ensureStartupLocalTLSTrust = previousEnsure
		presentStartupTLSTrustFailure = previousPresent
		setStartupTLSTrustNotice(previousNotice)
	})
	setTestUserHome(t, t.TempDir())
	ensureStartupLocalTLSTrust = func(context.Context, string) (bool, error) {
		return false, errors.New("NSS DB busy")
	}
	var displayed string
	presentStartupTLSTrustFailure = func(message string) { displayed = message }
	called := false
	var stderr bytes.Buffer
	code := runConDependencias(context.Background(), []string{"afirma://sign?op=sign"}, &stderr,
		func(context.Context, io.Writer, string) int {
			called = true
			return 0
		})
	if code != 1 || called || !strings.Contains(displayed, "Firefox") || !strings.Contains(stderr.String(), "NSS DB busy") {
		t.Fatalf("trust failure was not reported and stopped: code=%d called=%v displayed=%q stderr=%q", code, called, displayed, stderr.String())
	}
}
