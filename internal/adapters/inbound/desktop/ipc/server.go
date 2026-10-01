// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package ipc implementa el adaptador de entrada IPC para la GUI Qt/QML.
//
// Protocolo: Unix socket (o named pipe en Windows) con mensajes JSON
// terminados en \n. Cada linea es una peticion o una respuesta independiente.
//
//	Peticion:  {"action":"sign","params":{...}}\n
//	Respuesta: {"ok":true,"action":"sign","data":{...}}\n
//
// Seguridad del socket:
//   - Unix: permisos 0600 en el fichero de socket.
//   - Windows: named pipe local con DACL limitada a la sesion actual.
//   - Verificacion de credenciales del peer via SO_PEERCRED en Linux: solo procesos
//     con el mismo UID que el servidor pueden operar.
//   - Maximo de 10 conexiones simultaneas (semaforo).
//   - Timeout inicial de 60 s para evitar slow-client antes de la primera trama.
//   - Timeout de inactividad de 30 min tras autenticar y atender al frontend.
//   - Timeout de escritura de 60 s para evitar clientes que dejen de leer.
//   - Buffer por linea limitado a 4 MB.
package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/clockdiagnostic"
	"grxfirma/internal/application"
	"grxfirma/internal/observability/correlation"
	"grxfirma/internal/ports"
)

const (
	maxConexionesSimultaneas = 10
	timeoutPrimeraLectura    = 60 * time.Second
	timeoutInactividad       = 30 * time.Minute
	timeoutEscritura         = 60 * time.Second
	maxBytesLinea            = 4 * 1024 * 1024 // 4 MB
)

// Servidor escucha en un endpoint IPC local y despacha peticiones al Manejador.
type Servidor struct {
	// SocketPath es la ruta del socket Unix o named pipe. Si esta vacio se
	// genera automaticamente.
	SocketPath string

	manejador  *Manejador
	listener   net.Listener
	mu         sync.Mutex
	cerrado    bool
	iniciando  bool
	conexiones map[net.Conn]struct{}
	semaforo   chan struct{} // limita conexiones simultaneas
	peerPID    vinculacionPIDFrontend
}

// New construye un Servidor IPC con el manejador dado.
func New(manejador *Manejador) *Servidor {
	return &Servidor{
		manejador:  manejador,
		conexiones: make(map[net.Conn]struct{}),
		semaforo:   make(chan struct{}, maxConexionesSimultaneas),
	}
}

