// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/revocationclient"
)

const maxBytesCRL = 64 << 20

// CRLLocal es una CRL leída del directorio, con su emisor ya extraído.
type CRLLocal struct {
	DER   []byte
	lista *x509.RevocationList
}

type entradaCRL struct {
	modificado time.Time
	tamano     int64
	crls       []CRLLocal
}

// AlmacenCRL lee CRL de un directorio local fijado por configuración. Se
// relee en cada consulta para admitir actualizaciones sin reiniciar, pero
// cada fichero solo se vuelve a analizar si cambian su fecha o su tamaño.
type AlmacenCRL struct {
	dir   string
	mu    sync.Mutex
	cache map[string]entradaCRL
}

// NuevoAlmacenCRL comprueba que el directorio existe y es legible.
func NuevoAlmacenCRL(dir string) (*AlmacenCRL, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("verificacionlocal: directorio de CRL vacío")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("verificacionlocal: no se puede leer el directorio de CRL: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("verificacionlocal: la ruta de CRL debe ser un directorio")
	}
	return &AlmacenCRL{dir: dir, cache: map[string]entradaCRL{}}, nil
}

// CRLsDe devuelve las CRL del directorio cuyo emisor coincide con el sujeto
// del emisor indicado. La autenticación y la vigencia se comprueban después.
func (a *AlmacenCRL) CRLsDe(ctx context.Context, emisor *x509.Certificate) ([]CRLLocal, error) {
	if a == nil || emisor == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ficheros, err := listarFicheros(a.dir, []string{".crl", ".pem", ".der"})
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	vigentes := make(map[string]struct{}, len(ficheros))
	var out []CRLLocal
	for _, fichero := range ficheros {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vigentes[fichero] = struct{}{}
		crls := a.cargar(fichero)
		for _, crl := range crls {
			if bytes.Equal(crl.lista.RawIssuer, emisor.RawSubject) {
				out = append(out, crl)
			}
		}
	}
	for ruta := range a.cache {
		if _, sigue := vigentes[ruta]; !sigue {
			delete(a.cache, ruta)
		}
	}
	return out, nil
}

// cargar devuelve las CRL de un fichero; un fichero ilegible o mal formado
// se ignora (no aporta evidencia) sin impedir usar los demás.
func (a *AlmacenCRL) cargar(ruta string) []CRLLocal {
	info, err := os.Stat(ruta)
	if err != nil {
		return nil
	}
	if previa, ok := a.cache[ruta]; ok && previa.modificado.Equal(info.ModTime()) && previa.tamano == info.Size() {
		return previa.crls
	}
	contenido, err := leerAcotado(ruta, maxBytesCRL)
	if err != nil {
		a.cache[ruta] = entradaCRL{modificado: info.ModTime(), tamano: info.Size()}
		return nil
	}
	var bloques [][]byte
	if bytes.Contains(contenido, []byte("-----BEGIN")) {
		resto := contenido
		for {
			var bloque *pem.Block
			bloque, resto = pem.Decode(resto)
			if bloque == nil {
				break
			}
			if bloque.Type == "X509 CRL" {
				bloques = append(bloques, bloque.Bytes)
			}
		}
	} else {
		bloques = [][]byte{contenido}
	}
	var crls []CRLLocal
	for _, der := range bloques {
		lista, err := x509.ParseRevocationList(der)
		if err != nil {
			continue
		}
		crls = append(crls, CRLLocal{DER: bytes.Clone(der), lista: lista})
	}
	a.cache[ruta] = entradaCRL{modificado: info.ModTime(), tamano: info.Size(), crls: crls}
	return crls
}

// crlAplicable decide si una CRL cubre de forma completa al certificado. La
// regla vive en revocationclient para que la verificación local, la consulta
// en línea y la evidencia embebida apliquen exactamente el mismo criterio.
func crlAplicable(lista *x509.RevocationList, cert *x509.Certificate) (bool, string) {
	return revocationclient.CRLAplicable(lista, cert)
}
