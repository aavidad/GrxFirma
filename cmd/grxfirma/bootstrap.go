// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"grxfirma/internal/adapters/inbound/common/cli"
	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/hashmanifest"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/metrics"
	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/adapters/outbound/common/revocationclient"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/systemtrust"
	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/adapters/outbound/desktop/afirmahandler"
	"grxfirma/internal/adapters/outbound/desktop/certcatalogagg"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
	"grxfirma/internal/adapters/outbound/desktop/macoskeychain"
	"grxfirma/internal/adapters/outbound/desktop/nssstore"
	"grxfirma/internal/adapters/outbound/desktop/pdfpreview"
	"grxfirma/internal/adapters/outbound/desktop/proxyhttp"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/adapters/outbound/desktop/tokenruntime"
	"grxfirma/internal/adapters/outbound/desktop/truststore"
	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/tokenpin"
)

// ---------------------------------------------------------------------------
// Adaptadores minimos de bootstrapping
// ---------------------------------------------------------------------------

// catalogoMemoria lista los certificados cargados en memoria al arrancar.
type catalogoMemoria struct {
	certs []domain.CertificateRef
}

func (c *catalogoMemoria) List(_ context.Context) ([]domain.CertificateRef, error) {
	return c.certs, nil
}

// proveedorClaves resuelve la clave privada para un certificado dado.
type proveedorClavesMemoria struct {
	claves map[string]ports.SigningKey
}

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
	var ultimo error
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
			ultimo = err
		}
	}
	if ultimo != nil {
		return nil, ultimo
	}
	return nil, fmt.Errorf("clave no encontrada para el certificado %s", ref.ID)
}

func esErrorProveedorNoAplicable(err error) bool {
	return errors.Is(err, macoskeychain.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, wincertstore.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, tokenruntime.ErrNotApplicable) || errors.Is(err, tokenruntime.ErrIdentityUnknown) ||
		strings.Contains(strings.ToLower(err.Error()), "nssstore: solo disponible en linux")
}

// aprobacionAutomatica aprueba sin intervencion del usuario.
// En versiones futuras puede sustituirse por un dialogo de terminal o GUI.
type aprobacionAutomatica struct{}

func (a *aprobacionAutomatica) Request(_ context.Context, _ string) (bool, error) {
	return true, nil
}

// relojReal delega en time.Now.
type relojReal struct{}

func (r relojReal) Now() time.Time { return time.Now() }

type entradaCert struct {
	ref   domain.CertificateRef
	clave ports.SigningKey
}

type serviciosGrxFirma struct {
	configDir          string
	rutaP12            string
	password           string
	rutaCert           string
	rutaClave          string
	directorioP12      string
	catalogo           ports.CertificateCatalog
	claves             ports.SigningKeyProvider
	firmar             *application.SignDocumentUseCase
	procesarLote       *application.ProcessBatchUseCase
	verificar          *application.VerifySignatureUseCase
	crearHash          *application.CreateHashUseCase
	comprobarHash      *application.CheckHashUseCase
	crearHashDir       *application.CreateDirectoryHashManifestUseCase
	comprobarHashDir   *application.CheckDirectoryHashManifestUseCase
	informeHashDir     ports.DirectoryHashReportCodec
	gestionDominios    *application.ManageTrustedDomainUseCase
	proteger           *application.ProtectDocumentUseCase
	protegerFirmando   *application.ProtectAndSignDocumentUseCase
	desproteger        *application.UnprotectDocumentUseCase
	exportarProteccion *application.ExportProtectionRecipientUseCase
	importarProteccion *application.ImportProtectionRecipientUseCase
	proteccion         *protector.LocalCombinedKeyring
	// motor y tsaPermitidas permiten al servidor REST fijar su modo de TSA de
	// opciones (lote 13a) después de construir los servicios.
	motor         *deskSigner.MotorFirmaGo
	tsaPermitidas []string
	auditor       *application.AuditUseCase
}

func construirMotorFirmaConHTTP(httpClient *http.Client) *deskSigner.MotorFirmaGo {
	motor := deskSigner.NuevoMotorFirmaGo(relojReal{})
	if tsaURL := os.Getenv("GRXFIRMA_TSA_URL"); tsaURL != "" {
		tsa := tsaclient.New(tsaURL)
		tsa.HTTPClient = httpClient
		motor.WithTimestampAuthority(tsa)
	}
	motor.WithRevocationProvider(revocationclient.NewWithHTTPClient(httpClient))
	return motor
}

