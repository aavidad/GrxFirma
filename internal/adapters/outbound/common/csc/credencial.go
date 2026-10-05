// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"

	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/domain"
)

const (
	maxCertificadosCadena = 10
	maxCredencialID       = 512
)

// ModoAutorizacion es el modo de autorización de la credencial.
type ModoAutorizacion string

const (
	ModoImplicito ModoAutorizacion = "implicit"
	ModoExplicito ModoAutorizacion = "explicit"
	ModoOAuth2    ModoAutorizacion = "oauth2code"
)

// Credencial describe una credencial de firma remota ya validada.
type Credencial struct {
	ID          string
	Certificado *x509.Certificate
	Cadena      []*x509.Certificate
	// SCAL es "1" o "2". Con SCAL2 la autorización queda ligada a los
	// resúmenes concretos que se van a firmar.
	SCAL        string
	Modo        ModoAutorizacion
	PIN         bool
	OTP         bool
	OTPEnLinea  bool
	Algoritmos  []string
	authDataV21 bool
}

// Referencia construye la referencia de catálogo con la misma forma que las
// identidades locales: el identificador es la huella SHA-256 completa.
func (cr *Credencial) Referencia() domain.CertificateRef {
	huella := sha256.Sum256(cr.Certificado.Raw)
	fp := hex.EncodeToString(huella[:])
	subject := cr.Certificado.Subject.CommonName
	if subject == "" {
		subject = cr.Certificado.Subject.String()
	}
	issuer := cr.Certificado.Issuer.CommonName
	if issuer == "" {
		issuer = cr.Certificado.Issuer.String()
	}
	tipo, org, nif := certutil.ClasificarCertificado(cr.Certificado)
	ref := domain.CertificateRef{
		ID:            fp,
		Subject:       subject,
		Issuer:        issuer,
		NotAfter:      cr.Certificado.NotAfter,
		Fingerprint:   fp,
		DER:           append([]byte(nil), cr.Certificado.Raw...),
		HasSigningKey: true,
		Tipo:          tipo,
		Organizacion:  org,
		NIF:           nif,
	}
	for _, c := range cr.Cadena {
		ref.ChainDER = append(ref.ChainDER, append([]byte(nil), c.Raw...))
	}
	return ref
}

// textoSCAL acepta "1"/"2" como cadena o como número: hay servicios de
// ambos tipos.
type textoSCAL string

func (s *textoSCAL) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = textoSCAL(v)
		return nil
	}
	*s = textoSCAL(string(b))
	return nil
}

type presencia struct {
	Presence string `json:"presence"`
	Type     string `json:"type"`
}

type objetoAuth struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type respuestaInfoCredencial struct {
	Key struct {
		Status string   `json:"status"`
		Algo   []string `json:"algo"`
	} `json:"key"`
	Cert struct {
		Status       string   `json:"status"`
		Certificates []string `json:"certificates"`
	} `json:"cert"`
	// CSC 2.0
	AuthMode string     `json:"authMode"`
	PIN      *presencia `json:"PIN"`
	OTP      *presencia `json:"OTP"`
	// CSC 2.1 y posteriores
	Auth *struct {
		Mode    string       `json:"mode"`
		Objects []objetoAuth `json:"objects"`
	} `json:"auth"`
	SCAL textoSCAL `json:"SCAL"`
}

type peticionInfoCredencial struct {
	CredentialID string `json:"credentialID"`
	Certificates string `json:"certificates"`
	CertInfo     bool   `json:"certInfo"`
	AuthInfo     bool   `json:"authInfo"`
}

func credencialIDValido(id string) bool {
	if id == "" || len(id) > maxCredencialID {
		return false
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Credencial obtiene y valida la información de una credencial
// (credentials/info) con su certificado y cadena.
func (c *Cliente) Credencial(ctx context.Context, id string) (*Credencial, error) {
	if !credencialIDValido(id) {
		return nil, nuevoError(CodigoParametroInvalido, "credentialID", nil)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t, err := c.tokenSesion()
	if err != nil {
		return nil, err
	}
	var r respuestaInfoCredencial
	peticion := peticionInfoCredencial{CredentialID: id, Certificates: "chain", CertInfo: true, AuthInfo: true}
	if err := postJSON(ctx, c.http, unirRuta(c.base, "credentials/info"), t, peticion, &r); err != nil {
		return nil, err
	}
	return interpretarCredencial(id, r)
}

func interpretarCredencial(id string, r respuestaInfoCredencial) (*Credencial, error) {
	if r.Key.Status != "" && !strings.EqualFold(r.Key.Status, "enabled") {
		return nil, nuevoError(CodigoCredencialNoValida, "key", nil)
	}
	if r.Cert.Status != "" && !strings.EqualFold(r.Cert.Status, "valid") {
		return nil, nuevoError(CodigoCredencialNoValida, "cert", nil)
	}
	if len(r.Cert.Certificates) == 0 || len(r.Cert.Certificates) > maxCertificadosCadena {
		return nil, nuevoError(CodigoCredencialNoValida, "chain", nil)
	}
	certs := make([]*x509.Certificate, 0, len(r.Cert.Certificates))
	for _, b64 := range r.Cert.Certificates {
		der, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, nuevoError(CodigoRespuestaInvalida, "certificate", err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, nuevoError(CodigoRespuestaInvalida, "certificate", err)
		}
		certs = append(certs, cert)
	}
	switch certs[0].PublicKey.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
	default:
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "key", nil)
	}
	cred := &Credencial{
		ID:          id,
		Certificado: certs[0],
		Cadena:      cadenaEncadenada(certs),
		SCAL:        strings.TrimSpace(string(r.SCAL)),
		Algoritmos:  append([]string(nil), r.Key.Algo...),
	}
	if cred.SCAL == "" {
		cred.SCAL = "1"
	}
	if cred.SCAL != "1" && cred.SCAL != "2" {
		return nil, nuevoError(CodigoRespuestaInvalida, "SCAL", nil)
	}
	modo := r.AuthMode
	if r.Auth != nil {
		modo = r.Auth.Mode
		cred.authDataV21 = true
		for _, o := range r.Auth.Objects {
			switch strings.ToUpper(o.ID) {
			case "PIN":
				cred.PIN = true
			case "OTP":
				cred.OTP = true
			}
		}
	} else {
		cred.PIN = r.PIN != nil && strings.EqualFold(r.PIN.Presence, "true")
		cred.OTP = r.OTP != nil && strings.EqualFold(r.OTP.Presence, "true")
		cred.OTPEnLinea = cred.OTP && strings.EqualFold(r.OTP.Type, "online")
	}
	switch ModoAutorizacion(strings.ToLower(strings.TrimSpace(modo))) {
	case ModoImplicito, "":
		cred.Modo = ModoImplicito
	case ModoExplicito:
		cred.Modo = ModoExplicito
	case ModoOAuth2:
		cred.Modo = ModoOAuth2
	default:
		return nil, nuevoError(CodigoRespuestaInvalida, "authMode", nil)
	}
	return cred, nil
}

// cadenaEncadenada devuelve certs[1:] solo si cada certificado está firmado
// por el siguiente. Si no, la cadena del servicio se descarta entera: no se
// incrusta en la firma una cadena que no corresponde al certificado (los
// verificadores construirán la suya con sus propias anclas).
func cadenaEncadenada(certs []*x509.Certificate) []*x509.Certificate {
	for i := 0; i+1 < len(certs); i++ {
		if certs[i].CheckSignatureFrom(certs[i+1]) != nil {
			return nil
		}
	}
	return append([]*x509.Certificate(nil), certs[1:]...)
}
