// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package filesystem_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/filesystem"
)

// --- Tests de AlmacenTemporal ---

func TestAlmacenTemporal_EscribirYBorrar(t *testing.T) {
	almacen, err := filesystem.NuevoAlmacenTemporal()
	if err != nil {
		t.Fatalf("no se pudo crear el almacen: %v", err)
	}
	defer almacen.Cerrar()

	datos := []byte("datos de firma de prueba")
	ruta, err := almacen.Write(context.Background(), datos)
	if err != nil {
		t.Fatalf("error al escribir temporal: %v", err)
	}

	// El fichero debe existir.
	if _, err := os.Stat(ruta); err != nil {
		t.Fatalf("el fichero temporal no existe: %v", err)
	}

	// Borrarlo.
	if err := almacen.Delete(context.Background(), ruta); err != nil {
		t.Fatalf("error al borrar temporal: %v", err)
	}

	// Ya no debe existir.
	if _, err := os.Stat(ruta); !os.IsNotExist(err) {
		t.Fatal("el fichero temporal deberia haber sido eliminado")
	}
}

func TestAlmacenTemporal_DatosVacios(t *testing.T) {
	almacen, _ := filesystem.NuevoAlmacenTemporal()
	defer almacen.Cerrar()

	_, err := almacen.Write(context.Background(), nil)
	if err == nil {
		t.Fatal("se esperaba error al escribir datos vacios")
	}
}

func TestAlmacenTemporal_PathTraversalRechazado(t *testing.T) {
	almacen, _ := filesystem.NuevoAlmacenTemporal()
	defer almacen.Cerrar()

	err := almacen.Delete(context.Background(), "/etc/passwd")
	if err == nil {
		t.Fatal("se esperaba error al intentar borrar una ruta fuera del almacen")
	}
}

func TestAlmacenTemporal_CerrarLimpiaTodo(t *testing.T) {
	almacen, err := filesystem.NuevoAlmacenTemporal()
	if err != nil {
		t.Fatalf("no se pudo crear el almacen: %v", err)
	}

	ruta, _ := almacen.Write(context.Background(), []byte("datos"))
	dir := filepath.Dir(ruta)

	if err := almacen.Cerrar(); err != nil {
		t.Fatalf("error al cerrar el almacen: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("el directorio de temporales deberia haber sido eliminado al cerrar")
	}
}

// --- Tests de EscritorResultado ---

func TestEscritorResultado_EscribirNuevo(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "resultado.csig")

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaFallar)
	rutaFinal, err := escritor.Escribir(ruta, []byte("firma cades"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if rutaFinal != ruta {
		t.Fatalf("se esperaba ruta %s, se obtuvo %s", ruta, rutaFinal)
	}

	contenido, _ := os.ReadFile(rutaFinal)
	if string(contenido) != "firma cades" {
		t.Fatal("el contenido del fichero no coincide")
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(rutaFinal)
		if statErr != nil {
			t.Fatalf("no se pudieron comprobar los permisos: %v", statErr)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("permisos del resultado = %04o, se esperaba 0600", got)
		}
	}
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("no se pudo inspeccionar el directorio de salida: %v", err)
	}
	if len(entradas) != 1 || entradas[0].Name() != "resultado.csig" {
		t.Fatalf("quedaron ficheros temporales tras publicar: %v", entradas)
	}
}

func TestEscritorResultado_PoliticaFallar_FicheroExistente(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "existente.csig")
	os.WriteFile(ruta, []byte("anterior"), 0o600)

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaFallar)
	_, err := escritor.Escribir(ruta, []byte("nuevo"))
	if err == nil {
		t.Fatal("se esperaba error por fichero existente con politica Fallar")
	}
}