// PrepararVinculacionPIDFrontend activa un gate fail-closed que debe quedar
// pendiente antes de abrir el listener. El launcher lo resolvera al crear el
// proceso grafico, publicando su PID exacto o denegando el arranque.
func (s *Servidor) PrepararVinculacionPIDFrontend() error {
	if !soportaVinculacionPIDPeer() {
		return errors.New("la vinculacion exacta por PID no esta soportada en esta plataforma")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil || s.iniciando {
		return errors.New("la vinculacion del frontend debe configurarse antes del listener")
	}
	return s.peerPID.preparar()
}

// PublicarPIDFrontend autoriza conexiones del proceso grafico indicado. Se
// permiten reconexiones del mismo proceso mientras el launcher siga vivo.
func (s *Servidor) PublicarPIDFrontend(pid uint32) error {
	return s.peerPID.publicar(pid)
}

// DenegarPIDFrontend resuelve o revoca el gate sin aceptar ningun PID.
func (s *Servidor) DenegarPIDFrontend() {
	s.peerPID.denegar()
}

// VincularPIDFrontend configura de una vez el PID conocido por un backend
// iniciado en modo --server por el propio frontend.
func (s *Servidor) VincularPIDFrontend(pid uint32) error {
	if err := s.PrepararVinculacionPIDFrontend(); err != nil {
		return err
	}
	if err := s.PublicarPIDFrontend(pid); err != nil {
		s.DenegarPIDFrontend()
		return err
	}
	return nil
}

// SoportaVinculacionPIDFrontend indica si el SO permite obtener del canal IPC
// el PID real del proceso peer sin confiar en datos enviados por este.
func SoportaVinculacionPIDFrontend() bool {
	return soportaVinculacionPIDPeer()
}

// Escuchar arranca el servidor en la ruta de socket indicada.
// Bloquea hasta que ctx se cancela o se llama a Cerrar.
func (s *Servidor) Escuchar(ctx context.Context, socketPath string) error {
	if socketPath == "" {
		socketPath = socketPathPorDefecto()
	}

	s.mu.Lock()
	if s.cerrado {
		s.mu.Unlock()
		return nil
	}
	if s.listener != nil || s.iniciando {
		s.mu.Unlock()
		return errors.New("el servidor IPC ya esta escuchando")
	}
	s.iniciando = true
	s.SocketPath = socketPath
	s.mu.Unlock()

	ln, err := escucharIPC(socketPath)
	if err != nil {
		s.mu.Lock()
		s.iniciando = false
		s.mu.Unlock()
		return errors.Join(errors.New("abriendo socket IPC"), err)
	}

	s.mu.Lock()
	s.iniciando = false
	if s.cerrado {
		s.mu.Unlock()
		_ = ln.Close()
		_ = limpiarIPC(socketPath)
		return nil
	}
	s.listener = ln
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.Cerrar()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			cerrado := s.cerrado
			s.mu.Unlock()
			if cerrado || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return errors.Join(errors.New("aceptando conexion IPC"), err)
		}

		// Verificar credenciales del SO y, cuando el launcher lo exige,
		// esperar la publicacion del PID exacto antes de pasar la conexion.
		peerPID, err := verificarPeer(conn)
		if err != nil {
			_ = conn.Close()
			continue
		}
		if err := s.peerPID.autorizar(ctx, peerPID); err != nil {
			_ = conn.Close()
			continue
		}
		if !s.registrarConexion(conn) {
			_ = conn.Close()
			continue
		}

		go s.servirConexion(ctx, conn)
	}
}

// Cerrar cierra el listener y elimina el endpoint cuando la plataforma lo
// representa mediante un fichero.
func (s *Servidor) Cerrar() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cerrado {
		return
	}
	s.cerrado = true
	if s.listener != nil {
		_ = s.listener.Close()
	}
	for conn := range s.conexiones {
		_ = conn.Close()
	}
	s.conexiones = nil
	if s.SocketPath != "" {
		_ = limpiarIPC(s.SocketPath)
	}
}

func (s *Servidor) registrarConexion(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cerrado {
		return false
	}
	s.conexiones[conn] = struct{}{}
	return true
}

func (s *Servidor) olvidarConexion(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conexiones, conn)
}