// configurarTSAOpcionesREST fija el modo de TSA de opciones del servidor REST
// (lote 13a): una TSA interna se admite con aviso en el registro y, si la
// política de máquina define tsa_permitidas, solo valen las de la lista.
func configurarTSAOpcionesREST(servicios *serviciosGrxFirma) {
	if servicios == nil || servicios.motor == nil {
		return
	}
	servicios.motor.AvisarTSADeOpcionesEnRedLocal().ConTSAPermitidas(servicios.tsaPermitidas)
}

func cargarDirectorioP12(dir, password string, metricas ports.OperationMetrics) ([]entradaCert, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("no se pudo leer el directorio de certificados %s: %w", dir, err)
	}

	var resultado []entradaCert
	importador := pkcs12importer.New()
	ctx := context.Background()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		nombre := strings.ToLower(e.Name())
		if !strings.HasSuffix(nombre, ".p12") && !strings.HasSuffix(nombre, ".pfx") {
			continue
		}
		ruta := filepath.Join(dir, e.Name())
		identidad, err := importador.ImportP12File(ctx, ruta, password)
		if err != nil {
			continue
		}
		if metricas != nil {
			metricas.RecordCertificateSource(ctx, "p12")
		}
		resultado = append(resultado, entradaCert{
			ref:   identidad.Reference,
			clave: deskSigner.NuevaClaveLocalConCadena(identidad.Signer, identidad.Certificate, identidad.Chain),
		})
	}
	return resultado, nil
}

func cargarIdentidadesLocales(
	ctx context.Context,
	rutaP12,
	password,
	rutaCert,
	rutaClave,
	p12Dir,
	p12Password string,
	metricas ports.OperationMetrics,
) ([]entradaCert, error) {
	importador := pkcs12importer.New()
	var resultado []entradaCert

	switch {
	case rutaP12 != "":
		identidad, err := importador.ImportP12File(ctx, rutaP12, password)
		if err != nil {
			return nil, err
		}
		if metricas != nil {
			metricas.RecordCertificateSource(ctx, "p12")
		}
		resultado = append(resultado, entradaCert{
			ref:   identidad.Reference,
			clave: deskSigner.NuevaClaveLocalConCadena(identidad.Signer, identidad.Certificate, identidad.Chain),
		})
	case rutaCert != "" && rutaClave != "":
		identidad, err := importador.ImportPEMFiles(ctx, rutaCert, rutaClave)
		if err != nil {
			return nil, err
		}
		if metricas != nil {
			metricas.RecordCertificateSource(ctx, "pem")
		}
		resultado = append(resultado, entradaCert{
			ref:   identidad.Reference,
			clave: deskSigner.NuevaClaveLocalConCadena(identidad.Signer, identidad.Certificate, identidad.Chain),
		})
	}

	if p12Dir != "" {
		entradasDir, err := cargarDirectorioP12(p12Dir, p12Password, metricas)
		if err != nil {
			return nil, err
		}
		resultado = append(resultado, entradasDir...)
	}

	return resultado, nil
}

func construirFuentesCertificados(
	ctx context.Context,
	rutaP12,
	password,
	rutaCert,
	rutaClave,
	p12Dir,
	p12Password string,
	metricas ports.OperationMetrics,
) (ports.CertificateCatalog, ports.SigningKeyProvider, error) {
	entradas, err := cargarIdentidadesLocales(ctx, rutaP12, password, rutaCert, rutaClave, p12Dir, p12Password, metricas)
	if err != nil {
		return nil, nil, err
	}

	refs := make([]domain.CertificateRef, 0, len(entradas))
	claves := make(map[string]ports.SigningKey, len(entradas))
	seen := make(map[string]struct{})
	for _, entrada := range entradas {
		fp := entrada.ref.Fingerprint
		if fp == "" {
			fp = entrada.ref.ID
		}
		if _, ok := seen[fp]; ok {
			continue
		}
		seen[fp] = struct{}{}
		refs = append(refs, entrada.ref)
		claves[entrada.ref.ID] = entrada.clave
	}

	catalogoMem := &catalogoMemoria{certs: refs}
	proveedorMem := &proveedorClavesMemoria{claves: claves}
	nss := nssstore.New()
	mac := macoskeychain.New()
	win := wincertstore.New()

	catalogo := certcatalogagg.New(
		catalogoMem,
		nss,
		mac,
		win,
	)
	proveedor := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{
		proveedorMem,
		nss,
		mac,
		win,
	}}

	return catalogo, proveedor, nil
}