func TestEscritorResultado_PoliticaForzar_Sobreescribe(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "existente.csig")
	if err := os.WriteFile(ruta, []byte("anterior"), 0o640); err != nil {
		t.Fatalf("no se pudo preparar el fichero existente: %v", err)
	}

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaForzar)
	_, err := escritor.Escribir(ruta, []byte("nuevo"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	contenido, _ := os.ReadFile(ruta)
	if string(contenido) != "nuevo" {
		t.Fatal("el fichero no fue sobreescrito")
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(ruta)
		if statErr != nil {
			t.Fatalf("no se pudieron comprobar los permisos: %v", statErr)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("permisos tras sobreescribir = %04o, se esperaba 0640", got)
		}
	}
}

func TestEscritorResultado_PoliticaRenombrar(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "resultado.csig")
	os.WriteFile(ruta, []byte("anterior"), 0o600)

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaRenombrar)
	rutaFinal, err := escritor.Escribir(ruta, []byte("nuevo"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if rutaFinal == ruta {
		t.Fatal("se esperaba una ruta alternativa, no la original")
	}

	contenido, _ := os.ReadFile(rutaFinal)
	if string(contenido) != "nuevo" {
		t.Fatal("el contenido del fichero renombrado no coincide")
	}
}

func TestEscritorResultado_DatosVacios(t *testing.T) {
	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaFallar)
	_, err := escritor.Escribir("/tmp/prueba.csig", nil)
	if err == nil {
		t.Fatal("se esperaba error al escribir datos vacios")
	}
}

func TestEscritorResultado_RechazaEnlaceSimbolicoComoDestino(t *testing.T) {
	dir := t.TempDir()
	victima := filepath.Join(dir, "victima.txt")
	ruta := filepath.Join(dir, "resultado.csig")
	if err := os.WriteFile(victima, []byte("intacto"), 0o600); err != nil {
		t.Fatalf("no se pudo preparar el fichero victima: %v", err)
	}
	if err := os.Symlink(victima, ruta); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaForzar)
	if _, err := escritor.Escribir(ruta, []byte("sobrescrito")); err == nil {
		t.Fatal("se esperaba error al usar un enlace simbolico como destino")
	}

	contenido, err := os.ReadFile(victima)
	if err != nil {
		t.Fatalf("no se pudo leer el fichero victima: %v", err)
	}
	if string(contenido) != "intacto" {
		t.Fatalf("el enlace simbolico modifico el fichero victima: %q", contenido)
	}
	info, err := os.Lstat(ruta)
	if err != nil {
		t.Fatalf("el enlace simbolico original desaparecio: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("el destino dejo de ser un enlace simbolico")
	}
}

func TestEscritorResultado_RechazaEnlaceSimbolicoEnDirectorio(t *testing.T) {
	dir := t.TempDir()
	directorioReal := filepath.Join(dir, "real")
	if err := os.Mkdir(directorioReal, 0o700); err != nil {
		t.Fatalf("no se pudo preparar el directorio real: %v", err)
	}
	enlace := filepath.Join(dir, "enlace")
	if err := os.Symlink(directorioReal, enlace); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}

	ruta := filepath.Join(enlace, "resultado.csig")
	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaFallar)
	if _, err := escritor.Escribir(ruta, []byte("firma")); err == nil {
		t.Fatal("se esperaba error por enlace simbolico en el directorio")
	}
	if _, err := os.Stat(filepath.Join(directorioReal, "resultado.csig")); !os.IsNotExist(err) {
		t.Fatal("se escribio un resultado a traves del enlace simbolico")
	}
}

