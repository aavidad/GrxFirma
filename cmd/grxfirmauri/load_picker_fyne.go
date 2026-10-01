// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	legacyLoadAppleScriptSeparator = "\x00"
	legacyLoadGTKSeparator         = "\x1f"
)

func selectLegacyLoadPaths(ctx context.Context, initialPath string, exts string, multi bool) ([]string, error) {
	initialPath = strings.TrimSpace(initialPath)
	if nativeProtocolUIEnabled() {
		return selectNativeWindowsLoadPaths(
			ctx,
			initialPath,
			exts,
			multi,
		)
	}
	switch runtime.GOOS {
	case "windows":
		// Nunca se construye un script de PowerShell con datos del portal:
		// PowerShell también cierra cadenas con comillas tipográficas
		// (U+2018..U+201B), lo que permitía inyectar órdenes desde `exts`.
		return selectNativeWindowsLoadPaths(ctx, initialPath, exts, multi)
	case "darwin":
		var script string
		if multi {
			script = "set xs to choose file with prompt \"" + osaQuote(tl("Seleccionar fichero")) + "\" with multiple selections allowed\nset separator to ASCII character 0\nset out to \"\"\nrepeat with f in xs\nset p to POSIX path of f\nif out is \"\" then\nset out to p\nelse\nset out to out & separator & p\nend if\nend repeat\nreturn out"
		} else {
			script = "set p to POSIX path of (choose file with prompt \"" + osaQuote(tl("Seleccionar fichero")) + "\")"
		}
		cmd := exec.CommandContext(ctx, "osascript", "-e", script)
		configureGUICommand(cmd)
		out, err := cmd.Output()
		if err != nil {
			if ctx.Err() != nil || isAppleScriptCancelErr(err) {
				return nil, errLegacyLoadCanceled
			}
			return nil, err
		}
		return legacyLoadPathsFromCommandOutput(out, multi, legacyLoadAppleScriptSeparator)
	default:
		args := legacyLoadGTKArgs(exts, multi)
		for _, executable := range []string{"zenity", "qarma"} {
			cmd := exec.CommandContext(ctx, executable, args...)
			configureGUICommand(cmd)
			out, err := cmd.Output()
			if err == nil {
				return legacyLoadPathsFromCommandOutput(out, multi, legacyLoadGTKSeparator)
			}
			if ctx.Err() != nil || isLinuxDialogCancelErr(err) {
				return nil, errLegacyLoadCanceled
			}
		}

		kargs := []string{"--getopenfilename", effectiveLegacyLoadInitialPath(initialPath)}
		if multi {
			kargs = append(kargs, "--multiple", "--separate-output")
		}
		if filter := legacyLoadKDialogFilter(exts); filter != "" {
			kargs = append(kargs, filter)
		}
		cmd := exec.CommandContext(ctx, "kdialog", kargs...)
		configureGUICommand(cmd)
		out, err := cmd.Output()
		if err != nil {
			if ctx.Err() != nil || isLinuxDialogCancelErr(err) {
				return nil, errLegacyLoadCanceled
			}
			return nil, err
		}
		return legacyLoadPathsFromCommandOutput(out, multi, "\n")
	}
}

func legacyLoadGTKArgs(exts string, multi bool) []string {
	args := []string{"--file-selection", "--title=" + tl("Seleccionar fichero")}
	if multi {
		args = append(args, "--multiple", "--separator="+legacyLoadGTKSeparator)
	}
	if filter := legacyLoadZenityFilter(exts); filter != "" {
		args = append(args, "--file-filter="+filter)
	}
	return args
}

func legacyLoadPathsFromCommandOutput(out []byte, multi bool, separator string) ([]string, error) {
	value := legacyDialogOutputValue(out)
	if value == "" {
		return nil, errLegacyLoadCanceled
	}
	if !multi {
		return []string{value}, nil
	}
	if separator == "" {
		return nil, errors.New("separador de multiselección vacío")
	}
	parts := strings.Split(value, separator)
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			items = append(items, part)
		}
	}
	if len(items) == 0 {
		return nil, errLegacyLoadCanceled
	}
	return items, nil
}

func legacyDialogOutputValue(out []byte) string {
	value := string(out)
	value = strings.TrimSuffix(value, "\n")
	value = strings.TrimSuffix(value, "\r")
	return value
}

func effectiveLegacyLoadInitialPath(initialPath string) string {
	if strings.TrimSpace(initialPath) != "" {
		return initialPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, candidate := range []string{filepath.Join(home, "Descargas"), filepath.Join(home, "Downloads"), home} {
		return candidate
	}
	return ""
}

func legacyLoadZenityFilter(exts string) string {
	patterns := legacyLoadPatterns(exts)
	if len(patterns) == 0 {
		return ""
	}
	return tl("Ficheros permitidos") + " | " + strings.Join(patterns, " ")
}

func legacyLoadKDialogFilter(exts string) string {
	patterns := legacyLoadPatterns(exts)
	if len(patterns) == 0 {
		return ""
	}
	return strings.Join(patterns, " ")
}

func legacyLoadPatterns(exts string) []string {
	parts := strings.Split(strings.TrimSpace(exts), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(part, "."))
		if part == "" {
			continue
		}
		out = append(out, "*."+part)
	}
	return out
}

func isLinuxDialogCancelErr(err error) bool {
	return dialogExitCode(err) == 1
}

func isAppleScriptCancelErr(err error) bool {
	if dialogExitCode(err) != 1 {
		return false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	stderr := strings.ToLower(string(exitErr.Stderr))
	return strings.Contains(stderr, "(-128)") ||
		strings.Contains(stderr, "number -128")
}

func dialogExitCode(err error) int {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1
	}
	return exitErr.ExitCode()
}

func osaQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\"", "\\\"")
}
