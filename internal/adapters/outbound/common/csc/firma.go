// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"io"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	"grxfirma/internal/adapters/outbound/common/secmem"
)

// OID de los algoritmos que usa el prototipo.
const (
	oidSHA256       = "2.16.840.1.101.3.4.2.1"
	oidSHA384       = "2.16.840.1.101.3.4.2.2"
	oidSHA512       = "2.16.840.1.101.3.4.2.3"
	oidRSA          = "1.2.840.113549.1.1.1"
	oidRSASHA256    = "1.2.840.113549.1.1.11"
	oidRSASHA384    = "1.2.840.113549.1.1.12"
	oidRSASHA512    = "1.2.840.113549.1.1.13"
	oidRSAPSS       = "1.2.840.113549.1.1.10"
	oidMGF1         = "1.2.840.113549.1.1.8"
	oidECPublicKey  = "1.2.840.10045.2.1"
	oidECDSASHA256  = "1.2.840.10045.4.3.2"
	oidECDSASHA384  = "1.2.840.10045.4.3.3"
	oidECDSASHA512  = "1.2.840.10045.4.3.4"
	maxSecretoBytes = 1024
	maxFirmaBytes   = 8 * 1024
	maxSADBytes     = 16 * 1024
)

func oidHash(h crypto.Hash) (string, bool) {
	switch h {
	case crypto.SHA256:
		return oidSHA256, true
	case crypto.SHA384:
		return oidSHA384, true
	case crypto.SHA512:
		return oidSHA512, true
	default:
		return "", false
	}
}

// algoritmoFirma elige el OID de signAlgo entre los que admite la clave. La
// especificación pide que sea uno de key/algo; unos servicios anuncian el
// algoritmo de clave (rsaEncryption) y otros el combinado con el resumen.
func algoritmoFirma(cred *Credencial, h crypto.Hash, pss bool) (oid string, params []byte, err error) {
	admite := func(o string) bool { return len(cred.Algoritmos) == 0 || contiene(cred.Algoritmos, o) }
	switch cred.Certificado.PublicKey.(type) {
	case *rsa.PublicKey:
		if pss {
			if !admite(oidRSAPSS) {
				return "", nil, nuevoError(CodigoAlgoritmoNoSoportado, oidRSAPSS, nil)
			}
			params, err := parametrosPSS(h)
			return oidRSAPSS, params, err
		}
		combinado := map[crypto.Hash]string{crypto.SHA256: oidRSASHA256, crypto.SHA384: oidRSASHA384, crypto.SHA512: oidRSASHA512}[h]
		switch {
		case admite(combinado):
			return combinado, nil, nil
		case contiene(cred.Algoritmos, oidRSA):
			return oidRSA, nil, nil
		}
	case *ecdsa.PublicKey:
		if pss {
			break
		}
		combinado := map[crypto.Hash]string{crypto.SHA256: oidECDSASHA256, crypto.SHA384: oidECDSASHA384, crypto.SHA512: oidECDSASHA512}[h]
		switch {
		case admite(combinado):
			return combinado, nil, nil
		case contiene(cred.Algoritmos, oidECPublicKey):
			return oidECPublicKey, nil, nil
		}
	}
	return "", nil, nuevoError(CodigoAlgoritmoNoSoportado, "", nil)
}

type parametrosRSAPSS struct {
	Hash    pkix.AlgorithmIdentifier `asn1:"explicit,tag:0"`
	MGF     pkix.AlgorithmIdentifier `asn1:"explicit,tag:1"`
	SaltLen int                      `asn1:"explicit,tag:2"`
	Trailer int                      `asn1:"optional,explicit,tag:3,default:1"`
}

