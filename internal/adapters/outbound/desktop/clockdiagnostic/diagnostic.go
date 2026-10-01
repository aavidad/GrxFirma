// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package clockdiagnostic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// MaximumAcceptedSkew compensa la resolución de un segundo de HTTP Date y
	// la latencia de ida y vuelta. Un desfase mayor se muestra, pero nunca se
	// atribuye por sí solo al equipo ni al servidor.
	MaximumAcceptedSkew = 5 * time.Second

	remoteProbeTimeout     = 4 * time.Second
	maximumOriginLength    = 2048
	maximumResponseHeaders = 32 * 1024
	localClockCode         = "local_clock"
	remoteClockCode        = "remote_clock"
	governmentAFirmaCode   = "government_afirma"
	statusSuccess          = "success"
	statusFailure          = "failure"
	statusUnknown          = "unknown"
	ownerLocal             = "app_local"
	ownerRemote            = "remote_service"
	ownerGovernmentAFirma  = "@firma"
	ownerUnknown           = "unknown"
	evidenceLocalClock     = "clock:local-read"
	evidenceHTTPProbe      = "clock:http-probe:authorized-origin"
	evidenceHTTPDate       = "clock:http-date:authorized-origin"
)

var timeNowUTC = func() time.Time {
	return time.Now().UTC()
}

var evidenceReferencePattern = regexp.MustCompile(
	`^[A-Za-z0-9._:-]{1,160}$`,
)

var nonPublicNetworkPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// AuthorizedObservedOrigin representa un origen que otro componente ya ha
// observado en una operación real y ha autorizado mediante la política de
// confianza. Este tipo nunca se construye a partir de parámetros IPC.
type AuthorizedObservedOrigin struct {
	URL         string
	EvidenceRef string
}

// AuthorizedOriginSource proporciona únicamente orígenes observados y
// autorizados. Si no existe esa evidencia, el diagnóstico remoto no se ejecuta.
type AuthorizedOriginSource interface {
	LastAuthorizedObservedHTTPSOrigin(
		ctx context.Context,
	) (AuthorizedObservedOrigin, bool)
}

// Step es una fase visual saneada del diagnóstico de reloj.
type Step struct {
	Code            string `json:"code"`
	Label           string `json:"label"`
	Status          string `json:"status"`
	Owner           string `json:"owner,omitempty"`
	UserMessage     string `json:"userMessage,omitempty"`
	SuggestedAction string `json:"suggestedAction,omitempty"`
	EvidenceRef     string `json:"evidenceRef,omitempty"`
}

// Report contiene solo resultados de comprobaciones acotadas. No expone URL,
// cabeceras, cookies, credenciales ni respuestas remotas.
type Report struct {
	ThresholdSeconds int64  `json:"thresholdSeconds"`
	Steps            []Step `json:"steps"`
}

type localSnapshot struct {
	observedAtUTC   time.Time
	timeService     string
	serviceObserved bool
}

type httpDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Service ejecuta el diagnóstico. Sin una fuente de origen autorizado, solo
// inspecciona el reloj local y publica las fases remotas como no comprobadas.
type Service struct {
	origins AuthorizedOriginSource
	client  httpDoer
	now     func() time.Time
	local   func() localSnapshot
}

func New(origins AuthorizedOriginSource) *Service {
	return &Service{
		origins: origins,
		client:  newRestrictedHTTPClient(),
		now:     time.Now,
		local:   inspectLocalClock,
	}
}

