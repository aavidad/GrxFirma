// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certaccess

import "os"

// removeSensitiveFile intenta sobrescribir el contenido antes de eliminar el
// temporal. No promete borrado físico en SSD, snapshots o sistemas copy-on-
// write, pero reduce la exposición en sistemas de ficheros convencionales.
func removeSensitiveFile(path string) {
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0) // #nosec G304 -- ruta creada por os.CreateTemp en este paquete.
	if err == nil {
		if info, statErr := file.Stat(); statErr == nil && info.Size() > 0 {
			zeros := make([]byte, 32*1024)
			remaining := info.Size()
			for remaining > 0 {
				chunk := int64(len(zeros))
				if remaining < chunk {
					chunk = remaining
				}
				if _, writeErr := file.Write(zeros[:chunk]); writeErr != nil {
					break
				}
				remaining -= chunk
			}
			_ = file.Sync()
		}
		_ = file.Close()
	}
	_ = os.Remove(path)
}
