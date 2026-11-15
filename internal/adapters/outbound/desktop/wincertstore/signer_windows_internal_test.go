// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package wincertstore

import (
	"errors"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
)

func TestAdquirirClavePreferida_PrefiereCNGConFallbackCAPISinCambiarPropiedad(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return true }))
	for _, spec := range []uint32{windows.CERT_NCRYPT_KEY_SPEC, windows.AT_SIGNATURE, windows.AT_KEYEXCHANGE} {
		for _, owned := range []bool{false, true} {
			want := claveWindowsAdquirida{manejador: windows.Handle(123), especificacion: spec, liberarManejador: owned}
			calls := 0
			got, err := adquirirClavePreferida(func(flags uint32) (claveWindowsAdquirida, error) {
				calls++
				if flags != windows.CRYPT_ACQUIRE_PREFER_NCRYPT_KEY_FLAG|windows.CRYPT_ACQUIRE_COMPARE_KEY_FLAG {
					t.Fatalf("flags = %#x; falta prioridad CNG con comprobacion de clave", flags)
				}
				return want, nil
			})
			if err != nil || got != want || calls != 1 {
				t.Fatalf("adquisicion = %+v, %v, calls=%d; esperada %+v", got, err, calls, want)
			}
		}
	}
}

// Sin escritorio la clave se pide en silencio: un proveedor que necesite PIN
// o confirmación falla en vez de abrir una ventana que nadie ve.
func TestAdquirirClavePreferida_SinEscritorioPideSilencio(t *testing.T) {
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
	var recibidas uint32
	_, err := adquirirClavePreferida(func(flags uint32) (claveWindowsAdquirida, error) {
		recibidas = flags
		return claveWindowsAdquirida{}, syscall.Errno(nteSilentContext)
	})
	if recibidas&windows.CRYPT_ACQUIRE_SILENT_FLAG == 0 {
		t.Fatalf("flags = %#x; sin escritorio falta CRYPT_ACQUIRE_SILENT_FLAG", recibidas)
	}
	if !errors.Is(err, escritorio.ErrSinEscritorio) || !errors.Is(err, syscall.Errno(nteSilentContext)) {
		t.Fatalf("error sin escritorio = %v", err)
	}
}

func TestAdquirirClavePreferida_NoReintentaErroresNiAdmiteHandleNulo(t *testing.T) {
	wantErr := errors.New("proveedor no disponible")
	calls := 0
	got, err := adquirirClavePreferida(func(uint32) (claveWindowsAdquirida, error) {
		calls++
		return claveWindowsAdquirida{}, wantErr
	})
	if !errors.Is(err, wantErr) || got != (claveWindowsAdquirida{}) || calls != 1 {
		t.Fatalf("resultado de error = %+v, %v, calls=%d", got, err, calls)
	}
	got, err = adquirirClavePreferida(func(uint32) (claveWindowsAdquirida, error) {
		return claveWindowsAdquirida{}, nil
	})
	if err == nil || got != (claveWindowsAdquirida{}) {
		t.Fatal("se admitio un handle nulo")
	}
}

func TestNormalizarEstadoNCrypt_ConservaLos32BitsDeLaABI(t *testing.T) {
	t.Parallel()

	estado := uintptr(0xc0000001)
	if ^uintptr(0) > mascaraEstadoNCrypt {
		estado |= uintptr(0x12345678) << 32
	}

	if obtenido := normalizarEstadoNCrypt(estado); obtenido != uintptr(0xc0000001) {
		t.Fatalf("normalizarEstadoNCrypt() = 0x%x, want 0xc0000001", obtenido)
	}
}
