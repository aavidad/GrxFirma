// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package afirmahandler elige qué programa atiende el protocolo afirma:// en
// Windows: GrxFirma (registro por usuario, HKCU) o AutoFirma, la aplicación
// Java del Gobierno (registro para todo el equipo, HKLM). En Linux lo hace
// XDGSelector sobre mimeapps.list (xdg.go).
//
// Es un porte en Go de la lógica de propiedad de
// packaging/windows/afirmauri-registration.ps1 (instantánea, conjuntos de
// valores propios y antiguos, restauración comparando antes de escribir) para
// que la aplicación de escritorio no tenga que ejecutar PowerShell. Nunca se
// modifica un valor que no sea de GrxFirma: si alguien cambió el registro de
// afirma:// desde la última vez, se aborta sin tocar nada.
package afirmahandler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Tipos de valor con los nombres de Microsoft.Win32.RegistryValueKind, que son
// los que guarda la instantánea escrita por el instalador.
const (
	KindString       = "String"
	KindExpandString = "ExpandString"
	KindBinary       = "Binary"
	KindDWord        = "DWord"
	KindMultiString  = "MultiString"
	KindQWord        = "QWord"
	KindNone         = "None"
	KindUnknown      = "Unknown"
)

// Hive distingue el registro del usuario del registro del equipo.
type Hive int

const (
	CurrentUser Hive = iota
	LocalMachine
)

// ValueSnapshot es un valor del registro tal como lo serializa la instantánea
// del instalador (mismas claves JSON y misma codificación de los datos).
type ValueSnapshot struct {
	Path         string
	Name         string
	KeyExisted   bool
	ValueExisted bool
	// Kind es "" cuando el valor no existe.
	Kind string
	// Data guarda el dato codificado: base64 para Binary y None, decimal para
	// DWord y QWord y el texto tal cual para el resto.
	Data string
	// Multi guarda los textos de un MultiString.
	Multi []string
}

type valueSnapshotJSON struct {
	Path         string          `json:"Path"`
	Name         string          `json:"Name"`
	KeyExisted   bool            `json:"KeyExisted"`
	ValueExisted bool            `json:"ValueExisted"`
	Kind         *string         `json:"Kind"`
	Value        json.RawMessage `json:"Value"`
}