func TestEscritorResultado_AdmiteAliasDeRaizTemporalPeroNoEnlacesInternos(t *testing.T) {
	raizReal := t.TempDir()
	padreAlias := t.TempDir()
	raizAlias := filepath.Join(padreAlias, "temporal-alias")
	if err := os.Symlink(raizReal, raizAlias); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}
	t.Setenv("TMPDIR", raizAlias)
	t.Setenv("TMP", raizAlias)
	t.Setenv("TEMP", raizAlias)
	// GetTempPath2 usa SystemTemp cuando el proceso es LocalSystem.
	t.Setenv("SystemTemp", raizAlias)
	if filepath.Clean(os.TempDir()) != filepath.Clean(raizAlias) {
		t.Fatalf("la fixture no seleccionó la raíz temporal: %q", os.TempDir())
	}

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaFallar)
	ruta := filepath.Join(raizAlias, "resultado.csig")
	if _, err := escritor.Escribir(ruta, []byte("firma")); err != nil {
		t.Fatalf("se rechazo el alias de la raiz temporal elegida por el proceso: %v", err)
	}
	if contenido, err := os.ReadFile(filepath.Join(raizReal, "resultado.csig")); err != nil {
		t.Fatalf("no se escribio a traves del alias temporal: %v", err)
	} else if string(contenido) != "firma" {
		t.Fatalf("contenido inesperado: %q", contenido)
	}

	directorioReal := filepath.Join(raizReal, "destino-real")
	if err := os.Mkdir(directorioReal, 0o700); err != nil {
		t.Fatalf("no se pudo crear el destino real: %v", err)
	}
	enlaceInterno := filepath.Join(raizAlias, "destino-alias")
	if err := os.Symlink(directorioReal, enlaceInterno); err != nil {
		t.Fatalf("no se pudo crear el enlace interno: %v", err)
	}
	if _, err := escritor.Escribir(
		filepath.Join(enlaceInterno, "escape.csig"),
		[]byte("no escribir"),
	); err == nil {
		t.Fatal("se acepto un enlace simbolico por debajo de la raiz temporal")
	}
	if _, err := os.Stat(filepath.Join(directorioReal, "escape.csig")); !os.IsNotExist(err) {
		t.Fatal("se escribio a traves de un enlace interno no confiable")
	}
}

func TestEscritorResultado_RenombrarOmiteEnlaceSimbolico(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "resultado.csig")
	if err := os.WriteFile(ruta, []byte("anterior"), 0o600); err != nil {
		t.Fatalf("no se pudo preparar el resultado existente: %v", err)
	}
	victima := filepath.Join(dir, "victima.txt")
	if err := os.WriteFile(victima, []byte("intacto"), 0o600); err != nil {
		t.Fatalf("no se pudo preparar el fichero victima: %v", err)
	}
	if err := os.Symlink(victima, filepath.Join(dir, "resultado_001.csig")); err != nil {
		t.Skipf("el sistema no permite crear enlaces simbolicos: %v", err)
	}

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaRenombrar)
	rutaFinal, err := escritor.Escribir(ruta, []byte("nuevo"))
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	esperada := filepath.Join(dir, "resultado_002.csig")
	if rutaFinal != esperada {
		t.Fatalf("ruta final = %s, se esperaba %s", rutaFinal, esperada)
	}
	contenido, err := os.ReadFile(victima)
	if err != nil {
		t.Fatalf("no se pudo leer el fichero victima: %v", err)
	}
	if string(contenido) != "intacto" {
		t.Fatalf("el enlace alternativo modifico el fichero victima: %q", contenido)
	}
}

func TestEscritorResultado_FallarPublicaUnaSolaVezConcurrencia(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "resultado.csig")

	const escritores = 12
	inicio := make(chan struct{})
	resultados := make(chan error, escritores)
	var grupo sync.WaitGroup
	for i := 0; i < escritores; i++ {
		grupo.Add(1)
		go func(indice int) {
			defer grupo.Done()
			<-inicio
			escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaFallar)
			_, err := escritor.Escribir(ruta, []byte{byte('A' + indice)})
			resultados <- err
		}(i)
	}
	close(inicio)
	grupo.Wait()
	close(resultados)

	exitos := 0
	for err := range resultados {
		if err == nil {
			exitos++
		}
	}
	if exitos != 1 {
		t.Fatalf("escrituras exitosas = %d, se esperaba exactamente una", exitos)
	}
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("no se pudo leer el resultado publicado: %v", err)
	}
	if len(contenido) != 1 || !strings.ContainsRune("ABCDEFGHIJKL", rune(contenido[0])) {
		t.Fatalf("contenido publicado inesperado: %q", contenido)
	}
}

func TestEscritorResultado_RechazaComponentePadre(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "subdir") + string(os.PathSeparator) + ".." +
		string(os.PathSeparator) + "resultado.csig"

	escritor := filesystem.NuevoEscritorResultado(filesystem.PoliticaFallar)
	if _, err := escritor.Escribir(ruta, []byte("firma")); err == nil {
		t.Fatal("se esperaba error por componente padre en la ruta")
	}
}
