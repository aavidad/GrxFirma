// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/certutil"
	pkcs12importer "grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/common/securefile"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// trustConClave indica que el certificado tiene clave privada asociada en el almacén NSS.
// certutil -L muestra "u,u,u" para certificados con clave.
const trustConClave = "u,u,u"

// Almacen implementa ports.CertificateCatalog leyendo almacenes NSS en Linux.
type Almacen struct {
	// certutil es el path al binario certutil; defecto: busca en $PATH.
	certutil string
	// pk12util es el path al binario pk12util; defecto: busca en $PATH.
	pk12util string
	// openssl es el path al binario openssl; defecto: busca en $PATH.
	openssl string
	// rutas son los directorios de bases de datos NSS a explorar.
	rutas []string
}

// New crea un Almacen NSS usando las rutas estándar del sistema.
func New() *Almacen {
	return &Almacen{
		certutil: "certutil",
		pk12util: "pk12util",
		openssl:  "openssl",
		rutas:    rutasEstandar(),
	}
}

// NewConRutas crea un Almacen NSS con rutas y binario certutil inyectados (útil para tests).
func NewConRutas(certutilPath string, rutas []string) *Almacen {
	return &Almacen{
		certutil: certutilPath,
		pk12util: "pk12util",
		openssl:  "openssl",
		rutas:    rutas,
	}
}

// NewConHerramientas crea un Almacen NSS con certutil y pk12util inyectados.
func NewConHerramientas(certutilPath, pk12utilPath string, rutas []string) *Almacen {
	return &Almacen{
		certutil: certutilPath,
		pk12util: pk12utilPath,
		openssl:  "openssl",
		rutas:    rutas,
	}
}

// List implementa ports.CertificateCatalog devolviendo los certificados con clave privada
// encontrados en todos los almacenes NSS del sistema.
func (a *Almacen) List(ctx context.Context) ([]domain.CertificateRef, error) {
	seen := make(map[string]struct{})
	var resultado []domain.CertificateRef

	for _, ruta := range a.rutas {
		if ctx.Err() != nil {
			return resultado, ctx.Err()
		}
		if !dirExiste(ruta) {
			continue
		}
		refs, err := a.listarEnRuta(ctx, ruta)
		if err != nil {
			// Una ruta inaccesible no es un error fatal; continuamos con las demás.
			continue
		}
		for _, ref := range refs {
			if _, dup := seen[ref.Fingerprint]; dup {
				continue
			}
			seen[ref.Fingerprint] = struct{}{}
			resultado = append(resultado, ref)
		}
	}

	return resultado, nil
}

