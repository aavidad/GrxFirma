// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package truststore implementa ports.TrustPolicy con soporte para allowlist del
// sistema, decisiones persistidas del usuario y modo TOFU (Trust On First Use).
package truststore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/machinepolicy"
)

const (
	// SystemAllowlistPath es la ruta por defecto de la allowlist del sistema.
	SystemAllowlistPath    = "/etc/grxfirma/allowed-domains.json"
	userDecisionsFile      = "trusted-domains.json"
	seedMarkerFile         = "trusted-domains.seeded"
	compatSeedMarkerFile   = "trusted-domains.compatibility-v1.seeded"
	seedHardeningMarker    = "trusted-domains.hardening-2026-09.migrated"
	maxTruststoreFileBytes = 1024 * 1024
)

// dominiosConfiadosPorDefecto es la semilla de dominios de administraciones
// públicas españolas que se confían automáticamente (sin prompt TOFU) en
// despliegues de escritorio no gestionados.
//
// DECISIÓN DE PRODUCTO (hallazgo de auditoría H-10, riesgo aceptado): esta
// allowlist es deliberadamente amplia (comodines por dominio autonómico y
// estatal) para preservar la paridad de compatibilidad con AutoFirma 1.9, donde
// estos portales operan sin fricción. NO se reduce.
//
// El riesgo (un subdominio comprometido dentro de estos espacios podría iniciar
// firmas sin prompt) se acota así:
//   - Solo aplica a despliegues NO gestionados. Un despliegue gestionado fija
//     la allowlist del sistema (SystemAllowlistPath, o HKLM en Windows) y puede
//     desactivar TOFU mediante la política de máquina, ignorando esta semilla.
//   - La firma sigue exigiendo la interacción del usuario con su certificado.
//   - Un patrón de dominio solo autoriza orígenes HTTPS (originMatches).
//
// Ver docs/DECISION_ALLOWLIST_DOMINIOS.md para la justificación completa.
var dominiosConfiadosPorDefecto = []string{
	"valide.redsara.es",
	"*.gob.es",
	"*.administracion.gob.es",
	"*.dipgra.es",
	"*.junta-andalucia.es",
	"*.xunta.gal",
	"*.gva.es",
	"*.generalitat.cat",
	"*.euskadi.eus",
	"*.jcyl.es",
	"*.navarra.es",
	"*.aragon.es",
	"*.cantabria.es",
	"*.asturias.es",
	"*.extremadura.es",
	"*.larioja.org",
	"*.carm.es",
	"*.canarias.org",
	"*.caib.es",
	"*.ceuta.es",
	"*.melilla.es",
	// Único origen local confiado: la página del propio firmador local de
	// GrxFirma. Otros servidores locales se autorizan expresamente.
	"https://127.0.0.1:63118",
	"https://localhost:63118",
}

// origenesLocalesPropios sustituyen al antiguo loopback en cualquier puerto.
var origenesLocalesPropios = []string{
	"https://127.0.0.1:63118",
	"https://localhost:63118",
}

// origenesRetiradosDeLaSemilla son entradas que versiones anteriores
// sembraban automáticamente y que ya no se consideran de confianza por
// defecto: un dominio de una empresa privada y el loopback en cualquier
// puerto (cualquier servidor web local del equipo). La migración las retira
// una sola vez de los perfiles sembrados; el usuario o la política de
// máquina pueden volver a autorizarlas de forma explícita.
var origenesRetiradosDeLaSemilla = []string{
	"*.guadaltel.es",
	"localhost",
	"127.0.0.1",
	"::1",
}

// dominiosCompatibilidadV1 se añaden una sola vez también a perfiles que ya
// habían completado la semilla original. El marcador independiente permite
// que el usuario los elimine después sin que vuelvan a aparecer al reiniciar.
var dominiosCompatibilidadV1 = []string{
	"valide.redsara.es",
}

// ErrHeadlessUnknownOrigin se devuelve en modo headless cuando el origen es desconocido.
var ErrHeadlessUnknownOrigin = errors.New("origen desconocido en modo headless: no es posible pedir confirmacion al usuario")
var ErrTOFUDisabledUnknownOrigin = errors.New("origen desconocido y TOFU deshabilitado por politica")

