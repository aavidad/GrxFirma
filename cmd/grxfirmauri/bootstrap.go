// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"grxfirma/internal/adapters/inbound/desktop/session"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	"grxfirma/internal/adapters/outbound/common/metrics"
	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/revocationclient"
	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/adapters/outbound/desktop/certcatalogagg"
	"grxfirma/internal/adapters/outbound/desktop/certselectionstore"
	"grxfirma/internal/adapters/outbound/desktop/macoskeychain"
	"grxfirma/internal/adapters/outbound/desktop/nssstore"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/adapters/outbound/desktop/tokenruntime"
	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
	"grxfirma/presentation/desktop/tokenpin"
)

type catalogoMemoria struct{ certs []domain.CertificateRef }

func (c *catalogoMemoria) List(_ context.Context) ([]domain.CertificateRef, error) {
	return c.certs, nil
}

type proveedorClavesMemoria struct{ claves map[string]ports.SigningKey }

func (p *proveedorClavesMemoria) KeyFor(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	if k, ok := p.claves[ref.ID]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("clave no encontrada para el certificado %s", ref.ID)
}

type proveedorClavesAgregado struct {
	fuentes []ports.SigningKeyProvider
}

func (p *proveedorClavesAgregado) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	var errores []error
	for _, fuente := range p.fuentes {
		if fuente == nil {
			continue
		}
		clave, err := fuente.KeyFor(ctx, ref)
		if err == nil && clave != nil {
			return clave, nil
		}
		ports.CloseSigningKey(clave)
		if err != nil && !esErrorProveedorNoAplicable(err) {
			errores = append(errores, err)
		}
	}
	if len(errores) > 0 {
		sort.SliceStable(errores, func(i, j int) bool { return errores[i].Error() < errores[j].Error() })
		return nil, fmt.Errorf("clave de firma no disponible para el certificado %s: %w", ref.ID, errors.Join(errores...))
	}
	return nil, fmt.Errorf("clave de firma no disponible para el certificado %s: no hay proveedor aplicable", ref.ID)
}

func esErrorProveedorNoAplicable(err error) bool {
	return errors.Is(err, macoskeychain.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, wincertstore.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, tokenruntime.ErrNotApplicable) || errors.Is(err, tokenruntime.ErrIdentityUnknown) ||
		strings.Contains(strings.ToLower(err.Error()), "nssstore: solo disponible en linux")
}

type relojReal struct{}

func (r relojReal) Now() time.Time { return time.Now() }

func construirMotorFirma(configDir string, client *http.Client) *deskSigner.MotorFirmaGo {
	motor := deskSigner.NuevoMotorFirmaGo(relojReal{})
	clienteHTTP := resolverClienteHTTPRuntime(configDir, client)
	if tsaURL := strings.TrimSpace(os.Getenv("GRXFIRMA_TSA_URL")); tsaURL != "" {
		motor.WithTimestampAuthority(construirTSAClienteRuntime(tsaURL, clienteHTTP))
	}
	motor.WithRevocationProvider(construirRevocationProviderRuntime(clienteHTTP))
	return motor
}

func construirClienteHTTPRuntimeSeguro(configDir string) *http.Client {
	return construirClienteTrifasico(configDir)
}

func resolverClienteHTTPRuntime(configDir string, client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return construirClienteHTTPRuntimeSeguro(configDir)
}

func construirTSAClienteRuntime(tsaURL string, client *http.Client) *tsaclient.Client {
	tsa := tsaclient.New(strings.TrimSpace(tsaURL))
	if client != nil {
		tsa.HTTPClient = client
	}
	return tsa
}

func construirRevocationProviderRuntime(client *http.Client) *revocationclient.Client {
	return revocationclient.NewWithHTTPClient(client)
}

type runtimeAfirmaURI struct {
	parser                    *afirmauri.Adaptador
	orquestador               *session.Orchestrator
	orquestadorSinPreferencia *session.Orchestrator
	catalogo                  ports.CertificateCatalog
	claves                    ports.SigningKeyProvider
	selector                  certpicker.CertSelector
	clienteHTTP               *http.Client
	motor                     *deskSigner.MotorFirmaGo
	configDir                 string
	metricas                  ports.OperationMetrics
	trustPolicy               ports.TrustPolicy
}

type preferenceStore interface {
	LoadSession(ctx context.Context, origin string) (string, bool, error)
	SaveSession(ctx context.Context, origin, certificateID string) error
	LoadPersistent(ctx context.Context, origin string) (string, bool, error)
	SavePersistent(ctx context.Context, origin, certificateID string) error
}

type constructorFuentesCertificados func(
	p12Dir string,
	p12Password string,
	metricas ports.OperationMetrics,
) (ports.CertificateCatalog, ports.SigningKeyProvider, error)

func construirRuntimeAfirmaURI(configDir, p12Dir, p12Password string) (*runtimeAfirmaURI, error) {
	return construirRuntimeAfirmaURIConDependencias(
		configDir,
		p12Dir,
		p12Password,
		construirFuentesCertificados,
	)
}

