// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/cscremota"
)

// Firma remota CSC desde las interfaces de escritorio.
//
// El motor guarda la sesión: los tokens, el SAD y la clave remota nunca se
// envían a la interfaz. La interfaz solo recibe el estado, los hosts que debe
// mostrar antes de abrir el navegador y la descripción de los certificados
// remotos. El PIN y el OTP llegan en Base64 dentro de la petición de firma
// (se decodifican directamente en []byte), solo se aceptan para un
// certificado remoto y se borran al terminar la petición.

const (
	maxCSCServiceURL = 2048
	maxCSCClientID   = 256
	maxCSCCertID     = 128
	maxCSCParams     = 8 * 1024
)

// SesionCSC es lo que el IPC necesita de la sesión de firma remota.
type SesionCSC interface {
	Estado() cscremota.Estado
	Configurar(ctx context.Context, urlServicio, clientID string) (cscremota.Descubrimiento, error)
	Conectar(ctx context.Context) ([]cscremota.CredencialRemota, int, error)
	Desconectar()
	Credencial(certID string) (cscremota.CredencialRemota, bool)
	EnviarOTP(ctx context.Context, certID string) error
}

type resultadoCSCEstado struct {
	Allowed bool `json:"allowed"`
	// ProhibitedByPolicy distingue la prohibición de la organización de la
	// firma remota simplemente desactivada en config.json.
	ProhibitedByPolicy bool   `json:"prohibitedByPolicy"`
	ServiceURL         string `json:"serviceUrl"`
	ClientID           string `json:"clientId"`
	Discovered         bool   `json:"discovered"`
	Connected          bool   `json:"connected"`
	ServiceHost        string `json:"serviceHost"`
	OAuthHost          string `json:"oauthHost"`
	ServiceName        string `json:"serviceName"`
}

type resultadoCSCDescubrimiento struct {
	ServiceHost string `json:"serviceHost"`
	OAuthHost   string `json:"oauthHost"`
	ServiceName string `json:"serviceName"`
}

type resultadoCSCCredencial struct {
	CertificateID string `json:"certificateId"`
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	NotAfter      string `json:"notAfter"`
	SCAL          string `json:"scal"`
	Mode          string `json:"mode"`
	PIN           bool   `json:"pin"`
	OTP           bool   `json:"otp"`
	OTPOnline     bool   `json:"otpOnline"`
	// MultiSign es cuántas firmas admite el prestador con una sola
	// autorización. Con 2 o más, un lote con OTP se puede firmar si cabe.
	MultiSign int `json:"multiSign"`
}

type resultadoCSCConexion struct {
	Credentials []resultadoCSCCredencial `json:"credentials"`
	Omitted     int                      `json:"omitted"`
}

type paramsCSCConfigurar struct {
	ServiceURL string `json:"serviceUrl"`
	ClientID   string `json:"clientId"`
}

type paramsCSCCertificado struct {
	CertificateID string `json:"certificateId"`
}

// paramsFirmaRemota extrae de una petición de firma solo lo necesario para
// decidir si el certificado es remoto. encoding/json decodifica el Base64 de
// remotePin y remoteOtp directamente en []byte, que se puede borrar.
type paramsFirmaRemota struct {
	CertificateID    string `json:"certificateId"`
	CertificateIndex int    `json:"certificateIndex"`
	RemotePIN        []byte `json:"remotePin"`
	RemoteOTP        []byte `json:"remoteOtp"`
	// AdditionalCertificateIDs son los firmantes adicionales de una
	// multifirma: no usan la autorización conjunta y, si son remotos, no
	// pueden pedir PIN ni OTP (los secretos son solo del principal).
	AdditionalCertificateIDs []string `json:"additionalCertificateIds"`
}

func isRemoteSigningAction(action string) bool {
	switch action {
	case "sign", "sign_batch", "sign_multicosign", "protect_sign":
		return true
	}
	return false
}

// certResuelto guarda en el contexto de la petición el certificado que
// prepararFirmaRemota ha resuelto, para que el manejador de la acción firme
// con ese mismo y no vuelva a resolver un índice contra una lista que otra
// petición haya podido refrescar entretanto.
type certResuelto struct {
	id       string
	indice   int
	resuelto string
}

type claveCertResuelto struct{}

func certResueltoDe(ctx context.Context, certID string, indice int) (string, bool) {
	r, ok := ctx.Value(claveCertResuelto{}).(certResuelto)
	if !ok || r.id != strings.TrimSpace(certID) || r.indice != indice {
		return "", false
	}
	return r.resuelto, true
}

// decodeCSCParams exige un único objeto JSON sin campos desconocidos.
func decodeCSCParams(raw json.RawMessage, dst any) error {
	if len(raw) > maxCSCParams {
		return errors.New("params demasiado grandes")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("params deben ser un objeto")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("contenido sobrante")
	}
	return nil
}