// MarshalJSON reproduce el formato de ConvertTo-Json del instalador.
func (v ValueSnapshot) MarshalJSON() ([]byte, error) {
	out := valueSnapshotJSON{
		Path:         v.Path,
		Name:         v.Name,
		KeyExisted:   v.KeyExisted,
		ValueExisted: v.ValueExisted,
		Value:        json.RawMessage("null"),
	}
	if v.ValueExisted {
		kind := v.Kind
		out.Kind = &kind
		var err error
		if v.Kind == KindMultiString {
			multi := v.Multi
			if multi == nil {
				multi = []string{}
			}
			out.Value, err = json.Marshal(multi)
		} else {
			out.Value, err = json.Marshal(v.Data)
		}
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON acepta lo que escribe el instalador y lo que escribe este
// paquete. Un MultiString de un solo elemento puede llegar como texto.
func (v *ValueSnapshot) UnmarshalJSON(raw []byte) error {
	var in valueSnapshotJSON
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	*v = ValueSnapshot{
		Path:         in.Path,
		Name:         in.Name,
		KeyExisted:   in.KeyExisted,
		ValueExisted: in.ValueExisted,
	}
	if in.Kind != nil {
		v.Kind = *in.Kind
	}
	value := strings.TrimSpace(string(in.Value))
	if value == "" || value == "null" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		var multi []string
		if err := json.Unmarshal(in.Value, &multi); err != nil {
			return err
		}
		v.Multi = multi
		return nil
	}
	var text string
	if err := json.Unmarshal(in.Value, &text); err != nil {
		return err
	}
	if v.Kind == KindMultiString {
		v.Multi = []string{text}
		return nil
	}
	v.Data = text
	return nil
}

// Registry es el acceso mínimo al registro que necesita el selector. Las
// escrituras son siempre en el registro del usuario (HKCU).
type Registry interface {
	// Read devuelve el valor en la forma de la instantánea.
	Read(hive Hive, path, name string) (ValueSnapshot, error)
	// Write crea la clave si falta y escribe el valor con su tipo.
	Write(path, name string, value ValueSnapshot) error
	// DeleteValue borra el valor; no es error que no exista.
	DeleteValue(path, name string) error
	// Inspect informa de si la clave existe, cuántos valores tiene y el nombre
	// de sus subclaves.
	Inspect(path string) (exists bool, values int, subkeys []string, err error)
	// DeleteKey borra una clave sin subclaves.
	DeleteKey(path string) error
}

func absentSnapshot(path, name string) ValueSnapshot {
	return ValueSnapshot{Path: path, Name: name}
}

func ownedString(path, name, data string) ValueSnapshot {
	return ValueSnapshot{
		Path:         path,
		Name:         name,
		KeyExisted:   true,
		ValueExisted: true,
		Kind:         KindString,
		Data:         data,
	}
}

// snapshotsEqual es Test-AfirmaRegistrySnapshotsEqual: rutas sin distinguir
// mayúsculas, nombre exacto y, para String, dato sin distinguir mayúsculas.
func snapshotsEqual(left, right ValueSnapshot) bool {
	if !strings.EqualFold(left.Path, right.Path) ||
		left.Name != right.Name ||
		left.ValueExisted != right.ValueExisted {
		return false
	}
	if !left.ValueExisted {
		return true
	}
	if left.Kind != right.Kind {
		return false
	}
	if left.Kind == KindMultiString {
		if len(left.Multi) != len(right.Multi) {
			return false
		}
		for i := range left.Multi {
			if left.Multi[i] != right.Multi[i] {
				return false
			}
		}
		return true
	}
	if left.Kind == KindString {
		return strings.EqualFold(left.Data, right.Data)
	}
	return left.Data == right.Data
}

// validateSnapshotValue comprueba que un valor guardado se puede restaurar
// (ConvertFrom-AfirmaSnapshotValue).
func validateSnapshotValue(v ValueSnapshot) error {
	switch v.Kind {
	case KindString, KindExpandString, KindMultiString:
		return nil
	case KindBinary, KindNone:
		_, err := base64.StdEncoding.DecodeString(v.Data)
		return err
	case KindDWord:
		_, err := strconv.ParseUint(v.Data, 10, 32)
		return err
	case KindQWord:
		_, err := strconv.ParseUint(v.Data, 10, 64)
		return err
	default:
		return fmt.Errorf("tipo de valor no restaurable: %q", v.Kind)
	}
}

var errRegistryChanged = errors.New("el registro afirma:// cambió durante la operación")

// restoreValue es Restore-AfirmaRegistryValueSnapshot: solo escribe si el
// valor actual sigue siendo el esperado.
func restoreValue(reg Registry, target, expected ValueSnapshot) error {
	current, err := reg.Read(CurrentUser, target.Path, target.Name)
	if err != nil {
		return err
	}
	if snapshotsEqual(current, target) {
		return nil
	}
	if !snapshotsEqual(current, expected) {
		return fmt.Errorf("%w: HKCU\\%s", errRegistryChanged, target.Path)
	}
	if target.ValueExisted {
		if err := validateSnapshotValue(target); err != nil {
			return err
		}
		return reg.Write(target.Path, target.Name, target)
	}
	return reg.DeleteValue(target.Path, target.Name)
}

func treeHasValues(reg Registry, path string) (bool, error) {
	exists, values, subkeys, err := reg.Inspect(path)
	if err != nil || !exists {
		return false, err
	}
	if values > 0 {
		return true, nil
	}
	for _, sub := range subkeys {
		has, err := treeHasValues(reg, path+`\`+sub)
		if err != nil || has {
			return has, err
		}
	}
	return false, nil
}

func deleteEmptyTree(reg Registry, path string) error {
	exists, _, subkeys, err := reg.Inspect(path)
	if err != nil || !exists {
		return err
	}
	for _, sub := range subkeys {
		if err := deleteEmptyTree(reg, path+`\`+sub); err != nil {
			return err
		}
	}
	return reg.DeleteKey(path)
}

// removeEmptySnapshotKeys es Remove-AfirmaEmptySnapshotKeys: retira las
// claves que no existían antes y han quedado vacías.
func removeEmptySnapshotKeys(reg Registry, snapshots []ValueSnapshot) error {
	seen := map[string]bool{}
	var paths []string
	for _, s := range snapshots {
		key := strings.ToLower(s.Path)
		if !s.KeyExisted && !seen[key] {
			seen[key] = true
			paths = append(paths, s.Path)
		}
	}
	// De la más larga a la más corta, para vaciar primero las hojas.
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if len(paths[j]) > len(paths[i]) {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}
	for _, path := range paths {
		exists, values, subkeys, err := reg.Inspect(path)
		if err != nil {
			return err
		}
		if exists && values == 0 && len(subkeys) == 0 {
			if err := reg.DeleteKey(path); err != nil {
				return err
			}
		}
	}
	if len(snapshots) == 0 {
		return nil
	}
	root := snapshots[0]
	for _, s := range snapshots[1:] {
		if len(s.Path) < len(root.Path) {
			root = s
		}
	}
	if root.KeyExisted {
		return nil
	}
	has, err := treeHasValues(reg, root.Path)
	if err != nil || has {
		return err
	}
	return deleteEmptyTree(reg, root.Path)
}