// listarEnRutaCertutil enumera los certificados con clave privada en un
// directorio NSS concreto mediante el subproceso certutil. Es la ruta por
// defecto; con el build tag nss_cgo el listado usa libnss3 directamente y
// esta función queda como fallback (ver list_cgo.go / list_nocgo.go).
func (a *Almacen) listarEnRutaCertutil(ctx context.Context, ruta string) ([]domain.CertificateRef, error) {
	nicknames, err := a.listarNicknames(ctx, ruta)
	if err != nil {
		return nil, err
	}

	var refs []domain.CertificateRef
	for _, nick := range nicknames {
		if ctx.Err() != nil {
			break
		}
		ref, err := a.exportarCertificado(ctx, ruta, nick)
		if err != nil {
			continue // ignorar certs que no se pueden exportar
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// KeyFor implementa ports.SigningKeyProvider resolviendo la clave privada desde
// NSS mediante exportación temporal PKCS#12 del certificado solicitado.
func (a *Almacen) KeyFor(ctx context.Context, certificate domain.CertificateRef) (ports.SigningKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for _, ruta := range a.rutas {
		if !dirExiste(ruta) {
			continue
		}
		nicknames, err := a.listarNicknames(ctx, ruta)
		if err != nil {
			continue
		}
		for _, nick := range nicknames {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			ref, err := a.exportarCertificado(ctx, ruta, nick)
			if err != nil {
				continue
			}
			if ref.Fingerprint != certificate.Fingerprint {
				continue
			}
			return a.exportarClaveLocal(ctx, ruta, nick, certificate)
		}
	}

	return nil, fmt.Errorf("nssstore: certificado no encontrado en NSS: %s", certificate.Fingerprint)
}

func (a *Almacen) exportarClaveLocal(ctx context.Context, ruta, nickname string, esperado domain.CertificateRef) (ports.SigningKey, error) {
	pk12utilPath, err := resolveNSSExecutable(a.pk12util)
	if err != nil {
		return nil, fmt.Errorf("resolver pk12util: %w", err)
	}
	tmpFile, err := os.CreateTemp("", "grxfirma-nss-*.p12")
	if err != nil {
		return nil, fmt.Errorf("crear temporal NSS: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	tmpNSSDir, err := clonarBaseNSSTemporal(ruta)
	if err != nil {
		return nil, fmt.Errorf("clonando base NSS temporal: %w", err)
	}
	defer os.RemoveAll(tmpNSSDir)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Exportación no interactiva. Clonamos la base NSS a un directorio temporal
	// porque pk12util necesita acceso de escritura incluso para exportar y los
	// perfiles activos del navegador suelen abrirse en modo solo lectura.
	slotPassword := os.Getenv("GRXFIRMA_NSS_SLOT_PASSWORD")
	exportPassword, err := resolverPasswordExportacionNSS(os.Getenv("GRXFIRMA_NSS_EXPORT_PASSWORD"))
	if err != nil {
		return nil, err
	}
	// CWE-214: nunca pasar contraseñas por argv (visibles en la tabla de
	// procesos). pk12util admite las variantes de fichero -k (slot) y -w (P12)
	// en lugar de -K/-W.
	slotPwPath, err := escribirPasswordTemporal(slotPassword)
	if err != nil {
		return nil, err
	}
	defer os.Remove(slotPwPath)
	exportPwPath, err := escribirPasswordTemporal(exportPassword)
	if err != nil {
		return nil, err
	}
	defer os.Remove(exportPwPath)
	// #nosec G204 -- tool paths are defaults or explicit constructor
	// dependencies; NSS paths/nickname are separate pk12util arguments.
	cmd := exec.CommandContext(
		ctx,
		pk12utilPath,
		"-o", tmpPath,
		"-n", nickname,
		"-d", rutaCertutil(tmpNSSDir),
		"-k", slotPwPath,
		"-w", exportPwPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pk12util -o para %q: %w (%s)", nickname, err, strings.TrimSpace(string(out)))
	}

	pemBundle, pemErr := a.convertirP12APEM(ctx, tmpPath, exportPassword)
	if pemErr == nil {
		// pemBundle contiene la clave privada en claro (T057): zeroizar al salir.
		defer secmem.Zeroize(pemBundle)
		clave, err := claveLocalDesdePEM(pemBundle, esperado)
		if err == nil {
			return clave, nil
		}
		pemErr = err
	}

	identidad, err := pkcs12importer.New().ImportP12File(ctx, tmpPath, exportPassword)
	if err == nil {
		if strings.EqualFold(identidad.Reference.Fingerprint, esperado.Fingerprint) {
			return desktopsigner.NuevaClaveLocalConCadena(identidad.Signer, identidad.Certificate, identidad.Chain), nil
		}
		err = fmt.Errorf("el P12 exportado no coincide con el certificado solicitado: esperado=%s obtenido=%s", esperado.Fingerprint, identidad.Reference.Fingerprint)
	}

	if pemErr != nil {
		return nil, fmt.Errorf("importando P12 exportado desde NSS: %w; fallback openssl: %v", err, pemErr)
	}
	return nil, fmt.Errorf("importando P12 exportado desde NSS: %w", err)
}

func resolverPasswordExportacionNSS(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured, nil
	}
	var raw [24]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generar contraseña temporal de exportación NSS: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (a *Almacen) convertirP12APEM(ctx context.Context, p12Path, password string) ([]byte, error) {
	if strings.TrimSpace(a.openssl) == "" {
		return nil, fmt.Errorf("openssl no configurado")
	}
	opensslPath, err := resolveNSSExecutable(a.openssl)
	if err != nil {
		return nil, fmt.Errorf("resolver openssl: %w", err)
	}
	pwPath, err := escribirPasswordTemporal(password)
	if err != nil {
		return nil, err
	}
	defer os.Remove(pwPath)

	// #nosec G204 -- openssl is a fixed/default or explicitly injected tool;
	// the P12 and private password-file paths are distinct arguments.
	cmd := exec.CommandContext(ctx, opensslPath, "pkcs12", "-in", p12Path, "-nodes", "-passin", "file:"+pwPath)
	out, err := cmd.CombinedOutput()
	defer secmem.Zeroize(out)
	if err != nil {
		return nil, fmt.Errorf("openssl pkcs12: %w", err)
	}
	pem := extraerBloquesPEM(out)
	if len(pem) == 0 {
		return nil, fmt.Errorf("openssl no devolvio bloques PEM utilizables")
	}
	return pem, nil
}

func escribirPasswordTemporal(password string) (string, error) {
	pwFile, err := os.CreateTemp("", "grxfirma-openssl-pw-*")
	if err != nil {
		return "", fmt.Errorf("crear fichero temporal de contraseña openssl: %w", err)
	}
	pwPath := pwFile.Name()
	ok := false
	defer func() {
		_ = pwFile.Close()
		if !ok {
			_ = os.Remove(pwPath)
		}
	}()

	if err := os.Chmod(pwPath, 0o600); err != nil {
		return "", fmt.Errorf("proteger fichero temporal de contraseña openssl: %w", err)
	}
	if _, err := pwFile.WriteString(password); err != nil {
		return "", fmt.Errorf("escribir contraseña temporal openssl: %w", err)
	}
	if err := pwFile.Close(); err != nil {
		return "", fmt.Errorf("cerrar contraseña temporal openssl: %w", err)
	}
	ok = true
	return pwPath, nil
}

func extraerBloquesPEM(data []byte) []byte {
	var b strings.Builder
	inside := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-----BEGIN ") {
			inside = true
		}
		if inside {
			b.WriteString(trimmed)
			b.WriteByte('\n')
		}
		if strings.HasPrefix(trimmed, "-----END ") {
			inside = false
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

func claveLocalDesdePEM(data []byte, esperado domain.CertificateRef) (ports.SigningKey, error) {
	firmante, err := parsearClavePrivadaPEM(data)
	if err != nil {
		return nil, err
	}
	certs, err := parsearCertificadosPEM(data)
	if err != nil {
		return nil, err
	}
	hoja, cadena, err := seleccionarCertificadoFirmante(certs, firmante, esperado)
	if err != nil {
		return nil, err
	}
	return desktopsigner.NuevaClaveLocalConCadena(firmante, hoja, cadena), nil
}

func parsearClavePrivadaPEM(data []byte) (crypto.Signer, error) {
	rest := data
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		switch block.Type {
		case "RSA PRIVATE KEY":
			priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("clave RSA PKCS#1 invalida: %w", err)
			}
			return priv, nil
		case "EC PRIVATE KEY":
			priv, err := x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("clave EC invalida: %w", err)
			}
			return priv, nil
		case "PRIVATE KEY":
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("clave PKCS#8 invalida: %w", err)
			}
			switch k := key.(type) {
			case *rsa.PrivateKey:
				return k, nil
			case *ecdsa.PrivateKey:
				return k, nil
			default:
				return nil, fmt.Errorf("tipo de clave PKCS#8 no soportado: %T", key)
			}
		}
	}
	return nil, fmt.Errorf("no se encontro una clave privada en el PEM exportado")
}

func parsearCertificadosPEM(data []byte) ([]*x509.Certificate, error) {
	rest := data
	certs := make([]*x509.Certificate, 0, 4)
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certificado PEM invalido: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no se encontro ningun certificado en el PEM exportado")
	}
	return certs, nil
}

func seleccionarCertificadoFirmante(certs []*x509.Certificate, firmante crypto.Signer, esperado domain.CertificateRef) (*x509.Certificate, []*x509.Certificate, error) {
	if len(certs) == 0 {
		return nil, nil, fmt.Errorf("no hay certificados para asociar a la clave exportada")
	}
	pubFirmante, err := x509.MarshalPKIXPublicKey(firmante.Public())
	if err != nil {
		return nil, nil, fmt.Errorf("clave exportada sin clave publica utilizable: %w", err)
	}

	fpEsperada := strings.ToLower(strings.TrimSpace(esperado.Fingerprint))
	var hoja *x509.Certificate
	for _, cert := range certs {
		if fpEsperada == "" || fingerprintSHA256(cert) != fpEsperada {
			continue
		}
		pubCert, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
		if err != nil {
			continue
		}
		if bytes.Equal(pubFirmante, pubCert) {
			hoja = cert
			break
		}
	}
	if hoja == nil {
		for _, cert := range certs {
			pubCert, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
			if err != nil {
				continue
			}
			if bytes.Equal(pubFirmante, pubCert) {
				hoja = cert
				break
			}
		}
	}
	if hoja == nil {
		return nil, nil, fmt.Errorf("ningun certificado exportado coincide con la clave privada")
	}
	if fpEsperada != "" && fingerprintSHA256(hoja) != fpEsperada {
		return nil, nil, fmt.Errorf("la exportacion NSS devolvio otro certificado: esperado=%s obtenido=%s", fpEsperada, fingerprintSHA256(hoja))
	}

	cadena := make([]*x509.Certificate, 0, len(certs)-1)
	for _, cert := range certs {
		if bytes.Equal(cert.Raw, hoja.Raw) {
			continue
		}
		cadena = append(cadena, cert)
	}
	return hoja, cadena, nil
}

func fingerprintSHA256(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

func clonarBaseNSSTemporal(origen string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "grxfirma-nssdb-*")
	if err != nil {
		return "", err
	}

	// Un perfil Firefox activo contiene lock (a menudo un enlace simbólico
	// colgante). La exportación solo necesita la base SQL NSS, no el perfil.
	// La lista fija evita tanto ese cerrojo como copiar datos ajenos al almacén.
	for _, name := range []string{
		"cert9.db", "cert9.db-wal", "cert9.db-shm",
		"key4.db", "key4.db-wal", "key4.db-shm",
		"pkcs11.txt",
	} {
		src := filepath.Join(origen, name)
		dst := filepath.Join(tmpDir, name)
		if err := copiarArchivo(src, dst); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", err
		}
	}
	return tmpDir, nil
}