func (s *Service) Diagnose(ctx context.Context) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	local := s.local()
	report := Report{
		ThresholdSeconds: int64(MaximumAcceptedSkew / time.Second),
		Steps: []Step{
			localUnknownStep(local),
			remoteUncheckedStep(),
			afirmaUncheckedStep(),
		},
	}

	if s.origins == nil {
		return report
	}
	observed, ok := s.origins.LastAuthorizedObservedHTTPSOrigin(ctx)
	if !ok {
		return report
	}
	if !evidenceReferencePattern.MatchString(observed.EvidenceRef) {
		report.Steps[1] = Step{
			Code:            remoteClockCode,
			Label:           "Hora del servidor remoto observado",
			Status:          statusUnknown,
			Owner:           ownerUnknown,
			UserMessage:     "El origen no incluye una referencia válida que demuestre que fue observado y autorizado; no se realizó ninguna conexión.",
			SuggestedAction: "Repita una operación real desde una sede HTTPS autorizada y vuelva a ejecutar el diagnóstico.",
		}
		return report
	}
	endpoint, err := validateObservedOrigin(observed.URL)
	if err != nil {
		report.Steps[1] = Step{
			Code:            remoteClockCode,
			Label:           "Hora del servidor remoto observado",
			Status:          statusUnknown,
			Owner:           ownerUnknown,
			UserMessage:     "El origen observado no cumple las restricciones de la sonda segura; no se realizó ninguna conexión.",
			SuggestedAction: "Repita una operación real desde una sede HTTPS autorizada y vuelva a ejecutar el diagnóstico.",
		}
		return report
	}

	remote, err := s.probeHTTPDate(ctx, endpoint)
	if err != nil {
		report.Steps[1] = Step{
			Code:            remoteClockCode,
			Label:           "Hora del servidor remoto observado",
			Status:          statusFailure,
			Owner:           ownerUnknown,
			UserMessage:     "No se pudo obtener una cabecera Date válida del origen HTTPS observado y autorizado. El fallo no se atribuye al equipo ni al servidor sin más evidencia.",
			SuggestedAction: "Compruebe la conexión y repita la operación real; conserve el diagnóstico si vuelve a fallar.",
			EvidenceRef:     evidenceHTTPProbe,
		}
		return report
	}

	absoluteSkew := remote.skew
	if absoluteSkew < 0 {
		absoluteSkew = -absoluteSkew
	}
	if absoluteSkew <= MaximumAcceptedSkew {
		detail := fmt.Sprintf(
			"El reloj local y la cabecera Date del origen observado difieren %s, dentro del umbral documentado de %s.",
			formatDuration(absoluteSkew),
			MaximumAcceptedSkew,
		)
		report.Steps[0] = Step{
			Code:            localClockCode,
			Label:           "Reloj local",
			Status:          statusSuccess,
			Owner:           ownerLocal,
			UserMessage:     detail + " " + localServiceDetail(local),
			SuggestedAction: "No es necesario corregir la hora para esta evidencia.",
			EvidenceRef:     evidenceLocalClock,
		}
		report.Steps[1] = Step{
			Code:            remoteClockCode,
			Label:           "Hora del servidor remoto observado",
			Status:          statusSuccess,
			Owner:           ownerRemote,
			UserMessage:     detail + " La latencia de ida y vuelta se compensó usando su punto medio.",
			SuggestedAction: "Continúe con el diagnóstico de la operación real si la firma sigue fallando.",
			EvidenceRef:     evidenceHTTPDate,
		}
		return report
	}

	detail := fmt.Sprintf(
		"Se observó un desfase de %s entre el reloj local y la cabecera Date remota, superior al umbral de %s. Esta comparación no demuestra cuál de los dos relojes es incorrecto.",
		formatDuration(absoluteSkew),
		MaximumAcceptedSkew,
	)
	report.Steps[0] = Step{
		Code:            localClockCode,
		Label:           "Reloj local",
		Status:          statusFailure,
		Owner:           ownerUnknown,
		UserMessage:     detail + " " + localServiceDetail(local),
		SuggestedAction: "Active el ajuste automático de hora y repita la comparación antes de atribuir el fallo.",
		EvidenceRef:     evidenceLocalClock,
	}
	report.Steps[1] = Step{
		Code:            remoteClockCode,
		Label:           "Hora del servidor remoto observado",
		Status:          statusFailure,
		Owner:           ownerUnknown,
		UserMessage:     detail + " La latencia de ida y vuelta se compensó usando su punto medio.",
		SuggestedAction: "Repita la comprobación y contacte con la sede solo si el desfase se confirma con otra fuente fiable.",
		EvidenceRef:     evidenceHTTPDate,
	}
	return report
}

type remoteClockResult struct {
	skew time.Duration
}

func (s *Service) probeHTTPDate(
	ctx context.Context,
	endpoint *url.URL,
) (remoteClockResult, error) {
	probeContext, cancel := context.WithTimeout(ctx, remoteProbeTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(
		probeContext,
		http.MethodHead,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return remoteClockResult{}, err
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("User-Agent", "GrxFirma-clock-diagnostic/1")

	started := s.now().UTC()
	response, err := s.client.Do(request)
	finished := s.now().UTC()
	if err != nil {
		return remoteClockResult{}, err
	}
	if response == nil {
		return remoteClockResult{}, errors.New("respuesta HTTP vacía")
	}
	if response.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1))
		_ = response.Body.Close()
	}
	serverDate, err := http.ParseTime(response.Header.Get("Date"))
	if err != nil {
		return remoteClockResult{}, errors.New("cabecera Date ausente o inválida")
	}
	if finished.Before(started) {
		return remoteClockResult{}, errors.New("reloj monotónico incoherente")
	}
	midpoint := started.Add(finished.Sub(started) / 2)
	return remoteClockResult{skew: serverDate.UTC().Sub(midpoint)}, nil
}

