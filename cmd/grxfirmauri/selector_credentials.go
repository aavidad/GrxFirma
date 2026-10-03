// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"crypto/x509"
	"errors"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/certcatalogagg"
	"grxfirma/internal/adapters/outbound/desktop/sessioncertstore"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
)

type selectorCredenciales struct {
	mu           sync.Mutex
	selector     certpicker.CertSelector
	catalogo     ports.CertificateCatalog
	claves       ports.SigningKeyProvider
	temporal     *sessioncertstore.Store
	solicitar    func(context.Context) ([]byte, []byte, error)
	avisar       func(context.Context, string, string)
	cleanupTimer *time.Timer
	generation   uint64
	operation    chan struct{}
}

func nuevoSelectorCredenciales(selector certpicker.CertSelector, catalogo ports.CertificateCatalog, claves ports.SigningKeyProvider, solicitar func(context.Context) ([]byte, []byte, error), avisar func(context.Context, string, string)) (*selectorCredenciales, ports.CertificateCatalog, ports.SigningKeyProvider) {
	temporal := sessioncertstore.New()
	catalogoAgregado := certcatalogagg.New(temporal, catalogo)
	clavesAgregadas := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{temporal, claves}}
	return &selectorCredenciales{selector: selector, catalogo: catalogoAgregado, claves: clavesAgregadas, temporal: temporal, solicitar: solicitar, avisar: avisar, operation: make(chan struct{}, 1)}, catalogoAgregado, clavesAgregadas
}

func (s *selectorCredenciales) SupportsCredentialLoading() bool { return true }

func (s *selectorCredenciales) BeginCredentialOperation(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case s.operation <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-s.operation }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *selectorCredenciales) ClearCredentials() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
}

func (s *selectorCredenciales) clearLocked() {
	s.generation++
	if s.cleanupTimer != nil {
		s.cleanupTimer.Stop()
		s.cleanupTimer = nil
	}
	s.temporal.Clear()
}

// Un selectcert puede preceder a sign en otra petición. La identidad se conserva
// hasta esa firma, con un máximo de cinco minutos si la web abandona el flujo.
func (s *selectorCredenciales) scheduleCleanupLocked() {
	s.generation++
	generation := s.generation
	if s.cleanupTimer != nil {
		s.cleanupTimer.Stop()
	}
	cleanup := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if generation == s.generation {
			s.clearLocked()
		}
	}
	s.cleanupTimer = time.AfterFunc(5*time.Minute, cleanup)
}

func (s *selectorCredenciales) Select(ctx context.Context, candidatos []domain.CertificateRef) (result certpicker.ResultadoSeleccion, resultErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if resultErr != nil {
			s.clearLocked()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			s.clearLocked()
			return certpicker.ResultadoSeleccion{}, err
		}
		seleccion, err := s.selector.Select(ctx, candidatos)
		switch {
		case errors.Is(err, certpicker.ErrCargarCertificado):
			data, password, loadErr := s.solicitar(ctx)
			if loadErr == nil {
				loadErr = s.cargar(ctx, data, password)
			}
			secmem.Zeroize(data)
			secmem.Zeroize(password)
			if ctx.Err() != nil {
				return certpicker.ResultadoSeleccion{}, ctx.Err()
			}
			if loadErr != nil && !errors.Is(loadErr, certpicker.ErrSeleccionCancelada) && !errors.Is(loadErr, context.Canceled) {
				s.aviso(ctx, "No se pudo cargar el certificado", "Comprueba el fichero y la contraseña. El certificado debe estar vigente y permitir firma digital.")
			}
		case errors.Is(err, certpicker.ErrActualizarCertificados):
			// Volver a consultar los almacenes permite insertar otro dispositivo
			// o importar una identidad en el gestor del sistema.
		case err != nil:
			s.clearLocked()
			return certpicker.ResultadoSeleccion{}, err
		default:
			if !contieneCertificado(candidatos, seleccion.Certificado) {
				return certpicker.ResultadoSeleccion{}, errors.New("el certificado seleccionado no pertenece al catálogo mostrado")
			}
			key, keyErr := s.claves.KeyFor(ctx, seleccion.Certificado)
			if keyErr == nil {
				keyErr = validarCredencialFirma(key, time.Now())
			}
			ports.CloseSigningKey(key)
			if keyErr == nil {
				s.scheduleCleanupLocked()
				return seleccion, nil
			}
			if ctx.Err() != nil {
				return certpicker.ResultadoSeleccion{}, ctx.Err()
			}
			s.aviso(ctx, "El certificado no permite continuar", "Selecciona un certificado vigente con clave de firma, o carga otro fichero P12 o PFX.")
		}
		var listErr error
		candidatos, listErr = s.catalogo.List(ctx)
		if listErr != nil {
			return certpicker.ResultadoSeleccion{}, errors.New("no se pudo actualizar el catálogo de certificados")
		}
	}
}

func (s *selectorCredenciales) cargar(ctx context.Context, data, password []byte) error {
	if len(data) == 0 || len(data) > maxTemporaryCredentialBytes || len(password) > maxTemporaryPasswordBytes {
		return errors.New("tamaño de credencial o contraseña no permitido")
	}
	result, err := application.NuevoTemporaryCertificateUseCase(s.temporal).Use(ctx, application.UseTemporaryCertificateCommand{Data: data, Password: string(password)})
	if err != nil {
		return err
	}
	key, err := s.temporal.KeyFor(ctx, result.Certificate)
	if err == nil {
		err = validarCredencialFirma(key, time.Now())
	}
	ports.CloseSigningKey(key)
	if err != nil {
		s.temporal.Remove(result.Certificate.ID)
	}
	return err
}

func (s *selectorCredenciales) aviso(ctx context.Context, title, detail string) {
	if s.avisar != nil {
		s.avisar(ctx, title, detail)
	}
}

func contieneCertificado(candidatos []domain.CertificateRef, elegido domain.CertificateRef) bool {
	if strings.TrimSpace(elegido.ID) == "" || strings.TrimSpace(elegido.Fingerprint) == "" {
		return false
	}
	for _, cert := range candidatos {
		if cert.ID == elegido.ID && cert.Fingerprint == elegido.Fingerprint {
			return true
		}
	}
	return false
}

func validarCredencialFirma(key ports.SigningKey, ahora time.Time) error {
	if key == nil {
		return errors.New("clave de firma no disponible")
	}
	cadena := key.CertificateChainDER()
	if len(cadena) == 0 {
		return errors.New("certificado de firma no disponible")
	}
	cert, err := x509.ParseCertificate(cadena[0])
	if err != nil {
		return errors.New("certificado de firma inválido")
	}
	if ahora.Before(cert.NotBefore) || ahora.After(cert.NotAfter) {
		return errors.New("certificado fuera de vigencia")
	}
	if cert.IsCA || (cert.KeyUsage != 0 && cert.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0) {
		return errors.New("certificado no autorizado para firma digital")
	}
	return nil
}

// ElegirPosicionSello delega en la interfaz envuelta la elección de la
// posición de la firma visible (visibleSignature=want).
func (s *selectorCredenciales) ElegirPosicionSello(ctx context.Context) (string, string, error) {
	if sel, ok := s.selector.(certpicker.SelectorPosicionSello); ok {
		return sel.ElegirPosicionSello(ctx)
	}
	return "", "", certpicker.ErrPosicionSelloNoDisponible
}

func (s *selectorCredenciales) ElegirSelloEnEditor(ctx context.Context, documento domain.Document, nombreCertificado string) ([]byte, error) {
	return certpicker.EjecutarEditorSello(ctx, documento, nombreCertificado)
}