func copiarArchivo(origen, destino string) error {
	src, err := securefile.OpenRead(origen)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	// El destino pertenece al clon privado de MkdirTemp y su nombre procede
	// de la lista fija de archivos NSS, por lo que no puede salir de él.
	dst, err := os.OpenFile(destino, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()) // #nosec G304 -- destino usa un nombre NSS fijo dentro del clon privado.
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func rutaCertutil(ruta string) string {
	if strings.HasPrefix(ruta, "sql:") || strings.HasPrefix(ruta, "dbm:") {
		return ruta
	}
	if dirExiste(filepath.Join(ruta, "cert9.db")) || dirExiste(filepath.Join(ruta, "key4.db")) {
		return "sql:" + ruta
	}
	return ruta
}

func resolveNSSExecutable(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", errors.New("herramienta NSS no configurada")
	}
	resolved, err := exec.LookPath(configured)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("la herramienta NSS no es un ejecutable regular: %s", absolute)
	}
	return absolute, nil
}

// listarNicknames ejecuta certutil -L y devuelve los nicknames con clave privada (u,u,u).
func (a *Almacen) listarNicknames(ctx context.Context, ruta string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	certutilPath, err := resolveNSSExecutable(a.certutil)
	if err != nil {
		return nil, err
	}

	// #nosec G204 -- certutil is fixed or explicitly injected for tests and
	// the NSS database is passed as a distinct argument.
	cmd := exec.CommandContext(ctx, certutilPath, "-L", "-d", rutaCertutil(ruta))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("certutil -L en %s: %w", ruta, err)
	}

	return parsearListaNSS(out), nil
}

