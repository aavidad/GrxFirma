// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package session implementa el orquestador de sesión GUI para el protocolo afirma://.
// Coordina la validación de origen, la selección de certificado, la importación de clave
// y la ejecución del protocolo trifásico (simple o batch), mostrando progreso al usuario.
package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
	"grxfirma/presentation/desktop/certpicker"
	"grxfirma/presentation/desktop/progressdialog"
)

// Orchestrator coordina una sesión de firma afirma://:
//  1. Valida el origen de la solicitud (TrustPolicy).
//  2. Lista los certificados disponibles y muestra el selector.
//  3. Importa la clave de firma (P12/PKCS#11).
//  4. Ejecuta el protocolo trifásico (simple o batch).
//  5. Muestra progreso y notificación de resultado.
type Orchestrator struct {
	trustValidator                      *originvalidator.Validator // T055
	legacyParser                        *afirmauri.Adaptador
	certCatalog                         ports.CertificateCatalog // T036
	keyProvider                         ports.SigningKeyProvider
	certSelector                        certpicker.CertSelector         // T038
	signer                              ports.SignerEngine              // motor de firma
	triphaseExec                        simpleExecutor                  // T043
	batchExec                           batchExecutor                   // T044
	progress                            progressdialog.ProgressProvider // T039
	notify                              ports.DesktopNotification       // T041
	eventos                             ports.EventPublisher
	preferencias                        preferenceStore
	requireExplicitCertificateSelection bool
	timeout                             time.Duration
}

type preferenceStore interface {
	LoadSession(ctx context.Context, origin string) (string, bool, error)
	SaveSession(ctx context.Context, origin, certificateID string) error
	LoadPersistent(ctx context.Context, origin string) (string, bool, error)
	SavePersistent(ctx context.Context, origin, certificateID string) error
}

type simpleExecutor interface {
	Execute(ctx context.Context, session domain.ExchangeSession, job domain.SignatureJob) (domain.SignatureResult, error)
	RetrieveRaw(ctx context.Context, session domain.ExchangeSession) ([]byte, error)
	Upload(ctx context.Context, session domain.ExchangeSession, data []byte) error
	UploadSignature(ctx context.Context, session domain.ExchangeSession, certDER, signature []byte) error
	UploadCertificate(ctx context.Context, session domain.ExchangeSession, certDER []byte, legacyParams url.Values) error
	FirmarConServidor(ctx context.Context, req triphase.SolicitudFirmaServidor) ([]byte, error)
}

type batchExecutor interface {
	Execute(ctx context.Context, jobs []triphase.BatchJob) []triphase.BatchResult
}

type remoteBatchExecutor interface {
	ExecuteLegacyRemote(ctx context.Context, req afirmauri.RemoteBatchCommand) error
}

// Config agrupa las dependencias necesarias para construir un Orchestrator.
type Config struct {
	TrustValidator                      *originvalidator.Validator
	ParserLegacy                        *afirmauri.Adaptador
	CertCatalog                         ports.CertificateCatalog
	KeyProvider                         ports.SigningKeyProvider
	CertSelector                        certpicker.CertSelector
	Signer                              ports.SignerEngine
	TriphaseExec                        simpleExecutor
	BatchExec                           batchExecutor
	Progress                            progressdialog.ProgressProvider
	Notify                              ports.DesktopNotification
	Eventos                             ports.EventPublisher
	Preferencias                        preferenceStore
	RequireExplicitCertificateSelection bool
	Timeout                             time.Duration
}

