// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package cscremota mantiene en el motor de escritorio la sesión de firma
// remota CSC que manejan las interfaces gráficas a través del IPC.
//
// La sesión guarda el cliente CSC, los certificados remotos de la cuenta y la
// configuración (dirección del servicio e identificador de cliente). Es a la
// vez un catálogo de certificados y un proveedor de claves, de modo que las
// credenciales remotas aparecen junto a las locales y se firman con el mismo
// motor. Los tokens nunca salen de este paquete ni del cliente CSC.
//
// Cada operación vuelve a leer la configuración y la política: si la firma
// remota deja de estar permitida, la sesión se cierra y no se ofrece ningún
// certificado remoto. El PIN y el OTP viajan en el contexto de la petición
// de firma que los trae ([ContextoConSecretos]) y solo los lee esa petición.
package cscremota

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/net/idna"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/adapters/outbound/common/securefile"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const (
	// FicheroConfiguracion guarda la dirección del servicio y el client_id.
	// No contiene secretos.
	FicheroConfiguracion = "firma_remota_csc.json"

	maxURLServicio      = 2048
	maxFicheroConfig    = 16 * 1024
	maxNombreServicio   = 120
	esperaAutorizacion  = 4 * time.Minute
	maxSecretoPeticion  = 256
	maxCredencialesVist = 100
)

// Códigos propios de la sesión. Se traducen como los del cliente CSC, con la
// clave de catálogo "csc.error.<codigo>".
const (
	CodigoDesactivada         csc.Codigo = "desactivada"
	CodigoProhibida           csc.Codigo = "prohibida"
	CodigoNoConfigurada       csc.Codigo = "no_configurada"
	CodigoNoConectada         csc.Codigo = "no_conectada"
	CodigoParesOAuthInvalidos csc.Codigo = "pares_oauth_invalidos"
	CodigoOTPLote             csc.Codigo = "otp_lote"
	CodigoSecretoNoPedido     csc.Codigo = "secreto_no_pedido"
	// CodigoAdicionalConSecretos: en una multifirma solo el firmante
	// principal puede usar un certificado remoto que pide PIN u OTP.
	CodigoAdicionalConSecretos csc.Codigo = "adicional_con_secretos" // #nosec G101 -- código de error, no un secreto.
	// CodigoProtegerConSecretos: «proteger y firmar» con un certificado
	// remoto que pide PIN u OTP sin que la petición los traiga.
	CodigoProtegerConSecretos csc.Codigo = "proteger_con_secretos" // #nosec G101 -- código de error, no un secreto.
)

// ErrNoAplicable indica que el certificado pedido no es remoto: el proveedor
// agregado debe probar con la siguiente fuente.
var ErrNoAplicable = errors.New("cscremota: el certificado no es remoto")

func nuevoError(codigo csc.Codigo) error { return &csc.Error{Codigo: codigo} }

// Opciones configura la sesión. Solo ConfigDir es obligatorio.
type Opciones struct {
	ConfigDir   string
	PoliticaDir string
	// HTTP es el cliente base (proxy de la organización). El cliente CSC lo
	// endurece.
	HTTP *http.Client
	// Navegador abre el URL de autorización en el navegador del sistema.
	Navegador func(context.Context, string) error
	// Idioma se envía al descubrir el servicio.
	Idioma string
	// TextoCallback es el texto ya traducido que ve la persona en el
	// navegador al volver.
	TextoCallback string
	// CargarConfig sustituye la lectura de config.json y de la política (pruebas).
	CargarConfig func() (config.Config, config.Policy, error)
	// CatalogoLocal son los certificados locales (almacenes del sistema,
	// PKCS#12, tarjetas). Al conectar se descarta toda credencial remota con
	// la huella de uno local: el mismo identificador no puede designar una
	// clave local y otra del prestador.
	CatalogoLocal ports.CertificateCatalog
}

// Estado resume la sesión para la interfaz. No incluye tokens.
type Estado struct {
	Permitida bool
	// Prohibida indica que no está permitida porque la política de la
	// organización lo decide así (config.json no puede cambiarlo).
	Prohibida    bool
	URL          string
	ClientID     string
	Descubierto  bool
	Conectada    bool
	HostServicio string
	HostOAuth    string
	Nombre       string
}

