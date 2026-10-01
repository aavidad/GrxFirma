// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PoliticaSobreescritura define el comportamiento cuando el fichero de salida ya existe.
type PoliticaSobreescritura int

const (
	// PoliticaFallar devuelve error si el fichero ya existe.
	PoliticaFallar PoliticaSobreescritura = iota

	// PoliticaRenombrar genera un nombre alternativo si el fichero ya existe.
	PoliticaRenombrar

	// PoliticaForzar sobreescribe el fichero existente sin preguntar.
	PoliticaForzar
)

// EscritorResultado escribe el resultado de firma en una ruta de salida en disco.
type EscritorResultado struct {
	politica PoliticaSobreescritura
}

// NuevoEscritorResultado crea un EscritorResultado con la politica indicada.
func NuevoEscritorResultado(politica PoliticaSobreescritura) *EscritorResultado {
	return &EscritorResultado{politica: politica}
}

// Escribir guarda los datos firmados en la ruta indicada.
// Devuelve la ruta final donde se escribio el fichero (puede diferir si se renombro).
func (e *EscritorResultado) Escribir(rutaSalida string, datos []byte) (string, error) {
	if len(datos) == 0 {
		return "", errors.New("no se puede escribir un resultado de firma vacio")
	}
	if rutaSalida == "" {
		return "", errors.New("la ruta de salida no puede estar vacia")
	}
	if strings.IndexByte(rutaSalida, 0) >= 0 {
		return "", errors.New("la ruta de salida contiene un caracter nulo")
	}
	if contieneComponentePadre(rutaSalida) {
		return "", errors.New("la ruta de salida contiene componentes padre no permitidos")
	}

	if err := prepararDirectorioSeguro(filepath.Dir(rutaSalida)); err != nil {
		return "", err
	}

	rutaFinal, modo, err := e.resolverRuta(rutaSalida)
	if err != nil {
		return "", err
	}

	temporal, err := crearTemporalResultado(filepath.Dir(rutaFinal), modo, datos)
	if err != nil {
		return "", err
	}
	defer os.Remove(temporal)

	// Acorta la ventana entre validar los componentes y publicar. La operacion
	// final no sigue el destino: lo crea en exclusiva o reemplaza su entrada.
	if err := prepararDirectorioSeguro(filepath.Dir(rutaFinal)); err != nil {
		return "", err
	}

	switch e.politica {
	case PoliticaForzar:
		if err := validarDestinoForzado(rutaFinal); err != nil {
			return "", err
		}
		if err := reemplazarFicheroAtomico(temporal, rutaFinal); err != nil {
			return "", fmt.Errorf("no se pudo publicar el fichero de salida: %w", err)
		}
		return rutaFinal, nil

	case PoliticaFallar:
		if err := publicarSinSobrescribir(temporal, rutaFinal); err != nil {
			if errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("el fichero de salida ya existe: %s", rutaFinal)
			}
			return "", fmt.Errorf("no se pudo publicar el fichero de salida: %w", err)
		}
		return rutaFinal, nil

	case PoliticaRenombrar:
		for intentos := 0; intentos < 1000; intentos++ {
			err := publicarSinSobrescribir(temporal, rutaFinal)
			if err == nil {
				return rutaFinal, nil
			}
			if !errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("no se pudo publicar el fichero de salida: %w", err)
			}
			rutaFinal, err = generarRutaAlternativa(rutaSalida)
			if err != nil {
				return "", err
			}
		}
		return "", fmt.Errorf("no se pudo encontrar una ruta alternativa para: %s", rutaSalida)

	default:
		return "", errors.New("politica de sobreescritura desconocida")
	}
}

func (e *EscritorResultado) resolverRuta(ruta string) (string, os.FileMode, error) {
	info, err := os.Lstat(ruta)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", 0, fmt.Errorf("no se pudo comprobar el fichero de salida: %w", err)
	}
	existeFichero := err == nil

	if !existeFichero {
		return ruta, 0o600, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", 0, fmt.Errorf("el fichero de salida no puede ser un enlace simbolico: %s", ruta)
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("la ruta de salida no es un fichero regular: %s", ruta)
	}

	switch e.politica {
	case PoliticaFallar:
		return "", 0, fmt.Errorf("el fichero de salida ya existe: %s", ruta)

	case PoliticaForzar:
		return ruta, info.Mode().Perm(), nil

	case PoliticaRenombrar:
		rutaAlternativa, err := generarRutaAlternativa(ruta)
		return rutaAlternativa, 0o600, err

	default:
		return "", 0, errors.New("politica de sobreescritura desconocida")
	}
}

// generarRutaAlternativa genera una ruta alternativa añadiendo un sufijo numerico.
func generarRutaAlternativa(ruta string) (string, error) {
	ext := filepath.Ext(ruta)
	base := ruta[:len(ruta)-len(ext)]

	for i := 1; i <= 999; i++ {
		candidata := fmt.Sprintf("%s_%03d%s", base, i, ext)
		if _, err := os.Lstat(candidata); errors.Is(err, os.ErrNotExist) {
			return candidata, nil
		} else if err != nil {
			return "", fmt.Errorf("no se pudo comprobar una ruta alternativa: %w", err)
		}
	}
	return "", fmt.Errorf("no se pudo encontrar una ruta alternativa para: %s", ruta)
}

func contieneComponentePadre(ruta string) bool {
	volumen := filepath.VolumeName(ruta)
	sinVolumen := strings.TrimPrefix(ruta, volumen)
	for _, componente := range strings.FieldsFunc(sinVolumen, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if componente == ".." {
			return true
		}
	}
	return false
}