// parametrosPSS codifica RSASSA-PSS-params (RFC 4055) con MGF1 del mismo
// resumen y sal de la longitud del resumen.
func parametrosPSS(h crypto.Hash) ([]byte, error) {
	oidTexto, _ := oidHash(h)
	oid, err := parsearOID(oidTexto)
	if err != nil {
		return nil, err
	}
	hashAlg := pkix.AlgorithmIdentifier{Algorithm: oid, Parameters: asn1.NullRawValue}
	hashDER, err := asn1.Marshal(hashAlg)
	if err != nil {
		return nil, nuevoError(CodigoParametroInvalido, "pss", err)
	}
	mgf, err := parsearOID(oidMGF1)
	if err != nil {
		return nil, err
	}
	p := parametrosRSAPSS{
		Hash:    hashAlg,
		MGF:     pkix.AlgorithmIdentifier{Algorithm: mgf, Parameters: asn1.RawValue{FullBytes: hashDER}},
		SaltLen: h.Size(),
		Trailer: 1,
	}
	der, err := asn1.Marshal(p)
	if err != nil {
		return nil, nuevoError(CodigoParametroInvalido, "pss", err)
	}
	return der, nil
}

func parsearOID(s string) (asn1.ObjectIdentifier, error) {
	partes := strings.Split(s, ".")
	oid := make(asn1.ObjectIdentifier, 0, len(partes))
	for _, p := range partes {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, nuevoError(CodigoParametroInvalido, "oid", err)
		}
		oid = append(oid, n)
	}
	return oid, nil
}

// autorizacionCredencial es el resultado de credentials/authorize (SAD) o, en
// modo oauth2code, el token de credencial. Se borra con destruir.
type autorizacionCredencial struct {
	sad   *secmem.Blob
	token *token
}

func (a *autorizacionCredencial) destruir() {
	if a == nil {
		return
	}
	if a.sad != nil {
		a.sad.Destroy()
	}
	a.token.destruir()
}

type datoAuth struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type peticionAutorizar struct {
	CredentialID     string     `json:"credentialID"`
	NumSignatures    int        `json:"numSignatures"`
	Hashes           []string   `json:"hashes"`
	HashAlgorithmOID string     `json:"hashAlgorithmOID"`
	PIN              string     `json:"PIN,omitempty"`
	OTP              string     `json:"OTP,omitempty"`
	AuthData         []datoAuth `json:"authData,omitempty"`
}

type respuestaAutorizar struct {
	SADTexto  rawSAD `json:"SAD"`
	ExpiresIn int64  `json:"expiresIn"`
}

// rawSAD conserva el SAD como bytes para poder borrarlo.
type rawSAD []byte

func (r *rawSAD) UnmarshalJSON(b []byte) error {
	v, err := cadenaJSONSinCopia(b)
	if err != nil {
		return err
	}
	*r = v
	return nil
}

