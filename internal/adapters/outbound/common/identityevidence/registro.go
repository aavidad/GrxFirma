// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package identityevidence persiste evidencia cifrada y encadenada contra manipulaciones.
package identityevidence

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/ports"
)

const dominio = "grxfirma/evidencia-identidad/v1"

var (
	// ErrConfiguracionInvalida evita arrancar con almacenamiento débil.
	ErrConfiguracionInvalida = errors.New("configuración de evidencia de identidad inválida")
	// ErrRegistroCorrupto impide añadir evidencia sobre una cadena no verificable.
	ErrRegistroCorrupto = errors.New("registro de evidencia de identidad corrupto")
)

// Configuracion aporta una clave externa versionada y una ruta privada.
type Configuracion struct {
	Ruta         string
	Clave        []byte
	VersionClave string
	Aleatorio    io.Reader
}

// Registro escribe tramas AES-256-GCM enlazadas mediante HMAC-SHA-256.
type Registro struct {
	mu           sync.Mutex
	ruta         string
	clave        []byte
	versionClave string
	aleatorio    io.Reader
	ultimoMAC    []byte
	ficheroInfo  os.FileInfo
	dirInfo      os.FileInfo
}

type trama struct {
	VersionClave string `json:"versionClave"`
	Referencia   string `json:"referencia"`
	Nonce        string `json:"nonce"`
	Cifrado      string `json:"cifrado"`
	MACAnterior  string `json:"macAnterior"`
	MAC          string `json:"mac"`
}

// Nuevo valida permisos y toda la cadena existente antes de aceptar escrituras.
func Nuevo(configuracion Configuracion) (*Registro, error) {
	if len(configuracion.Clave) != 32 || !textoSeguro(configuracion.VersionClave) || configuracion.Ruta == "" || !filepath.IsAbs(configuracion.Ruta) {
		return nil, ErrConfiguracionInvalida
	}
	if configuracion.Aleatorio == nil {
		configuracion.Aleatorio = rand.Reader
	}
	if err := prepararRuta(configuracion.Ruta); err != nil {
		return nil, err
	}
	r := &Registro{ruta: configuracion.Ruta, clave: append([]byte(nil), configuracion.Clave...), versionClave: configuracion.VersionClave, aleatorio: configuracion.Aleatorio}
	var err error
	r.ficheroInfo, err = infoRutaSinEnlaces(r.ruta, false)
	if err != nil {
		return nil, err
	}
	r.dirInfo, err = infoRutaSinEnlaces(filepath.Dir(r.ruta), true)
	if err != nil {
		return nil, err
	}
	ultimo, err := verificarRegistro(configuracion.Ruta, r.clave, r.versionClave, r.ficheroInfo)
	if err != nil {
		return nil, err
	}
	r.ultimoMAC = ultimo
	return r, nil
}