// Descubrimiento es lo que la persona debe ver antes de abrir el navegador.
type Descubrimiento struct {
	HostServicio string
	HostOAuth    string
	Nombre       string
}

// CredencialRemota describe un certificado remoto disponible.
type CredencialRemota struct {
	Ref        domain.CertificateRef
	SCAL       string
	Modo       csc.ModoAutorizacion
	PIN        bool
	OTP        bool
	OTPEnLinea bool
}

type remota struct {
	cred *csc.Credencial
	ref  domain.CertificateRef
}

// Sesion es segura para usarla desde varias goroutines.
type Sesion struct {
	opc Opciones

	mu           sync.Mutex
	generacion   uint64
	url          string
	clientID     string
	cliente      *csc.Cliente
	hostServicio string
	hostOAuth    string
	nombre       string
	conectada    bool
	creds        map[string]*remota
	orden        []string
	cargada      bool
}

// Nueva crea una sesión vacía. No contacta con ningún servicio.
func Nueva(opc Opciones) *Sesion {
	if opc.PoliticaDir == "" {
		opc.PoliticaDir = config.DirPolicyDefecto
	}
	return &Sesion{opc: opc}
}

// permiso lee configuración y política y devuelve los pares OAuth si la firma
// remota está permitida.
func (s *Sesion) permiso() ([]csc.ParOAuth, error) {
	var (
		cfg      config.Config
		politica config.Policy
		err      error
	)
	if s.opc.CargarConfig != nil {
		cfg, politica, err = s.opc.CargarConfig()
	} else {
		cfg, err = config.Load(s.opc.ConfigDir)
		if err == nil {
			politica, err = config.LoadPolicy(s.opc.PoliticaDir)
		}
	}
	// Una configuración o una política ilegibles no pueden autorizar nada.
	if err != nil {
		return nil, nuevoError(CodigoDesactivada)
	}
	if !cfg.FirmaRemotaCSCActiva(politica) {
		if config.FirmaRemotaCSCProhibida(politica) {
			return nil, nuevoError(CodigoProhibida)
		}
		return nil, nuevoError(CodigoDesactivada)
	}
	pares, err := csc.ParsearParesOAuth(cfg.FirmaRemotaCSCOAuth)
	if err != nil {
		return nil, nuevoError(CodigoParesOAuthInvalidos)
	}
	return pares, nil
}

// Permitida indica si la política y la configuración permiten la firma
// remota. Si no, cierra la sesión que hubiera.
func (s *Sesion) Permitida() bool {
	if _, err := s.permiso(); err != nil {
		s.mu.Lock()
		s.cerrarLocked()
		s.mu.Unlock()
		return false
	}
	return true
}

// Estado devuelve el estado de la sesión y la configuración guardada.
func (s *Sesion) Estado() Estado {
	_, errPermiso := s.permiso()
	s.mu.Lock()
	defer s.mu.Unlock()
	if errPermiso != nil {
		s.cerrarLocked()
		return Estado{Prohibida: CodigoVisible(errPermiso) == CodigoProhibida}
	}
	s.cargarLocked()
	return Estado{
		Permitida:    true,
		URL:          s.url,
		ClientID:     s.clientID,
		Descubierto:  s.cliente != nil,
		Conectada:    s.conectada,
		HostServicio: s.hostServicio,
		HostOAuth:    s.hostOAuth,
		Nombre:       s.nombre,
	}
}