// exportarCertificado exporta un certificado PEM y construye su CertificateRef.
func (a *Almacen) exportarCertificado(ctx context.Context, ruta, nickname string) (domain.CertificateRef, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	certutilPath, err := resolveNSSExecutable(a.certutil)
	if err != nil {
		return domain.CertificateRef{}, err
	}

	// #nosec G204 -- same controlled certutil dependency; nickname and NSS
	// database are argv values and are never interpreted by a shell.
	cmd := exec.CommandContext(ctx, certutilPath, "-L", "-d", rutaCertutil(ruta), "-n", nickname, "-a")
	out, err := cmd.Output()
	if err != nil {
		return domain.CertificateRef{}, fmt.Errorf("certutil -L -n %q: %w", nickname, err)
	}

	cert, err := parsearPEM(out)
	if err != nil {
		return domain.CertificateRef{}, fmt.Errorf("parsear PEM de %q: %w", nickname, err)
	}

	return construirRef(cert), nil
}

// parsearListaNSS extrae los nicknames con trust "u,u,u" de la salida de certutil -L.
func parsearListaNSS(output []byte) []string {
	var nicknames []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		linea := scanner.Text()
		if !strings.Contains(linea, trustConClave) {
			continue
		}
		// El nickname puede contener espacios; el trust attribute está al final separado por espacios.
		// Formato: "Nickname                                         u,u,u"
		idx := strings.LastIndex(linea, "  ")
		if idx <= 0 {
			continue
		}
		nick := strings.TrimSpace(linea[:idx])
		if nick != "" {
			nicknames = append(nicknames, nick)
		}
	}
	return nicknames
}

