// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"os"
	"path/filepath"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

// escribirFicheroSalida es la escritura por defecto de las salidas de la CLI.
// Nunca escribe a través de un enlace simbólico y nunca deja un fichero
// truncado a medias:
//   - si el destino existe, debe ser un fichero regular y se sustituye de forma
//     atómica (el enlace se reemplaza, no se sigue);
//   - si no existe, se escribe un temporal en el mismo directorio y se publica
//     sin reemplazar nada, de modo que un enlace plantado entre la comprobación
//     de resolverRutaSalida y la escritura hace fallar la operación.
func escribirFicheroSalida(ruta string, datos []byte, perm os.FileMode) error {
	info, err := os.Lstat(ruta)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return &os.PathError{Op: "open", Path: ruta, Err: os.ErrInvalid}
		}
		return securefile.WriteFileAtomic(ruta, datos, perm)
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	return crearFicheroSinReemplazar(ruta, datos, perm)
}

func crearFicheroSinReemplazar(ruta string, datos []byte, perm os.FileMode) error {
	dir := filepath.Dir(ruta)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(ruta)+".tmp-*")
	if err != nil {
		return err
	}
	tmpRuta := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpRuta)
	}()
	if err := tmp.Chmod(perm.Perm()); err != nil {
		return err
	}
	if _, err := tmp.Write(datos); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	err = os.Link(tmpRuta, ruta)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrExist) {
		return &os.PathError{Op: "link", Path: ruta, Err: os.ErrExist}
	}
	// Sistemas de ficheros sin enlaces duros: creación exclusiva, que tampoco
	// sigue enlaces (falla si el nombre existe, aunque sea un enlace colgante).
	f, errExcl := os.OpenFile(ruta, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm.Perm())
	if errExcl != nil {
		return errExcl
	}
	if _, err := f.Write(datos); err != nil {
		_ = f.Close()
		_ = os.Remove(ruta)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(ruta)
		return err
	}
	return nil
}