// New construye un Orchestrator con la configuración proporcionada.
func New(cfg Config) *Orchestrator {
	return &Orchestrator{
		trustValidator:                      cfg.TrustValidator,
		legacyParser:                        cfg.ParserLegacy,
		certCatalog:                         cfg.CertCatalog,
		keyProvider:                         cfg.KeyProvider,
		certSelector:                        cfg.CertSelector,
		signer:                              cfg.Signer,
		triphaseExec:                        cfg.TriphaseExec,
		batchExec:                           cfg.BatchExec,
		progress:                            cfg.Progress,
		notify:                              cfg.Notify,
		eventos:                             cfg.Eventos,
		preferencias:                        cfg.Preferencias,
		requireExplicitCertificateSelection: cfg.RequireExplicitCertificateSelection,
		timeout:                             resolverTimeout(cfg.Timeout),
	}
}

// HandleRequest procesa una solicitud afirma:// completa siguiendo el flujo:
//  1. Valida el origen de la solicitud.
//  2. Lista los certificados disponibles.
//  3. Muestra el diálogo de progreso.
//  4. El usuario selecciona el certificado.
//  5. Ejecuta el protocolo trifásico (simple o batch).
//  6. Actualiza el progreso y cierra el diálogo.
//  7. Notifica el resultado al usuario.
func (o *Orchestrator) HandleRequest(ctx context.Context, solicitud afirmauri.Solicitud) error {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	release, err := certpicker.BeginCredentialOperation(ctx, o.certSelector)
	if err != nil {
		return err
	}
	defer release()
	if solicitud.Operacion != afirmauri.OperacionSelectCert {
		defer certpicker.ClearCredentials(o.certSelector)
	}
	o.publicar(ctx, "session.start", "inicio de sesion afirma://")

	// Paso 1: Validar el origen de la solicitud.
	if err := o.validarOrigenes(ctx, solicitud); err != nil {
		o.publicarError(ctx, "session.error.trust", err)
		o.notificar(ctx, "Solicitud rechazada", err.Error())
		return fmt.Errorf("validación de origen fallida: %w", err)
	}
	o.publicar(ctx, "session.trust.ok", "origen validado")

	// Paso 2: Obtener la lista de certificados disponibles.
	certs, err := o.certCatalog.List(ctx)
	if err != nil {
		o.publicarError(ctx, "session.error.catalog", err)
		o.notificar(ctx, "Error de firma", fmt.Sprintf("No se pudieron obtener los certificados: %s", err.Error()))
		return fmt.Errorf("error listando certificados: %w", err)
	}
	if len(certs) == 0 && !certpicker.SupportsCredentialLoading(o.certSelector) {
		o.publicar(ctx, "session.error.catalog.empty", "sin certificados disponibles")
		o.notificar(ctx, "Error de firma", "No hay certificados disponibles para firmar.")
		return errors.New("no hay certificados disponibles")
	}
	o.publicar(ctx, "session.catalog.ok", fmt.Sprintf("certificados=%d", len(certs)))
	certsFiltrados, detalleFiltro := filtrarCatalogoLegacy(certs, solicitud.Options)
	log.Printf(
		"[Session] candidatos certificados total=%d filtrados=%d fallback=%t filtro=%s operacion=%s",
		len(certs),
		len(certsFiltrados),
		detalleFiltro.fallback,
		detalleFiltro.descripcion,
		solicitud.Operacion,
	)
	o.publicar(ctx, "session.catalog.filtered", fmt.Sprintf("total=%d filtrados=%d fallback=%t filtro=%s", len(certs), len(certsFiltrados), detalleFiltro.fallback, detalleFiltro.descripcion))

	// Paso 3: Seleccionar certificado.
	// No abrimos el diálogo de progreso antes del selector en ninguna operación.
	// En Fyne ese diálogo es modal y puede dejar la lista visible pero no
	// interactuable si se superpone al selector.
	var reporter progressdialog.Reporter
	o.publicar(ctx, "session.progress.select_certificate", "seleccionando certificado")

	// Comprobar si el contexto fue cancelado antes de continuar.
	if err := ctx.Err(); err != nil {
		o.publicarError(ctx, "session.error.context", err)
		return err
	}

	// Paso 4: Seleccionar el certificado.
	origenPreferencia := primerOrigen(solicitud.Origenes)
	certSeleccionado, automatico, err := o.seleccionarCertificado(ctx, solicitud.Operacion, origenPreferencia, certsFiltrados)
	if err != nil {
		if errors.Is(err, certpicker.ErrSeleccionCancelada) {
			o.publicarError(ctx, "session.error.selection_cancelled", err)
			o.notificar(ctx, "Firma cancelada", "El usuario canceló la selección de certificado.")
			return fmt.Errorf("selección de certificado cancelada: %w", err)
		}
		o.publicarError(ctx, "session.error.selection", err)
		o.notificar(ctx, "Error de firma", fmt.Sprintf("Error en la selección de certificado: %s", err.Error()))
		return fmt.Errorf("error en la selección de certificado: %w", err)
	}
	if certSeleccionado.IsExpired(time.Now()) {
		return errors.New("el certificado seleccionado está caducado; utilice otro certificado vigente")
	}
	if aviso := application.AvisoCaducidadCertificado(certSeleccionado, time.Now()); aviso != "" {
		avisos.Registrar("Certificado próximo a caducar", aviso)
	}
	o.publicar(ctx, "session.selection.ok", certSeleccionado.ID)
	log.Printf(
		"[Session] certificado seleccionado automatico=%t operacion=%s",
		automatico,
		solicitud.Operacion,
	)

	reporter = o.progress.MostrarProgreso(ctx, "Firma electrónica")
	defer reporter.Cerrar()
	reporter.SetMensaje("Realizando la firma...")
	reporter.SetProgreso(0.3)

	clave, err := o.resolverClave(ctx, certSeleccionado)
	if err != nil {
		ports.CloseSigningKey(clave)
		o.publicarError(ctx, "session.error.key_resolution", err)
		o.notificar(ctx, "Error de firma", fmt.Sprintf("No se pudo preparar la clave de firma: %s", err.Error()))
		return fmt.Errorf("error resolviendo la clave de firma: %w", err)
	}
	defer ports.CloseSigningKey(clave)
	ctx = triphase.ContextWithSigningKey(ctx, clave)

	// Paso 5: Ejecutar el protocolo trifásico (simple o batch).
	o.publicar(ctx, "session.progress.sign", "ejecutando protocolo de firma")

	if err := o.ejecutarProtocolo(ctx, solicitud, certSeleccionado, clave); err != nil {
		reporter.SetProgreso(1.0)
		o.publicarError(ctx, "session.error.protocol", err)
		o.notificar(ctx, "Firma cancelada", fmt.Sprintf("La firma no se pudo completar: %s", err.Error()))
		return fmt.Errorf("error en el protocolo de firma: %w", err)
	}

	// Paso 6: Actualizar progreso y cerrar el diálogo.
	reporter.SetMensaje("Firma completada.")
	reporter.SetProgreso(1.0)
	o.publicar(ctx, "session.complete", "firma completada")

	// Paso 7: Notificar el resultado al usuario.
	o.notificar(ctx, "Firma completada", "El documento ha sido firmado correctamente.")
	return nil
}