// servirConexion procesa todas las peticiones de una conexion hasta que el
// cliente cierra o hay un error de lectura.
func (s *Servidor) servirConexion(ctx context.Context, conn net.Conn) {
	defer func() {
		s.olvidarConexion(conn)
		_ = conn.Close()
	}()

	// Ocupar una ranura del semaforo; si esta lleno, rechazar la conexion.
	select {
	case s.semaforo <- struct{}{}:
		defer func() { <-s.semaforo }()
	default:
		// Demasiadas conexiones simultaneas.
		resp := ipcErrorResponse(
			"",
			"server_busy",
			ipcPhaseAdmission,
			"servidor ocupado, demasiadas conexiones",
			true,
		)
		_ = encodeResponseWithDeadline(conn, json.NewEncoder(conn), resp)
		return
	}

	// Un único lector observa la desconexión incluso mientras el motor está
	// ocupado. El dispatch sigue siendo secuencial; nunca se leen dos veces los
	// mismos bytes ni se acumula una cola ilimitada de peticiones/credenciales.
	ctx, cancelConnection := context.WithCancel(ctx)
	frames := make(chan []byte)
	readerDone := make(chan struct{})
	_ = conn.SetReadDeadline(time.Now().Add(timeoutPrimeraLectura))
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	go func() {
		defer close(readerDone)
		defer close(frames)
		defer cancelConnection()
		readIPCFrames(ctx, conn, frames)
	}()
	defer func() {
		cancelConnection()
		_ = conn.Close()
		stopClose()
		<-readerDone
	}()
	enc := json.NewEncoder(conn)
	sendResponse := func(resp respuesta) error {
		if err := encodeResponseWithDeadline(conn, enc, resp); err != nil {
			return err
		}
		// Sólo hay timeout de inactividad cuando se espera una petición, no
		// mientras el cliente espera al motor o a una autorización local.
		_ = conn.SetReadDeadline(time.Now().Add(timeoutInactividad))
		return nil
	}

	for {
		var linea []byte
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			linea = frame
		}
		_ = conn.SetReadDeadline(time.Time{})
		if ctx.Err() != nil {
			secmem.Zeroize(linea)
			return
		}

		var p peticion
		parseErr := json.Unmarshal(linea, &p)
		secmem.Zeroize(linea)
		if parseErr != nil {
			// A malformed request can also contain private input.
			secmem.Zeroize(p.Params)
			resp := ipcErrorResponse(
				"",
				"invalid_request",
				ipcPhaseProtocol,
				"formato de peticion invalido",
				false,
			)
			if err := sendResponse(resp); err != nil {
				return
			}
			continue
		}
		sensitive := isSensitiveIPCAction(p.Action)

		if err := validateDesktopIPCProtocol(p); err != nil {
			if sensitive {
				secmem.Zeroize(p.Params)
				p.Params = nil
			}
			errorCode := "invalid_request"
			message := "peticion IPC versionada incompleta"
			if errors.Is(err, errUnsupportedDesktopIPCProtocol) {
				errorCode = "unsupported_protocol"
				message = "protocolo IPC no soportado"
			}
			resp := ipcErrorResponse(
				"",
				errorCode,
				ipcPhaseProtocol,
				message,
				false,
			)
			if errors.Is(err, errInvalidDesktopIPCRequest) &&
				p.Protocol == desktopIPCProtocolV1 {
				resp.Protocol = desktopIPCProtocolV1
			}
			if err := sendResponse(resp); err != nil {
				return
			}
			continue
		}

		requestCtx, err := withIPCCorrelation(ctx, p)
		if err != nil {
			if sensitive {
				secmem.Zeroize(p.Params)
				p.Params = nil
			}
			// No reflejar el ID rechazado: podría contener controles o datos
			// elegidos para inyectarlos en logs y superficies de soporte.
			resp := ipcErrorResponse(
				"",
				"invalid_correlation",
				ipcPhaseProtocol,
				"identificadores de correlacion invalidos",
				false,
			)
			resp.Protocol = p.Protocol
			if err := sendResponse(resp); err != nil {
				return
			}
			continue
		}

		var resp respuesta
		if isLocalTokenSettingsAction(p.Action) {
			var allowed bool
			requestCtx, allowed = s.authorizeLocalTokenSettings(requestCtx, conn)
			if !allowed {
				resp = tokenSettingsFrontendRequired(p.Action)
			} else {
				resp = s.manejador.despacharConTimeout(requestCtx, p)
			}
		} else {
			resp = s.manejador.despacharConTimeout(requestCtx, p)
		}
		if sensitive {
			// Params es la copia controlable que conserva json.RawMessage. Los
			// strings creados por json.Unmarshal se limitan al dispatch y el
			// caso de uso zeroiza la clave binaria decodificada.
			secmem.Zeroize(p.Params)
			p.Params = nil
		}
		resp.Protocol = p.Protocol
		resp.RequestID = p.RequestID
		resp.TraceID = effectiveIPCTraceID(p)
		if err := sendResponse(resp); err != nil {
			return
		}
	}
}

// readIPCFrames conserva framing y límites. Hay como máximo una trama copiada
// pendiente además del buffer del scanner. Bajo backpressure por pipelining el
// lector puede esperar al dispatch y la detección de EOF se retrasa; el cliente
// desktop normal espera una respuesta antes de enviar la siguiente petición.
// EOF (también half-close de escritura) termina la sesión, no es un mensaje de
// commit. El protocolo persistente no utiliza half-close para pedir respuestas.
func readIPCFrames(ctx context.Context, conn net.Conn, frames chan<- []byte) {
	scanner := bufio.NewScanner(conn)
	buffer := make([]byte, maxBytesLinea)
	defer secmem.Zeroize(buffer)
	scanner.Buffer(buffer, maxBytesLinea)
	for {
		if !scanner.Scan() {
			return
		}
		if len(scanner.Bytes()) == 0 {
			continue
		}
		frame := append([]byte(nil), scanner.Bytes()...)
		secmem.Zeroize(scanner.Bytes())
		select {
		case frames <- frame:
		case <-ctx.Done():
			secmem.Zeroize(frame)
			return
		}
	}
}