// SeedMarkerPath devuelve la ruta del marcador que indica que la semilla inicial
// de dominios públicos ya fue aplicada sobre el perfil del usuario.
func SeedMarkerPath(configDir string) string {
	return filepath.Join(configDir, seedMarkerFile)
}

// GestorConfianza implementa ports.TrustPolicy con persistencia JSON.
type GestorConfianza struct {
	mu              sync.RWMutex
	configDir       string
	headless        bool
	tofuEnabled     bool
	systemAllowlist string
	systemAllowed   map[string]struct{}
	userDecisions   map[string]domain.TrustStatus
}

// Options parametriza la construcción del truststore sin acoplarlo al origen
// de configuración.
type Options struct {
	SystemAllowlistFile string
	Headless            bool
	TOFUEnabled         bool
	ExtraAllowed        []string
}

// New construye un GestorConfianza cargando la allowlist del sistema desde
// SystemAllowlistPath y las decisiones persistidas del usuario desde configDir.
// Si headless es true, los origenes desconocidos retornan error en lugar de TrustPending.
func New(configDir string, headless bool) (*GestorConfianza, error) {
	return NewWithOptions(configDir, Options{
		SystemAllowlistFile: SystemAllowlistPath,
		Headless:            headless,
		TOFUEnabled:         true,
	})
}

// NewWithSystemAllowlist es igual que New pero permite especificar una ruta de
// allowlist del sistema diferente a la por defecto. Util en tests.
func NewWithSystemAllowlist(configDir, systemAllowlistFile string, headless bool) (*GestorConfianza, error) {
	return NewWithOptions(configDir, Options{
		SystemAllowlistFile: systemAllowlistFile,
		Headless:            headless,
		TOFUEnabled:         true,
	})
}

// NewWithOptions construye un GestorConfianza permitiendo parametrizar la
// allowlist del sistema, una allowlist extra administrada y si TOFU queda
// habilitado para origenes desconocidos.
func NewWithOptions(configDir string, opts Options) (*GestorConfianza, error) {
	systemAllowlistFile := opts.SystemAllowlistFile
	if systemAllowlistFile == "" {
		systemAllowlistFile = SystemAllowlistPath
	}
	g := &GestorConfianza{
		configDir:       configDir,
		headless:        opts.Headless,
		tofuEnabled:     opts.TOFUEnabled,
		systemAllowlist: systemAllowlistFile,
		systemAllowed:   make(map[string]struct{}),
		userDecisions:   make(map[string]domain.TrustStatus),
	}
	if err := g.loadSystemAllowlist(); err != nil {
		return nil, fmt.Errorf("error cargando allowlist del sistema: %w", err)
	}
	g.mergeExtraAllowed(opts.ExtraAllowed)
	_, err := g.loadUserDecisions()
	if err != nil {
		return nil, fmt.Errorf("error cargando decisiones del usuario: %w", err)
	}
	if g.tofuEnabled {
		if err := g.seedUserDecisions(); err != nil {
			return nil, fmt.Errorf("error sembrando decisiones iniciales: %w", err)
		}
		if err := g.migrarSemillaEndurecida(); err != nil {
			return nil, fmt.Errorf("error migrando la semilla de confianza: %w", err)
		}
	}
	return g, nil
}

// Evaluate devuelve la decision de confianza para el origen dado.
// Orden de evaluacion:
//  1. Allowlist del sistema → TrustAllowed
//  2. Decisiones persistidas del usuario → TrustAllowed o TrustDenied
//  3. Origen desconocido → TrustPending (o error en modo headless / TOFU deshabilitado)
func (g *GestorConfianza) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if patron, ok := g.findAllowedMatch(g.systemAllowed, origin); ok {
		return domain.TrustDecision{
			Origin: origin,
			Status: domain.TrustAllowed,
			Reason: "dominio en allowlist del sistema: " + patron,
		}, nil
	}

	if patron, status, ok := g.findUserDecision(origin); ok {
		reason := "decision persistida del usuario"
		return domain.TrustDecision{
			Origin: origin,
			Status: status,
			Reason: reason + ": " + patron,
		}, nil
	}

	if !g.tofuEnabled {
		return domain.TrustDecision{}, fmt.Errorf("%w: %s", ErrTOFUDisabledUnknownOrigin, origin)
	}

	if g.headless {
		return domain.TrustDecision{}, fmt.Errorf("%w: %s", ErrHeadlessUnknownOrigin, origin)
	}

	return domain.TrustDecision{
		Origin: origin,
		Status: domain.TrustPending,
		Reason: "origen no conocido, pendiente de decision del usuario",
	}, nil
}