func (o *Orchestrator) seleccionarCertificado(ctx context.Context, operacion afirmauri.TipoOperacion, origen string, certs []domain.CertificateRef) (domain.CertificateRef, bool, error) {
	if !o.requireExplicitCertificateSelection && !certpicker.SupportsCredentialLoading(o.certSelector) {
		if cert, ok, err := o.certificadoPreferido(ctx, operacion, origen, certs); err != nil {
			return domain.CertificateRef{}, false, err
		} else if ok {
			return cert, true, nil
		}
		// Con un único certificado ya no se elige en silencio: el selector
		// es el consentimiento del usuario (igual que en el canal WebSocket).
	}
	if len(certs) == 0 && !certpicker.SupportsCredentialLoading(o.certSelector) {
		return domain.CertificateRef{}, false, errors.New("no hay certificados compatibles: ninguno cumple los requisitos de la web")
	}

	if o.certSelector == nil {
		return domain.CertificateRef{}, false, errors.New("selector de certificado no configurado")
	}
	resultado, err := o.certSelector.Select(certpicker.ConContextoSolicitud(ctx, origen, string(operacion)), certs)
	if err != nil {
		return domain.CertificateRef{}, false, err
	}
	if err := o.recordarSeleccion(ctx, origen, resultado); err != nil {
		log.Printf("[Session] aviso guardando preferencia de certificado origen=%s cert=%s err=%v", origen, resultado.Certificado.ID, err)
	}
	return resultado.Certificado, false, nil
}

