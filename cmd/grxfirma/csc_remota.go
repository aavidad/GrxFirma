// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"golang.org/x/term"

	"grxfirma/internal/adapters/inbound/common/secretinput"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/adapters/outbound/desktop/proxyhttp"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/appdirs"
)

// opcionesCSC recoge las opciones de la firma remota CSC (prototipo).
type opcionesCSC struct {
	url        string
	clientID   string
	credencial string
	listar     bool
	opcionMala string
}

func (o opcionesCSC) solicitada() bool {
	return o.listar || o.url != "" || o.clientID != "" || o.credencial != "" || o.opcionMala != ""
}

// extraerFlagsCSC retira del argv las opciones -csc-*. Ninguna lleva
// secretos: el PIN y el OTP se piden por stdin y no hay secreto de cliente.
func extraerFlagsCSC(args []string) (cfg opcionesCSC, resto []string) {
	conValor := map[string]*string{
		"csc-url":        &cfg.url,
		"csc-client-id":  &cfg.clientID,
		"csc-credencial": &cfg.credencial,
	}
	for i := 0; i < len(args); i++ {
		nombre := strings.TrimLeft(args[i], "-")
		if !strings.HasPrefix(args[i], "-") || !strings.HasPrefix(nombre, "csc-") {
			resto = append(resto, args[i])
			continue
		}
		switch nombre {
		case "csc-listar-credenciales":
			cfg.listar = true
		default:
			clave, valor, enLinea := strings.Cut(nombre, "=")
			destino, ok := conValor[clave]
			switch {
			case ok && enLinea && valor != "":
				*destino = valor
			case ok && !enLinea && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-"):
				*destino = args[i+1]
				i++
			default:
				if cfg.opcionMala == "" {
					cfg.opcionMala = "-" + clave
				}
			}
		}
	}
	return cfg, resto
}

// entornoCSC agrupa las dependencias de ejecutarCSC para poder probarla.
type entornoCSC struct {
	ctx         context.Context
	logger      *slog.Logger
	t           func(string, ...any) string
	idioma      string
	salida      io.Writer
	errores     io.Writer
	stdin       io.Reader
	configDir   string
	politicaDir string
	httpBase    *http.Client
	navegador   func(context.Context, string) error
	// hostServicio se muestra junto al del servidor OAuth antes de abrir
	// el navegador.
	hostServicio string
	// ejecutarCLI lanza el adaptador CLI con la credencial remota como única
	// fuente de certificados.
	ejecutarCLI func(remota *fuenteRemota, args []string) int
}

func nuevoEntornoCSC(ctx context.Context, logger *slog.Logger, t func(string, ...any) string, idioma string) entornoCSC {
	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)
	return entornoCSC{
		ctx:         ctx,
		logger:      logger,
		t:           t,
		idioma:      idioma,
		salida:      os.Stdout,
		errores:     os.Stderr,
		stdin:       entradaSecretos(os.Stdin),
		configDir:   configDir,
		politicaDir: config.DirPolicyDefecto,
		httpBase:    proxyhttp.New(configDir),
		navegador:   csc.AbrirNavegadorSistema,
		ejecutarCLI: func(remota *fuenteRemota, args []string) int {
			adaptador, err := construirAdaptadorCon("", "", "", "", remota)
			if err != nil {
				logger.ErrorContext(ctx, "no se pudo arrancar el binario", "op", "bootstrap", "error", err)
				return 1
			}
			return adaptador.Run(ctx, args)
		},
	}
}

// ejecutarCSC atiende las órdenes de firma remota. Devuelve el código de
// salida del proceso. locales indica si se pasaron credenciales del equipo.
func ejecutarCSC(e entornoCSC, opc opcionesCSC, locales bool, args []string) int {
	avisar := func(clave string, a ...any) int {
		_, _ = fmt.Fprintln(e.errores, e.t(clave, a...))
		return 2
	}
	if opc.opcionMala != "" {
		return avisar("csc.cli.opcion_invalida", opc.opcionMala)
	}
	if locales {
		return avisar("csc.cli.incompatible")
	}
	cfg, err := config.Load(e.configDir)
	if err != nil {
		e.logger.ErrorContext(e.ctx, "configuración ilegible", "op", "csc-config", "error", err)
		return avisar("csc.cli.desactivada")
	}
	politica, err := config.LoadPolicy(e.politicaDir)
	if err != nil {
		// Una política presente pero ilegible no puede autorizar nada.
		e.logger.ErrorContext(e.ctx, "política ilegible", "op", "csc-policy", "error", err)
		return avisar("csc.cli.desactivada")
	}
	if !cfg.FirmaRemotaCSCActiva(politica) {
		return avisar("csc.cli.desactivada")
	}
	pares, err := csc.ParsearParesOAuth(cfg.FirmaRemotaCSCOAuth)
	if err != nil {
		return avisar("csc.cli.pares_oauth_invalidos")
	}
	if strings.TrimSpace(opc.url) == "" || strings.TrimSpace(opc.clientID) == "" {
		return avisar("csc.cli.faltan_opciones")
	}
	if !opc.listar && strings.TrimSpace(opc.credencial) == "" {
		return avisar("csc.cli.falta_credencial")
	}

	if u, err := url.Parse(strings.TrimSpace(opc.url)); err == nil {
		e.hostServicio = u.Host
	}
	cliente, err := csc.Nuevo(csc.Opciones{
		ParesOAuth:     pares,
		URLServicio:    strings.TrimSpace(opc.url),
		ClientID:       strings.TrimSpace(opc.clientID),
		HTTP:           e.httpBase,
		AbrirNavegador: e.abrirNavegador,
		PedirSecreto:   e.pedirSecreto,
		TextoCallback:  e.t("csc.navegador.vuelta"),
		Idioma:         e.idioma,
	})
	if err != nil {
		return e.fallo(err)
	}
	defer func() { _ = cliente.Close() }()

	if err := cliente.Autorizar(e.ctx); err != nil {
		return e.fallo(err)
	}
	if opc.listar {
		return e.listar(cliente)
	}

	cred, err := cliente.Credencial(e.ctx, strings.TrimSpace(opc.credencial))
	if err != nil {
		return e.fallo(err)
	}
	firmante, err := cliente.Firmante(e.ctx, cred)
	if err != nil {
		return e.fallo(err)
	}
	ref := cred.Referencia()
	remota := &fuenteRemota{ref: ref, clave: deskSigner.NuevaClaveLocalConCadena(firmante, cred.Certificado, cred.Cadena)}
	if !tieneSeleccionCertificado(args) {
		args = append(args, "-certificado", ref.ID)
	}
	return e.ejecutarCLI(remota, args)
}

