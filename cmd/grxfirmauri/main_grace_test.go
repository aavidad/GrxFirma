// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
)

func TestSuccessGracePeriod_LocalhostSinOverride(t *testing.T) {
	t.Setenv("GRXFIRMA_SUCCESS_GRACE_MS", "")
	solicitud := afirmauri.Solicitud{
		Sesion: domain.ExchangeSession{
			UploadEndpoint: "http://127.0.0.1:41097/StorageService",
		},
	}

	got := successGracePeriod("afirma://batch?id=1&stservlet=http%3A%2F%2F127.0.0.1%3A41097%2FStorageService", solicitud)
	if got != 5*time.Second {
		t.Fatalf("gracia localhost = %s, want %s", got, 5*time.Second)
	}
}

func TestSuccessGracePeriod_RemotoSinOverride(t *testing.T) {
	t.Setenv("GRXFIRMA_SUCCESS_GRACE_MS", "")
	solicitud := afirmauri.Solicitud{
		Sesion: domain.ExchangeSession{
			UploadEndpoint: "https://demos.guadaltel.es/pfirmav3/afirma/StorageService",
		},
	}

	got := successGracePeriod("afirma://batch?id=1&stservlet=https%3A%2F%2Fdemos.guadaltel.es%2Fpfirmav3%2Fafirma%2FStorageService", solicitud)
	if got != 15*time.Second {
		t.Fatalf("gracia remota = %s, want %s", got, 15*time.Second)
	}
}

func TestSuccessGracePeriod_OverrideValido(t *testing.T) {
	t.Setenv("GRXFIRMA_SUCCESS_GRACE_MS", "1200")
	got := successGracePeriod("afirma://batch?id=1", afirmauri.Solicitud{})
	if got != 1200*time.Millisecond {
		t.Fatalf("gracia override = %s, want %s", got, 1200*time.Millisecond)
	}
}