func (o *Orchestrator) certificadoPreferido(ctx context.Context, operacion afirmauri.TipoOperacion, origen string, certs []domain.CertificateRef) (domain.CertificateRef, bool, error) {
	if o.preferencias == nil || strings.TrimSpace(origen) == "" {
		return domain.CertificateRef{}, false, nil
	}
	// Para selectcert se omite la preferencia de sesión (no persistente) pero sí
	// se respeta la preferencia persistente explícita del usuario.
	if operacion != afirmauri.OperacionSelectCert {
		if id, ok, err := o.preferencias.LoadSession(ctx, origen); err != nil {
			return domain.CertificateRef{}, false, err
		} else if ok {
			if cert, found := buscarCertificadoPorID(certs, id); found {
				return cert, true, nil
			}
		}
	}
	if id, ok, err := o.preferencias.LoadPersistent(ctx, origen); err != nil {
		return domain.CertificateRef{}, false, err
	} else if ok {
		if cert, found := buscarCertificadoPorID(certs, id); found {
			return cert, true, nil
		}
	}
	return domain.CertificateRef{}, false, nil
}

func (o *Orchestrator) recordarSeleccion(ctx context.Context, origen string, resultado certpicker.ResultadoSeleccion) error {
	if o.preferencias == nil || strings.TrimSpace(origen) == "" {
		return nil
	}
	switch resultado.Recuerdo {
	case certpicker.RecordarSesion:
		return o.preferencias.SaveSession(ctx, origen, resultado.Certificado.ID)
	case certpicker.RecordarSiempre:
		return o.preferencias.SavePersistent(ctx, origen, resultado.Certificado.ID)
	default:
		return nil
	}
}

func buscarCertificadoPorID(certs []domain.CertificateRef, id string) (domain.CertificateRef, bool) {
	for _, cert := range certs {
		if cert.ID == id {
			return cert, true
		}
	}
	return domain.CertificateRef{}, false
}

func primerOrigen(origenes []string) string {
	for _, origen := range origenes {
		if strings.TrimSpace(origen) != "" {
			return strings.TrimSpace(origen)
		}
	}
	return ""
}

// validarOrigenes valida todos los orígenes declarados en la solicitud.
func (o *Orchestrator) validarOrigenes(ctx context.Context, solicitud afirmauri.Solicitud) error {
	if o.trustValidator == nil {
		return nil
	}
	for _, origen := range solicitud.Origenes {
		if err := o.trustValidator.Validate(ctx, origen); err != nil {
			return err
		}
	}
	return nil
}

// ejecutarProtocolo protege el flujo de firma decidiendo el modo (simple, batch o identificación).
func (o *Orchestrator) ejecutarProtocolo(ctx context.Context, solicitud afirmauri.Solicitud, cert domain.CertificateRef, clave ports.SigningKey) error {
	if solicitud.Operacion == afirmauri.OperacionSelectCert {
		return o.ejecutarIdentificacion(ctx, solicitud, cert, clave)
	}

	if solicitud.RemoteBatch != nil {
		return o.ejecutarBatchRemoto(ctx, solicitud)
	}

	if solicitud.Operacion == afirmauri.OperacionLote && solicitud.RetrieveCommand != nil {
		return o.ejecutarBatchRecuperado(ctx, solicitud, clave)
	}

	// Modo batch: la solicitud contiene un BatchCommand con múltiples jobs.
	if solicitud.BatchCommand != nil && len(solicitud.BatchCommand.Jobs) > 1 {
		return o.ejecutarBatch(ctx, solicitud)
	}

	// Modo simple: ejecutar el protocolo trifásico para un único documento.
	return o.ejecutarSimple(ctx, solicitud, clave)
}