func tieneSeleccionCertificado(args []string) bool {
	for _, a := range args {
		switch strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0] {
		case "certificado", "cert-id", "id", "id-certificado":
			return true
		}
	}
	return false
}

func (e entornoCSC) listar(cliente *csc.Cliente) int {
	ids, err := cliente.ListarCredenciales(e.ctx)
	if err != nil {
		return e.fallo(err)
	}
	if len(ids) == 0 {
		_, _ = fmt.Fprintln(e.salida, e.t("csc.cli.sin_credenciales"))
		return 0
	}
	for _, id := range ids {
		cred, err := cliente.Credencial(e.ctx, id)
		if err != nil {
			_, _ = fmt.Fprintln(e.salida, e.t("csc.cli.credencial_no_disponible", textoSeguro(id), e.mensaje(err)))
			continue
		}
		ref := cred.Referencia()
		_, _ = fmt.Fprintln(e.salida, e.t("csc.cli.credencial",
			textoSeguro(id),
			textoSeguro(ref.Subject),
			textoSeguro(ref.Issuer),
			ref.NotAfter.Format(time.DateOnly),
			cred.SCAL,
			e.t("csc.modo."+string(cred.Modo)),
		))
	}
	return 0
}

// abrirNavegador muestra a qué servicio y a qué servidor de autorización se
// va a dar acceso y abre el navegador. No escribe el URL: lleva el state de
// la petición y no debe quedar en la terminal ni en registros.
func (e entornoCSC) abrirNavegador(ctx context.Context, destino string) error {
	hostOAuth := ""
	if u, err := url.Parse(destino); err == nil {
		hostOAuth = u.Host
	}
	_, _ = fmt.Fprintln(e.errores, e.t("csc.cli.hosts", textoSeguro(e.hostServicio), textoSeguro(hostOAuth)))
	if e.navegador == nil {
		return errors.New("csc: no browser")
	}
	return e.navegador(ctx, destino)
}

func (e entornoCSC) pedirSecreto(_ context.Context, tipo csc.TipoSecreto) ([]byte, error) {
	clave := "csc.prompt.pin"
	if tipo == csc.SecretoOTP {
		clave = "csc.prompt.otp"
	}
	valor, err := secretinput.Read(e.stdin, e.errores, e.t(clave))
	if err != nil {
		return nil, err
	}
	return []byte(valor), nil
}

// fallo traduce el error para la persona y lo registra sin secretos: los
// errores del paquete csc nunca contienen tokens, PIN, OTP ni SAD.
func (e entornoCSC) fallo(err error) int {
	e.logger.ErrorContext(e.ctx, "firma remota CSC", "op", "csc", "error", err)
	_, _ = fmt.Fprintln(e.errores, e.mensaje(err))
	return 1
}

func (e entornoCSC) mensaje(err error) string {
	var errCSC *csc.Error
	if errors.As(err, &errCSC) {
		texto := e.t("csc.error." + string(errCSC.Codigo))
		if errCSC.Detalle != "" {
			texto += " (" + errCSC.Detalle + ")"
		}
		return texto
	}
	var errSecreto *secretinput.Error
	if errors.As(err, &errSecreto) {
		return secretinput.UserMessage(err, e.t)
	}
	return e.t("csc.error.generico")
}

// entradaSecretos devuelve stdin tal cual en una terminal (para leer sin
// eco) y, si viene de una tubería, un lector que entrega byte a byte: así
// cada lectura de secreto consume solo su línea y el OTP no se pierde en el
// búfer de la lectura del PIN.
func entradaSecretos(f *os.File) io.Reader {
	fd := int(f.Fd()) // #nosec G115 -- conversión exigida por x/term para el descriptor de os.File.
	if term.IsTerminal(fd) {
		return f
	}
	return &lectorByteAByte{r: f}
}

type lectorByteAByte struct{ r io.Reader }

func (l *lectorByteAByte) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return l.r.Read(p[:1])
}

// textoSeguro evita que un dato del servidor o del certificado inyecte
// secuencias de control en la terminal.
func textoSeguro(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return '?'
		}
		return r
	}, s)
}
