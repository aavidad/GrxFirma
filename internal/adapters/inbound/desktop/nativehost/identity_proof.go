// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package nativehost

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

const (
	maximoCanonIdentidadNativa = 16 * 1024
	tamanoFragmentoIdentidad   = 64 * 1024
)

// GeneradorPruebaIdentidadLocal define el caso de uso consumido por Native Messaging.
type GeneradorPruebaIdentidadLocal interface {
	Generar(ctx context.Context, reto domain.RetoIdentidad) (domain.PruebaIdentidad, error)
}

// WithGeneradorPruebaIdentidad habilita la acción sólo con un caso de uso completo.
func (a *Adaptador) WithGeneradorPruebaIdentidad(
	generador GeneradorPruebaIdentidadLocal,
) *Adaptador {
	if a == nil {
		return nil
	}
	a.ProbarIdentidad = generador
	return a
}

type solicitudPruebaIdentidadNativaJSON struct {
	RequestID           any    `json:"requestId"`
	Action              string `json:"action"`
	CanonicalPayloadB64 string `json:"canonicalPayloadB64"`
	RequesterOrigin     string `json:"requesterOrigin"`
}

type contenidoIdentidadNativaJSON struct {
	Contract          string `json:"contract"`
	ChallengeID       string `json:"challengeId"`
	RegisteredClient  string `json:"registeredClient"`
	Purpose           string `json:"purpose"`
	Operation         string `json:"operation"`
	Audience          string `json:"audience"`
	TenantContextHash string `json:"tenantContextHash"`
	SessionBinding    string `json:"sessionBinding"`
	Origin            string `json:"origin"`
	ConsentID         string `json:"consentId"`
	ConsentVersion    string `json:"consentVersion"`
	PolicyID          string `json:"policyId"`
	PolicyVersion     string `json:"policyVersion"`
	Nonce             string `json:"nonce"`
	IssuedAt          string `json:"issuedAt"`
	ExpiresAt         string `json:"expiresAt"`
}

type identityProofMetadataJSON struct {
	Contract           string   `json:"contract"`
	ChallengeID        string   `json:"challengeId"`
	CertificateB64     string   `json:"certificateB64"`
	ChainB64           []string `json:"chainB64"`
	Format             string   `json:"format"`
	SignatureAlgorithm string   `json:"signatureAlgorithm"`
	DigestAlgorithm    string   `json:"digestAlgorithm"`
}

func (a *Adaptador) handleIdentityProof(
	ctx context.Context,
	reqID string,
	payload []byte,
) ([][]byte, error) {
	if a == nil || a.ProbarIdentidad == nil {
		return marshalResponses([]response{errorIdentityResponse(reqID, "identity.unavailable")})
	}
	solicitud, err := decodificarSolicitudPruebaIdentidad(payload, reqID)
	if err != nil {
		return marshalResponses([]response{errorIdentityResponse(reqID, "identity.invalid_request")})
	}
	reto, err := retoIdentidadNativo(solicitud.CanonicalPayloadB64, solicitud.RequesterOrigin)
	if err != nil {
		return marshalResponses([]response{errorIdentityResponse(reqID, "identity.invalid_request")})
	}
	prueba, err := a.ProbarIdentidad.Generar(ctx, reto)
	if err != nil {
		codigo := "identity.unavailable"
		if errors.Is(err, application.ErrPruebaIdentidadLocalInvalida) {
			codigo = "identity.invalid_request"
		}
		return marshalResponses([]response{errorIdentityResponse(reqID, codigo)})
	}
	if prueba.Validar() != nil || prueba.RetoID != reto.Solicitud.RetoID {
		return marshalResponses([]response{errorIdentityResponse(reqID, "identity.invalid_proof")})
	}
	return marshalResponses(fragmentarPruebaIdentidad(reqID, prueba))
}

func decodificarSolicitudPruebaIdentidad(
	payload []byte,
	reqID string,
) (solicitudPruebaIdentidadNativaJSON, error) {
	var solicitud solicitudPruebaIdentidadNativaJSON
	if objetoJSONDuplicado(payload) || decodificarJSONEstricto(payload, &solicitud) != nil ||
		solicitud.Action != "proveIdentity" || normalizeRequestID(solicitud.RequestID) != reqID ||
		solicitud.RequesterOrigin == "" || solicitud.RequesterOrigin != normalizeRequesterOrigin(solicitud.RequesterOrigin) ||
		!strings.HasPrefix(solicitud.RequesterOrigin, "https://") {
		return solicitud, domain.ErrSolicitudIdentidadInvalida
	}
	return solicitud, nil
}

func retoIdentidadNativo(canonB64, origen string) (domain.RetoIdentidad, error) {
	canon, err := decodificarBase64Canonico(canonB64, maximoCanonIdentidadNativa)
	if err != nil || objetoJSONDuplicado(canon) {
		return domain.RetoIdentidad{}, domain.ErrSolicitudIdentidadInvalida
	}
	var dto contenidoIdentidadNativaJSON
	if decodificarJSONEstricto(canon, &dto) != nil || dto.Origin != origen {
		return domain.RetoIdentidad{}, domain.ErrSolicitudIdentidadInvalida
	}
	solicitud, err := dto.dominio()
	if err != nil {
		return domain.RetoIdentidad{}, err
	}
	return domain.RetoIdentidad{Solicitud: solicitud, ContenidoCanonico: append([]byte(nil), canon...)}, nil
}