// ejecutarIdentificacion realiza la subida del certificado DER para autenticación remota.
func (o *Orchestrator) ejecutarIdentificacion(ctx context.Context, solicitud afirmauri.Solicitud, _ domain.CertificateRef, clave ports.SigningKey) error {
	log.Printf("[Session] identificacion start upload=%s request_id=%s key=%t origenes=%v", solicitud.Sesion.UploadEndpoint, solicitud.Sesion.RequestID, strings.TrimSpace(solicitud.Sesion.SessionKey) != "", solicitud.Origenes)
	if clave == nil {
		return errors.New("identificación fallida: no se pudo obtener la clave de firma")
	}
	chain := clave.CertificateChainDER()
	if len(chain) == 0 {
		return errors.New("identificación fallida: el certificado seleccionado no tiene datos DER")
	}

	// En Autofirma V1, la identificación sube el certificado DER con una
	// codificación distinta a la firma normal.
	log.Printf("[Session] identificacion upload_certificate der_len=%d", len(chain[0]))
	return o.triphaseExec.UploadCertificate(ctx, solicitud.Sesion, chain[0], solicitud.LegacyParams)
}

func (o *Orchestrator) ejecutarBatchRemoto(ctx context.Context, solicitud afirmauri.Solicitud) error {
	if solicitud.RemoteBatch == nil {
		return errors.New("la solicitud no contiene batch remoto")
	}
	if o.batchExec == nil {
		return errors.New("el ejecutor batch no está configurado")
	}
	remoteExec, ok := o.batchExec.(remoteBatchExecutor)
	if !ok {
		return errors.New("el ejecutor batch no soporta lotes remotos legacy")
	}
	return remoteExec.ExecuteLegacyRemote(ctx, *solicitud.RemoteBatch)
}

func (o *Orchestrator) ejecutarBatchRecuperado(ctx context.Context, solicitud afirmauri.Solicitud, clave ports.SigningKey) error {
	if o.triphaseExec == nil {
		return errors.New("el ejecutor trifásico no está configurado")
	}
	if o.legacyParser == nil {
		return errors.New("el parser legacy no está configurado")
	}
	raw, err := o.triphaseExec.RetrieveRaw(ctx, solicitud.Sesion)
	if err != nil {
		return fmt.Errorf("retrieve remoto del lote fallido: %w", err)
	}
	remote, batchCmd, origenes, err := afirmauri.ResolveRetrievedBatch(raw, solicitud.Sesion)
	if err != nil {
		return fmt.Errorf("payload batch remoto inválido: %w", err)
	}
	if len(origenes) > 0 {
		if err := o.validarOrigenes(ctx, afirmauri.Solicitud{Origenes: origenes}); err != nil {
			return fmt.Errorf("validación de origen del lote recuperado fallida: %w", err)
		}
	}
	if remote != nil {
		clon := solicitud
		clon.RemoteBatch = remote
		clon.RetrieveCommand = nil
		return o.ejecutarBatchRemoto(ctx, clon)
	}
	if batchCmd != nil {
		clon := solicitud
		clon.BatchCommand = batchCmd
		clon.RetrieveCommand = nil
		if len(batchCmd.Jobs) > 1 {
			return o.ejecutarBatch(ctx, clon)
		}
		return o.ejecutarSimple(ctx, clon, clave)
	}
	return errors.New("el retrieve remoto no produjo un lote procesable")
}

