// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ProtocolKey es la clave que registra el instalador en HKCU.
const ProtocolKey = `Software\Classes\afirma`

// Nombres de fichero del componente AfirmaURI instalado.
const (
	ExecutableName = "grxfirma-afirmauri.exe"
	IconName       = "grxfirma-grx.ico"
	SnapshotName   = "afirma-protocol-snapshot.json"
)

// legacyIconNames son los iconos que registraron versiones anteriores
// ($script:AfirmaLegacyIconFileNames del instalador).
var legacyIconNames = []string{"grxfirma-diputacion.ico", "grxfirma.ico"}

const maxSnapshotBytes = 1 << 20

// Paths son las rutas del componente AfirmaURI de esta instalación.
type Paths struct {
	InstallDir string
	Executable string
	Icon       string
	Snapshot   string
}

// PathsFor construye las rutas a partir de la carpeta AfirmaURI.
func PathsFor(installDir string) Paths {
	return Paths{
		InstallDir: installDir,
		Executable: installDir + `\` + ExecutableName,
		Icon:       installDir + `\` + IconName,
		Snapshot:   installDir + `\` + SnapshotName,
	}
}

// registrationPlan es Get-AfirmaProtocolRegistrationPlan.
func registrationPlan(executable, icon string) []ValueSnapshot {
	if strings.TrimSpace(icon) == "" {
		icon = executable
	}
	return []ValueSnapshot{
		ownedString(ProtocolKey, "", "URL:GrxFirma Protocol"),
		ownedString(ProtocolKey, "URL Protocol", ""),
		ownedString(ProtocolKey+`\DefaultIcon`, "", icon+",0"),
		ownedString(ProtocolKey+`\shell\open\command`, "", `"`+executable+`" "%1"`),
	}
}

// ownerSets devuelve el plan actual seguido de los conjuntos que escribieron
// versiones anteriores de esta misma instalación (solo cambia el icono, y
// siempre dentro de la carpeta del ejecutable actual).
func ownerSets(paths Paths) ([][]ValueSnapshot, error) {
	separator := strings.LastIndexAny(paths.Executable, `\/`)
	if separator <= 0 {
		return nil, errors.New("la ruta del ejecutable de afirma:// no es absoluta")
	}
	installDir := paths.Executable[:separator]
	current := registrationPlan(paths.Executable, paths.Icon)
	sets := [][]ValueSnapshot{current}
	candidates := []string{""}
	for _, name := range legacyIconNames {
		candidates = append(candidates, installDir+`\`+name)
	}
	for _, icon := range candidates {
		candidate := registrationPlan(paths.Executable, icon)
		if snapshotsEqual(candidate[2], current[2]) {
			continue
		}
		sets = append(sets, candidate)
	}
	return sets, nil
}

func valuesMatchSet(values, set []ValueSnapshot) bool {
	if len(values) != len(set) {
		return false
	}
	for i := range values {
		if !snapshotsEqual(values[i], set[i]) {
			return false
		}
	}
	return true
}

func findMatchingSet(values []ValueSnapshot, sets [][]ValueSnapshot) []ValueSnapshot {
	for _, set := range sets {
		if valuesMatchSet(values, set) {
			return set
		}
	}
	return nil
}

func readValues(reg Registry, plan []ValueSnapshot) ([]ValueSnapshot, error) {
	values := make([]ValueSnapshot, len(plan))
	for i, entry := range plan {
		v, err := reg.Read(CurrentUser, entry.Path, entry.Name)
		if err != nil {
			return nil, err
		}
		values[i] = v
	}
	return values, nil
}

type snapshotDocument struct {
	SchemaVersion int             `json:"schema_version"`
	ProtocolKey   string          `json:"protocol_key"`
	OwnerValues   []ValueSnapshot `json:"owner_values"`
	Snapshots     []ValueSnapshot `json:"snapshots"`
}

type protocolState struct {
	OwnerValues []ValueSnapshot
	Snapshots   []ValueSnapshot
}

var errSnapshotInvalid = errors.New("la instantánea del protocolo afirma:// no es válida")

// readSnapshotFile lee la instantánea sin seguir enlaces y con límite de
// tamaño. Devuelve (nil, nil) si no existe.
func readSnapshotFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: no es un fichero normal", errSnapshotInvalid)
	}
	if info.Size() > maxSnapshotBytes {
		return nil, fmt.Errorf("%w: supera el límite permitido", errSnapshotInvalid)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSnapshotBytes {
		return nil, fmt.Errorf("%w: supera el límite permitido", errSnapshotInvalid)
	}
	return data, nil
}

// parseSnapshot es Read-AfirmaProtocolSnapshot.
func parseSnapshot(data []byte, plan []ValueSnapshot, accepted [][]ValueSnapshot) (*protocolState, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var doc snapshotDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: JSON no válido", errSnapshotInvalid)
	}
	if doc.SchemaVersion != 1 || !strings.EqualFold(doc.ProtocolKey, ProtocolKey) {
		return nil, fmt.Errorf("%w: esquema o destino no válido", errSnapshotInvalid)
	}
	if len(doc.OwnerValues) != len(plan) || len(doc.Snapshots) != len(plan) {
		return nil, fmt.Errorf("%w: incompleta", errSnapshotInvalid)
	}
	for i := range plan {
		for _, record := range []ValueSnapshot{doc.OwnerValues[i], doc.Snapshots[i]} {
			if !strings.EqualFold(record.Path, plan[i].Path) || record.Name != plan[i].Name {
				return nil, fmt.Errorf("%w: ruta no permitida", errSnapshotInvalid)
			}
		}
		if !doc.OwnerValues[i].ValueExisted || doc.OwnerValues[i].Kind != KindString {
			return nil, fmt.Errorf("%w: marcador de propiedad no válido", errSnapshotInvalid)
		}
		if doc.Snapshots[i].ValueExisted {
			if err := validateSnapshotValue(doc.Snapshots[i]); err != nil {
				return nil, fmt.Errorf("%w: valor no válido", errSnapshotInvalid)
			}
		}
	}
	if findMatchingSet(doc.OwnerValues, accepted) == nil {
		return nil, fmt.Errorf("%w: no pertenece a este ejecutable", errSnapshotInvalid)
	}
	return &protocolState{OwnerValues: doc.OwnerValues, Snapshots: doc.Snapshots}, nil
}

func encodeSnapshot(owner, snapshots []ValueSnapshot) ([]byte, error) {
	data, err := json.MarshalIndent(snapshotDocument{
		SchemaVersion: 1,
		ProtocolKey:   ProtocolKey,
		OwnerValues:   owner,
		Snapshots:     snapshots,
	}, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(data, '\r', '\n'), nil
}

// writeFileAtomic escribe en un temporal de la misma carpeta y lo renombra.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