// Configurar valida la dirección y el client_id como la CLI, descubre el
// servicio (sin abrir el navegador) y devuelve los hosts que la persona debe
// ver antes de conectar. Cierra la sesión anterior y guarda la configuración.
func (s *Sesion) Configurar(ctx context.Context, urlServicio, clientID string) (Descubrimiento, error) {
	pares, err := s.permiso()
	if err != nil {
		s.Desconectar()
		return Descubrimiento{}, err
	}
	urlServicio = strings.TrimSpace(urlServicio)
	clientID = strings.TrimSpace(clientID)
	if urlServicio == "" || clientID == "" || len(urlServicio) > maxURLServicio {
		return Descubrimiento{}, &csc.Error{Codigo: csc.CodigoParametroInvalido}
	}
	hostServicio, err := hostVisible(urlServicio)
	if err != nil {
		return Descubrimiento{}, err
	}
	cliente, err := csc.Nuevo(csc.Opciones{
		URLServicio:        urlServicio,
		ClientID:           clientID,
		HTTP:               s.opc.HTTP,
		AbrirNavegador:     s.abrirNavegador,
		PedirSecreto:       pedirSecreto,
		TextoCallback:      s.opc.TextoCallback,
		EsperaAutorizacion: esperaAutorizacion,
		Idioma:             s.opc.Idioma,
		ParesOAuth:         pares,
		EnvioOTPManual:     true,
	})
	if err != nil {
		return Descubrimiento{}, err
	}
	info, err := cliente.Info(ctx)
	if err != nil {
		_ = cliente.Close()
		return Descubrimiento{}, err
	}
	hostOAuth, err := hostVisible(info.OAuth2)
	if err != nil {
		_ = cliente.Close()
		return Descubrimiento{}, err
	}
	d := Descubrimiento{HostServicio: hostServicio, HostOAuth: hostOAuth, Nombre: textoVisible(info.Name, maxNombreServicio)}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cerrarLocked()
	s.cargada = true
	s.url, s.clientID = urlServicio, clientID
	s.cliente = cliente
	s.hostServicio, s.hostOAuth, s.nombre = d.HostServicio, d.HostOAuth, d.Nombre
	// Si no se puede guardar, la sesión sigue valiendo en memoria; solo no
	// se recordará la próxima vez.
	_ = s.guardarLocked()
	return d, nil
}

// Conectar autoriza el acceso en el navegador del sistema y carga los
// certificados remotos de la cuenta. Devuelve los que se pueden usar y
// cuántos se han descartado por no ser válidos.
func (s *Sesion) Conectar(ctx context.Context) ([]CredencialRemota, int, error) {
	if _, err := s.permiso(); err != nil {
		s.Desconectar()
		return nil, 0, err
	}
	s.mu.Lock()
	cliente, gen := s.cliente, s.generacion
	s.mu.Unlock()
	if cliente == nil {
		return nil, 0, nuevoError(CodigoNoConfigurada)
	}
	// La red y el navegador se esperan sin el cerrojo: el listado de
	// certificados no debe quedar bloqueado mientras la persona autoriza.
	if err := cliente.Autorizar(ctx); err != nil {
		return nil, 0, err
	}
	ids, err := cliente.ListarCredenciales(ctx)
	if err != nil {
		return nil, 0, err
	}
	locales := s.huellasLocales(ctx)
	creds := make(map[string]*remota, len(ids))
	orden := make([]string, 0, len(ids))
	descartadas := 0
	for _, id := range ids {
		if len(orden) >= maxCredencialesVist {
			descartadas++
			continue
		}
		cred, err := cliente.Credencial(ctx, id)
		if err != nil {
			descartadas++
			continue
		}
		ref := cred.Referencia()
		if _, repetida := creds[ref.ID]; repetida {
			descartadas++
			continue
		}
		// Coincide con un certificado local: se omite y la interfaz avisa
		// de que hay certificados remotos que no se muestran.
		if locales[strings.ToLower(ref.ID)] || locales[strings.ToLower(ref.Fingerprint)] {
			descartadas++
			continue
		}
		creds[ref.ID] = &remota{cred: cred, ref: ref}
		orden = append(orden, ref.ID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generacion != gen || s.cliente != cliente {
		return nil, 0, &csc.Error{Codigo: csc.CodigoSesionCerrada}
	}
	s.creds, s.orden, s.conectada = creds, orden, true
	return s.listaLocked(), descartadas, nil
}

// huellasLocales devuelve los identificadores y huellas de los certificados
// locales, en minúsculas. Si el catálogo local no responde, la firma remota
// sigue disponible: el proveedor de claves consulta antes las fuentes
// locales, así que un identificador repetido nunca acaba en el prestador.
func (s *Sesion) huellasLocales(ctx context.Context) map[string]bool {
	huellas := map[string]bool{}
	if s.opc.CatalogoLocal == nil {
		return huellas
	}
	refs, err := s.opc.CatalogoLocal.List(ctx)
	if err != nil {
		return huellas
	}
	for _, r := range refs {
		for _, v := range []string{r.ID, r.Fingerprint} {
			if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
				huellas[v] = true
			}
		}
	}
	return huellas
}

// Desconectar revoca los tokens y borra de la memoria el cliente y los
// certificados remotos. La configuración guardada se conserva; para volver
// a conectar hay que comprobar de nuevo el servicio.
func (s *Sesion) Desconectar() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cerrarLocked()
}