// ejecutarSimple ejecuta el protocolo trifásico para un único job.
func (o *Orchestrator) ejecutarSimple(ctx context.Context, solicitud afirmauri.Solicitud, clave ports.SigningKey) error {
	if o.triphaseExec == nil {
		return errors.New("el ejecutor trifásico no está configurado")
	}
	if solicitud.RetrieveCommand != nil {
		raw, err := o.triphaseExec.RetrieveRaw(ctx, solicitud.Sesion)
		if err != nil {
			return fmt.Errorf("retrieve remoto fallido: %w", err)
		}
		raw, err = triphase.DecodeRetrievePayload(raw, solicitud.Sesion.SessionKey, solicitud.Sesion.RetrieveEndpoint, solicitud.Sesion.UploadEndpoint)
		if err != nil {
			return fmt.Errorf("retrieve remoto: %w", err)
		}
		if cmd, resuelta, ok, err := afirmauri.ResolveRetrievedSign(raw, solicitud); err != nil {
			return fmt.Errorf("payload remoto de firma inválido: %w", err)
		} else if ok {
			if o.signer == nil {
				return errors.New("el motor de firma no está configurado")
			}
			signResult, err := o.signer.Sign(ctx, domain.SignatureJob{
				Document: cmd.Document,
				Format:   cmd.Format,
				Action:   cmd.Action,
				Options:  cmd.Options,
			}, clave)
			if err != nil {
				return fmt.Errorf("firma local fallida: %w", err)
			}
			if clave == nil || len(clave.CertificateChainDER()) == 0 {
				return errors.New("la clave de firma seleccionada no tiene certificado DER")
			}
			if err := o.triphaseExec.UploadSignature(ctx, resuelta.Sesion, clave.CertificateChainDER()[0], signResult.Data); err != nil {
				return fmt.Errorf("subida de firma legacy fallida: %w", err)
			}
			return nil
		}
	}

	var job domain.SignatureJob
	if solicitud.SignCommand != nil {
		job = domain.SignatureJob{
			Document: solicitud.SignCommand.Document,
			Format:   solicitud.SignCommand.Format,
			Action:   solicitud.SignCommand.Action,
			Options:  solicitud.SignCommand.Options,
		}
	} else if solicitud.BatchCommand != nil && len(solicitud.BatchCommand.Jobs) == 1 {
		job = solicitud.BatchCommand.Jobs[0]
	} else {
		// Protocolo trifásico puro: el job se recupera del servidor remoto.
		job = domain.SignatureJob{
			Format:  solicitud.Formato,
			Action:  solicitud.AccionFirma,
			Options: solicitud.Options,
		}
	}

	// La web puede pedir que el usuario sitúe la firma visible (Java:
	// visibleSignature=want).
	if solicitud.SignCommand != nil {
		opciones, err := certpicker.ResolverSelloVisible(ctx, o.certSelector, job.Format, job.Options)
		if err != nil {
			return err
		}
		job.Options = opciones
	}

	// Formatos "tri" con serverUrl (FIRe, @firma): firma trifásica real contra
	// el servidor, como AutoFirma Java; "dat" es el documento o su
	// identificador y la clave solo firma el resumen recibido.
	if formato := solicitud.LegacyParams.Get("format"); solicitud.SignCommand != nil && triphase.EsFormatoTrifasico(formato) {
		if servidor := strings.TrimSpace(job.Options["serverUrl"]); servidor != "" {
			if clave == nil || len(clave.CertificateChainDER()) == 0 {
				return errors.New("la clave de firma seleccionada no tiene certificado DER")
			}
			firma, err := o.triphaseExec.FirmarConServidor(triphase.ContextWithSigningKey(ctx, clave), triphase.SolicitudFirmaServidor{
				ServerURL: servidor, FormatoLegacy: formato, Accion: job.Action,
				Algoritmo: job.Options["algorithm"], Datos: job.Document.Content, ExtraParams: job.Options,
			})
			if err != nil {
				return err
			}
			if err := o.triphaseExec.UploadSignature(ctx, solicitud.Sesion, clave.CertificateChainDER()[0], firma); err != nil {
				return fmt.Errorf("subida de la firma trifásica fallida: %w", err)
			}
			return nil
		}
	}

	_, err := o.triphaseExec.Execute(ctx, solicitud.Sesion, job)
	return err
}

