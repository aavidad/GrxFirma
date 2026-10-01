// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desksigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
	"software.sslmate.com/src/go-pkcs12"
)

type credentialSelectorFunc func(context.Context, []domain.CertificateRef) (certpicker.ResultadoSeleccion, error)

func (f credentialSelectorFunc) Select(ctx context.Context, refs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
	return f(ctx, refs)
}

func TestWebCredential_CargarDesdeCatalogoVacioUnicoCaducadoOMultipleYFirmar(t *testing.T) {
	for _, initial := range []string{"vacio", "unico", "caducado", "multiple"} {
		t.Run(initial, func(t *testing.T) {
			ctx := context.Background()
			var refs []domain.CertificateRef
			if initial != "vacio" {
				refs = append(refs, domain.CertificateRef{ID: "anterior", Subject: "Certificado anterior", Fingerprint: "anterior", NotAfter: time.Now().Add(time.Hour)})
			}
			if initial == "caducado" {
				refs[0].NotAfter = time.Now().Add(-time.Hour)
			}
			if initial == "multiple" {
				refs = append(refs, domain.CertificateRef{ID: "segundo", Fingerprint: "segundo", NotAfter: time.Now().Add(time.Hour)})
			}
			data, cert := generarP12ConCert(t, "Credencial temporal QA", "clave-prueba")
			password := []byte("clave-prueba")
			calls := 0
			picker := credentialSelectorFunc(func(_ context.Context, shown []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				calls++
				if calls == 1 {
					return certpicker.ResultadoSeleccion{}, certpicker.ErrCargarCertificado
				}
				for _, ref := range shown {
					if ref.Subject == "Credencial temporal QA" {
						return certpicker.ResultadoSeleccion{Certificado: ref}, nil
					}
				}
				t.Fatal("la credencial cargada no aparece en el selector")
				return certpicker.ResultadoSeleccion{}, certpicker.ErrSeleccionCancelada
			})
			selector, catalogo, claves := nuevoSelectorCredenciales(picker, certificateCatalogStub{certs: refs}, keyProviderStub{err: errors.New("sin clave anterior")}, func(context.Context) ([]byte, []byte, error) { return data, password, nil }, nil)
			t.Cleanup(selector.ClearCredentials)
			handler := &legacyWebSocketHandler{catalogo: catalogo, keys: claves, selector: selector}
			// Una instantánea previa no debe ocultar la recuperación ni la carga.
			handler.warmCertificates(ctx)
			ref, der, err := handler.selectCertificate(ctx, afirmauri.Solicitud{Operacion: afirmauri.OperacionFirma})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || !bytes.Equal(der, cert.Raw) {
				t.Fatalf("selección inesperada: llamadas=%d", calls)
			}
			if !bytes.Equal(data, make([]byte, len(data))) || !bytes.Equal(password, make([]byte, len(password))) {
				t.Fatal("buffers de credencial no borrados")
			}
			key, err := claves.KeyFor(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			doc, _ := domain.NewDocument("prueba.txt", []byte("prueba sintética del fichero cargado desde la sede"), "text/plain")
			result, err := desksigner.NuevoMotorFirmaGo(relojReal{}).Sign(ctx, domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}, key)
			if err != nil {
				t.Fatal(err)
			}
			verification, _, err := commonsigner.NewCAdESVerifier().VerifyDetachedCMS(ctx, result.Data, doc.Content)
			if err != nil || verification.Integrity.Status != domain.VerificationStatusValid {
				t.Fatalf("firma temporal no válida: %v %+v", err, verification)
			}
		})
	}
}