// autorizarCredencial obtiene la autorización para firmar exactamente los
// resúmenes dados. Con SCAL2 el servicio liga la autorización a ellos; con
// SCAL1 se envían igualmente porque la especificación lo permite y acota el
// uso del SAD.
func (c *Cliente) autorizarCredencial(ctx context.Context, cred *Credencial, resumenes [][]byte, h crypto.Hash) (*autorizacionCredencial, error) {
	oidResumen, ok := oidHash(h)
	if !ok {
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "hash", nil)
	}
	hashes := make([]string, 0, len(resumenes))
	for _, r := range resumenes {
		hashes = append(hashes, base64.StdEncoding.EncodeToString(r))
	}

	if cred.Modo == ModoOAuth2 {
		urlHashes := make([]string, 0, len(resumenes))
		for _, r := range resumenes {
			urlHashes = append(urlHashes, base64.URLEncoding.EncodeToString(r))
		}
		extra := url.Values{}
		extra.Set("credentialID", cred.ID)
		extra.Set("numSignatures", strconv.Itoa(len(resumenes)))
		extra.Set("hashes", strings.Join(urlHashes, ","))
		extra.Set("hashAlgorithmOID", oidResumen)
		t, err := c.autorizarOAuth(ctx, "credential", extra)
		if err != nil {
			return nil, err
		}
		return &autorizacionCredencial{token: t}, nil
	}

	t, err := c.tokenSesion()
	if err != nil {
		return nil, err
	}
	peticion := peticionAutorizar{
		CredentialID:     cred.ID,
		NumSignatures:    len(resumenes),
		Hashes:           hashes,
		HashAlgorithmOID: oidResumen,
	}
	var secretos [][]byte
	defer func() {
		for _, s := range secretos {
			secmem.Zeroize(s)
		}
	}()
	if cred.Modo == ModoExplicito {
		if cred.OTP && cred.OTPEnLinea {
			if err := postJSON(ctx, c.http, unirRuta(c.base, "credentials/sendOTP"), t, map[string]string{"credentialID": cred.ID}, nil); err != nil {
				return nil, err
			}
		}
		pedir := func(tipo TipoSecreto, id string) error {
			if c.opc.PedirSecreto == nil {
				return nuevoError(CodigoSecreto, "", nil)
			}
			valor, err := c.opc.PedirSecreto(ctx, tipo)
			if err != nil {
				secmem.Zeroize(valor)
				return nuevoError(CodigoSecreto, "", err)
			}
			secretos = append(secretos, valor)
			if len(valor) == 0 || len(valor) > maxSecretoBytes {
				return nuevoError(CodigoSecreto, "", nil)
			}
			if cred.authDataV21 {
				peticion.AuthData = append(peticion.AuthData, datoAuth{ID: id, Value: string(valor)})
			} else if tipo == SecretoPIN {
				peticion.PIN = string(valor)
			} else {
				peticion.OTP = string(valor)
			}
			return nil
		}
		if cred.PIN {
			if err := pedir(SecretoPIN, "PIN"); err != nil {
				return nil, err
			}
		}
		if cred.OTP {
			if err := pedir(SecretoOTP, "OTP"); err != nil {
				return nil, err
			}
		}
	}
	var respuesta respuestaAutorizar
	defer secmem.Zeroize(respuesta.SADTexto)
	if err := postJSON(ctx, c.http, unirRuta(c.base, "credentials/authorize"), t, peticion, &respuesta); err != nil {
		return nil, err
	}
	if len(respuesta.SADTexto) == 0 || len(respuesta.SADTexto) > maxSADBytes {
		return nil, nuevoError(CodigoRespuestaInvalida, "SAD", nil)
	}
	return &autorizacionCredencial{sad: secmem.New(respuesta.SADTexto)}, nil
}

type peticionFirmarHash struct {
	CredentialID     string   `json:"credentialID"`
	SAD              string   `json:"SAD,omitempty"`
	Hashes           []string `json:"hashes"`
	HashAlgorithmOID string   `json:"hashAlgorithmOID"`
	SignAlgo         string   `json:"signAlgo"`
	SignAlgoParams   string   `json:"signAlgoParams,omitempty"`
	OperationMode    string   `json:"operationMode"`
}

type respuestaFirmarHash struct {
	Signatures []string `json:"signatures"`
}

// firmarResumenes llama a signatures/signHash con la autorización obtenida.
func (c *Cliente) firmarResumenes(ctx context.Context, cred *Credencial, auth *autorizacionCredencial, resumenes [][]byte, h crypto.Hash, pss bool) ([][]byte, error) {
	oidResumen, ok := oidHash(h)
	if !ok {
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "hash", nil)
	}
	signAlgo, params, err := algoritmoFirma(cred, h, pss)
	if err != nil {
		return nil, err
	}
	peticion := peticionFirmarHash{
		CredentialID:     cred.ID,
		HashAlgorithmOID: oidResumen,
		SignAlgo:         signAlgo,
		OperationMode:    "S",
	}
	for _, r := range resumenes {
		peticion.Hashes = append(peticion.Hashes, base64.StdEncoding.EncodeToString(r))
	}
	if len(params) > 0 {
		peticion.SignAlgoParams = base64.StdEncoding.EncodeToString(params)
	}
	var portador []byte
	if auth.token != nil {
		portador = auth.token.bytes()
	} else {
		t, err := c.tokenSesion()
		if err != nil {
			return nil, err
		}
		portador = t
		peticion.SAD = string(auth.sad.Bytes())
	}
	var respuesta respuestaFirmarHash
	if err := postJSON(ctx, c.http, unirRuta(c.base, "signatures/signHash"), portador, peticion, &respuesta); err != nil {
		return nil, err
	}
	if len(respuesta.Signatures) != len(resumenes) {
		return nil, nuevoError(CodigoRespuestaInvalida, "signatures", nil)
	}
	firmas := make([][]byte, 0, len(resumenes))
	for _, s := range respuesta.Signatures {
		if len(s) > 2*maxFirmaBytes {
			return nil, nuevoError(CodigoRespuestaInvalida, "signature", nil)
		}
		f, err := base64.StdEncoding.DecodeString(s)
		if err != nil || len(f) == 0 {
			return nil, nuevoError(CodigoRespuestaInvalida, "signature", err)
		}
		firmas = append(firmas, f)
	}
	return firmas, nil
}