// prepararDirectorioSeguro crea los directorios que falten sin aceptar enlaces
// simbolicos ni componentes que no sean directorios.
func prepararDirectorioSeguro(directorio string) error {
	absoluto, err := filepath.Abs(directorio)
	if err != nil {
		return fmt.Errorf("no se pudo resolver el directorio de salida: %w", err)
	}
	absoluto = canonicalizarRaizSalidaConocida(absoluto)

	volumen := filepath.VolumeName(absoluto)
	raiz := volumen + string(os.PathSeparator)
	relativo, err := filepath.Rel(raiz, absoluto)
	if err != nil {
		return fmt.Errorf("no se pudo resolver el directorio de salida: %w", err)
	}

	actual := raiz
	if relativo == "." {
		return validarDirectorio(actual)
	}
	for _, componente := range strings.Split(relativo, string(os.PathSeparator)) {
		if componente == "" || componente == "." {
			continue
		}
		actual = filepath.Join(actual, componente)
		info, statErr := os.Lstat(actual)
		if errors.Is(statErr, os.ErrNotExist) {
			if mkdirErr := os.Mkdir(actual, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return fmt.Errorf("no se pudo crear el directorio de salida: %w", mkdirErr)
			}
			info, statErr = os.Lstat(actual)
		}
		if statErr != nil {
			return fmt.Errorf("no se pudo comprobar el directorio de salida: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("el directorio de salida contiene un enlace simbolico: %s", actual)
		}
		if !info.IsDir() {
			return fmt.Errorf("un componente de la ruta de salida no es un directorio: %s", actual)
		}
	}
	return nil
}

// canonicalizarRaizSalidaConocida elimina solamente los alias que forman parte
// de una raiz elegida por el propio proceso (el temporal del sistema o el home
// del usuario). macOS expone, por ejemplo, /var como alias de /private/var y
// os.TempDir devuelve rutas bajo ese alias. Los enlaces que aparezcan por
// debajo de estas raices siguen recorriendose con Lstat y se rechazan.
func canonicalizarRaizSalidaConocida(ruta string) string {
	raices := []string{os.TempDir()}
	if home, err := os.UserHomeDir(); err == nil {
		raices = append(raices, home)
	}

	mejorRuta := ruta
	mejorLongitud := -1
	for _, raiz := range raices {
		raizAbsoluta, err := filepath.Abs(raiz)
		if err != nil {
			continue
		}
		relativa, err := filepath.Rel(raizAbsoluta, ruta)
		if err != nil || relativaSaleDeRaiz(relativa) {
			continue
		}
		raizCanonica, err := filepath.EvalSymlinks(raizAbsoluta)
		if err != nil || raizCanonica == raizAbsoluta {
			continue
		}
		if len(raizAbsoluta) <= mejorLongitud {
			continue
		}
		mejorRuta = filepath.Join(raizCanonica, relativa)
		mejorLongitud = len(raizAbsoluta)
	}
	return mejorRuta
}

func relativaSaleDeRaiz(relativa string) bool {
	return relativa == ".." ||
		strings.HasPrefix(relativa, ".."+string(os.PathSeparator)) ||
		filepath.IsAbs(relativa)
}

func validarDirectorio(ruta string) error {
	info, err := os.Lstat(ruta)
	if err != nil {
		return fmt.Errorf("no se pudo comprobar el directorio de salida: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("el directorio de salida contiene un enlace simbolico: %s", ruta)
	}
	if !info.IsDir() {
		return fmt.Errorf("la ruta de salida no es un directorio: %s", ruta)
	}
	return nil
}

func crearTemporalResultado(directorio string, modo os.FileMode, datos []byte) (ruta string, err error) {
	fichero, err := os.CreateTemp(directorio, ".grxfirma-resultado-*.tmp")
	if err != nil {
		return "", fmt.Errorf("no se pudo crear el fichero temporal de salida: %w", err)
	}
	ruta = fichero.Name()
	defer func() {
		if err != nil {
			_ = fichero.Close()
			_ = os.Remove(ruta)
		}
	}()

	if err = fichero.Chmod(modo.Perm()); err != nil {
		return "", fmt.Errorf("no se pudieron establecer los permisos del fichero temporal: %w", err)
	}
	if _, err = fichero.Write(datos); err != nil {
		return "", fmt.Errorf("no se pudo escribir el fichero temporal de salida: %w", err)
	}
	if err = fichero.Sync(); err != nil {
		return "", fmt.Errorf("no se pudo sincronizar el fichero temporal de salida: %w", err)
	}
	if err = fichero.Close(); err != nil {
		return "", fmt.Errorf("no se pudo cerrar el fichero temporal de salida: %w", err)
	}
	return ruta, nil
}

// publicarSinSobrescribir enlaza el temporal con el nombre definitivo. La
// creacion del enlace es atomica y falla si cualquier entrada (incluido un
// enlace simbolico) aparece en el destino durante la operacion.
func publicarSinSobrescribir(temporal, destino string) error {
	if err := os.Link(temporal, destino); err != nil {
		return err
	}
	if err := os.Remove(temporal); err != nil {
		return fmt.Errorf("resultado publicado, pero no se pudo retirar el temporal: %w", err)
	}
	return nil
}

func validarDestinoForzado(ruta string) error {
	info, err := os.Lstat(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("no se pudo comprobar el fichero de salida: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("el fichero de salida no puede ser un enlace simbolico: %s", ruta)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("la ruta de salida no es un fichero regular: %s", ruta)
	}
	return nil
}