func construirRuntimeAfirmaURIConDependencias(
	configDir,
	p12Dir,
	p12Password string,
	construirFuentes constructorFuentesCertificados,
	selectorInyectado ...certpicker.CertSelector,
) (*runtimeAfirmaURI, error) {
	start := time.Now()
	metricas := metrics.NewDefault()
	certStart := time.Now()
	if construirFuentes == nil {
		return nil, errors.New("constructor de fuentes de certificados no configurado")
	}
	catalogo, claves, err := construirFuentes(p12Dir, p12Password, metricas)
	if err != nil {
		return nil, err
	}
	if tokenruntime.EnabledInBuild() {
		tokens := tokenruntime.New(configDir, tokenpin.New("es").Request)
		catalogo = certcatalogagg.New(catalogo, tokens)
		claves = &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{claves, tokens}}
	}
	slog.Info("grxfirma_afirmauri_runtime_cert_sources_ready", "elapsed_ms", time.Since(certStart).Milliseconds())

	trustStart := time.Now()
	politica, err := newTrustPolicy(configDir)
	if err != nil {
		return nil, err
	}
	slog.Info("grxfirma_afirmauri_runtime_trust_ready", "elapsed_ms", time.Since(trustStart).Milliseconds())

	selector := newCertSelector()
	if len(selectorInyectado) > 0 && selectorInyectado[0] != nil {
		// Los arneses de protocolo inyectan su UI, también al compilar con Fyne.
		selector = selectorInyectado[0]
	} else if credentialLoadingAvailable() {
		selector, catalogo, claves = nuevoSelectorCredenciales(selector, catalogo, claves, solicitarCredencialTemporal, mostrarAvisoCredencialTemporal)
	}
	progreso := newProgressProvider()
	httpStart := time.Now()
	clienteHTTP := construirClienteHTTPRuntimeSeguro(configDir)
	motor := construirMotorFirma(configDir, clienteHTTP)
	slog.Info("grxfirma_afirmauri_runtime_http_signer_ready", "elapsed_ms", time.Since(httpStart).Milliseconds())
	ejecutor := triphase.New(clienteHTTP)
	lote := triphase.NewBatch(ejecutor)
	crearOrquestador := func(preferencias preferenceStore) *session.Orchestrator {
		return session.New(session.Config{
			TrustValidator:                      originvalidator.New(politica),
			ParserLegacy:                        afirmauri.New(nil),
			CertCatalog:                         catalogo,
			KeyProvider:                         claves,
			CertSelector:                        selector,
			Preferencias:                        preferencias,
			Signer:                              motor,
			TriphaseExec:                        ejecutor,
			BatchExec:                           lote,
			Progress:                            progreso,
			Notify:                              nil,
			Eventos:                             nil,
			RequireExplicitCertificateSelection: nativeProtocolUIEnabled() || tokenruntime.EnabledInBuild(),
			Timeout:                             5 * time.Minute,
		})
	}
	preferencias := certselectionstore.New(configDir)
	orq := crearOrquestador(preferencias)
	orqSinPreferencias := crearOrquestador(nil)

	runtime := &runtimeAfirmaURI{
		parser:                    afirmauri.New(politica),
		orquestador:               orq,
		orquestadorSinPreferencia: orqSinPreferencias,
		catalogo:                  catalogo,
		claves:                    claves,
		selector:                  selector,
		clienteHTTP:               clienteHTTP,
		motor:                     motor,
		configDir:                 configDir,
		metricas:                  metricas,
		trustPolicy:               politica,
	}
	slog.Info("grxfirma_afirmauri_runtime_ready", "elapsed_ms", time.Since(start).Milliseconds())
	return runtime, nil
}

func (r *runtimeAfirmaURI) orquestadorPara(solicitud afirmauri.Solicitud) *session.Orchestrator {
	if solicitud.Operacion == afirmauri.OperacionSelectCert {
		return r.orquestador
	}
	if r.orquestadorSinPreferencia != nil {
		return r.orquestadorSinPreferencia
	}
	return r.orquestador
}

func construirFuentesCertificados(p12Dir, p12Password string, metricas ports.OperationMetrics) (ports.CertificateCatalog, ports.SigningKeyProvider, error) {
	start := time.Now()
	memCatalogo, memClaves, err := cargarDirectorioP12(p12Dir, p12Password, metricas)
	if err != nil {
		return nil, nil, err
	}

	nss := nssstore.New()
	mac := macoskeychain.New()
	windowsStore := wincertstore.New()
	agregadoCatalogo := certcatalogagg.New(
		nss,
		mac,
		windowsStore,
		memCatalogo,
	)
	agregadoClaves := &proveedorClavesAgregado{
		fuentes: []ports.SigningKeyProvider{
			nss,
			mac,
			windowsStore,
			memClaves,
		},
	}

	slog.Info("grxfirma_afirmauri_cert_sources_constructed", "elapsed_ms", time.Since(start).Milliseconds())
	return agregadoCatalogo, agregadoClaves, nil
}

func cargarDirectorioP12(dir, password string, metricas ports.OperationMetrics) (ports.CertificateCatalog, ports.SigningKeyProvider, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &catalogoMemoria{}, &proveedorClavesMemoria{claves: map[string]ports.SigningKey{}}, nil
		}
		return nil, nil, fmt.Errorf("no se pudo leer el directorio de certificados %s: %w", dir, err)
	}

	importador := pkcs12importer.New()
	ctx := context.Background()
	certs := make([]domain.CertificateRef, 0, len(entries))
	claves := make(map[string]ports.SigningKey, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		nombre := strings.ToLower(entry.Name())
		if !strings.HasSuffix(nombre, ".p12") && !strings.HasSuffix(nombre, ".pfx") {
			continue
		}
		ruta := filepath.Join(dir, entry.Name())
		identidad, err := importador.ImportP12File(ctx, ruta, password)
		if err != nil {
			continue
		}
		if metricas != nil {
			metricas.RecordCertificateSource(ctx, "p12")
		}
		certs = append(certs, identidad.Reference)
		claves[identidad.Reference.ID] = deskSigner.NuevaClaveLocalConCadena(identidad.Signer, identidad.Certificate, identidad.Chain)
	}

	return &catalogoMemoria{certs: certs}, &proveedorClavesMemoria{claves: claves}, nil
}