func (s *Sesion) cerrarLocked() {
	s.generacion++
	if s.cliente != nil {
		_ = s.cliente.Close()
	}
	s.cliente = nil
	s.conectada = false
	s.creds = nil
	s.orden = nil
	s.hostServicio, s.hostOAuth, s.nombre = "", "", ""
}

// Credencial devuelve la credencial remota con ese identificador de
// certificado, si la sesión está conectada.
func (s *Sesion) Credencial(certID string) (CredencialRemota, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.creds[certID]
	if !ok || !s.conectada {
		return CredencialRemota{}, false
	}
	return describir(r), true
}

// EnviarOTP pide al prestador que envíe el código de un solo uso de una
// credencial con OTP en línea.
func (s *Sesion) EnviarOTP(ctx context.Context, certID string) error {
	if _, err := s.permiso(); err != nil {
		s.Desconectar()
		return err
	}
	s.mu.Lock()
	r, ok := s.creds[certID]
	cliente, conectada := s.cliente, s.conectada
	s.mu.Unlock()
	if !ok || !conectada || cliente == nil {
		return nuevoError(CodigoNoConectada)
	}
	return cliente.EnviarOTP(ctx, r.cred)
}

// List implementa ports.CertificateCatalog con los certificados remotos de
// la sesión conectada. Nunca contacta con el servicio.
func (s *Sesion) List(_ context.Context) ([]domain.CertificateRef, error) {
	if !s.Permitida() {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.conectada {
		return nil, nil
	}
	refs := make([]domain.CertificateRef, 0, len(s.orden))
	for _, id := range s.orden {
		refs = append(refs, copiarRef(s.creds[id].ref))
	}
	return refs, nil
}

// KeyFor implementa ports.SigningKeyProvider. El firmante usa el contexto de
// la petición, de modo que los secretos y la cancelación de esa petición
// son los únicos que intervienen en la firma.
func (s *Sesion) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	s.mu.Lock()
	r, ok := s.creds[ref.ID]
	cliente, conectada := s.cliente, s.conectada
	s.mu.Unlock()
	if !ok || !conectada || cliente == nil {
		return nil, ErrNoAplicable
	}
	if _, err := s.permiso(); err != nil {
		s.Desconectar()
		return nil, err
	}
	peticion := peticionDe(ctx)
	if peticion != nil && !peticion.paraCertificado(ref.ID) {
		// El PIN y el OTP de esta petición son de otro certificado. Una
		// credencial que los necesita no puede firmar con ellos; una que no
		// los necesita firma sin verlos.
		if necesitaSecretos(r) {
			return nil, nuevoError(CodigoSecretoNoPedido)
		}
		ctx = context.WithValue(ctx, claveSecretos{}, (*Peticion)(nil))
	}
	firmante, err := cliente.Firmante(ctx, r.cred)
	if err != nil {
		return nil, err
	}
	envuelto := &firmanteAnotado{firmante: firmante, peticion: peticion}
	return deskSigner.NuevaClaveLocalConCadena(envuelto, r.cred.Certificado, r.cred.Cadena), nil
}

// Close cierra la sesión (al salir el motor).
func (s *Sesion) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cerrarLocked()
	return nil
}

func (s *Sesion) listaLocked() []CredencialRemota {
	lista := make([]CredencialRemota, 0, len(s.orden))
	for _, id := range s.orden {
		lista = append(lista, describir(s.creds[id]))
	}
	return lista
}

func necesitaSecretos(r *remota) bool {
	return r.cred.Modo == csc.ModoExplicito && (r.cred.PIN || r.cred.OTP)
}

func describir(r *remota) CredencialRemota {
	return CredencialRemota{
		Ref:        copiarRef(r.ref),
		SCAL:       r.cred.SCAL,
		Modo:       r.cred.Modo,
		PIN:        r.cred.Modo == csc.ModoExplicito && r.cred.PIN,
		OTP:        r.cred.Modo == csc.ModoExplicito && r.cred.OTP,
		OTPEnLinea: r.cred.Modo == csc.ModoExplicito && r.cred.OTP && r.cred.OTPEnLinea,
	}
}