func TestWebCredential_PasswordIncorrectaPermiteReintentoSinExponerSecretos(t *testing.T) {
	data := generarP12(t, "QA reintento", "correcta")
	defer clear(data)
	loads, choices := 0, 0
	var delivered [][]byte
	var notices []string
	selector, _, _ := nuevoSelectorCredenciales(credentialSelectorFunc(func(_ context.Context, refs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
		choices++
		if choices <= 2 {
			return certpicker.ResultadoSeleccion{}, certpicker.ErrCargarCertificado
		}
		if len(refs) != 1 {
			t.Fatalf("catálogo tras reintento: %d", len(refs))
		}
		return certpicker.ResultadoSeleccion{Certificado: refs[0]}, nil
	}), certificateCatalogStub{}, keyProviderStub{err: errors.New("sin clave")}, func(context.Context) ([]byte, []byte, error) {
		loads++
		password := []byte("secreto-incorrecto-que-no-debe-salir")
		if loads == 2 {
			password = []byte("correcta")
		}
		copyData := bytes.Clone(data)
		delivered = append(delivered, copyData, password)
		return copyData, password, nil
	}, func(_ context.Context, title, detail string) { notices = append(notices, title+detail) })
	t.Cleanup(selector.ClearCredentials)
	if _, err := selector.Select(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if loads != 2 || len(notices) != 1 || strings.Contains(notices[0], "secreto-incorrecto") {
		t.Fatalf("reintento/aviso inesperado: loads=%d notices=%v", loads, notices)
	}
	for _, buffer := range delivered {
		if !bytes.Equal(buffer, make([]byte, len(buffer))) {
			t.Fatal("buffer no borrado")
		}
	}
}

func TestWebCredential_CancelarCargaRegresaASelectorYCancelarSesionTermina(t *testing.T) {
	choices := 0
	buffer := []byte("descartar")
	selector, _, _ := nuevoSelectorCredenciales(credentialSelectorFunc(func(context.Context, []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
		choices++
		if choices == 1 {
			return certpicker.ResultadoSeleccion{}, certpicker.ErrCargarCertificado
		}
		return certpicker.ResultadoSeleccion{}, certpicker.ErrSeleccionCancelada
	}), certificateCatalogStub{}, keyProviderStub{}, func(context.Context) ([]byte, []byte, error) { return buffer, nil, certpicker.ErrSeleccionCancelada }, func(context.Context, string, string) { t.Fatal("cancelar carga no debe mostrarse como error") })
	_, err := selector.Select(context.Background(), nil)
	if !errors.Is(err, certpicker.ErrSeleccionCancelada) || choices != 2 {
		t.Fatalf("cancelación inesperada: %v choices=%d", err, choices)
	}
	if !bytes.Equal(buffer, make([]byte, len(buffer))) {
		t.Fatal("buffer cancelado no borrado")
	}
}

func TestWebCredential_RechazaCredencialCaducadaFuturaYUsoSinFirma(t *testing.T) {
	for _, name := range []string{"caducada", "futura", "sin_firma"} {
		t.Run(name, func(t *testing.T) {
			priv, cert := generarMaterialCertificado(t, "QA inválida")
			switch name {
			case "caducada":
				cert.NotAfter = time.Now().Add(-time.Minute)
			case "futura":
				cert.NotBefore = time.Now().Add(time.Minute)
			case "sin_firma":
				cert.KeyUsage = x509.KeyUsageKeyEncipherment
			}
			der, err := x509.CreateCertificate(rand.Reader, cert, cert, priv.Public(), priv)
			if err != nil {
				t.Fatal(err)
			}
			cert, err = x509.ParseCertificate(der)
			if err != nil {
				t.Fatal(err)
			}
			data, err := pkcs12.Modern.Encode(priv, cert, nil, "qa")
			if err != nil {
				t.Fatal(err)
			}
			defer clear(data)
			selector, _, _ := nuevoSelectorCredenciales(nil, certificateCatalogStub{}, keyProviderStub{}, nil, nil)
			if err := selector.cargar(context.Background(), data, []byte("qa")); err == nil {
				t.Fatal("aceptada credencial no apta para firma")
			}
			refs, _ := selector.temporal.List(context.Background())
			if len(refs) != 0 {
				t.Fatal("credencial rechazada quedó en memoria")
			}
		})
	}
}

func TestWebCredential_ActualizarConsultaCatalogoOtraVez(t *testing.T) {
	data := generarP12(t, "QA refresh", "qa")
	defer clear(data)
	selector, _, _ := nuevoSelectorCredenciales(nil, certificateCatalogStub{}, keyProviderStub{}, nil, nil)
	t.Cleanup(selector.ClearCredentials)
	calls := 0
	selector.selector = credentialSelectorFunc(func(_ context.Context, refs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
		calls++
		if calls == 1 {
			if err := selector.cargar(context.Background(), data, []byte("qa")); err != nil {
				t.Fatal(err)
			}
			return certpicker.ResultadoSeleccion{}, certpicker.ErrActualizarCertificados
		}
		if len(refs) != 1 {
			t.Fatal("catálogo no actualizado")
		}
		return certpicker.ResultadoSeleccion{Certificado: refs[0]}, nil
	})
	if _, err := selector.Select(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestWebCredential_HeadlessNoMaterializaCaducado(t *testing.T) {
	h := &legacyWebSocketHandler{keys: keyProviderFunc(func(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
		t.Fatal("se solicitó clave caducada")
		return nil, nil
	})}
	_, _, err := h.materializarCertificado(context.Background(), domain.CertificateRef{NotAfter: time.Now().Add(-time.Hour)})
	if err == nil {
		t.Fatal("certificado caducado aceptado")
	}
}

func TestWebCredential_SelectCertConservaIdentidadHastaFirmaYDespuesLaRetira(t *testing.T) {
	ctx := context.Background()
	data := generarP12(t, "QA ciclo de firma", "qa")
	loads := 0
	selector, catalogo, claves := nuevoSelectorCredenciales(credentialSelectorFunc(func(_ context.Context, refs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
		if len(refs) == 0 {
			return certpicker.ResultadoSeleccion{}, certpicker.ErrCargarCertificado
		}
		return certpicker.ResultadoSeleccion{Certificado: refs[0]}, nil
	}), certificateCatalogStub{}, keyProviderStub{err: errors.New("sin clave")}, func(context.Context) ([]byte, []byte, error) { loads++; return data, []byte("qa"), nil }, nil)
	t.Cleanup(selector.ClearCredentials)
	approval := &userApprovalStub{approved: true}
	motor := desksigner.NuevoMotorFirmaGo(relojReal{})
	h := &legacyWebSocketHandler{catalogo: catalogo, keys: claves, selector: selector, approval: approval,
		signUC: application.NuevoSignDocumentUseCase(catalogo, claves, motor, approval, nil, nil)}
	selectCtx, cancelSelect := context.WithCancel(ctx)
	if _, _, err := h.HandleLegacy(selectCtx, "", afirmauri.Solicitud{Operacion: afirmauri.OperacionSelectCert}); err != nil {
		t.Fatal(err)
	}
	cancelSelect() // La petición selectcert termina antes de la siguiente sign.
	refs, _ := catalogo.List(ctx)
	if len(refs) != 1 {
		t.Fatal("selectcert retiró la identidad antes de firmar")
	}
	doc, _ := domain.NewDocument("qa.txt", []byte("datos de prueba"), "text/plain")
	result, _, err := h.HandleLegacy(ctx, "", afirmauri.Solicitud{Operacion: afirmauri.OperacionFirma, SignCommand: &application.SignCommand{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}})
	if err != nil || result.Texto == "" {
		t.Fatalf("firma no completada: %v", err)
	}
	refs, _ = catalogo.List(ctx)
	if loads != 1 || len(refs) != 0 {
		t.Fatalf("ciclo carga/limpieza incorrecto: loads=%d refs=%d", loads, len(refs))
	}
}

func TestWebCredential_SesionAbandonadaRetiraIdentidades(t *testing.T) {
	for _, cause := range []string{"timeout", "cancelacion_selector"} {
		t.Run(cause, func(t *testing.T) {
			data := generarP12(t, "QA limpieza", "qa")
			defer clear(data)
			selector, _, _ := nuevoSelectorCredenciales(nil, certificateCatalogStub{}, keyProviderStub{}, nil, nil)
			t.Cleanup(selector.ClearCredentials)
			if err := selector.cargar(context.Background(), data, []byte("qa")); err != nil {
				t.Fatal(err)
			}
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				selector.mu.Lock()
				selector.scheduleCleanupLocked()
				selector.mu.Unlock()
				if cause == "timeout" {
					time.Sleep(5 * time.Minute)
				} else {
					cancel()
					if _, err := selector.Select(ctx, nil); !errors.Is(err, context.Canceled) {
						t.Fatal("selector no cancelado")
					}
				}
				synctest.Wait()
				refs, _ := selector.temporal.List(context.Background())
				if len(refs) != 0 {
					t.Fatal("credencial abandonada retenida")
				}
			})
		})
	}
}

func TestWebCredential_LimitesYSeleccionAjena(t *testing.T) {
	selector, _, _ := nuevoSelectorCredenciales(nil, certificateCatalogStub{}, keyProviderStub{}, nil, nil)
	for _, input := range []struct{ data, password []byte }{
		{data: make([]byte, maxTemporaryCredentialBytes+1)},
		{data: []byte("no debe parsearse"), password: make([]byte, maxTemporaryPasswordBytes+1)},
	} {
		if err := selector.cargar(context.Background(), input.data, input.password); err == nil {
			t.Fatal("límites ignorados")
		}
	}
	for _, ref := range []domain.CertificateRef{{}, {ID: "no-mostrado", Fingerprint: "no-mostrado"}} {
		if contieneCertificado([]domain.CertificateRef{{}}, ref) {
			t.Fatal("selección vacía o ajena aceptada")
		}
	}
}

func TestWebCredential_OperacionesConcurrentesEsperanOCancelan(t *testing.T) {
	selector, _, _ := nuevoSelectorCredenciales(nil, certificateCatalogStub{}, keyProviderStub{}, nil, nil)
	firstRelease, err := selector.BeginCredentialOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := selector.BeginCredentialOperation(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("la espera no respeta cancelación")
	}
	firstRelease()
	firstRelease() // Cerrar dos veces no libera una operación distinta.
	secondRelease, err := selector.BeginCredentialOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	secondRelease()
}
