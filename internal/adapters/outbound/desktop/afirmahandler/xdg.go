// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"grxfirma/internal/ports"
)

// Selector para escritorios XDG (Linux): el programa que abre afirma:// es el
// .desktop indicado en la sección [Default Applications] de mimeapps.list.
// Todo se hace leyendo y escribiendo ficheros, sin ejecutar xdg-mime.

const (
	// AfirmaMimeType es el tipo MIME del protocolo de los portales.
	AfirmaMimeType = "x-scheme-handler/afirma"
	// GrxFirmaDesktopID es el lanzador del protocolo que instala GrxFirma.
	GrxFirmaDesktopID = "grxfirma.desktop"
	// XDGPreferenceName guarda la elección en ~/.config/grxfirma para que el
	// instalador la respete al actualizar.
	XDGPreferenceName = "afirma-protocol-handler"

	defaultAppsSection = "Default Applications"
	maxXDGFileBytes    = 1 << 20
)

// autoFirmaDesktopIDs son los lanzadores de AutoFirma para Linux: el paquete
// oficial (1.8 y 1.9) instala afirma.desktop; algunas distribuciones lo
// renombran a autofirma.desktop.
var autoFirmaDesktopIDs = []string{"afirma.desktop", "autofirma.desktop"}

// XDGEnv son las rutas del usuario según la especificación XDG.
type XDGEnv struct {
	Home string
	// ConfigHome es $XDG_CONFIG_HOME (por defecto ~/.config).
	ConfigHome string
	// ConfigDirs es $XDG_CONFIG_DIRS (por defecto /etc/xdg).
	ConfigDirs []string
	// DataHome es $XDG_DATA_HOME (por defecto ~/.local/share).
	DataHome string
	// DataDirs es $XDG_DATA_DIRS (por defecto /usr/local/share y /usr/share).
	DataDirs []string
	// Desktops son los nombres de $XDG_CURRENT_DESKTOP en minúsculas.
	Desktops []string
}

