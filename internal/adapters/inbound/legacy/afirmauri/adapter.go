// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
	"grxfirma/internal/security/cryptopolicy"
)

// TipoOperacion representa la operacion funcional pedida por la URI legacy.
type TipoOperacion string

const (
	OperacionFirma      TipoOperacion = "sign"
	OperacionLote       TipoOperacion = "batch"
	OperacionSelectCert TipoOperacion = "selectcert"
	OperacionSave       TipoOperacion = "save"
	OperacionLoad       TipoOperacion = "load"
	OperacionSignSave   TipoOperacion = "signandsave"
)

// MaxURISize acota el tamaño total de una URI afirma:// entrante (T055). Los
// documentos grandes se intercambian por RetrieveService, no en línea; una URI
// por encima de este límite es abuso o error y se rechaza antes de parsearla
// para no reservar memoria por un payload arbitrario.
const MaxURISize = 10 * 1024 * 1024

// Solicitud es la traduccion neutra de una URI afirma:// al nucleo interno.
type Solicitud struct {
	Operacion       TipoOperacion
	AccionFirma     domain.SignatureAction
	Formato         domain.SignatureFormat
	Options         map[string]string
	LegacyParams    url.Values
	Origenes        []string
	Sesion          domain.ExchangeSession
	SignCommand     *application.SignCommand
	BatchCommand    *application.ProcessBatchCommand
	RemoteBatch     *RemoteBatchCommand
	RetrieveCommand *application.RetrieveRequestCommand
	// Version es la version del protocolo afirma:// indicada por el parametro v o ver (0-4).
	// El valor 0 indica que no se especifico version (o se especifico explicitamente la 0).
	Version int
}

// RemoteBatchCommand representa un lote remoto legado compatible con la V1 Go.
// Conserva el payload original y los endpoints de pre/postfirma para que la
// ejecución batch no pase por el flujo retrieve genérico.
type RemoteBatchCommand struct {
	Session          domain.ExchangeSession
	Payload          []byte
	IsJSONBatch      bool
	PreSignEndpoint  string
	PostSignEndpoint string
	NeedCert         bool
	LegacyParams     url.Values
}

// Adaptador implementa la capa anticorrupcion para afirma://.
type Adaptador struct {
	trust ports.TrustPolicy
}

type parseOptions struct {
	bySocket bool
}

// New construye el adaptador afirma://.
func New(trust ports.TrustPolicy) *Adaptador {
	return &Adaptador{trust: trust}
}

// Parse traduce una URI afirma:// a una solicitud interna.
func (a *Adaptador) Parse(ctx context.Context, rawURI string) (Solicitud, error) {
	return a.parse(ctx, rawURI, parseOptions{})
}

// ParseSocket traduce una URI afirma:// al modo de compatibilidad websocket/service.
// En este modo, como en V1, no se exigen los servlets remotos si el propio socket
// es el canal de ida y vuelta de la operación.
func (a *Adaptador) ParseSocket(ctx context.Context, rawURI string) (Solicitud, error) {
	return a.parse(ctx, rawURI, parseOptions{bySocket: true})
}

// ValidateOrigins expone la misma validación de confianza usada internamente
// por el parser, para que el borde websocket pueda añadir el Origin real del
// navegador cuando el mensaje afirma:// no lo transporta explícitamente.
func (a *Adaptador) ValidateOrigins(ctx context.Context, origenes []string) error {
	return a.validarOrigenes(ctx, origenes)
}