func copiarRef(ref domain.CertificateRef) domain.CertificateRef {
	ref.DER = append([]byte(nil), ref.DER...)
	if len(ref.ChainDER) > 0 {
		cadena := make([][]byte, 0, len(ref.ChainDER))
		for _, c := range ref.ChainDER {
			cadena = append(cadena, append([]byte(nil), c...))
		}
		ref.ChainDER = cadena
	}
	return ref
}

// abrirNavegador solo abre la autorización del servidor OAuth ya mostrado a
// la persona; el cliente CSC ya ha comprobado que está permitido.
func (s *Sesion) abrirNavegador(ctx context.Context, destino string) error {
	if s.opc.Navegador == nil {
		return &csc.Error{Codigo: csc.CodigoNavegador}
	}
	return s.opc.Navegador(ctx, destino)
}

type configGuardada struct {
	URL      string `json:"url"`
	ClientID string `json:"client_id"`
}

// cargarLocked recupera la última configuración válida, sin contactar con
// la red. Una configuración que ya no pasa la validación se ignora.
func (s *Sesion) cargarLocked() {
	if s.cargada {
		return
	}
	s.cargada = true
	if s.opc.ConfigDir == "" {
		return
	}
	datos, err := securefile.ReadFileLimit(filepath.Join(s.opc.ConfigDir, FicheroConfiguracion), maxFicheroConfig)
	if err != nil {
		return
	}
	var g configGuardada
	if json.Unmarshal(datos, &g) != nil {
		return
	}
	g.URL, g.ClientID = strings.TrimSpace(g.URL), strings.TrimSpace(g.ClientID)
	if g.URL == "" || g.ClientID == "" || len(g.URL) > maxURLServicio {
		return
	}
	if _, err := hostVisible(g.URL); err != nil {
		return
	}
	prueba, err := csc.Nuevo(csc.Opciones{
		URLServicio:    g.URL,
		ClientID:       g.ClientID,
		HTTP:           s.opc.HTTP,
		AbrirNavegador: s.abrirNavegador,
	})
	if err != nil {
		return
	}
	_ = prueba.Close()
	s.url, s.clientID = g.URL, g.ClientID
}

func (s *Sesion) guardarLocked() error {
	if s.opc.ConfigDir == "" {
		return nil
	}
	datos, err := json.Marshal(configGuardada{URL: s.url, ClientID: s.clientID})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.opc.ConfigDir, 0o700); err != nil {
		return err
	}
	return securefile.WriteFileAtomic(filepath.Join(s.opc.ConfigDir, FicheroConfiguracion), datos, 0o600)
}

// hostVisible devuelve el host (con puerto si no es el 443) tal como se
// conecta, en ASCII: un nombre internacionalizado se muestra en punycode para
// que no se pueda confundir con otro parecido.
func hostVisible(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
		return "", &csc.Error{Codigo: csc.CodigoURLInvalida}
	}
	host, err := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if err != nil || host == "" {
		return "", &csc.Error{Codigo: csc.CodigoURLInvalida}
	}
	if puerto := u.Port(); puerto != "" && puerto != "443" {
		return net.JoinHostPort(host, puerto), nil
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]", nil
	}
	return host, nil
}

// textoVisible quita caracteres de control y acota un texto del servidor.
func textoVisible(s string, maximo int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maximo {
			break
		}
		if unicode.IsControl(r) || r == unicode.ReplacementChar || unicode.Is(unicode.Bidi_Control, r) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// firmanteAnotado delega en el firmante remoto y apunta su error en la
// petición, porque el motor de firma lo envuelve en mensajes genéricos.
type firmanteAnotado struct {
	firmante *csc.FirmanteRemoto
	peticion *Peticion
}

func (f *firmanteAnotado) Public() crypto.PublicKey { return f.firmante.Public() }

func (f *firmanteAnotado) Sign(r io.Reader, resumen []byte, opts crypto.SignerOpts) ([]byte, error) {
	firma, err := f.firmante.Sign(r, resumen, opts)
	f.peticion.anotar(err)
	return firma, err
}