func withIPCCorrelation(ctx context.Context, p peticion) (context.Context, error) {
	// La ausencia completa se conserva para clientes anteriores a requestId.
	// En cambio, un campo presente vacío/null es una entrada inválida.
	if (p.requestIDPresent && p.RequestID == "") ||
		(p.traceIDPresent && p.TraceID == "") {
		return ctx, correlation.ErrInvalidID
	}
	if p.RequestID == "" && p.TraceID == "" {
		return correlation.WithGenerated(ctx)
	}
	return correlation.With(ctx, p.RequestID, p.TraceID)
}

func isSensitiveIPCAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "proxy_secret_store", "import_certificate", "import_certificate_to_store", "certificate_export_public",
		"use_temporary_certificate", "protect", "protect_sign", "unprotect", "save_token_settings":
		return true
	default:
		return false
	}
}

func encodeResponseWithDeadline(conn net.Conn, enc *json.Encoder, resp respuesta) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeoutEscritura)); err != nil {
		return err
	}
	err := enc.Encode(resp)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

func effectiveIPCTraceID(p peticion) string {
	traceID := p.TraceID
	if traceID == "" {
		traceID = p.RequestID
	}
	return traceID
}

// ---------------------------------------------------------------------------
// Opcion fluent para inyectar dependencias opcionales
// ---------------------------------------------------------------------------

// WithCatalogo inyecta el catalogo de certificados.
func (s *Servidor) WithCatalogo(c ports.CertificateCatalog) *Servidor {
	s.manejador.Catalogo = c
	return s
}

// WithClaves inyecta el proveedor de claves/cadena de certificados.
func (s *Servidor) WithClaves(p ports.SigningKeyProvider) *Servidor {
	s.manejador.Claves = p
	return s
}

// WithFirmar inyecta el caso de uso de firma.
func (s *Servidor) WithFirmar(uc SignDocumentUseCase) *Servidor {
	s.manejador.Firmar = uc
	return s
}

// WithMultiCofirmar inyecta el caso de uso de cofirma múltiple guiada.
func (s *Servidor) WithMultiCofirmar(uc MultiCoSignUseCase) *Servidor {
	s.manejador.MultiCofirmar = uc
	return s
}

// WithProcesarLote inyecta el caso de uso de firma por lote.
func (s *Servidor) WithProcesarLote(uc ProcessBatchUseCase) *Servidor {
	s.manejador.ProcesarLote = uc
	return s
}

// WithVerificar inyecta el caso de uso de verificacion.
func (s *Servidor) WithVerificar(uc VerifySignatureUseCase) *Servidor {
	s.manejador.Verificar = uc
	return s
}

// WithProtection inyecta los casos de uso de protección y el catálogo de destinatarios.
func (s *Servidor) WithProtection(proteger ProtectDocumentUseCase, protegerFirmando ProtectAndSignDocumentUseCase, desproteger UnprotectDocumentUseCase, destinatarios ProtectionRecipients) *Servidor {
	s.manejador.Proteger = proteger
	s.manejador.ProtegerFirmando = protegerFirmando
	s.manejador.Desproteger = desproteger
	s.manejador.Destinatarios = destinatarios
	return s
}

// WithHashes inyecta los casos de uso de hash de fichero.
func (s *Servidor) WithHashes(crear CreateHashUseCase, comprobar CheckHashUseCase) *Servidor {
	s.manejador.CrearHash = crear
	s.manejador.ComprobarHash = comprobar
	return s
}

// WithDirectoryHashes inyecta los casos de uso de hash de directorio.
func (s *Servidor) WithDirectoryHashes(crear CreateDirectoryHashUseCase, comprobar CheckDirectoryHashUseCase) *Servidor {
	s.manejador.CrearHashDir = crear
	s.manejador.ComprobarHashDir = comprobar
	return s
}