// ---------------------------------------------------------------------------
// construirAdaptador monta la pila completa a partir de P12 o PEM.
// ---------------------------------------------------------------------------

// fuenteRemota sustituye todas las fuentes locales de certificados por una
// única credencial remota (firma CSC): así la CLI no puede firmar por error
// con otro certificado del equipo.
type fuenteRemota struct {
	ref   domain.CertificateRef
	clave ports.SigningKey
}

func construirServicios(rutaP12, password, rutaCert, rutaClave string) (*serviciosGrxFirma, error) {
	return construirServiciosCon(rutaP12, password, rutaCert, rutaClave, nil)
}

func construirServiciosCon(rutaP12, password, rutaCert, rutaClave string, remota *fuenteRemota) (*serviciosGrxFirma, error) {
	ctx := context.Background()
	metricas := metrics.NewDefault()
	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)
	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, fmt.Errorf("cargando configuracion: %w", err)
	}
	p12Dir := cfg.DirectorioP12
	if p12Dir == "" {
		p12Dir = filepath.Join(configDir, "pkcs12")
	}
	if override := os.Getenv("GRXFIRMA_PKCS12_DIR"); override != "" {
		p12Dir = override
	}
	// main ya retiró GRXFIRMA_PKCS12_PASSWORD del entorno y la transfiere al
	// adaptador CLI; aquí no queda nada que leer (H-04).
	var catalogo ports.CertificateCatalog
	var claves ports.SigningKeyProvider
	if remota != nil {
		catalogo = &catalogoMemoria{certs: []domain.CertificateRef{remota.ref}}
		claves = &proveedorClavesMemoria{claves: map[string]ports.SigningKey{remota.ref.ID: remota.clave}}
	} else {
		catalogo, claves, err = construirFuentesCertificados(ctx, rutaP12, password, rutaCert, rutaClave, p12Dir, password, metricas)
		if err != nil {
			return nil, err
		}
		if tokenruntime.EnabledInBuild() {
			tokens := tokenruntime.New(configDir, tokenpin.New("es").Request)
			catalogo = certcatalogagg.New(catalogo, tokens)
			claves = &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{claves, tokens}}
		}
	}

	httpClient := proxyhttp.New(configDir)
	// Lote 13a (retrocompatibilidad, revisa E1-12): en la CLI y en el
	// escritorio la tsaURL la teclea o la elige el propio usuario, así que
	// vale una TSA interna sin más. El servidor REST cambia este modo en
	// runRESTServer (aviso en el registro y lista de la política).
	motor := construirMotorFirmaConHTTP(httpClient).PermitirTSADeOpcionesEnRedLocal()
	registradorAuditoria, err := auditlog.NewDefault()
	if err != nil {
		return nil, err
	}
	auditor := application.NuevoAuditUseCase(relojReal{}, registradorAuditoria)
	aprobador := &aprobacionAutomatica{}
	verificador := application.NuevoVerifySignatureUseCase(
		systemtrust.New(),
		commonsigner.NewMultiVerifierWithHTTPClient(httpClient),
		auditor,
	)
	politica, err := truststore.New(configDir, true)
	if err != nil {
		return nil, fmt.Errorf("cargando politica de confianza: %w", err)
	}
	gestionDominios := application.NuevoManageTrustedDomainUseCase(politica, auditor, nil)

	ucFirmar := application.NuevoSignDocumentUseCase(catalogo, claves, motor, aprobador, auditor, nil).WithMetrics(metricas)
	ucLote := application.NuevoProcessBatchUseCase(catalogo, claves, motor, aprobador, auditor, nil).WithMetrics(metricas)
	ucCrearHash := application.NuevoCreateHashUseCase()
	ucComprobarHash := application.NuevoCheckHashUseCase()
	treeReader := filesystem.NuevoDirectoryTreeReader()
	manifestCodec := hashmanifest.NuevoCodec()
	ucCrearHashDir := application.NuevoCreateDirectoryHashManifestUseCase(treeReader, manifestCodec)
	ucComprobarHashDir := application.NuevoCheckDirectoryHashManifestUseCase(treeReader, manifestCodec)
	keyringProteccion := protector.NuevoLocalCombinedKeyring(configDir, appdirs.Data(home), catalogo, claves)
	motorProteccion := protector.NuevoAdaptiveProtector()
	motorProteccionFirmada := protector.NuevoCMSSignedEnvelopedProtector()
	ucProteger := application.NuevoProtectDocumentUseCase(keyringProteccion, motorProteccion, aprobador, auditor, nil)
	ucProtegerFirmando := application.NuevoProtectAndSignDocumentUseCase(catalogo, claves, keyringProteccion, motorProteccionFirmada, aprobador, auditor, nil)
	ucDesproteger := application.NuevoUnprotectDocumentUseCase(keyringProteccion, motorProteccion, auditor, nil)
	ucExportarProteccion := application.NuevoExportProtectionRecipientUseCase(keyringProteccion)
	ucImportarProteccion := application.NuevoImportProtectionRecipientUseCase(keyringProteccion)

	return &serviciosGrxFirma{
		configDir:          configDir,
		rutaP12:            rutaP12,
		password:           password,
		rutaCert:           rutaCert,
		rutaClave:          rutaClave,
		directorioP12:      p12Dir,
		catalogo:           catalogo,
		claves:             claves,
		firmar:             ucFirmar,
		procesarLote:       ucLote,
		verificar:          verificador,
		crearHash:          ucCrearHash,
		comprobarHash:      ucComprobarHash,
		crearHashDir:       ucCrearHashDir,
		comprobarHashDir:   ucComprobarHashDir,
		informeHashDir:     manifestCodec,
		gestionDominios:    gestionDominios,
		proteger:           ucProteger,
		protegerFirmando:   ucProtegerFirmando,
		desproteger:        ucDesproteger,
		exportarProteccion: ucExportarProteccion,
		importarProteccion: ucImportarProteccion,
		proteccion:         keyringProteccion,
		motor:              motor,
		tsaPermitidas:      cfg.TSAPermitidas,
		auditor:            auditor,
	}, nil
}