// XDGEnvFrom construye las rutas a partir de HOME y de una función que lee
// variables de entorno. Las rutas relativas se ignoran, como pide la
// especificación.
func XDGEnvFrom(home string, getenv func(string) string) XDGEnv {
	abs := func(value, fallback string) string {
		if filepath.IsAbs(value) {
			return filepath.Clean(value)
		}
		return fallback
	}
	list := func(value string, fallback []string) []string {
		var out []string
		for _, item := range strings.Split(value, ":") {
			if filepath.IsAbs(item) {
				out = append(out, filepath.Clean(item))
			}
		}
		if len(out) == 0 {
			return fallback
		}
		return out
	}
	env := XDGEnv{
		Home:       home,
		ConfigHome: abs(getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config")),
		ConfigDirs: list(getenv("XDG_CONFIG_DIRS"), []string{"/etc/xdg"}),
		DataHome:   abs(getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share")),
		DataDirs:   list(getenv("XDG_DATA_DIRS"), []string{"/usr/local/share", "/usr/share"}),
	}
	for _, name := range strings.Split(getenv("XDG_CURRENT_DESKTOP"), ":") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && !strings.ContainsAny(name, `/\`) && name != "." && name != ".." {
			env.Desktops = append(env.Desktops, name)
		}
	}
	return env
}

// XDGSelector implementa ports.ProtocoloAfirma con mimeapps.list.
type XDGSelector struct {
	env XDGEnv
	mu  sync.Mutex
}

var _ ports.ProtocoloAfirma = (*XDGSelector)(nil)

// NewXDGSelector crea el selector para las rutas indicadas.
func NewXDGSelector(env XDGEnv) *XDGSelector {
	return &XDGSelector{env: env}
}

// readSmallRegularFile lee un fichero normal (siguiendo enlaces, que son
// habituales en mimeapps.list gestionados con dotfiles) con límite de tamaño.
// Devuelve (nil, nil) si no existe.
func readSmallRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s no es un fichero normal", filepath.Base(path))
	}
	if info.Size() > maxXDGFileBytes {
		return nil, fmt.Errorf("%s supera el tamaño permitido", filepath.Base(path))
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxXDGFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxXDGFileBytes {
		return nil, fmt.Errorf("%s supera el tamaño permitido", filepath.Base(path))
	}
	return data, nil
}

// iniLines parte el contenido en líneas sin el salto final de cada una.
func iniLines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func sectionName(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) >= 2 && trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']' {
		return trimmed[1 : len(trimmed)-1], true
	}
	return "", false
}

func keyValue(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed[0] == '#' {
		return "", "", false
	}
	key, value, ok := strings.Cut(trimmed, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

// iniValue devuelve el primer valor de key en la sección indicada.
func iniValue(data []byte, section, key string) (string, bool) {
	current := ""
	for _, line := range iniLines(data) {
		if name, ok := sectionName(line); ok {
			current = name
			continue
		}
		if current != section {
			continue
		}
		if k, v, ok := keyValue(line); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func validDesktopID(id string) bool {
	return strings.HasSuffix(id, ".desktop") && len(id) > len(".desktop") &&
		!strings.ContainsAny(id, "/\\\x00") && !strings.HasPrefix(id, ".")
}

// applicationDirs son las carpetas de lanzadores por orden de prioridad.
func (s *XDGSelector) applicationDirs() []string {
	dirs := []string{filepath.Join(s.env.DataHome, "applications")}
	for _, dir := range s.env.DataDirs {
		dirs = append(dirs, filepath.Join(dir, "applications"))
	}
	return dirs
}

// mimeappsFiles son los mimeapps.list por orden de prioridad.
func (s *XDGSelector) mimeappsFiles() []string {
	var files []string
	add := func(dir string) {
		for _, desktop := range s.env.Desktops {
			files = append(files, filepath.Join(dir, desktop+"-mimeapps.list"))
		}
		files = append(files, filepath.Join(dir, "mimeapps.list"))
	}
	add(s.env.ConfigHome)
	for _, dir := range s.env.ConfigDirs {
		add(dir)
	}
	add(filepath.Join(s.env.DataHome, "applications"))
	for _, dir := range s.env.DataDirs {
		add(filepath.Join(dir, "applications"))
	}
	return files
}

// desktopEntry es lo que interesa de un lanzador .desktop.
type desktopEntry struct {
	path    string
	program string
	afirma  bool
}

// findDesktop busca el lanzador; el primero encontrado tapa a los demás.
func (s *XDGSelector) findDesktop(id string) (*desktopEntry, error) {
	if !validDesktopID(id) {
		return nil, nil
	}
	for _, dir := range s.applicationDirs() {
		path := filepath.Join(dir, id)
		data, err := readSmallRegularFile(path)
		if err != nil {
			return nil, err
		}
		if data == nil {
			continue
		}
		if hidden, _ := iniValue(data, "Desktop Entry", "Hidden"); strings.EqualFold(hidden, "true") {
			return nil, nil
		}
		entry := &desktopEntry{path: path}
		if exec, ok := iniValue(data, "Desktop Entry", "Exec"); ok {
			entry.program = execProgram(exec)
		}
		mimes, _ := iniValue(data, "Desktop Entry", "MimeType")
		for _, mime := range strings.Split(mimes, ";") {
			if strings.TrimSpace(mime) == AfirmaMimeType {
				entry.afirma = true
			}
		}
		return entry, nil
	}
	return nil, nil
}

// execProgram extrae el programa de una línea Exec, saltando «env VAR=valor».
func execProgram(exec string) string {
	var fields []string
	var current strings.Builder
	quoted, inField := false, false
	for i := 0; i < len(exec); i++ {
		c := exec[i]
		switch {
		case c == '"':
			quoted, inField = !quoted, true
		case c == '\\' && quoted && i+1 < len(exec):
			i++
			current.WriteByte(exec[i])
		case (c == ' ' || c == '\t') && !quoted:
			if inField {
				fields = append(fields, current.String())
				current.Reset()
				inField = false
			}
		default:
			current.WriteByte(c)
			inField = true
		}
	}
	if inField {
		fields = append(fields, current.String())
	}
	for i, field := range fields {
		if i == 0 && filepath.Base(field) == "env" {
			continue
		}
		if strings.Contains(field, "=") && !strings.Contains(field, "/") {
			continue
		}
		return field
	}
	return ""
}

func (s *XDGSelector) grxFirma() (*desktopEntry, error) {
	entry, err := s.findDesktop(GrxFirmaDesktopID)
	if err != nil || entry == nil || !entry.afirma ||
		!strings.Contains(strings.ToLower(filepath.Base(entry.program)), "afirmauri") {
		return nil, err
	}
	return entry, nil
}

// autoFirma devuelve el lanzador de AutoFirma y su identificador.
func (s *XDGSelector) autoFirma() (string, *desktopEntry, error) {
	for _, id := range autoFirmaDesktopIDs {
		entry, err := s.findDesktop(id)
		if err != nil {
			return "", nil, err
		}
		if entry == nil || !entry.afirma ||
			!strings.HasPrefix(strings.ToLower(filepath.Base(entry.program)), "autofirma") {
			continue
		}
		if filepath.IsAbs(entry.program) && !regularFileExists(entry.program) {
			continue
		}
		return id, entry, nil
	}
	return "", nil, nil
}

// defaultHandler devuelve el lanzador que abre afirma:// según mimeapps.list:
// el primero de la lista con más prioridad que esté instalado.
func (s *XDGSelector) defaultHandler() (string, *desktopEntry, error) {
	for _, file := range s.mimeappsFiles() {
		data, err := readSmallRegularFile(file)
		if err != nil {
			return "", nil, err
		}
		if data == nil {
			continue
		}
		value, ok := iniValue(data, defaultAppsSection, AfirmaMimeType)
		if !ok {
			continue
		}
		for _, id := range strings.Split(value, ";") {
			id = strings.TrimSpace(id)
			entry, err := s.findDesktop(id)
			if err != nil {
				return "", nil, err
			}
			if entry != nil {
				return id, entry, nil
			}
		}
	}
	return "", nil, nil
}

func (s *XDGSelector) preferencePath() string {
	return filepath.Join(s.env.Home, ".config", "grxfirma", XDGPreferenceName)
}

func (s *XDGSelector) preference() string {
	data, err := readSmallRegularFile(s.preferencePath())
	if err != nil || data == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(string(data))) {
	case ports.ProgramaAfirmaGrxFirma:
		return ports.ProgramaAfirmaGrxFirma
	case ports.ProgramaAfirmaAutoFirma:
		return ports.ProgramaAfirmaAutoFirma
	}
	return ""
}

// Estado lee qué programa abre ahora afirma://.
func (s *XDGSelector) Estado(ctx context.Context) (ports.EstadoProtocoloAfirma, error) {
	if err := ctx.Err(); err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status()
}

func (s *XDGSelector) status() (ports.EstadoProtocoloAfirma, error) {
	estado := ports.EstadoProtocoloAfirma{Soportado: true, Preferencia: s.preference()}
	grx, err := s.grxFirma()
	if err != nil {
		return estado, err
	}
	estado.GrxFirmaInstalada = grx != nil
	autoID, auto, err := s.autoFirma()
	if err != nil {
		return estado, err
	}
	if auto != nil {
		estado.AutoFirmaInstalada = true
		estado.RutaAutoFirma = auto.program
	}
	id, entry, err := s.defaultHandler()
	if err != nil {
		return estado, err
	}
	switch {
	case entry == nil:
		// Sin elección expresa el escritorio usa cualquiera de los instalados.
		switch {
		case grx != nil && auto == nil:
			estado.Actual, estado.RutaActual = ports.ProgramaAfirmaGrxFirma, grx.program
		case auto != nil && grx == nil:
			estado.Actual, estado.RutaActual = ports.ProgramaAfirmaAutoFirma, auto.program
		default:
			estado.Actual = ports.ProgramaAfirmaNinguno
		}
	case id == GrxFirmaDesktopID && grx != nil:
		estado.Actual, estado.RutaActual = ports.ProgramaAfirmaGrxFirma, grx.program
	case auto != nil && id == autoID:
		estado.Actual, estado.RutaActual = ports.ProgramaAfirmaAutoFirma, auto.program
	default:
		estado.Actual = ports.ProgramaAfirmaOtro
		estado.RutaActual = entry.program
		if estado.RutaActual == "" {
			estado.RutaActual = id
		}
	}
	return estado, nil
}

// Elegir deja afirma:// al programa pedido y guarda la preferencia.
func (s *XDGSelector) Elegir(ctx context.Context, programa string) (ports.EstadoProtocoloAfirma, error) {
	if err := ctx.Err(); err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var desktopID string
	switch strings.ToLower(strings.TrimSpace(programa)) {
	case ports.ProgramaAfirmaGrxFirma:
		grx, err := s.grxFirma()
		if err != nil {
			return ports.EstadoProtocoloAfirma{}, err
		}
		if grx == nil {
			return ports.EstadoProtocoloAfirma{}, ports.ErrGrxFirmaAfirmaNoInstalada
		}
		desktopID, programa = GrxFirmaDesktopID, ports.ProgramaAfirmaGrxFirma
	case ports.ProgramaAfirmaAutoFirma:
		id, auto, err := s.autoFirma()
		if err != nil {
			return ports.EstadoProtocoloAfirma{}, err
		}
		if auto == nil {
			return ports.EstadoProtocoloAfirma{}, ports.ErrAutoFirmaNoInstalada
		}
		desktopID, programa = id, ports.ProgramaAfirmaAutoFirma
	default:
		return ports.EstadoProtocoloAfirma{}, ports.ErrProgramaAfirmaDesconocido
	}

	// El fichero principal del usuario siempre; los demás ficheros del
	// usuario solo si ya nombraban afirma://, para que no tapen la elección.
	main := filepath.Join(s.env.ConfigHome, "mimeapps.list")
	if err := setDefaultInFile(main, desktopID, true); err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	for _, file := range s.userMimeappsFiles() {
		if file == main {
			continue
		}
		if err := setDefaultInFile(file, desktopID, false); err != nil {
			return ports.EstadoProtocoloAfirma{}, err
		}
	}
	if err := writeFileAtomicMode(s.preferencePath(), []byte(programa+"\n"), 0o600); err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	estado, err := s.status()
	if err != nil {
		return estado, err
	}
	if estado.Actual != programa {
		// Un mimeapps.list del sistema u otra configuración manda sobre la
		// del usuario; no se toca nada fuera de su carpeta personal.
		return estado, ports.ErrProtocoloAfirmaAjeno
	}
	return estado, nil
}

// userMimeappsFiles son los mimeapps.list del usuario que pueden nombrar
// afirma://, incluidos los de Firefox en Snap y Flatpak que configura el
// instalador.
func (s *XDGSelector) userMimeappsFiles() []string {
	var files []string
	for _, desktop := range s.env.Desktops {
		files = append(files, filepath.Join(s.env.ConfigHome, desktop+"-mimeapps.list"))
	}
	files = append(files,
		filepath.Join(s.env.ConfigHome, "mimeapps.list"),
		filepath.Join(s.env.DataHome, "applications", "mimeapps.list"),
		filepath.Join(s.env.Home, "snap", "firefox", "common", ".config", "mimeapps.list"),
		filepath.Join(s.env.Home, ".var", "app", "org.mozilla.firefox", "config", "mimeapps.list"),
	)
	return files
}

// setDefaultInFile pone afirma:// = desktopID en [Default Applications]
// conservando el resto del fichero. Si create es falso y el fichero no
// nombraba afirma://, no lo toca.
func setDefaultInFile(path, desktopID string, create bool) error {
	data, err := readSmallRegularFile(path)
	if err != nil {
		return err
	}
	if data == nil && !create {
		return nil
	}
	updated, found := setDefaultApplication(data, AfirmaMimeType, desktopID)
	if !found && !create {
		return nil
	}
	if bytes.Equal(updated, data) {
		return nil
	}
	target := path
	mode := os.FileMode(0o644)
	if data != nil {
		// Si es un enlace, se escribe en su destino para no romperlo.
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			target = resolved
		}
		if info, err := os.Stat(target); err == nil {
			mode = info.Mode().Perm()
		}
	}
	return writeFileAtomicMode(target, updated, mode)
}

// setDefaultApplication devuelve el contenido con key=value en la sección
// [Default Applications] y si la clave ya estaba. Las demás líneas no cambian;
// las repeticiones de la clave en esa sección se quitan.
func setDefaultApplication(data []byte, key, value string) ([]byte, bool) {
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	entry := key + "=" + value
	lines := iniLines(data)
	out := make([]string, 0, len(lines)+3)
	current := ""
	found, inFirst := false, false
	insertAt := -1 // tras la última línea con contenido de la primera sección
	for _, line := range lines {
		if name, ok := sectionName(line); ok {
			inFirst = name == defaultAppsSection && insertAt < 0
			if inFirst {
				insertAt = len(out) + 1
			}
			current = name
			out = append(out, line)
			continue
		}
		if current == defaultAppsSection {
			if k, _, ok := keyValue(line); ok && k == key {
				if !found {
					out = append(out, entry)
					found = true
				}
				continue
			}
		}
		out = append(out, line)
		if inFirst && strings.TrimSpace(line) != "" {
			insertAt = len(out)
		}
	}
	switch {
	case found:
	case insertAt >= 0:
		out = append(out[:insertAt], append([]string{entry}, out[insertAt:]...)...)
	default:
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, "["+defaultAppsSection+"]", entry)
	}
	return []byte(strings.Join(out, newline) + newline), found
}

// writeFileAtomicMode escribe en un temporal de la misma carpeta, lo vuelca a
// disco y lo renombra; crea la carpeta si falta.
func writeFileAtomicMode(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