// RegistrarIdentidad cifra, encadena y sincroniza una evidencia antes de responder.
func (r *Registro) RegistrarIdentidad(ctx context.Context, evidencia ports.EvidenciaIdentidad) (string, error) {
	if ctx == nil || ctx.Err() != nil || evidencia.RetoID == "" || len(evidencia.ContenidoCanonico) == 0 {
		return "", ErrConfiguracionInvalida
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	identificador := make([]byte, 16)
	if _, err := io.ReadFull(r.aleatorio, identificador); err != nil {
		return "", err
	}
	referencia := dominio + "/" + hex.EncodeToString(identificador)
	plano, err := json.Marshal(evidencia)
	if err != nil {
		return "", err
	}
	bloque, err := aes.NewCipher(r.clave)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(bloque)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(r.aleatorio, nonce); err != nil {
		return "", err
	}
	cifrado := aead.Seal(nil, nonce, plano, []byte(dominio+"\x00"+r.versionClave+"\x00"+referencia))
	t := trama{VersionClave: r.versionClave, Referencia: referencia, Nonce: base64.RawURLEncoding.EncodeToString(nonce),
		Cifrado: base64.RawURLEncoding.EncodeToString(cifrado), MACAnterior: hex.EncodeToString(r.ultimoMAC)}
	t.MAC = hex.EncodeToString(calcularMAC(r.clave, r.ultimoMAC, t))
	linea, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	if err := r.comprobarRutaOriginal(); err != nil {
		return "", err
	}
	fichero, err := abrirFicheroRegistro(r.ruta, false)
	if err != nil {
		return "", err
	}
	info, err := fichero.Stat()
	if err != nil || !os.SameFile(r.ficheroInfo, info) {
		_ = fichero.Close()
		return "", ErrConfiguracionInvalida
	}
	if err := r.comprobarRutaOriginal(); err != nil {
		_ = fichero.Close()
		return "", err
	}
	if err := ctx.Err(); err != nil {
		_ = fichero.Close()
		return "", err
	}
	if _, err = fichero.Write(append(linea, '\n')); err == nil {
		err = fichero.Sync()
	}
	errCierre := fichero.Close()
	if err != nil {
		return "", err
	}
	if errCierre != nil {
		return "", errCierre
	}
	r.ultimoMAC, _ = hex.DecodeString(t.MAC)
	return referencia, nil
}

func prepararRuta(ruta string) error {
	directorio := filepath.Dir(ruta)
	if err := prepararDirectorioPrivado(directorio); err != nil {
		return err
	}
	if info, err := os.Lstat(directorio); err != nil || !info.IsDir() {
		return ErrConfiguracionInvalida
	}
	if err := comprobarPermisosPrivados(directorio, true); err != nil {
		return err
	}
	crear := false
	if info, err := os.Lstat(ruta); err == nil {
		if !info.Mode().IsRegular() {
			return ErrConfiguracionInvalida
		}
		if err := comprobarPermisosPrivados(ruta, false); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		crear = true
	}
	f, err := abrirFicheroRegistro(ruta, crear)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	info, err := os.Lstat(ruta)
	if err != nil || !info.Mode().IsRegular() {
		return ErrConfiguracionInvalida
	}
	return comprobarPermisosPrivados(ruta, false)
}

func (r *Registro) comprobarRutaOriginal() error {
	for ruta, original := range map[string]os.FileInfo{r.ruta: r.ficheroInfo, filepath.Dir(r.ruta): r.dirInfo} {
		actual, err := infoRutaSinEnlaces(ruta, original.IsDir())
		if err != nil || actual.Mode()&os.ModeSymlink != 0 || !os.SameFile(original, actual) {
			return ErrConfiguracionInvalida
		}
		if err := comprobarPermisosPrivados(ruta, actual.IsDir()); err != nil {
			return err
		}
	}
	return nil
}

func infoRutaSinEnlaces(ruta string, directorio bool) (os.FileInfo, error) {
	open := securefile.OpenRead
	if directorio {
		open = securefile.OpenDir
	}
	f, err := open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// File.Stat obtiene la identidad desde el descriptor. En Windows, Lstat
	// puede cargar el ID de fichero perezosamente al llamar a os.SameFile.
	return f.Stat()
}

func verificarRegistro(ruta string, clave []byte, version string, original os.FileInfo) ([]byte, error) {
	f, err := securefile.OpenRead(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(original, info) {
		return nil, ErrConfiguracionInvalida
	}
	escaner := bufio.NewScanner(f)
	escaner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var anterior []byte
	for escaner.Scan() {
		var t trama
		if json.Unmarshal(escaner.Bytes(), &t) != nil || t.VersionClave != version || t.MACAnterior != hex.EncodeToString(anterior) {
			return nil, ErrRegistroCorrupto
		}
		mac, err := hex.DecodeString(t.MAC)
		if err != nil || !hmac.Equal(mac, calcularMAC(clave, anterior, t)) {
			return nil, ErrRegistroCorrupto
		}
		anterior = mac
	}
	if err := escaner.Err(); err != nil {
		return nil, ErrRegistroCorrupto
	}
	return anterior, nil
}

func calcularMAC(clave, anterior []byte, t trama) []byte {
	h := hmac.New(sha256.New, clave)
	h.Write([]byte(dominio))
	h.Write(anterior)
	h.Write([]byte(t.VersionClave + "\x00" + t.Referencia + "\x00" + t.Nonce + "\x00" + t.Cifrado))
	return h.Sum(nil)
}

func textoSeguro(valor string) bool {
	return valor != "" && valor == strings.TrimSpace(valor) && len(valor) <= 128 && !strings.ContainsAny(valor, "\r\n\t")
}

var _ ports.RegistroEvidenciaIdentidad = (*Registro)(nil)