// DefaultTrustedOrigins devuelve la semilla de dominios confiados usada en el primer arranque.
func DefaultTrustedOrigins() []string {
	out := make([]string, len(dominiosConfiadosPorDefecto))
	copy(out, dominiosConfiadosPorDefecto)
	return out
}

// DecisionsSnapshot devuelve la vista efectiva de confianza gestionada por este
// truststore. Es la API de lectura para UI/CLI; otros paquetes no deben leer
// trusted-domains.json ni las allowlists por su cuenta.
func (g *GestorConfianza) DecisionsSnapshot() []domain.TrustDecision {
	g.mu.RLock()
	defer g.mu.RUnlock()

	estado := make(map[string]domain.TrustStatus, len(g.systemAllowed)+len(g.userDecisions))
	motivos := make(map[string]string, len(g.systemAllowed)+len(g.userDecisions))
	for origin := range g.systemAllowed {
		if stringsTrim(origin) == "" {
			continue
		}
		estado[origin] = domain.TrustAllowed
		motivos[origin] = "allowlist del sistema"
	}

	sembrado := map[string]struct{}{}
	if _, err := os.Stat(SeedMarkerPath(g.configDir)); err == nil {
		for _, origin := range dominiosConfiadosPorDefecto {
			if stringsTrim(origin) != "" {
				sembrado[origin] = struct{}{}
			}
		}
	}
	for origin, status := range g.userDecisions {
		if status != domain.TrustAllowed && status != domain.TrustDenied {
			continue
		}
		estado[origin] = status
		if _, ok := sembrado[origin]; ok && status == domain.TrustAllowed {
			motivos[origin] = "semilla inicial de dominios públicos"
		} else {
			motivos[origin] = "decisión persistida del usuario"
		}
	}

	origenes := make([]string, 0, len(estado))
	for origin := range estado {
		origenes = append(origenes, origin)
	}
	sort.Strings(origenes)

	out := make([]domain.TrustDecision, 0, len(origenes))
	for _, origin := range origenes {
		out = append(out, domain.TrustDecision{
			Origin: origin,
			Status: estado[origin],
			Reason: motivos[origin],
		})
	}
	return out
}

// Allow persiste la decision de confianza "allowed" para el origen y la guarda en disco.
func (g *GestorConfianza) Allow(_ context.Context, origin string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.userDecisions[origin] = domain.TrustAllowed
	return g.saveUserDecisions()
}

// Deny persiste la decision de confianza "denied" para el origen y la guarda en disco.
func (g *GestorConfianza) Deny(_ context.Context, origin string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.userDecisions[origin] = domain.TrustDenied
	return g.saveUserDecisions()
}

// Remove elimina la decision persistida para el origen y la guarda en disco.
func (g *GestorConfianza) Remove(_ context.Context, origin string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.userDecisions, origin)
	return g.saveUserDecisions()
}

