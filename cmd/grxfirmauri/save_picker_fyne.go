// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func selectLegacySaveTargetPath(ctx context.Context, defaultPath string, exts string) (string, error) {
	defaultPath = effectiveLegacySaveDefaultPath(defaultPath)
	if nativeProtocolUIEnabled() {
		return selectNativeWindowsSaveTarget(
			ctx,
			defaultPath,
			exts,
		)
	}
	switch runtime.GOOS {
	case "windows":
		// Diálogo Win32 nativo: el nombre sugerido por el portal no se
		// interpreta nunca como código (antes se incrustaba en PowerShell).
		return selectNativeWindowsSaveTarget(ctx, defaultPath, exts)
	case "darwin":
		cmd := exec.CommandContext(ctx, "osascript", "-e", legacySaveAppleScript(defaultPath))
		configureGUICommand(cmd)
		return legacySavePathFromCommand(ctx, cmd, isAppleScriptCancelErr)
	default:
		args := legacySaveZenityArgs(defaultPath, exts)
		for _, executable := range []string{"zenity", "qarma"} {
			cmd := exec.CommandContext(ctx, executable, args...)
			configureGUICommand(cmd)
			path, err := legacySavePathFromCommand(ctx, cmd, isLinuxDialogCancelErr)
			if err == nil || isLegacySaveCancellation(err) {
				return path, err
			}
		}

		cmd := exec.CommandContext(ctx, "kdialog", legacySaveKDialogArgs(defaultPath, exts)...)
		configureGUICommand(cmd)
		return legacySavePathFromCommand(ctx, cmd, isLinuxDialogCancelErr)
	}
}

func legacySavePathFromCommand(ctx context.Context, cmd *exec.Cmd, isCancel func(error) bool) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil || isCancel != nil && isCancel(err) {
			return "", errLegacySaveCanceled
		}
		return "", err
	}
	path := legacyDialogOutputValue(out)
	if path == "" {
		return "", errLegacySaveCanceled
	}
	return path, nil
}

func effectiveLegacySaveDefaultPath(defaultPath string) string {
	defaultPath = strings.TrimSpace(defaultPath)
	name := strings.TrimSpace(filepath.Base(defaultPath))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "grxfirma_guardado"
	}

	dir := strings.TrimSpace(filepath.Dir(defaultPath))
	for dir != "" && dir != "." {
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			return filepath.Join(dir, name)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, name)
	}
	return name
}

func legacySaveAppleScript(defaultPath string) string {
	return "set p to choose file name with prompt \"" + osaQuote(tl("Guardar firma")) +
		"\" default name \"" + osaQuote(filepath.Base(defaultPath)) +
		"\" default location POSIX file \"" + osaQuote(filepath.Dir(defaultPath)+string(filepath.Separator)) + "\"\n" +
		"return POSIX path of p"
}

func legacySaveZenityArgs(defaultPath, exts string) []string {
	args := []string{
		"--file-selection",
		"--save",
		"--confirm-overwrite",
		"--title=" + tl("Guardar firma"),
		"--filename=" + defaultPath,
	}
	if filter := legacyLoadZenityFilter(exts); filter != "" {
		args = append(args, "--file-filter="+filter)
	}
	return args
}

func legacySaveKDialogArgs(defaultPath, exts string) []string {
	args := []string{"--getsavefilename", defaultPath}
	if filter := legacyLoadKDialogFilter(exts); filter != "" {
		args = append(args, filter)
	}
	return args
}