func (d contenidoIdentidadNativaJSON) dominio() (domain.SolicitudRetoIdentidad, error) {
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(d.Nonce)
	if err != nil || base64.RawURLEncoding.EncodeToString(nonce) != d.Nonce {
		return domain.SolicitudRetoIdentidad{}, domain.ErrSolicitudIdentidadInvalida
	}
	emitido, err := instanteIdentidadNativo(d.IssuedAt)
	if err != nil {
		return domain.SolicitudRetoIdentidad{}, err
	}
	expira, err := instanteIdentidadNativo(d.ExpiresAt)
	if err != nil {
		return domain.SolicitudRetoIdentidad{}, err
	}
	solicitud := domain.SolicitudRetoIdentidad{
		Contrato: d.Contract, RetoID: d.ChallengeID, ClienteRegistrado: d.RegisteredClient,
		Finalidad: d.Purpose, Operacion: d.Operation, Audiencia: d.Audience,
		HuellaContextoTenant: d.TenantContextHash, VinculoSesion: d.SessionBinding,
		Origen: d.Origin, ConsentimientoID: d.ConsentID, VersionConsentimiento: d.ConsentVersion,
		PoliticaID: d.PolicyID, VersionPolitica: d.PolicyVersion, Nonce: nonce,
		EmitidoEn: emitido, ExpiraEn: expira,
	}
	return solicitud, solicitud.Validar()
}

func instanteIdentidadNativo(valor string) (time.Time, error) {
	instante, err := time.Parse(time.RFC3339Nano, valor)
	if err != nil || !strings.HasSuffix(valor, "Z") {
		return time.Time{}, domain.ErrSolicitudIdentidadInvalida
	}
	return instante.UTC(), nil
}

func decodificarBase64Canonico(valor string, maximo int) ([]byte, error) {
	if valor == "" || strings.TrimSpace(valor) != valor || len(valor) > base64.StdEncoding.EncodedLen(maximo) {
		return nil, domain.ErrSolicitudIdentidadInvalida
	}
	contenido, err := base64.StdEncoding.Strict().DecodeString(valor)
	if err != nil || len(contenido) == 0 || len(contenido) > maximo ||
		base64.StdEncoding.EncodeToString(contenido) != valor {
		return nil, domain.ErrSolicitudIdentidadInvalida
	}
	return contenido, nil
}

func decodificarJSONEstricto(contenido []byte, destino any) error {
	decodificador := json.NewDecoder(bytes.NewReader(contenido))
	decodificador.DisallowUnknownFields()
	if err := decodificador.Decode(destino); err != nil {
		return err
	}
	if err := decodificador.Decode(&struct{}{}); err != io.EOF {
		return domain.ErrSolicitudIdentidadInvalida
	}
	return nil
}

func objetoJSONDuplicado(contenido []byte) bool {
	decodificador := json.NewDecoder(bytes.NewReader(contenido))
	inicio, err := decodificador.Token()
	if err != nil || inicio != json.Delim('{') {
		return true
	}
	vistas := make(map[string]struct{})
	for decodificador.More() {
		clave, correcta := siguienteClaveJSON(decodificador)
		if !correcta {
			return true
		}
		if _, existe := vistas[clave]; existe {
			return true
		}
		vistas[clave] = struct{}{}
		var valor json.RawMessage
		if decodificador.Decode(&valor) != nil {
			return true
		}
	}
	fin, err := decodificador.Token()
	return err != nil || fin != json.Delim('}')
}

func siguienteClaveJSON(decodificador *json.Decoder) (string, bool) {
	token, err := decodificador.Token()
	clave, correcta := token.(string)
	return clave, err == nil && correcta
}

func fragmentarPruebaIdentidad(reqID string, prueba domain.PruebaIdentidad) []response {
	firma := base64.StdEncoding.EncodeToString(prueba.Firma)
	metadatos := &identityProofMetadataJSON{
		Contract: domain.VersionContratoIdentidadReforzada, ChallengeID: prueba.RetoID,
		CertificateB64: base64.StdEncoding.EncodeToString(prueba.Certificado),
		ChainB64:       codificarCadenaIdentidad(prueba.Cadena), Format: prueba.Formato,
		SignatureAlgorithm: prueba.AlgoritmoFirma, DigestAlgorithm: prueba.AlgoritmoHuella,
	}
	total := (len(firma) + tamanoFragmentoIdentidad - 1) / tamanoFragmentoIdentidad
	respuestas := make([]response, 0, total)
	for indice, inicio := 0, 0; inicio < len(firma); indice, inicio = indice+1, inicio+tamanoFragmentoIdentidad {
		fin := min(inicio+tamanoFragmentoIdentidad, len(firma))
		respuestas = append(respuestas, response{RequestID: reqID, Success: true,
			Signature: firma[inicio:fin], SignatureLen: len(firma), IdentityProof: metadatos,
			Chunk: indice, TotalChunks: total})
	}
	return respuestas
}

func codificarCadenaIdentidad(cadena [][]byte) []string {
	resultado := make([]string, len(cadena))
	for indice := range cadena {
		resultado[indice] = base64.StdEncoding.EncodeToString(cadena[indice])
	}
	return resultado
}

func errorIdentityResponse(reqID, codigo string) response {
	return response{RequestID: reqID, Success: false, Code: codigo,
		Error: codigo, Chunk: 0}
}