// loadSystemAllowlist carga la allowlist del sistema si existe.
func (g *GestorConfianza) loadSystemAllowlist() error {
	if g.systemAllowlist == SystemAllowlistPath && machinepolicy.Native() {
		// En Windows /etc/... se resolvería como C:\etc, creable por
		// cualquier usuario. La allowlist del sistema solo procede de
		// HKLM\SOFTWARE\Policies\GrxFirma (valor allowed_domains).
		domains, _, err := machinepolicy.Strings(machinepolicy.AllowedDomains)
		if err != nil {
			return err
		}
		for _, d := range domains {
			g.systemAllowed[d] = struct{}{}
		}
		return nil
	}
	// Solo la política bajo /etc puede usar enlaces administrados por root.
	// Las rutas inyectadas en pruebas y las decisiones del usuario conservan
	// OpenRead con O_NOFOLLOW.
	read := securefile.ReadFileLimit
	if g.systemAllowlist == SystemAllowlistPath {
		read = securefile.ReadTrustedSystemFileLimit
	}
	data, err := read(g.systemAllowlist, maxTruststoreFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var domains []string
	if err := json.Unmarshal(data, &domains); err != nil {
		return fmt.Errorf("formato invalido en allowlist del sistema: %w", err)
	}
	for _, d := range domains {
		g.systemAllowed[d] = struct{}{}
	}
	return nil
}

func (g *GestorConfianza) mergeExtraAllowed(domains []string) {
	for _, d := range domains {
		d = stringsTrim(d)
		if d == "" {
			continue
		}
		g.systemAllowed[d] = struct{}{}
	}
}

// loadUserDecisions carga configDir/trusted-domains.json si existe.
func (g *GestorConfianza) loadUserDecisions() (bool, error) {
	path := filepath.Join(g.configDir, userDecisionsFile)
	data, err := securefile.ReadFileLimit(path, maxTruststoreFileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return true, fmt.Errorf("formato invalido en decisiones del usuario: %w", err)
	}
	for origin, statusStr := range raw {
		switch domain.TrustStatus(statusStr) {
		case domain.TrustAllowed, domain.TrustDenied:
			g.userDecisions[origin] = domain.TrustStatus(statusStr)
		default:
			// Estado desconocido, se ignora.
		}
	}
	return true, nil
}

// saveUserDecisions escribe configDir/trusted-domains.json con las decisiones actuales.
// Requiere que el caller tenga el mutex en escritura.
func (g *GestorConfianza) saveUserDecisions() error {
	if err := os.MkdirAll(g.configDir, 0o700); err != nil {
		return fmt.Errorf("no se pudo crear directorio de configuracion: %w", err)
	}
	raw := make(map[string]string, len(g.userDecisions))
	for origin, status := range g.userDecisions {
		raw[origin] = string(status)
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializando decisiones: %w", err)
	}
	path := filepath.Join(g.configDir, userDecisionsFile)
	return securefile.WriteFileAtomic(path, data, 0o600)
}

func (g *GestorConfianza) seedUserDecisions() error {
	marker := filepath.Join(g.configDir, seedMarkerFile)
	if _, err := securefile.ReadFileLimit(marker, 1024); err == nil {
		return g.seedCompatibilityOrigins()
	} else if !os.IsNotExist(err) {
		return err
	}

	cambios := 0
	for _, origin := range dominiosConfiadosPorDefecto {
		if _, ok := g.userDecisions[origin]; ok {
			continue
		}
		g.userDecisions[origin] = domain.TrustAllowed
		cambios++
	}
	if cambios > 0 {
		if err := g.saveUserDecisions(); err != nil {
			return err
		}
	} else if err := os.MkdirAll(g.configDir, 0o700); err != nil {
		return fmt.Errorf("no se pudo crear directorio de configuracion: %w", err)
	}
	if err := securefile.WriteFileAtomic(marker, []byte("seeded-default-trusted-domains\n"), 0o600); err != nil {
		return err
	}
	return g.seedCompatibilityOrigins()
}

func (g *GestorConfianza) seedCompatibilityOrigins() error {
	marker := filepath.Join(g.configDir, compatSeedMarkerFile)
	if _, err := securefile.ReadFileLimit(marker, 1024); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	cambios := 0
	for _, origin := range dominiosCompatibilidadV1 {
		if _, ok := g.userDecisions[origin]; ok {
			continue
		}
		g.userDecisions[origin] = domain.TrustAllowed
		cambios++
	}
	if cambios > 0 {
		if err := g.saveUserDecisions(); err != nil {
			return err
		}
	} else if err := os.MkdirAll(g.configDir, 0o700); err != nil {
		return fmt.Errorf("no se pudo crear directorio de configuracion: %w", err)
	}
	return securefile.WriteFileAtomic(
		marker,
		[]byte("seeded-web-compatibility-origins-v1\n"),
		0o600,
	)
}

// migrarSemillaEndurecida retira, una sola vez, las entradas que la semilla
// antigua añadía y que ya no son de confianza por defecto. Solo actúa sobre
// perfiles que recibieron esa semilla y solo sobre decisiones "allowed".
func (g *GestorConfianza) migrarSemillaEndurecida() error {
	marker := filepath.Join(g.configDir, seedHardeningMarker)
	if _, err := securefile.ReadFileLimit(marker, 1024); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	cambios := 0
	teniaLoopback := false
	for _, origin := range origenesRetiradosDeLaSemilla {
		if g.userDecisions[origin] == domain.TrustAllowed {
			delete(g.userDecisions, origin)
			cambios++
			teniaLoopback = teniaLoopback || origin != "*.guadaltel.es"
		}
	}
	// Solo se sustituye el loopback retirado; nunca se reintroducen
	// entradas que el usuario hubiera borrado.
	if teniaLoopback {
		for _, origin := range origenesLocalesPropios {
			if _, ok := g.userDecisions[origin]; !ok {
				g.userDecisions[origin] = domain.TrustAllowed
				cambios++
			}
		}
	}
	if cambios > 0 {
		if err := g.saveUserDecisions(); err != nil {
			return err
		}
	}
	return securefile.WriteFileAtomic(marker, []byte("hardened-seed-2026-09\n"), 0o600)
}

func (g *GestorConfianza) findAllowedMatch(allowed map[string]struct{}, origin string) (string, bool) {
	for patron := range allowed {
		if originMatches(origin, patron) {
			return patron, true
		}
	}
	return "", false
}

func (g *GestorConfianza) findUserDecision(origin string) (string, domain.TrustStatus, bool) {
	for patron, status := range g.userDecisions {
		if !g.tofuEnabled && status == domain.TrustAllowed {
			continue
		}
		if originMatches(origin, patron) {
			return patron, status, true
		}
	}
	return "", "", false
}

func originMatches(origin, patron string) bool {
	origin = strings.ToLower(stringsTrim(origin))
	patron = strings.ToLower(stringsTrim(patron))
	if origin == "" || patron == "" {
		return false
	}
	if patron == origin {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	host := stringsTrim(u.Hostname())
	if host == "" {
		return false
	}

	// Patrón con esquema (p. ej. "https://sede.ejemplo.es:8443" o
	// "chrome-extension://<id>"): coincidencia exacta de esquema, host
	// (admite "*.") y puerto efectivo.
	if strings.Contains(patron, "://") {
		p, err := url.Parse(patron)
		if err != nil || p.Host == "" || p.User != nil ||
			strings.TrimRight(p.Path, "/") != "" || p.RawQuery != "" || p.Fragment != "" {
			return false
		}
		return p.Scheme == u.Scheme &&
			hostMatches(host, p.Hostname()) &&
			puertoEfectivo(u) == puertoEfectivo(p)
	}

	// Patrón de dominio ("sede.ejemplo.es", "*.gob.es", "host:puerto"):
	// solo autoriza orígenes HTTPS. Antes bastaba con el nombre de host, de
	// modo que http:// o cualquier esquema quedaban igualmente autorizados.
	if u.Scheme != "https" {
		return false
	}
	hostPatron, puertoPatron := patron, ""
	if h, p, err := net.SplitHostPort(patron); err == nil {
		hostPatron, puertoPatron = h, p
	}
	if puertoPatron != "" && puertoPatron != puertoEfectivo(u) {
		return false
	}
	return hostMatches(host, strings.Trim(hostPatron, "[]"))
}

func hostMatches(host, patron string) bool {
	if patron == "" {
		return false
	}
	if len(patron) > 2 && patron[:2] == "*." {
		sufijo := patron[2:]
		return len(host) > len(sufijo)+1 && host[len(host)-len(sufijo):] == sufijo && host[len(host)-len(sufijo)-1] == '.'
	}
	return host == patron
}

func puertoEfectivo(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch u.Scheme {
	case "https", "wss":
		return "443"
	case "http", "ws":
		return "80"
	default:
		return ""
	}
}

func stringsTrim(v string) string {
	for len(v) > 0 && (v[0] == ' ' || v[0] == '\t' || v[0] == '\n' || v[0] == '\r') {
		v = v[1:]
	}
	for len(v) > 0 {
		n := len(v) - 1
		if v[n] != ' ' && v[n] != '\t' && v[n] != '\n' && v[n] != '\r' {
			break
		}
		v = v[:n]
	}
	return v
}

// Aseguramos en tiempo de compilacion que GestorConfianza implementa ports.TrustPolicy.
var _ ports.TrustPolicy = (*GestorConfianza)(nil)
