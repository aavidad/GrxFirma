// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"grxfirma/internal/ports"
)

// Preferencia guardada para que el instalador respete la elección al
// actualizar. La clave entera se borra al desinstalar GrxFirma.
const (
	PreferenceKey   = `Software\GrxFirma`
	PreferenceValue = "AfirmaProtocolHandler"
)

// autoFirmaExecutable es el ejecutable que registra el instalador oficial de
// AutoFirma: «C:\Program Files\Autofirma\Autofirma\Autofirma.exe "%1"».
const autoFirmaExecutable = "autofirma.exe"

// Selector implementa ports.ProtocoloAfirma sobre un Registry.
type Selector struct {
	reg   Registry
	paths Paths
	// fileExists permite simular en las pruebas los ejecutables instalados.
	fileExists func(string) bool
	mu         sync.Mutex
}

var _ ports.ProtocoloAfirma = (*Selector)(nil)

// NewSelector crea un selector con el registro y las rutas indicadas.
func NewSelector(reg Registry, paths Paths) *Selector {
	return &Selector{reg: reg, paths: paths, fileExists: regularFileExists}
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// commandExecutable extrae el ejecutable de una orden shell\open\command,
// entre comillas o no (AutoFirma la registra sin comillas y con espacios).
func commandExecutable(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	if strings.HasPrefix(command, `"`) {
		end := strings.Index(command[1:], `"`)
		if end < 0 {
			return ""
		}
		return command[1 : end+1]
	}
	lower := strings.ToLower(command)
	for from := 0; ; {
		idx := strings.Index(lower[from:], ".exe")
		if idx < 0 {
			break
		}
		end := from + idx + len(".exe")
		if end == len(command) || command[end] == ' ' || command[end] == '\t' {
			return command[:end]
		}
		from = end
	}
	if fields := strings.Fields(command); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func baseName(path string) string {
	if idx := strings.LastIndexAny(path, `\/`); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func isAbsoluteWindowsPath(path string) bool {
	return len(path) > 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

func commandText(v ValueSnapshot) string {
	if !v.ValueExisted || (v.Kind != KindString && v.Kind != KindExpandString) {
		return ""
	}
	return v.Data
}

// autoFirma busca el AutoFirma registrado para el equipo.
func (s *Selector) autoFirma() (installed bool, path string, err error) {
	command, err := s.reg.Read(LocalMachine, ProtocolKey+`\shell\open\command`, "")
	if err != nil {
		return false, "", err
	}
	exe := commandExecutable(commandText(command))
	if exe == "" || !isAbsoluteWindowsPath(exe) ||
		!strings.EqualFold(baseName(exe), autoFirmaExecutable) || !s.fileExists(exe) {
		return false, "", nil
	}
	return true, exe, nil
}

func (s *Selector) preference() string {
	v, err := s.reg.Read(CurrentUser, PreferenceKey, PreferenceValue)
	if err != nil || !v.ValueExisted || v.Kind != KindString {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(v.Data)) {
	case ports.ProgramaAfirmaGrxFirma:
		return ports.ProgramaAfirmaGrxFirma
	case ports.ProgramaAfirmaAutoFirma:
		return ports.ProgramaAfirmaAutoFirma
	}
	return ""
}

func (s *Selector) savePreference(programa string) error {
	return s.reg.Write(PreferenceKey, PreferenceValue, ownedString(PreferenceKey, PreferenceValue, programa))
}

// Estado lee del registro qué programa atiende ahora afirma://.
func (s *Selector) Estado(ctx context.Context) (ports.EstadoProtocoloAfirma, error) {
	if err := ctx.Err(); err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status()
}

func (s *Selector) status() (ports.EstadoProtocoloAfirma, error) {
	estado := ports.EstadoProtocoloAfirma{
		Soportado:         true,
		GrxFirmaInstalada: s.fileExists(s.paths.Executable),
		Preferencia:       s.preference(),
	}
	installed, autoPath, err := s.autoFirma()
	if err != nil {
		return estado, err
	}
	estado.AutoFirmaInstalada = installed
	estado.RutaAutoFirma = autoPath

	sets, err := ownerSets(s.paths)
	if err != nil {
		return estado, err
	}
	values, err := readValues(s.reg, sets[0])
	if err != nil {
		return estado, err
	}
	ownCommand := sets[0][3]
	command := values[3]
	if findMatchingSet(values, sets) == nil && !snapshotsEqual(command, ownCommand) {
		if !command.ValueExisted {
			// Sin orden propia del usuario, Windows usa la del equipo.
			command, err = s.reg.Read(LocalMachine, ProtocolKey+`\shell\open\command`, "")
			if err != nil {
				return estado, err
			}
		}
		exe := commandExecutable(commandText(command))
		switch {
		case exe == "":
			estado.Actual = ports.ProgramaAfirmaNinguno
		case strings.EqualFold(baseName(exe), autoFirmaExecutable):
			estado.Actual = ports.ProgramaAfirmaAutoFirma
			estado.RutaActual = exe
		default:
			estado.Actual = ports.ProgramaAfirmaOtro
			estado.RutaActual = exe
		}
		return estado, nil
	}
	estado.Actual = ports.ProgramaAfirmaGrxFirma
	estado.RutaActual = s.paths.Executable
	return estado, nil
}

// Elegir cambia el programa que atiende afirma:// y guarda la preferencia.
func (s *Selector) Elegir(ctx context.Context, programa string) (ports.EstadoProtocoloAfirma, error) {
	if err := ctx.Err(); err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	switch strings.ToLower(strings.TrimSpace(programa)) {
	case ports.ProgramaAfirmaGrxFirma:
		err = s.register()
		programa = ports.ProgramaAfirmaGrxFirma
	case ports.ProgramaAfirmaAutoFirma:
		err = s.retire()
		programa = ports.ProgramaAfirmaAutoFirma
	default:
		return ports.EstadoProtocoloAfirma{}, ports.ErrProgramaAfirmaDesconocido
	}
	if err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	if err := s.savePreference(programa); err != nil {
		return ports.EstadoProtocoloAfirma{}, err
	}
	return s.status()
}

func (s *Selector) loadState(plan []ValueSnapshot, accepted [][]ValueSnapshot) (*protocolState, []byte, error) {
	data, err := readSnapshotFile(s.paths.Snapshot)
	if err != nil || data == nil {
		return nil, nil, err
	}
	state, err := parseSnapshot(data, plan, accepted)
	if err != nil {
		return nil, nil, err
	}
	return state, data, nil
}

// register es Install-AfirmaProtocolRegistration: vuelve a dar afirma:// a
// GrxFirma conservando en la instantánea los valores de antes de GrxFirma.
func (s *Selector) register() error {
	if !s.fileExists(s.paths.Executable) {
		return ports.ErrGrxFirmaAfirmaNoInstalada
	}
	sets, err := ownerSets(s.paths)
	if err != nil {
		return err
	}
	plan := sets[0]
	current, err := readValues(s.reg, plan)
	if err != nil {
		return err
	}
	state, previousBytes, err := s.loadState(plan, sets)
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrProtocoloAfirmaAjeno, err)
	}
	var original []ValueSnapshot
	if state != nil {
		for i := range plan {
			knownOwner := snapshotsEqual(current[i], state.OwnerValues[i])
			for _, set := range sets {
				if knownOwner {
					break
				}
				knownOwner = snapshotsEqual(current[i], set[i])
			}
			if !knownOwner && !snapshotsEqual(current[i], state.Snapshots[i]) {
				return ports.ErrProtocoloAfirmaAjeno
			}
		}
		original = state.Snapshots
	} else if findMatchingSet(current, sets) != nil {
		original = make([]ValueSnapshot, len(plan))
		for i, entry := range plan {
			original[i] = absentSnapshot(entry.Path, entry.Name)
		}
	} else {
		original = current
	}

	encoded, err := encodeSnapshot(plan, original)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.paths.Snapshot, encoded); err != nil {
		return err
	}
	for attempted, entry := range plan {
		if err := s.reg.Write(entry.Path, entry.Name, entry); err != nil {
			return s.rollbackRegister(err, plan, current, attempted+1, previousBytes)
		}
	}
	return nil
}

func (s *Selector) rollbackRegister(cause error, plan, current []ValueSnapshot, attempted int, previousBytes []byte) error {
	var problems []string
	for i := attempted - 1; i >= 0; i-- {
		if err := restoreValue(s.reg, current[i], plan[i]); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if err := removeEmptySnapshotKeys(s.reg, current); err != nil {
		problems = append(problems, err.Error())
	}
	var fileErr error
	if previousBytes != nil {
		fileErr = writeFileAtomic(s.paths.Snapshot, previousBytes)
	} else {
		fileErr = os.Remove(s.paths.Snapshot)
	}
	if fileErr != nil && !errors.Is(fileErr, os.ErrNotExist) {
		problems = append(problems, fileErr.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("fallo registrando afirma://: %w; rollback incompleto: %s", cause, strings.Join(problems, "; "))
	}
	return cause
}

// retire deja afirma:// a AutoFirma: retira el registro de GrxFirma en HKCU
// y restaura lo que hubiera antes (normalmente nada, y entonces Windows usa el
// registro del equipo, que es el de AutoFirma).
func (s *Selector) retire() error {
	installed, _, err := s.autoFirma()
	if err != nil {
		return err
	}
	if !installed {
		return ports.ErrAutoFirmaNoInstalada
	}
	sets, err := ownerSets(s.paths)
	if err != nil {
		return err
	}
	plan := sets[0]
	current, err := readValues(s.reg, plan)
	if err != nil {
		return err
	}
	state, _, err := s.loadState(plan, sets)
	if err != nil {
		return fmt.Errorf("%w: %v", ports.ErrProtocoloAfirmaAjeno, err)
	}
	previous := make([]ValueSnapshot, len(plan))
	for i, entry := range plan {
		previous[i] = absentSnapshot(entry.Path, entry.Name)
	}
	if state != nil {
		previous = state.Snapshots
	}
	if findMatchingSet(current, sets) == nil {
		// Ya retirado (el registro coincide con lo de antes de GrxFirma):
		// no hay nada que hacer. Cualquier otra cosa es de otro programa.
		if valuesMatchSet(current, previous) {
			return nil
		}
		return ports.ErrProtocoloAfirmaAjeno
	}
	for i := range plan {
		if err := restoreValue(s.reg, previous[i], current[i]); err != nil {
			return s.rollbackRetire(err, current, previous, i)
		}
	}
	return removeEmptySnapshotKeys(s.reg, previous)
}

func (s *Selector) rollbackRetire(cause error, current, previous []ValueSnapshot, failed int) error {
	var problems []string
	for i := failed - 1; i >= 0; i-- {
		if err := restoreValue(s.reg, current[i], previous[i]); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("fallo restaurando el handler anterior: %w; rollback incompleto: %s", cause, strings.Join(problems, "; "))
	}
	if errors.Is(cause, errRegistryChanged) {
		return fmt.Errorf("%w: %v", ports.ErrProtocoloAfirmaAjeno, cause)
	}
	return cause
}
