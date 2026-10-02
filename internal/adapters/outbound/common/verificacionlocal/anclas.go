// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package verificacionlocal evalúa firmas sin depender de otras
// aplicaciones: anclas de confianza fijadas en un fichero o directorio, CRL
// locales y sellos de tiempo verificados contra esas mismas anclas. Las
// consultas remotas (OCSP/CRL por red, validación externa de sellos) son
// puntos de extensión opcionales y quedan desactivados por defecto.
package verificacionlocal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"grxfirma/internal/domain"
)

const (
	maxBytesFicheroAncla  = 4 << 20
	maxAnclas             = 2048
	maxFicherosDirectorio = 4096
)

// ErrSinAnclas indica que la ruta configurada no contiene ningún certificado.
var ErrSinAnclas = errors.New("verificacionlocal: la ruta de anclas no contiene certificados X.509")

// ProveedorAnclas implementa ports.TrustAnchorProvider con un conjunto fijo
// de anclas locales. Nunca recurre al almacén raíz del sistema.
type ProveedorAnclas struct {
	der [][]byte
}

// CargarAnclas lee un fichero (PEM con uno o varios certificados, o DER) o
// un directorio no recursivo con ficheros .pem, .crt, .cer o .der.
func CargarAnclas(ruta string) (*ProveedorAnclas, error) {
	ruta = strings.TrimSpace(ruta)
	if ruta == "" {
		return nil, errors.New("verificacionlocal: ruta de anclas vacía")
	}
	info, err := os.Stat(ruta)
	if err != nil {
		return nil, fmt.Errorf("verificacionlocal: no se puede leer la ruta de anclas: %w", err)
	}
	ficheros := []string{ruta}
	if info.IsDir() {
		ficheros, err = listarFicheros(ruta, []string{".pem", ".crt", ".cer", ".der"})
		if err != nil {
			return nil, err
		}
	}
	vistos := map[[sha256.Size]byte]struct{}{}
	var der [][]byte
	for _, fichero := range ficheros {
		contenido, err := leerAcotado(fichero, maxBytesFicheroAncla)
		if err != nil {
			return nil, err
		}
		certs, err := parsearCertificados(contenido)
		if err != nil {
			return nil, fmt.Errorf("verificacionlocal: ancla no válida en %s: %w", filepath.Base(fichero), err)
		}
		for _, cert := range certs {
			huella := sha256.Sum256(cert.Raw)
			if _, repetida := vistos[huella]; repetida {
				continue
			}
			vistos[huella] = struct{}{}
			der = append(der, bytes.Clone(cert.Raw))
			if len(der) > maxAnclas {
				return nil, fmt.Errorf("verificacionlocal: demasiadas anclas (máximo %d)", maxAnclas)
			}
		}
	}
	if len(der) == 0 {
		return nil, ErrSinAnclas
	}
	return &ProveedorAnclas{der: der}, nil
}

// Anchors devuelve una copia de las anclas sin habilitar las del sistema.
func (p *ProveedorAnclas) Anchors(ctx context.Context) (domain.CertificateChain, error) {
	if err := ctx.Err(); err != nil {
		return domain.CertificateChain{}, err
	}
	if p == nil {
		return domain.CertificateChain{}, nil
	}
	copia := make([][]byte, len(p.der))
	for i := range p.der {
		copia[i] = bytes.Clone(p.der[i])
	}
	return domain.CertificateChain{DERCertificates: copia}, nil
}

// Cantidad informa del número de anclas distintas cargadas.
func (p *ProveedorAnclas) Cantidad() int {
	if p == nil {
		return 0
	}
	return len(p.der)
}

func parsearCertificados(contenido []byte) ([]*x509.Certificate, error) {
	if !bytes.Contains(contenido, []byte("-----BEGIN")) {
		cert, err := x509.ParseCertificate(contenido)
		if err != nil {
			return nil, err
		}
		return []*x509.Certificate{cert}, nil
	}
	var out []*x509.Certificate
	resto := contenido
	for {
		var bloque *pem.Block
		bloque, resto = pem.Decode(resto)
		if bloque == nil {
			break
		}
		if bloque.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(bloque.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, cert)
	}
	return out, nil
}

// listarFicheros devuelve, en orden estable, los ficheros regulares del
// directorio con alguna de las extensiones admitidas. No sigue subdirectorios.
func listarFicheros(dir string, extensiones []string) ([]string, error) {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("verificacionlocal: no se puede listar %s: %w", filepath.Base(dir), err)
	}
	var out []string
	for _, entrada := range entradas {
		if !entrada.Type().IsRegular() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entrada.Name()))
		for _, admitida := range extensiones {
			if ext == admitida {
				out = append(out, filepath.Join(dir, entrada.Name()))
				break
			}
		}
		if len(out) > maxFicherosDirectorio {
			return nil, fmt.Errorf("verificacionlocal: demasiados ficheros en %s (máximo %d)", filepath.Base(dir), maxFicherosDirectorio)
		}
	}
	sort.Strings(out)
	return out, nil
}

func leerAcotado(ruta string, limite int64) ([]byte, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, fmt.Errorf("verificacionlocal: no se puede abrir %s: %w", filepath.Base(ruta), err)
	}
	defer f.Close()
	contenido, err := io.ReadAll(io.LimitReader(f, limite+1))
	if err != nil {
		return nil, fmt.Errorf("verificacionlocal: no se puede leer %s: %w", filepath.Base(ruta), err)
	}
	if int64(len(contenido)) > limite {
		return nil, fmt.Errorf("verificacionlocal: %s supera el tamaño máximo de %d bytes", filepath.Base(ruta), limite)
	}
	return contenido, nil
}