// ejecutarBatch ejecuta el protocolo trifásico en modo batch.
func (o *Orchestrator) ejecutarBatch(ctx context.Context, solicitud afirmauri.Solicitud) error {
	if o.batchExec == nil {
		return errors.New("el ejecutor batch no está configurado")
	}
	if solicitud.BatchCommand == nil {
		return errors.New("la solicitud batch no contiene datos de lote")
	}

	batchJobs := make([]triphase.BatchJob, 0, len(solicitud.BatchCommand.Jobs))
	for _, j := range solicitud.BatchCommand.Jobs {
		batchJobs = append(batchJobs, triphase.BatchJob{
			Job:     j,
			Session: solicitud.Sesion,
		})
	}

	if solicitud.BatchCommand.StopOnError {
		for index, job := range batchJobs {
			if err := ctx.Err(); err != nil {
				return err
			}
			resultados := o.batchExec.Execute(ctx, []triphase.BatchJob{job})
			if len(resultados) != 1 {
				return fmt.Errorf(
					"el ejecutor batch devolvió %d resultados para el trabajo %d",
					len(resultados),
					index,
				)
			}
			if resultados[0].Err != nil {
				return fmt.Errorf(
					"el lote se detuvo en el trabajo %d: %w",
					index,
					resultados[0].Err,
				)
			}
		}
		return nil
	}

	resultados := o.batchExec.Execute(ctx, batchJobs)

	// Recopilar errores del batch.
	var errs []error
	for _, r := range resultados {
		if r.Err != nil {
			errs = append(errs, r.Err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("el lote completó con %d error(es): %w", len(errs), errs[0])
	}
	return nil
}

// notificar envía una notificación al usuario ignorando el error (best-effort).
func (o *Orchestrator) notificar(ctx context.Context, titulo, cuerpo string) {
	if o.notify == nil {
		return
	}
	_ = o.notify.Notify(ctx, titulo, cuerpo)
}

func (o *Orchestrator) resolverClave(ctx context.Context, cert domain.CertificateRef) (ports.SigningKey, error) {
	if o.keyProvider == nil {
		return nil, errors.New("proveedor de claves no configurado")
	}
	return o.keyProvider.KeyFor(ctx, cert)
}

func (o *Orchestrator) publicar(ctx context.Context, tipo, payload string) {
	if o.eventos == nil {
		return
	}
	_ = o.eventos.Publish(ctx, ports.Event{
		Type:      tipo,
		Timestamp: time.Now().UTC(),
		Payload:   []byte(payload),
	})
}

func (o *Orchestrator) publicarError(ctx context.Context, tipo string, err error) {
	if err == nil {
		return
	}
	o.publicar(ctx, tipo, err.Error())
}

func resolverTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return 5 * time.Minute
}

type detalleFiltroLegacy struct {
	descripcion string
	fallback    bool
}

func filtrarCatalogoLegacy(certs []domain.CertificateRef, options map[string]string) ([]domain.CertificateRef, detalleFiltroLegacy) {
	// Mismo motor de filtros que el canal WebSocket (CertFilterManager de
	// AutoFirma Java). Sin fallback: si la web acota y nada cumple, no se
	// ofrecen certificados que la web ha excluido.
	res := application.FiltrarCertificados(certs, options, time.Now())
	return res.Certificados, detalleFiltroLegacy{descripcion: res.Descripcion}
}