func construirAdaptador(rutaP12, password, rutaCert, rutaClave string) (*cli.Adaptador, error) {
	return construirAdaptadorCon(rutaP12, password, rutaCert, rutaClave, nil)
}

// construirServiciosEscritorio compone los servicios de la interfaz Fyne
// local. Como la CLI, la verificación aplica lo que el usuario aceptó en este
// equipo (lote 17b); la lista se gestiona en la CLI, Qt y WinUI.
func construirServiciosEscritorio(rutaP12, password, rutaCert, rutaClave string) (*serviciosGrxFirma, error) {
	servicios, err := construirServicios(rutaP12, password, rutaCert, rutaClave)
	if err != nil {
		return nil, err
	}
	servicios.verificar.ConConfianzaForzada(config.NuevoAlmacenConfianzaForzada(servicios.configDir))
	return servicios, nil
}

func construirAdaptadorCon(rutaP12, password, rutaCert, rutaClave string, remota *fuenteRemota) (*cli.Adaptador, error) {
	servicios, err := construirServiciosCon(rutaP12, password, rutaCert, rutaClave, remota)
	if err != nil {
		return nil, err
	}
	// Lote 14: solo la CLI local aplica y gestiona lo que el usuario aceptó
	// al verificar. El servidor REST construye sus propios servicios sin esto.
	confianzaForzada := config.NuevoAlmacenConfianzaForzada(servicios.configDir)
	servicios.verificar.ConConfianzaForzada(confianzaForzada)

	return cli.New(servicios.firmar, servicios.procesarLote).
		WithConfianzaForzada(application.NuevoGestionConfianzaForzadaUseCase(confianzaForzada, servicios.auditor)).
		// Lote 15: ver y elegir el programa que abre las firmas de las webs.
		// Solo la CLI local y la aplicación de escritorio lo reciben.
		WithProtocoloAfirma(afirmahandler.ConPolitica(afirmahandler.New(), config.LectorPoliticaProgramaAfirma)).
		WithCatalogo(servicios.catalogo).
		WithClaves(servicios.claves).
		WithLocalizador(localizador.Detectar()).
		WithVerificador(servicios.verificar).
		WithHashes(servicios.crearHash, servicios.comprobarHash).
		WithDirectoryHashes(servicios.crearHashDir, servicios.comprobarHashDir).
		WithDirectoryHashReports(servicios.informeHashDir).
		WithGestionDominios(servicios.gestionDominios).
		WithProteccion(servicios.proteger, servicios.desproteger, servicios.proteccion).
		WithProteccionFirmada(servicios.protegerFirmando).
		WithIntercambioProteccion(servicios.exportarProteccion, servicios.importarProteccion).
		WithConfigDir(servicios.configDir).
		WithVisorPDF(pdfpreview.New()).
		WithVersion(version), nil
}