// FirmanteRemoto es un crypto.Signer cuya clave privada vive en el servicio
// CSC. Solo envía el resumen recibido; el motor lo calcula en local.
type FirmanteRemoto struct {
	ctx     context.Context
	cliente *Cliente
	cred    *Credencial
}

// Firmante devuelve el crypto.Signer remoto para una credencial. ctx limita
// las peticiones que haga Sign, que no recibe contexto propio.
func (c *Cliente) Firmante(ctx context.Context, cred *Credencial) (*FirmanteRemoto, error) {
	if cred == nil || cred.Certificado == nil {
		return nil, nuevoError(CodigoCredencialNoValida, "", nil)
	}
	return &FirmanteRemoto{ctx: ctx, cliente: c, cred: cred}, nil
}

// Public devuelve la clave pública del certificado de la credencial.
func (f *FirmanteRemoto) Public() crypto.PublicKey {
	return f.cred.Certificado.PublicKey
}

// Sign autoriza y firma un único resumen. Cada firma pide su propia
// autorización (y, si la credencial lo exige, su PIN u OTP), porque con
// SCAL2 la autorización queda ligada al resumen. La firma recibida se
// comprueba con la clave pública antes de devolverla.
func (f *FirmanteRemoto) Sign(_ io.Reader, resumen []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts == nil {
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "", nil)
	}
	h := opts.HashFunc()
	if _, ok := oidHash(h); !ok {
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "hash", nil)
	}
	if len(resumen) != h.Size() {
		return nil, nuevoError(CodigoParametroInvalido, "digest", nil)
	}
	pssOpts, pss := opts.(*rsa.PSSOptions)

	c := f.cliente
	c.mu.Lock()
	defer c.mu.Unlock()
	auth, err := c.autorizarCredencial(f.ctx, f.cred, [][]byte{resumen}, h)
	if err != nil {
		return nil, err
	}
	defer auth.destruir()
	firmas, err := c.firmarResumenes(f.ctx, f.cred, auth, [][]byte{resumen}, h, pss)
	if err != nil {
		return nil, err
	}
	return comprobarFirma(f.cred.Certificado.PublicKey, resumen, h, firmas[0], pssOpts)
}

// comprobarFirma impide devolver al motor una firma que no corresponde al
// certificado anunciado (servicio defectuoso o credencial cambiada). Para
// ECDSA acepta el formato DER y el r||s de longitud fija, y devuelve DER,
// que es lo que espera crypto.Signer.
func comprobarFirma(pub crypto.PublicKey, resumen []byte, h crypto.Hash, firma []byte, pss *rsa.PSSOptions) ([]byte, error) {
	switch k := pub.(type) {
	case *rsa.PublicKey:
		var err error
		if pss != nil {
			err = rsa.VerifyPSS(k, h, resumen, firma, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthAuto, Hash: h})
		} else {
			err = rsa.VerifyPKCS1v15(k, h, resumen, firma)
		}
		if err != nil {
			return nil, nuevoError(CodigoFirmaInvalida, "", nil)
		}
		return firma, nil
	case *ecdsa.PublicKey:
		if ecdsa.VerifyASN1(k, resumen, firma) {
			return firma, nil
		}
		tam := (k.Curve.Params().BitSize + 7) / 8
		if len(firma) == 2*tam {
			r := new(big.Int).SetBytes(firma[:tam])
			s := new(big.Int).SetBytes(firma[tam:])
			der, err := asn1.Marshal(struct{ R, S *big.Int }{r, s})
			if err == nil && ecdsa.VerifyASN1(k, resumen, der) {
				return der, nil
			}
		}
		return nil, nuevoError(CodigoFirmaInvalida, "", nil)
	default:
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "key", nil)
	}
}

var _ crypto.Signer = (*FirmanteRemoto)(nil)