func localUnknownStep(local localSnapshot) Step {
	return Step{
		Code:   localClockCode,
		Label:  "Reloj local",
		Status: statusUnknown,
		Owner:  ownerLocal,
		UserMessage: fmt.Sprintf(
			"El motor leyó la hora local %s. %s Sin una referencia remota autorizada no puede afirmar que el reloj esté sincronizado.",
			local.observedAtUTC.UTC().Format(time.RFC3339),
			localServiceDetail(local),
		),
		SuggestedAction: "Mantenga activado el ajuste automático de fecha y hora y repita una operación real para obtener una referencia remota.",
		EvidenceRef:     evidenceLocalClock,
	}
}

func localServiceDetail(local localSnapshot) string {
	if !local.serviceObserved {
		return "El estado del servicio de hora no está disponible en esta plataforma."
	}
	switch local.timeService {
	case "running":
		return "El servicio Hora de Windows está en ejecución; esto por sí solo no certifica la sincronización."
	case "stopped":
		return "El servicio Hora de Windows está detenido; esto por sí solo no demuestra que el reloj sea incorrecto."
	default:
		return "El estado del servicio Hora de Windows no pudo determinarse; no se ejecutaron comandos externos."
	}
}

func remoteUncheckedStep() Step {
	return Step{
		Code:            remoteClockCode,
		Label:           "Hora del servidor remoto observado",
		Status:          statusUnknown,
		Owner:           ownerRemote,
		UserMessage:     "No comprobado: el motor no conserva todavía un origen HTTPS observado y autorizado que pueda sondear sin aceptar endpoints arbitrarios.",
		SuggestedAction: "Abra este diagnóstico después de una operación real cuando el origen autorizado pueda vincularse de forma segura.",
	}
}

func afirmaUncheckedStep() Step {
	return Step{
		Code:            governmentAFirmaCode,
		Label:           "Plataforma @firma",
		Status:          statusUnknown,
		Owner:           ownerGovernmentAFirma,
		UserMessage:     "No comprobado: la hora HTTP de una sede no demuestra el estado de @firma y no existe una interacción directa observada con esa plataforma.",
		SuggestedAction: "Use la evidencia de una operación real que identifique expresamente @firma; no atribuya un fallo por descarte.",
	}
}

func validateObservedOrigin(raw string) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > maximumOriginLength {
		return nil, errors.New("origen vacío o demasiado largo")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("origen inválido")
	}
	if !strings.EqualFold(parsed.Scheme, "https") ||
		parsed.Hostname() == "" ||
		parsed.User != nil ||
		parsed.Opaque != "" ||
		(parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" ||
		parsed.ForceQuery ||
		parsed.Fragment != "" ||
		(parsed.Port() != "" && parsed.Port() != "443") ||
		!hasCanonicalHTTPSAuthority(parsed) {
		return nil, errors.New("el origen no es un origen HTTPS puro")
	}
	if address, err := netip.ParseAddr(parsed.Hostname()); err == nil &&
		!isPublicAddress(address) {
		return nil, errors.New("dirección no pública")
	}
	parsed.Scheme = "https"
	parsed.Path = "/"
	parsed.RawPath = ""
	return parsed, nil
}

func hasCanonicalHTTPSAuthority(parsed *url.URL) bool {
	host := parsed.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if parsed.Port() != "" {
		host += ":" + parsed.Port()
	}
	return strings.EqualFold(parsed.Host, host)
}

func newRestrictedHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            publicOnlyDialContext,
		ForceAttemptHTTP2:      true,
		DisableCompression:     true,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: maximumResponseHeaders,
		ResponseHeaderTimeout:  3 * time.Second,
		TLSHandshakeTimeout:    3 * time.Second,
		ExpectContinueTimeout:  1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   remoteProbeTimeout,
		CheckRedirect: func(
			_ *http.Request,
			_ []*http.Request,
		) error {
			return http.ErrUseLastResponse
		},
	}
}

func publicOnlyDialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("destino remoto no permitido")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, errors.New("no se pudo resolver el origen observado")
	}
	dialer := net.Dialer{Timeout: 3 * time.Second}
	for _, candidate := range addresses {
		if !isPublicAddress(candidate) {
			continue
		}
		connection, dialErr := dialer.DialContext(
			ctx,
			network,
			net.JoinHostPort(candidate.String(), port),
		)
		if dialErr == nil {
			return connection, nil
		}
	}
	return nil, errors.New("el origen observado no resolvió a una dirección pública accesible")
}

func isPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() ||
		!address.IsGlobalUnicast() ||
		address.IsPrivate() ||
		address.IsLoopback() ||
		address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() ||
		address.IsMulticast() ||
		address.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicNetworkPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func formatDuration(value time.Duration) string {
	return fmt.Sprintf("%.3f s", value.Seconds())
}