// WithDirectoryHashReports inyecta el codec de informes .hashreport.
func (s *Servidor) WithDirectoryHashReports(codec ports.DirectoryHashReportCodec) *Servidor {
	s.manejador.InformeHashDir = codec
	return s
}

// WithPreview inyecta el caso de uso de PDF preview.
func (s *Servidor) WithPreview(uc PdfPreviewUseCase) *Servidor {
	s.manejador.Preview = uc
	return s
}

// WithServicio inyecta el gestor de servicio.
func (s *Servidor) WithServicio(g ports.GestorServicio) *Servidor {
	s.manejador.Servicio = g
	return s
}

// WithSettings inyecta el adaptador de configuracion de usuario.
func (s *Servidor) WithSettings(c ports.ConfiguracionUsuario) *Servidor {
	s.manejador.Settings = c
	return s
}

// WithLocalTokenSettings configures the administrative service exclusively for
// the PID-bound desktop frontend. It is not a browser/REST signing option.
func (s *Servidor) WithLocalTokenSettings(settings ports.LocalTokenSettings) *Servidor {
	s.manejador.TokenSettings = settings
	return s
}

// WithUpdates inyecta la consulta informativa de GitHub Releases y la versión
// exacta del motor. La comprobación nunca descarga ni ejecuta artefactos.
func (s *Servidor) WithUpdates(checker UpdateChecker, currentVersion string) *Servidor {
	s.manejador.UpdateChecker = checker
	s.manejador.CurrentVersion = strings.TrimSpace(currentVersion)
	return s
}

// WithProxySecrets inyecta el adaptador de secretos de proxy por SO.
func (s *Servidor) WithProxySecrets(store ports.ProxySecretStore) *Servidor {
	s.manejador.ProxySecrets = store
	return s
}

// WithImportador inyecta el importador de certificados.
func (s *Servidor) WithImportador(i ports.CertificateImporter) *Servidor {
	s.manejador.Importador = i
	return s
}

// WithCertificateAccess inyecta los flujos explícitos de almacén persistente y
// de credenciales temporales en memoria.
func (s *Servidor) WithCertificateAccess(access *application.CertificateAccessUseCase, temporary *application.TemporaryCertificateUseCase) *Servidor {
	s.manejador.CertificateAccess = access
	s.manejador.TemporaryCertificates = temporary
	return s
}

// WithLocalizador inyecta el localizador de mensajes.
func (s *Servidor) WithLocalizador(l ports.Localizador) *Servidor {
	s.manejador.Loc = l
	return s
}

// WithConfigDir inyecta el directorio de configuracion.
func (s *Servidor) WithConfigDir(dir string) *Servidor {
	s.manejador.ConfigDir = dir
	return s
}

// NuevoServidor construye un Servidor con el manejador completamente configurado.
// Es el constructor de conveniencia para el entry point GUI.
func NuevoServidor(
	catalogo ports.CertificateCatalog,
	firmar SignDocumentUseCase,
	multiCofirmar MultiCoSignUseCase,
	procesarLote ProcessBatchUseCase,
	verificar VerifySignatureUseCase,
	crearHash CreateHashUseCase,
	comprobarHash CheckHashUseCase,
	crearHashDir CreateDirectoryHashUseCase,
	comprobarHashDir CheckDirectoryHashUseCase,
	informeHashDir ports.DirectoryHashReportCodec,
	preview PdfPreviewUseCase,
	servicio ports.GestorServicio,
	settings ports.ConfiguracionUsuario,
	importador ports.CertificateImporter,
	loc ports.Localizador,
	configDir string,
) *Servidor {
	m := &Manejador{
		Catalogo:         catalogo,
		Firmar:           firmar,
		MultiCofirmar:    multiCofirmar,
		ProcesarLote:     procesarLote,
		Verificar:        verificar,
		CrearHash:        crearHash,
		ComprobarHash:    comprobarHash,
		CrearHashDir:     crearHashDir,
		ComprobarHashDir: comprobarHashDir,
		InformeHashDir:   informeHashDir,
		Preview:          preview,
		Servicio:         servicio,
		Settings:         settings,
		Importador:       importador,
		Loc:              loc,
		ConfigDir:        configDir,
		ClockDiagnostics: clockdiagnostic.New(nil),
	}
	return New(m)
}