func (a *Adaptador) parse(ctx context.Context, rawURI string, opts parseOptions) (Solicitud, error) {
	if strings.TrimSpace(rawURI) == "" {
		return Solicitud{}, errors.New("la URI afirma:// no puede estar vacia")
	}
	if len(rawURI) > MaxURISize {
		return Solicitud{}, fmt.Errorf("la URI afirma:// excede el tamaño máximo permitido (%d bytes, máximo %d)", len(rawURI), MaxURISize)
	}

	normalized := normalizarURI(rawURI)

	u, err := url.Parse(normalized)
	if err != nil {
		if opts.bySocket {
			if solicitud, ok, compatErr := a.parseSocketRawLocalCompatibility(ctx, rawURI); ok {
				return solicitud, compatErr
			}
		}
		return Solicitud{}, fmt.Errorf("la URI afirma:// no es valida: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "afirma") {
		return Solicitud{}, errors.New("la URI debe usar el esquema afirma://")
	}

	parametros := u.Query()
	opRaw := parametro(parametros, "op", "operation", "action")
	action, signAction, operation, err := normalizarOperacion(extraerOperacion(u, opRaw))
	if err != nil {
		return Solicitud{}, err
	}

	version := extraerVersion(parametros)

	solicitud := Solicitud{
		Operacion:    operation,
		AccionFirma:  signAction,
		Formato:      domain.FormatCAdES,
		Options:      opcionesFirmaLegacy(parametros),
		LegacyParams: clonarValores(parametros),
		Version:      version,
	}

	if formato := strings.TrimSpace(parametro(parametros, "format", "signFormat")); formato != "" {
		f, err := application.ParseSignatureFormat(formato)
		if err != nil {
			return Solicitud{}, fmt.Errorf("formato de firma no valido en afirma://: %w", err)
		}
		solicitud.Formato = f
	}

	if operacionLegacyLocal(action) {
		if action == "signandsave" {
			if accion := accionCopSignAndSave(parametro(parametros, "cop")); accion != "" {
				solicitud.AccionFirma = accion
			}
			payload := parametro(parametros, "dat", "data", "ksb64")
			if payload == "" {
				return Solicitud{}, errors.New("faltan parametros obligatorios en afirma://: dat, data o ksb64")
			}
			cmd, err := construirSignCommand(payload, solicitud, parametros)
			if err != nil {
				return Solicitud{}, err
			}
			solicitud.SignCommand = cmd
		}
		return solicitud, nil
	}

	if opts.bySocket {
		aplicarHintsSesionSocket(&solicitud, parametros)
		switch action {
		case "selectcert":
			return solicitud, nil
		case "sign", "cosign", "countersign":
			if payload := parametro(parametros, "dat", "data", "ksb64"); payload != "" {
				cmd, err := construirSignCommandSocketCompatible(payload, solicitud, parametros)
				if err != nil {
					return Solicitud{}, err
				}
				solicitud.SignCommand = cmd
				return solicitud, nil
			}
			if parametrosSocketLegacySuficientes(parametros) {
				return solicitud, nil
			}
		case "batch":
			if payload := parametro(parametros, "dat", "data"); payload != "" {
				remote, err := construirRemoteBatchCommand(parametros, solicitud.Sesion, payload)
				if err != nil {
					return Solicitud{}, err
				}
				if remote != nil {
					origenesExtra, err := origenesDesdeEndpoints(remote.PreSignEndpoint, remote.PostSignEndpoint)
					if err != nil {
						return Solicitud{}, err
					}
					solicitud.Origenes = fusionarOrigenes(solicitud.Origenes, origenesExtra)
					if err := a.validarOrigenes(ctx, origenesExtra); err != nil {
						return Solicitud{}, err
					}
					solicitud.RemoteBatch = remote
					return solicitud, nil
				}
				cmd, err := a.ParseBatchPayloadFromBase64(payload, solicitud.Sesion)
				if err != nil {
					return Solicitud{}, err
				}
				solicitud.BatchCommand = &cmd
				return solicitud, nil
			}
		}
	}

	sesion, origenes, err := construirSesion(parametros)
	if err != nil {
		return Solicitud{}, err
	}
	solicitud.Sesion = sesion
	solicitud.Origenes = origenes

	if err := a.validarOrigenes(ctx, origenes); err != nil {
		return Solicitud{}, err
	}

	switch action {
	case "sign", "cosign", "countersign":
		if payload := parametro(parametros, "dat", "data", "ksb64"); payload != "" {
			if decodedRef, ok := detectarReferenciaRemotaEmbebida(payload, solicitud, parametros, sesion); ok {
				sesion.RequestID = decodedRef
				solicitud.Sesion = sesion
				solicitud.RetrieveCommand = &application.RetrieveRequestCommand{Session: sesion}
				return solicitud, nil
			}
			cmd, err := construirSignCommand(payload, solicitud, parametros)
			if err != nil {
				return Solicitud{}, err
			}
			solicitud.SignCommand = cmd
			return solicitud, nil
		}
		solicitud.RetrieveCommand = &application.RetrieveRequestCommand{Session: sesion}
		return solicitud, nil
	case "batch":
		if payload := parametro(parametros, "dat", "data"); payload != "" {
			remote, err := construirRemoteBatchCommand(parametros, sesion, payload)
			if err != nil {
				return Solicitud{}, err
			}
			if remote != nil {
				origenesExtra, err := origenesDesdeEndpoints(remote.PreSignEndpoint, remote.PostSignEndpoint)
				if err != nil {
					return Solicitud{}, err
				}
				solicitud.Origenes = fusionarOrigenes(solicitud.Origenes, origenesExtra)
				if err := a.validarOrigenes(ctx, origenesExtra); err != nil {
					return Solicitud{}, err
				}
				solicitud.RemoteBatch = remote
				return solicitud, nil
			}
			cmd, err := a.ParseBatchPayloadFromBase64(payload, sesion)
			if err != nil {
				return Solicitud{}, err
			}
			solicitud.BatchCommand = &cmd
			return solicitud, nil
		}
		solicitud.RetrieveCommand = &application.RetrieveRequestCommand{Session: sesion}
		return solicitud, nil
	case "selectcert":
		return solicitud, nil
	default:
		return Solicitud{}, fmt.Errorf("operacion afirma:// no soportada: %s", action)
	}
}

var remoteReferenceLike = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,200}$`)

func detectarReferenciaRemotaEmbebida(payload string, solicitud Solicitud, parametros url.Values, sesion domain.ExchangeSession) (string, bool) {
	if strings.TrimSpace(payload) == "" {
		return "", false
	}
	if strings.TrimSpace(sesion.UploadEndpoint) == "" || strings.TrimSpace(sesion.RetrieveEndpoint) == "" {
		return "", false
	}
	if strings.TrimSpace(parametro(parametros, "rtservlet", "retrieveservlet", "rtServlet", "retrieveServlet")) != "" {
		return "", false
	}
	if strings.TrimSpace(sesion.SessionKey) == "" {
		return "", false
	}
	formatLower := strings.ToLower(strings.TrimSpace(parametro(parametros, "format", "signFormat")))
	if formatLower == "" {
		formatLower = strings.ToLower(strings.TrimSpace(string(solicitud.Formato)))
	}
	if !strings.Contains(formatLower, "tri") {
		return "", false
	}
	// Con servidor trifásico (serverUrl), "dat" es el documento o su
	// identificador para ese servidor (FIRe), no una referencia que haya que
	// descargar: se firma contra el servidor como AutoFirma Java.
	for clave := range opcionesFirmaLegacy(parametros) {
		if strings.EqualFold(clave, "serverUrl") {
			return "", false
		}
	}
	decoded, err := decodeProtocolBase64(payload)
	if err != nil {
		return "", false
	}
	ref := strings.TrimSpace(string(decoded))
	if ref == "" || len(ref) > 200 {
		return "", false
	}
	if strings.ContainsAny(ref, "<>{}[]\n\r\t") || strings.HasPrefix(ref, "%PDF") {
		return "", false
	}
	if !remoteReferenceLike.MatchString(ref) {
		return "", false
	}
	return ref, true
}

func (a *Adaptador) parseSocketRawLocalCompatibility(ctx context.Context, rawURI string) (Solicitud, bool, error) {
	rawURI = strings.TrimSpace(rawURI)
	if !strings.HasPrefix(strings.ToLower(rawURI), "afirma://") {
		return Solicitud{}, false, nil
	}
	qidx := strings.IndexByte(rawURI, '?')
	if qidx < 0 || qidx+1 >= len(rawURI) {
		return Solicitud{}, false, nil
	}

	params := parseSocketRawQuery(rawURI[qidx+1:])
	opRaw := parametro(params, "op", "operation", "action")
	actionHost := strings.TrimPrefix(path.Clean(strings.TrimPrefix(rawURI[len("afirma://"):qidx], "/")), "/")
	action, signAction, operation, err := normalizarOperacion(extraerOperacionRawCompat(actionHost, opRaw))
	if err != nil {
		return Solicitud{}, false, nil
	}
	if !operacionLegacyLocal(action) {
		return Solicitud{}, false, nil
	}

	solicitud := Solicitud{
		Operacion:    operation,
		AccionFirma:  signAction,
		Formato:      domain.FormatCAdES,
		Options:      opcionesFirmaLegacy(params),
		LegacyParams: clonarValores(params),
		Version:      extraerVersion(params),
	}
	if formato := strings.TrimSpace(parametro(params, "format", "signFormat")); formato != "" {
		f, parseErr := application.ParseSignatureFormat(formato)
		if parseErr != nil {
			return Solicitud{}, true, fmt.Errorf("formato de firma no valido en afirma://: %w", parseErr)
		}
		solicitud.Formato = f
	}
	if action == "signandsave" {
		// Como Java, signandsave admite cofirma y contrafirma con "cop".
		if accion := accionCopSignAndSave(parametro(params, "cop")); accion != "" {
			solicitud.AccionFirma = accion
		}
		payload := parametro(params, "dat", "data", "ksb64")
		if payload == "" {
			return Solicitud{}, true, errors.New("faltan parametros obligatorios en afirma://: dat, data o ksb64")
		}
		cmd, cmdErr := construirSignCommand(payload, solicitud, params)
		if cmdErr != nil {
			return Solicitud{}, true, cmdErr
		}
		solicitud.SignCommand = cmd
	}
	if err := a.validarOrigenes(ctx, nil); err != nil {
		return Solicitud{}, true, err
	}
	return solicitud, true, nil
}

func parseSocketRawQuery(raw string) url.Values {
	values := make(url.Values)
	for len(raw) > 0 {
		sep := strings.IndexAny(raw, "&;")
		part := raw
		if sep >= 0 {
			part = raw[:sep]
			raw = raw[sep+1:]
		} else {
			raw = ""
		}
		if part == "" {
			continue
		}
		keyRaw, valueRaw, hasValue := strings.Cut(part, "=")
		keyDecoded, err := url.QueryUnescape(strings.TrimSpace(keyRaw))
		if err != nil {
			keyDecoded = strings.TrimSpace(keyRaw)
		}
		keyLower := strings.ToLower(strings.TrimSpace(keyDecoded))
		if hasValue && (keyLower == "dat" || keyLower == "data" || keyLower == "ksb64") {
			values.Add(keyDecoded, valueRaw+rawTailForDat(raw))
			break
		}
		if !hasValue {
			values.Add(keyDecoded, "")
			continue
		}
		valueDecoded, err := url.QueryUnescape(valueRaw)
		if err != nil {
			valueDecoded = valueRaw
		}
		values.Add(keyDecoded, valueDecoded)
	}
	return values
}

func rawTailForDat(rest string) string {
	if rest == "" {
		return ""
	}
	return "&" + rest
}

func extraerOperacionRawCompat(actionHost, opQuery string) string {
	if strings.TrimSpace(opQuery) != "" {
		return opQuery
	}
	return actionHost
}

func extraerOperacion(u *url.URL, opQuery string) string {
	if strings.TrimSpace(opQuery) != "" {
		return opQuery
	}
	if host := strings.TrimSpace(u.Host); host != "" {
		return host
	}
	return strings.TrimPrefix(path.Clean(u.Path), "/")
}

func normalizarOperacion(raw string) (string, domain.SignatureAction, TipoOperacion, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sign", "firmar":
		return "sign", domain.ActionSign, OperacionFirma, nil
	case "cosign", "cofirmar":
		return "cosign", domain.ActionCoSign, OperacionFirma, nil
	case "countersign", "contrafirmar":
		return "countersign", domain.ActionCounterSign, OperacionFirma, nil
	case "batch", "signbatch":
		return "batch", "", OperacionLote, nil
	case "selectcert":
		return "selectcert", "", OperacionSelectCert, nil
	case "save":
		return "save", "", OperacionSave, nil
	case "load":
		return "load", "", OperacionLoad, nil
	case "signandsave":
		return "signandsave", domain.ActionSign, OperacionSignSave, nil
	default:
		return "", "", "", fmt.Errorf("operacion afirma:// no reconocida: %s", raw)
	}
}

// accionCopSignAndSave traduce el parámetro "cop" de signandsave.
func accionCopSignAndSave(cop string) domain.SignatureAction {
	switch strings.ToLower(strings.TrimSpace(cop)) {
	case "cosign":
		return domain.ActionCoSign
	case "countersign":
		return domain.ActionCounterSign
	case "sign":
		return domain.ActionSign
	default:
		return ""
	}
}

func operacionLegacyLocal(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "save", "load", "signandsave":
		return true
	default:
		return false
	}
}

func aplicarHintsSesionSocket(solicitud *Solicitud, parametros url.Values) {
	if solicitud == nil {
		return
	}
	if solicitud.Sesion.RequestID == "" {
		solicitud.Sesion.RequestID = strings.TrimSpace(parametro(parametros, "fileid", "id", "fileId", "requestId"))
	}
	if solicitud.Sesion.SessionKey == "" {
		solicitud.Sesion.SessionKey = strings.TrimSpace(parametro(parametros, "key", "cipherKey"))
	}
	if solicitud.Sesion.UploadEndpoint == "" {
		solicitud.Sesion.UploadEndpoint = normalizarEndpoint(strings.TrimSpace(parametro(parametros, "stservlet", "storageservlet", "stServlet", "storageServlet")))
	}
	if solicitud.Sesion.RetrieveEndpoint == "" {
		retrieveURL := normalizarEndpoint(strings.TrimSpace(parametro(parametros, "rtservlet", "retrieveservlet", "rtServlet", "retrieveServlet")))
		if retrieveURL == "" {
			retrieveURL = solicitud.Sesion.UploadEndpoint
		}
		solicitud.Sesion.RetrieveEndpoint = retrieveURL
	}
	if solicitud.Sesion.State == "" && (solicitud.Sesion.RequestID != "" || solicitud.Sesion.UploadEndpoint != "" || solicitud.Sesion.RetrieveEndpoint != "") {
		solicitud.Sesion.State = domain.SessionActive
	}
}

func parametrosSocketLegacySuficientes(parametros url.Values) bool {
	return parametro(parametros, "idsession", "idSession", "dat", "data", "ksb64", "properties") != ""
}

func construirSesion(parametros url.Values) (domain.ExchangeSession, []string, error) {
	requestID := strings.TrimSpace(parametro(parametros, "fileid", "id", "fileId", "requestId"))
	retrieveURL := normalizarEndpoint(strings.TrimSpace(parametro(parametros, "rtservlet", "retrieveservlet", "rtServlet", "retrieveServlet")))
	uploadURL := normalizarEndpoint(strings.TrimSpace(parametro(parametros, "stservlet", "storageservlet", "stServlet", "storageServlet")))
	sessionKey := strings.TrimSpace(parametro(parametros, "key", "cipherKey"))

	if requestID == "" {
		return domain.ExchangeSession{}, nil, errors.New("faltan parametros obligatorios en afirma://: id o fileid")
	}
	if uploadURL == "" && retrieveURL == "" {
		return domain.ExchangeSession{}, nil, errors.New("faltan parametros obligatorios en afirma://: rtservlet o stservlet")
	}
	if uploadURL == "" {
		uploadURL = retrieveURL
	}
	if retrieveURL == "" {
		if derived := deriveRetrieveEndpointFromLegacyProperties(parametros); derived != "" {
			retrieveURL = derived
		} else {
			retrieveURL = uploadURL
		}
	}

	origenes, err := origenesDesdeEndpoints(uploadURL, retrieveURL)
	if err != nil {
		return domain.ExchangeSession{}, nil, err
	}

	return domain.ExchangeSession{
		RequestID:        requestID,
		SessionKey:       sessionKey,
		UploadEndpoint:   uploadURL,
		RetrieveEndpoint: retrieveURL,
		State:            domain.SessionActive,
	}, origenes, nil
}

func deriveRetrieveEndpointFromLegacyProperties(parametros url.Values) string {
	props := opcionesFirmaLegacy(parametros)
	if len(props) == 0 {
		return ""
	}
	for _, key := range []string{"serverUrl", "serverurl", "server_url"} {
		if raw := strings.TrimSpace(props[key]); raw != "" {
			return normalizarEndpoint(raw)
		}
	}
	return ""
}

func origenesDesdeEndpoints(endpoints ...string) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if strings.TrimSpace(endpoint) == "" {
			continue
		}
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("endpoint de afirma:// no valido: %w", err)
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, errors.New("endpoint de afirma:// sin esquema u host")
		}
		origin := u.Scheme + "://" + u.Host
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		out = append(out, origin)
	}
	return out, nil
}

func (a *Adaptador) validarOrigenes(ctx context.Context, origenes []string) error {
	if a == nil || a.trust == nil {
		return nil
	}
	for _, origen := range origenes {
		decision, err := a.trust.Evaluate(ctx, origen)
		if err != nil {
			return fmt.Errorf("no se pudo validar el origen %s: %w", origen, err)
		}
		if !decision.IsAllowed() {
			return fmt.Errorf("el origen %s no esta autorizado", origen)
		}
	}
	return nil
}

func construirSignCommand(payload string, solicitud Solicitud, parametros url.Values) (*application.SignCommand, error) {
	datos, err := decodeProtocolBase64(payload)
	if err != nil {
		return nil, fmt.Errorf("el payload embebido en afirma:// no es base64 valido: %w", err)
	}
	return construirSignCommandDesdeBytes(datos, solicitud, parametros)
}

func construirSignCommandSocketCompatible(payload string, solicitud Solicitud, parametros url.Values) (*application.SignCommand, error) {
	datos, err := decodeProtocolBase64(payload)
	if err == nil {
		return construirSignCommandDesdeBytes(datos, solicitud, parametros)
	}
	// Compatibilidad: algunos portales websocket legacy envían `dat=` como
	// contenido ya decodificado por la query (por ejemplo XML/XAdES en claro)
	// y no como base64 puro. En esos casos la V1 seguía mostrando el selector
	// y firmando el contenido recibido, en vez de abortar con SAF_03 antes del UI.
	if strings.TrimSpace(payload) == "" {
		return nil, fmt.Errorf("el payload embebido en afirma:// no es base64 valido: %w", err)
	}
	return construirSignCommandDesdeBytes([]byte(payload), solicitud, parametros)
}

func construirSignCommandDesdeBytes(datos []byte, solicitud Solicitud, parametros url.Values) (*application.SignCommand, error) {
	cmd, err := application.NewSignCommand(
		"entrada.bin",
		datos,
		"application/octet-stream",
		string(solicitud.Formato),
		string(solicitud.AccionFirma),
		"",
		opcionesFirmaLegacy(parametros),
	)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

func opcionesFirmaLegacy(parametros url.Values) map[string]string {
	props := make(map[string]string)
	for _, clave := range []string{"params", "properties", "extraParams", "extraparams"} {
		raw := strings.TrimSpace(parametro(parametros, clave))
		if raw == "" {
			continue
		}
		body := raw
		if decoded, err := decodeProtocolBase64(raw); err == nil {
			body = string(decoded)
		}
		for k, v := range decodeBatchExtraParams(body) {
			props[k] = v
		}
	}
	// Java admite la variante XML en el propio nombre del formato
	// (format=XAdES Enveloped): se traslada a la propiedad "format".
	if formato := strings.TrimSpace(parametro(parametros, "format", "signFormat")); strings.Contains(formato, " ") {
		if _, fijado := props["format"]; !fijado {
			props["format"] = formato
		}
	}
	// El algoritmo pedido en la URL (algorithm=SHA512withRSA) debe llegar al
	// motor, como en AutoFirma Java. Sin política de máquina que permita SHA-1,
	// una petición SHA-1 se eleva a SHA-256 en vez de fallar.
	if algoritmo := strings.TrimSpace(parametro(parametros, "algorithm")); algoritmo != "" {
		if _, fijado := props["algorithm"]; !fijado {
			props["algorithm"] = elevarAlgoritmoSHA1(algoritmo)
		}
	}
	if len(props) == 0 {
		return nil
	}
	return props
}

func elevarAlgoritmoSHA1(algoritmo string) string {
	normalizado := strings.ToUpper(strings.ReplaceAll(algoritmo, "-", ""))
	if !strings.HasPrefix(normalizado, "SHA1") || cryptopolicy.LegacySHA1Enabled() {
		return algoritmo
	}
	elevado := "SHA256withRSA"
	if i := strings.Index(strings.ToLower(algoritmo), "with"); i >= 0 {
		elevado = "SHA256with" + algoritmo[i+len("with"):]
	}
	avisos.Registrar(
		"Algoritmo reforzado a SHA-256",
		"el portal pidió "+algoritmo+", un algoritmo obsoleto e inseguro; se firma con "+elevado+", que el portal debe aceptar igualmente",
	)
	return elevado
}

// normalizarEndpoint aplica url.QueryUnescape a un valor de endpoint que puede
// haber llegado con doble-encoding (frecuente en integraciones Guadaltel/Portafirmas).
// Si el valor ya es una URL válida con esquema, lo devuelve sin modificar.
// Si tras decodificar parece una URL válida, devuelve la versión decodificada.
// Nunca produce panic ni error: ante cualquier ambigüedad devuelve el valor original.
func normalizarEndpoint(s string) string {
	if s == "" {
		return s
	}
	// Si ya tiene esquema válido no necesita decodificación adicional.
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	// Intentar un nivel extra de url-decode (doble-encoding: %253A → %3A → :)
	if decoded, err := url.QueryUnescape(s); err == nil {
		if strings.HasPrefix(decoded, "http://") || strings.HasPrefix(decoded, "https://") {
			return decoded
		}
	}
	return s
}

// normalizarURI aplica correcciones al encoding especial utilizado por algunas
// integraciones (Portafirmas/Guadaltel) antes de pasarla a url.Parse:
//
//  1. Doble-encoding: %25XX → %XX en los valores de query. Algunos integradores
//     encodean las URLs de servlet dos veces, resultando en %253A en vez de %3A,
//     %252F en vez de %2F, %2520 en vez de %20, etc. Se detecta la presencia de
//     %25 seguido de dos digitos hexadecimales y se elimina la capa extra.
//
//  2. '+' en valores de query: url.ParseQuery interpreta '+' como espacio segun
//     application/x-www-form-urlencoded, pero AutoFirma puede enviar '+' como
//     parte de valores base64 standard (caracter valido en la alfabeto base64).
//     Para evitar la conversion se sustituyen '+' en valores por %2B antes de
//     que url.ParseQuery los procese.
//
// Esta funcion nunca hace panic; si la entrada esta mal formada simplemente devuelve
// la cadena sin modificar para que el error quede en url.Parse.
func normalizarURI(raw string) string {
	if raw == "" {
		return raw
	}
	// Separar la query del resto para no tocar el path/host.
	sepQ := strings.IndexByte(raw, '?')
	if sepQ < 0 {
		return raw
	}
	base := raw[:sepQ+1]
	query := raw[sepQ+1:]

	// 1. Corregir doble-encoding en valores: %25XX → %XX.
	//    Solo se aplica cuando %25 va seguido de exactamente dos digitos hexadecimales
	//    (es decir, cuando codifica un '%' que a su vez inicia una secuencia %XX valida).
	query = desdoblarPorcentajeEnQuery(query)

	// 2. Los '+' en la query de una URL forman parte de la sintaxis
	//    application/x-www-form-urlencoded y url.ParseQuery los interpreta como espacio.
	//    AutoFirma usa '+' dentro de valores base64 sin codificar. Para que url.ParseQuery
	//    no los convierta a espacio los reemplazamos por %2B; la funcion
	//    decodeProtocolBase64 trabaja sobre el valor ya decodificado por url.ParseQuery,
	//    que devuelve el caracter '+' para la secuencia %2B.
	//    Solo se sustituyen '+' dentro de los *valores* (despues de '=') para
	//    no romper claves que pudieran contener '+'.
	query = normalizarPlusEnQuery(query)

	return base + query
}

// desdoblarPorcentajeEnQuery elimina doble-encoding en los valores de una query string.
// Convierte secuencias %25XX → %XX cuando XX son dos digitos hexadecimales.
func desdoblarPorcentajeEnQuery(query string) string {
	if !strings.Contains(query, "%25") {
		return query
	}
	var sb strings.Builder
	sb.Grow(len(query))
	for i := 0; i < len(query); {
		if i+4 < len(query) && query[i] == '%' && query[i+1] == '2' && query[i+2] == '5' &&
			esHex(query[i+3]) && esHex(query[i+4]) {
			// %25XX → %XX (eliminar la capa extra de encoding del '%')
			sb.WriteByte('%')
			sb.WriteByte(query[i+3])
			sb.WriteByte(query[i+4])
			i += 5
		} else {
			sb.WriteByte(query[i])
			i++
		}
	}
	return sb.String()
}

// esHex devuelve true si el byte es un digito hexadecimal valido.
func esHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// normalizarPlusEnQuery reemplaza los '+' que aparecen en los *valores* de una query string
// por %2B, preservando los '+' que pudieran estar en las claves (muy infrecuente).
func normalizarPlusEnQuery(query string) string {
	if !strings.ContainsRune(query, '+') {
		return query
	}
	var sb strings.Builder
	sb.Grow(len(query))
	inValue := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		switch {
		case c == '=':
			inValue = true
			sb.WriteByte(c)
		case c == '&' || c == ';':
			inValue = false
			sb.WriteByte(c)
		case c == '+' && inValue:
			sb.WriteString("%2B")
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// extraerVersion extrae la version del protocolo del parametro v o ver.
// Devuelve 0 si el parametro no existe o no es un entero valido.
func extraerVersion(parametros url.Values) int {
	raw := strings.TrimSpace(parametro(parametros, "v", "ver", "version"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parametro(v url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(v.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func construirRemoteBatchCommand(parametros url.Values, sesion domain.ExchangeSession, payloadB64 string) (*RemoteBatchCommand, error) {
	preURL := normalizarEndpoint(strings.TrimSpace(parametro(parametros, "batchpresignerurl", "batchPreSignerUrl", "batchPreSignerURL")))
	postURL := normalizarEndpoint(strings.TrimSpace(parametro(parametros, "batchpostsignerurl", "batchPostSignerUrl", "batchPostSignerURL")))
	if preURL == "" || postURL == "" {
		return nil, nil
	}
	raw, err := decodeProtocolBase64(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("el payload batch embebido en afirma:// no es base64 valido: %w", err)
	}
	isJSONBatch := parseBoolLegacy(parametro(parametros, "jsonbatch", "jsonBatch"))
	if !isJSONBatch {
		trimmed := strings.TrimSpace(string(raw))
		switch {
		case strings.HasPrefix(trimmed, "<"):
			if _, err := parseBatchXMLRequest(raw); err != nil {
				return nil, nil
			}
		default:
			if _, err := parseBatchJSONRequest(raw); err == nil {
				isJSONBatch = true
			} else {
				return nil, nil
			}
		}
	}
	return &RemoteBatchCommand{
		Session:          sesion,
		Payload:          raw,
		IsJSONBatch:      isJSONBatch,
		PreSignEndpoint:  preURL,
		PostSignEndpoint: postURL,
		NeedCert:         parseBoolLegacy(parametro(parametros, "needcert", "needCert")),
		LegacyParams:     clonarValores(parametros),
	}, nil
}

func parseBoolLegacy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "si", "on":
		return true
	default:
		return false
	}
}

func fusionarOrigenes(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, origen := range append(append([]string(nil), base...), extra...) {
		origen = strings.TrimSpace(origen)
		if origen == "" {
			continue
		}
		if _, ok := seen[origen]; ok {
			continue
		}
		seen[origen] = struct{}{}
		out = append(out, origen)
	}
	return out
}

func clonarValores(src url.Values) url.Values {
	if len(src) == 0 {
		return nil
	}
	dst := make(url.Values, len(src))
	for k, vals := range src {
		dst[k] = append([]string(nil), vals...)
	}
	return dst
}
