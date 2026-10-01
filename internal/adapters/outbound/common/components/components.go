// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package components comprueba la disponibilidad de las herramientas externas
// opcionales de las que depende GrxFirma en tiempo de ejecución (openssl,
// poppler, libnss3-tools, etc.) y produce diagnósticos legibles con pistas de
// instalación por plataforma.
//
// El núcleo de firma es Go puro y no necesita ninguna de estas herramientas; se
// usan solo en rutas concretas (importación P12 no estándar, previsualización
// PDF, almacén NSS del sistema…). Cuando una de esas rutas se ejercita y la
// herramienta falta, el usuario debe recibir un mensaje claro en vez de un
// error críptico del sistema.
package components

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// Componente describe una herramienta externa opcional y para qué se usa.
type Componente struct {
	// Nombre del ejecutable buscado en el PATH.
	Nombre string
	// ParaQue explica qué funcionalidad depende de este componente.
	ParaQue string
	// Plataformas donde el componente es relevante (valores de runtime.GOOS).
	// Vacío significa "todas".
	Plataformas []string
	// PistasInstalacion mapea runtime.GOOS -> instrucción de instalación.
	PistasInstalacion map[string]string
}

// Estado es el resultado de comprobar un componente en la máquina actual.
type Estado struct {
	Componente
	// Relevante indica si el componente aplica a la plataforma actual.
	Relevante bool
	// Disponible indica si se encontró el ejecutable en el PATH.
	Disponible bool
	// Ruta es la ruta resuelta cuando está disponible.
	Ruta string
	// PistaInstalacion es la instrucción de instalación para la plataforma actual.
	PistaInstalacion string
}

// catalogo enumera los componentes externos opcionales conocidos.
var catalogo = []Componente{
	{
		Nombre:      "openssl",
		ParaQue:     "importar ficheros PKCS#12 (.p12/.pfx) con algoritmos heredados que la ruta Go nativa no acepta",
		Plataformas: nil,
		PistasInstalacion: map[string]string{
			"linux":   "instala el paquete 'openssl' (Debian/Ubuntu: sudo apt install openssl)",
			"darwin":  "instala openssl con Homebrew: brew install openssl",
			"windows": "instala OpenSSL para Windows y añádelo al PATH (https://slproweb.com/products/Win32OpenSSL.html)",
		},
	},
	{
		Nombre:      "pdftoppm",
		ParaQue:     "previsualizar páginas de PDF en la interfaz de escritorio",
		Plataformas: []string{"linux", "darwin"},
		PistasInstalacion: map[string]string{
			"linux":  "instala 'poppler-utils' (Debian/Ubuntu: sudo apt install poppler-utils)",
			"darwin": "instala poppler con Homebrew: brew install poppler",
		},
	},
	{
		Nombre:      "pdfinfo",
		ParaQue:     "leer metadatos de PDF para la previsualización de escritorio",
		Plataformas: []string{"linux", "darwin"},
		PistasInstalacion: map[string]string{
			"linux":  "instala 'poppler-utils' (Debian/Ubuntu: sudo apt install poppler-utils)",
			"darwin": "instala poppler con Homebrew: brew install poppler",
		},
	},
	{
		Nombre:      "certutil",
		ParaQue:     "leer e instalar certificados en el almacén NSS del sistema (Linux)",
		Plataformas: []string{"linux"},
		PistasInstalacion: map[string]string{
			"linux": "instala 'libnss3-tools' (Debian/Ubuntu: sudo apt install libnss3-tools)",
		},
	},
	{
		Nombre:      "pk12util",
		ParaQue:     "importar ficheros PKCS#12 al almacén NSS del sistema (Linux)",
		Plataformas: []string{"linux"},
		PistasInstalacion: map[string]string{
			"linux": "instala 'libnss3-tools' (Debian/Ubuntu: sudo apt install libnss3-tools)",
		},
	},
}

// Comprobar devuelve el estado de todos los componentes relevantes para la
// plataforma actual, ordenados por nombre.
func Comprobar() []Estado {
	estados := make([]Estado, 0, len(catalogo))
	for _, c := range catalogo {
		estado := ComprobarComponente(c)
		if estado.Relevante {
			estados = append(estados, estado)
		}
	}
	sort.Slice(estados, func(i, j int) bool {
		return estados[i].Nombre < estados[j].Nombre
	})
	return estados
}

// ComprobarComponente evalúa un único componente en la máquina actual.
func ComprobarComponente(c Componente) Estado {
	e := Estado{Componente: c, Relevante: esRelevante(c), PistaInstalacion: pistaParaPlataforma(c)}
	if !e.Relevante {
		return e
	}
	if ruta, err := exec.LookPath(c.Nombre); err == nil {
		e.Disponible = true
		e.Ruta = ruta
	}
	return e
}

// Faltantes devuelve solo los componentes relevantes que no están disponibles.
func Faltantes() []Estado {
	var faltan []Estado
	for _, e := range Comprobar() {
		if e.Relevante && !e.Disponible {
			faltan = append(faltan, e)
		}
	}
	return faltan
}

// buscarComponente localiza un componente del catálogo por nombre.
func buscarComponente(nombre string) (Componente, bool) {
	for _, c := range catalogo {
		if c.Nombre == nombre {
			return c, true
		}
	}
	return Componente{}, false
}

func esRelevante(c Componente) bool {
	return esRelevanteEn(c, runtime.GOOS)
}

func esRelevanteEn(c Componente, goos string) bool {
	if len(c.Plataformas) == 0 {
		return true
	}
	for _, p := range c.Plataformas {
		if p == goos {
			return true
		}
	}
	return false
}

func pistaParaPlataforma(c Componente) string {
	if c.PistasInstalacion == nil {
		return ""
	}
	if pista, ok := c.PistasInstalacion[runtime.GOOS]; ok {
		return pista
	}
	return ""
}

// EnvolverErrorEjecucion enriquece un error de exec.Command relacionado con un
// componente ausente. Si el error indica que el ejecutable no se encontró,
// devuelve un mensaje claro con la finalidad y la pista de instalación; en caso
// contrario devuelve el error original sin cambios.
func EnvolverErrorEjecucion(nombre string, err error) error {
	if err == nil {
		return nil
	}
	if !errors.Is(err, exec.ErrNotFound) {
		// exec.LookPath dentro de exec.Command envuelve el fallo en *exec.Error.
		var execErr *exec.Error
		if !errors.As(err, &execErr) || !errors.Is(execErr.Err, exec.ErrNotFound) {
			return err
		}
	}
	if c, ok := buscarComponente(nombre); ok {
		return errors.New(ComprobarComponente(c).MensajeFalta())
	}
	return fmt.Errorf("falta el componente externo '%s' en el PATH", nombre)
}

// ErrorFalta devuelve un error con el mensaje legible de un componente ausente
// del catálogo. Si el nombre no está en el catálogo, devuelve un error genérico.
func ErrorFalta(nombre string) error {
	if c, ok := buscarComponente(nombre); ok {
		return errors.New(ComprobarComponente(c).MensajeFalta())
	}
	return fmt.Errorf("falta el componente externo '%s' en el PATH", nombre)
}

// MensajeFalta construye un mensaje legible para un componente ausente.
func (e Estado) MensajeFalta() string {
	var b strings.Builder
	b.WriteString("falta el componente externo '")
	b.WriteString(e.Nombre)
	b.WriteString("', necesario para ")
	b.WriteString(e.ParaQue)
	if e.PistaInstalacion != "" {
		b.WriteString(". Para instalarlo: ")
		b.WriteString(e.PistaInstalacion)
	}
	return b.String()
}
