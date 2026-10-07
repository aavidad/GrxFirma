// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package afirmahandler

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"grxfirma/internal/ports"
)

// New devuelve el selector de la instalación por usuario de GrxFirma
// (%LOCALAPPDATA%\Programs\GrxFirma\AfirmaURI). La carpeta sale de la API de
// carpetas conocidas, no de variables de entorno.
func New() ports.ProtocoloAfirma {
	localAppData, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil || localAppData == "" {
		return unsupported{}
	}
	return NewSelector(windowsRegistry{}, PathsFor(localAppData+`\Programs\GrxFirma\AfirmaURI`))
}

type windowsRegistry struct{}

func rootKey(hive Hive) registry.Key {
	if hive == LocalMachine {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}

func (windowsRegistry) Read(hive Hive, path, name string) (ValueSnapshot, error) {
	snap := absentSnapshot(path, name)
	key, err := registry.OpenKey(rootKey(hive), path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return snap, nil
	}
	if err != nil {
		return snap, err
	}
	defer key.Close()
	snap.KeyExisted = true
	_, valueType, err := key.GetValue(name, nil)
	if errors.Is(err, registry.ErrNotExist) {
		return snap, nil
	}
	if err != nil && !errors.Is(err, registry.ErrShortBuffer) {
		return snap, err
	}
	snap.ValueExisted = true
	switch valueType {
	case registry.SZ, registry.EXPAND_SZ:
		text, _, err := key.GetStringValue(name)
		if err != nil {
			return snap, err
		}
		snap.Kind = KindString
		if valueType == registry.EXPAND_SZ {
			snap.Kind = KindExpandString
		}
		snap.Data = text
	case registry.MULTI_SZ:
		values, _, err := key.GetStringsValue(name)
		if err != nil {
			return snap, err
		}
		snap.Kind = KindMultiString
		snap.Multi = values
	case registry.DWORD, registry.QWORD:
		number, _, err := key.GetIntegerValue(name)
		if err != nil {
			return snap, err
		}
		snap.Kind = KindDWord
		if valueType == registry.QWORD {
			snap.Kind = KindQWord
		}
		snap.Data = strconv.FormatUint(number, 10)
	case registry.BINARY:
		data, _, err := key.GetBinaryValue(name)
		if err != nil {
			return snap, err
		}
		snap.Kind = KindBinary
		snap.Data = base64.StdEncoding.EncodeToString(data)
	default:
		// Tipos que el instalador no sabe restaurar: se comparan como
		// desconocidos y nunca se sobrescriben.
		snap.Kind = KindUnknown
	}
	return snap, nil
}

func (windowsRegistry) Write(path, name string, value ValueSnapshot) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	switch value.Kind {
	case KindString:
		return key.SetStringValue(name, value.Data)
	case KindExpandString:
		return key.SetExpandStringValue(name, value.Data)
	case KindMultiString:
		return key.SetStringsValue(name, value.Multi)
	case KindDWord:
		number, err := strconv.ParseUint(value.Data, 10, 32)
		if err != nil {
			return err
		}
		return key.SetDWordValue(name, uint32(number))
	case KindQWord:
		number, err := strconv.ParseUint(value.Data, 10, 64)
		if err != nil {
			return err
		}
		return key.SetQWordValue(name, number)
	case KindBinary:
		data, err := base64.StdEncoding.DecodeString(value.Data)
		if err != nil {
			return err
		}
		return key.SetBinaryValue(name, data)
	default:
		return fmt.Errorf("tipo de valor no restaurable: %q", value.Kind)
	}
}

func (windowsRegistry) DeleteValue(path, name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func (windowsRegistry) Inspect(path string) (bool, int, []string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return false, 0, nil, nil
	}
	if err != nil {
		return false, 0, nil, err
	}
	defer key.Close()
	info, err := key.Stat()
	if err != nil {
		return true, 0, nil, err
	}
	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return true, 0, nil, err
	}
	return true, int(info.ValueCount), subkeys, nil
}

func (windowsRegistry) DeleteKey(path string) error {
	err := registry.DeleteKey(registry.CURRENT_USER, path)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
