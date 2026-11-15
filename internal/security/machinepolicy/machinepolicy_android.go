// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2

//go:build android

package machinepolicy

// Android no dispone de HKLM ni de /etc/grxfirma/policy.json como fuente
// administrada. La ausencia se representa aquí, sin recorrer /etc.
func native() bool                               { return false }
func readStrings(string) ([]string, bool, error) { return nil, false, nil }
func readString(string) (string, bool, error)    { return "", false, nil }
func readInt64(string) (int64, bool, error)      { return 0, false, nil }
func readOptIn(string) (bool, bool, error)       { return false, false, nil }