func validCSCText(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validCSCCertificateID(id string) bool {
	if id == "" || len(id) > maxCSCCertID {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (m *Manejador) handleCSC(ctx context.Context, action string, raw json.RawMessage) respuesta {
	if m.CSC == nil {
		if action == "csc_status" {
			return respuesta{OK: true, Action: action, Data: resultadoCSCEstado{}}
		}
		return m.cscError(action, &csc.Error{Codigo: cscremota.CodigoDesactivada})
	}
	switch action {
	case "csc_status":
		var vacio struct{}
		if err := decodeCSCParams(raw, &vacio); err != nil {
			return m.cscInvalid(action)
		}
		e := m.CSC.Estado()
		return respuesta{OK: true, Action: action, Data: resultadoCSCEstado{
			Allowed:            e.Permitida,
			ProhibitedByPolicy: e.Prohibida,
			ServiceURL:         e.URL,
			ClientID:           e.ClientID,
			Discovered:         e.Descubierto,
			Connected:          e.Conectada,
			ServiceHost:        e.HostServicio,
			OAuthHost:          e.HostOAuth,
			ServiceName:        e.Nombre,
		}}
	case "csc_configure":
		var p paramsCSCConfigurar
		if err := decodeCSCParams(raw, &p); err != nil {
			return m.cscInvalid(action)
		}
		p.ServiceURL, p.ClientID = strings.TrimSpace(p.ServiceURL), strings.TrimSpace(p.ClientID)
		if !validCSCText(p.ServiceURL, maxCSCServiceURL) || !validCSCText(p.ClientID, maxCSCClientID) {
			return m.cscInvalid(action)
		}
		d, err := m.CSC.Configurar(ctx, p.ServiceURL, p.ClientID)
		if err != nil {
			return m.cscError(action, err)
		}
		m.olvidarCertificadosRemotos()
		return respuesta{OK: true, Action: action, Data: resultadoCSCDescubrimiento{
			ServiceHost: d.HostServicio, OAuthHost: d.HostOAuth, ServiceName: d.Nombre,
		}}
	case "csc_connect":
		var vacio struct{}
		if err := decodeCSCParams(raw, &vacio); err != nil {
			return m.cscInvalid(action)
		}
		creds, omitidas, err := m.CSC.Conectar(ctx)
		if err != nil {
			return m.cscError(action, err)
		}
		m.olvidarCertificadosRemotos()
		lista := make([]resultadoCSCCredencial, 0, len(creds))
		for _, c := range creds {
			lista = append(lista, describirCredencialCSC(c))
		}
		return respuesta{OK: true, Action: action, Data: resultadoCSCConexion{Credentials: lista, Omitted: omitidas}}
	case "csc_disconnect":
		var vacio struct{}
		if err := decodeCSCParams(raw, &vacio); err != nil {
			return m.cscInvalid(action)
		}
		m.CSC.Desconectar()
		m.olvidarCertificadosRemotos()
		return respuesta{OK: true, Action: action, Data: struct{}{}}
	case "csc_send_otp":
		var p paramsCSCCertificado
		if err := decodeCSCParams(raw, &p); err != nil || !validCSCCertificateID(p.CertificateID) {
			return m.cscInvalid(action)
		}
		if err := m.CSC.EnviarOTP(ctx, p.CertificateID); err != nil {
			return m.cscError(action, err)
		}
		return respuesta{OK: true, Action: action, Data: struct{}{}}
	}
	return m.cscInvalid(action)
}

func describirCredencialCSC(c cscremota.CredencialRemota) resultadoCSCCredencial {
	return resultadoCSCCredencial{
		CertificateID: c.Ref.ID,
		Subject:       c.Ref.Subject,
		Issuer:        c.Ref.Issuer,
		NotAfter:      formatOptionalTime(c.Ref.NotAfter),
		SCAL:          c.SCAL,
		Mode:          string(c.Modo),
		PIN:           c.PIN,
		OTP:           c.OTP,
		OTPOnline:     c.OTPEnLinea,
		MultiSign:     max(c.Multisign, 1),
	}
}

// olvidarCertificadosRemotos invalida la caché de la última lista para que
// un índice antiguo no apunte a un certificado remoto que ya no existe.
func (m *Manejador) olvidarCertificadosRemotos() {
	m.setUltimosCerts(nil)
}

func (m *Manejador) cscInvalid(action string) respuesta {
	return ipcErrorResponse(action, "invalid_params", ipcPhaseOperation, m.t("error.formato_invalido"), false)
}

func (m *Manejador) cscError(action string, err error) respuesta {
	codigo := cscremota.CodigoVisible(err)
	if codigo == "" {
		codigo = "generico"
	}
	return ipcErrorResponse(action, "csc_"+string(codigo), ipcPhaseOperation, m.t("csc.error."+string(codigo)), false)
}

// prepararFirmaRemota decide, antes de despachar una firma, si el
// certificado es remoto. Si lo es, devuelve un contexto con el PIN y el OTP
// de la petición. Si la petición trae secretos para un certificado que no es
// remoto, se rechaza: el motor no acepta secretos que no ha pedido.
// liberar borra los secretos y debe llamarse siempre.
func (m *Manejador) prepararFirmaRemota(ctx context.Context, action string, raw json.RawMessage) (context.Context, *cscremota.Peticion, func(), *respuesta) {
	var p paramsFirmaRemota
	if err := json.Unmarshal(raw, &p); err != nil {
		// El manejador de la acción responde al formato inválido.
		secmem.Zeroize(p.RemotePIN)
		secmem.Zeroize(p.RemoteOTP)
		return ctx, nil, func() {}, nil
	}
	liberar := func() {
		secmem.Zeroize(p.RemotePIN)
		secmem.Zeroize(p.RemoteOTP)
	}
	conSecretos := len(p.RemotePIN) > 0 || len(p.RemoteOTP) > 0
	if !cscremota.SecretoValido(p.RemotePIN) || !cscremota.SecretoValido(p.RemoteOTP) {
		liberar()
		resp := m.cscInvalid(action)
		return ctx, nil, func() {}, &resp
	}
	// El certificado se resuelve una sola vez: el manejador de la acción
	// recupera este mismo del contexto.
	certID, err := m.resolverCertIDPreferido(ctx, p.CertificateID, p.CertificateIndex)
	if err != nil {
		certID = ""
	}
	if certID != "" {
		ctx = context.WithValue(ctx, claveCertResuelto{}, certResuelto{
			id: strings.TrimSpace(p.CertificateID), indice: p.CertificateIndex, resuelto: certID,
		})
	}
	var (
		cred   cscremota.CredencialRemota
		remota bool
	)
	if m.CSC != nil && certID != "" {
		cred, remota = m.CSC.Credencial(certID)
	}
	// Los secretos de la petición son del firmante principal. Un firmante
	// adicional remoto que pida PIN u OTP no tendría con qué firmar.
	if action == "sign_multicosign" && m.CSC != nil {
		for _, id := range p.AdditionalCertificateIDs {
			for _, candidato := range []string{id, strings.TrimSpace(id)} {
				if c, ok := m.CSC.Credencial(candidato); ok && (c.PIN || c.OTP) {
					liberar()
					resp := m.cscError(action, &csc.Error{Codigo: cscremota.CodigoAdicionalConSecretos})
					return ctx, nil, func() {}, &resp
				}
			}
		}
	}
	if !remota {
		if conSecretos {
			liberar()
			resp := m.cscError(action, &csc.Error{Codigo: cscremota.CodigoSecretoNoPedido})
			return ctx, nil, func() {}, &resp
		}
		return ctx, nil, liberar, nil
	}
	// Un lote con OTP solo se puede firmar si el prestador autoriza varias
	// firmas con un código (multisign); si el lote no cabe en una
	// autorización, lo rechaza la clave remota antes de firmar nada. La
	// multifirma por lotes no usa la autorización conjunta.
	if action == "sign_batch" && cred.OTP && (cred.Multisign < 2 || len(p.AdditionalCertificateIDs) > 0) {
		liberar()
		resp := m.cscError(action, &csc.Error{Codigo: cscremota.CodigoOTPLote})
		return ctx, nil, func() {}, &resp
	}
	if action == "protect_sign" && ((cred.PIN && len(p.RemotePIN) == 0) || (cred.OTP && len(p.RemoteOTP) == 0)) {
		liberar()
		resp := m.cscError(action, &csc.Error{Codigo: cscremota.CodigoProtegerConSecretos})
		return ctx, nil, func() {}, &resp
	}
	ctx, peticion := cscremota.ContextoConSecretos(ctx, certID, p.RemotePIN, p.RemoteOTP)
	return ctx, peticion, liberar, nil
}

// explicarFalloRemoto sustituye el mensaje genérico del motor por el del
// servicio de firma remota cuando la firma remota falló.
func (m *Manejador) explicarFalloRemoto(resp respuesta, peticion *cscremota.Peticion) respuesta {
	if resp.OK || peticion == nil {
		return resp
	}
	err := peticion.Error()
	if err == nil {
		return resp
	}
	fallo := m.cscError(resp.Action, err)
	resp.Error = fallo.Error
	resp.ErrorCode = fallo.ErrorCode
	return resp
}