// parsearPEM extrae el primer certificado de un bloque PEM.
func parsearPEM(data []byte) (*x509.Certificate, error) {
	rest := data
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		return x509.ParseCertificate(block.Bytes)
	}
	return nil, fmt.Errorf("no se encontró bloque CERTIFICATE en el PEM")
}

// construirRef crea un domain.CertificateRef a partir de un x509.Certificate.
func construirRef(cert *x509.Certificate) domain.CertificateRef {
	huella := sha256.Sum256(cert.Raw)
	subject := cert.Subject.CommonName
	if subject == "" {
		subject = cert.Subject.String()
	}
	issuer := cert.Issuer.CommonName
	if issuer == "" {
		issuer = cert.Issuer.String()
	}
	tipo, org, nif := certutil.ClasificarCertificado(cert)
	fingerprint := hex.EncodeToString(huella[:])
	return domain.CertificateRef{
		ID:            fingerprint,
		Subject:       subject,
		Issuer:        issuer,
		NotAfter:      cert.NotAfter,
		Fingerprint:   fingerprint,
		DER:           cert.Raw,
		HasSigningKey: true,
		Tipo:          tipo,
		Organizacion:  org,
		NIF:           nif,
	}
}

// rutasEstandar devuelve las rutas NSS estándar del sistema del usuario actual.
func rutasEstandar() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var rutas []string

	// Chrome / Chromium
	rutas = append(rutas, filepath.Join(home, ".pki", "nssdb"))

	// Firefox: todos los perfiles locales conocidos.
	for _, mozDir := range []string{
		filepath.Join(home, ".mozilla", "firefox"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
		filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"),
	} {
		entries, err := os.ReadDir(mozDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				rutas = append(rutas, filepath.Join(mozDir, e.Name()))
			}
		}
	}

	// Chromium corporativo
	rutas = append(rutas, "/etc/chromium/nssdb")

	return rutas
}

// dirExiste retorna true si la ruta existe y es un directorio.
func dirExiste(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

var _ ports.CertificateCatalog = (*Almacen)(nil)

var _ ports.SigningKeyProvider = (*Almacen)(nil)
