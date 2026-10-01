// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package machinepolicy

import (
	"errors"
	"math"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const accesoLectura = registry.QUERY_VALUE | registry.WOW64_64KEY

func native() bool { return true }

func abrir() (registry.Key, bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, RegistryKey, accesoLectura)
	if errors.Is(err, registry.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return k, true, nil
}

func readStrings(name string) ([]string, bool, error) {
	k, ok, err := abrir()
	if !ok || err != nil {
		return nil, false, err
	}
	defer k.Close()
	values, _, err := k.GetStringsValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, false, nil
	}
	if errors.Is(err, registry.ErrUnexpectedType) {
		// Se admite también REG_SZ con elementos separados por ';'.
		single, _, errSingle := k.GetStringValue(name)
		if errSingle != nil {
			return nil, false, errSingle
		}
		values = strings.Split(single, ";")
		err = nil
	}
	if err != nil {
		return nil, false, err
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out, true, nil
}

func readString(name string) (string, bool, error) {
	k, ok, err := abrir()
	if !ok || err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func readInt64(name string) (int64, bool, error) {
	k, ok, err := abrir()
	if !ok || err != nil {
		return 0, false, err
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if v > math.MaxInt64 {
		return 0, false, errors.New("valor de política fuera de rango")
	}
	return int64(v), true, nil // #nosec G115 -- comprobado contra math.MaxInt64.
}

func readOptIn(name string) (bool, bool, error) {
	n, present, err := readInt64(name)
	return n != 0, present, err
}
