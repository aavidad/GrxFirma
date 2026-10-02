// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"grxfirma/internal/adapters/outbound/common/logging"
)

const (
	officialChromiumExtensionID   = "pkefjandjcgdmhoonmhnllikibobijgg"
	officialFirefoxExtensionID    = "extension@dipgra.es"
	portafirmasFirefoxExtensionID = "portafirmas@dipgra.es"
	maxNativeManifestBytes        = 64 * 1024
)

var (
	chromiumExtensionIDPattern = regexp.MustCompile(`^[a-p]{32}$`)
	firefoxExtensionIDPattern  = regexp.MustCompile(`^[A-Za-z0-9._@{}-]{1,128}$`)
	parentWindowArgPattern     = regexp.MustCompile(`^--parent-window=[0-9]+$`)
	nativeHostManifestNames    = map[string]struct{}{
		"com.grxfirma.native":    {},
		"com.dipgra.grxfirma":    {},
		"com.dipgra.portafirmas": {},
	}
	builtinChromiumExtensionIDs = []string{
		officialChromiumExtensionID,
		"ipkpimgjhkjibkbhfdhggjldlaetbcoa",
		"knldjmfmopnpolahpmmgbagdohdnhkik",
	}
)

type nativeCallerPolicy struct {
	chromiumIDs      map[string]struct{}
	firefoxIDs       map[string]struct{}
	allowDevelopment bool
	executable       string
}

type nativeHostManifest struct {
	Name              string   `json:"name"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedOrigins    []string `json:"allowed_origins"`
	AllowedExtensions []string `json:"allowed_extensions"`
}

func newNativeCallerPolicy(executable, home string, allowDevelopment bool) nativeCallerPolicy {
	policy := nativeCallerPolicy{
		chromiumIDs:      make(map[string]struct{}),
		firefoxIDs:       make(map[string]struct{}),
		allowDevelopment: allowDevelopment,
		executable:       cleanAbsolutePath(executable),
	}
	for _, id := range builtinChromiumExtensionIDs {
		policy.addChromiumID(id)
	}
	policy.addFirefoxID(officialFirefoxExtensionID)
	policy.addFirefoxID(portafirmasFirefoxExtensionID)

	for _, path := range installedChromiumIDCandidates(policy.executable) {
		if id, ok := readSingleChromiumID(path); ok {
			policy.addChromiumID(id)
		}
	}
	for _, path := range nativeManifestCandidates(policy.executable, home) {
		manifest, ok := readNativeHostManifest(path)
		if !ok || !manifestTargetsExecutable(manifest.Path, policy.executable) {
			continue
		}
		for _, origin := range manifest.AllowedOrigins {
			if id, ok := chromiumIDFromOrigin(origin); ok {
				policy.addChromiumID(id)
			}
		}
		for _, id := range manifest.AllowedExtensions {
			policy.addFirefoxID(id)
		}
	}
	return policy
}

func (p *nativeCallerPolicy) addChromiumID(id string) {
	id = strings.TrimSpace(id)
	if p != nil && chromiumExtensionIDPattern.MatchString(id) {
		p.chromiumIDs[id] = struct{}{}
	}
}

func (p *nativeCallerPolicy) addFirefoxID(id string) {
	id = strings.TrimSpace(id)
	if p != nil && firefoxExtensionIDPattern.MatchString(id) {
		p.firefoxIDs[id] = struct{}{}
	}
}

// nativeCallerFromArgs obtiene la identidad que el navegador añade al proceso.
// No usa ningún campo del mensaje JSON, que está bajo control de la página.
func nativeCallerFromArgs(args []string, policy nativeCallerPolicy) (string, error) {
	if len(args) == 0 {
		return "", errors.New("el navegador no proporcionó la identidad de la extensión")
	}
	if caller, ok := chromiumCaller(args, policy); ok {
		return caller, nil
	}
	if caller, ok := firefoxCaller(args, policy); ok {
		return caller, nil
	}
	return "", errors.New("la identidad de la extensión no está autorizada")
}

func chromiumCaller(args []string, policy nativeCallerPolicy) (string, bool) {
	if len(args) == 0 || len(args) > 2 {
		return "", false
	}
	if len(args) == 2 &&
		(runtime.GOOS != "windows" || !parentWindowArgPattern.MatchString(args[1])) {
		return "", false
	}
	id, ok := chromiumIDFromOrigin(args[0])
	if !ok {
		return "", false
	}
	if _, allowed := policy.chromiumIDs[id]; !allowed && !policy.allowDevelopment {
		return "", false
	}
	return "chrome-extension://" + id + "/", true
}

func chromiumIDFromOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil ||
		parsed.Scheme != "chrome-extension" ||
		parsed.User != nil ||
		parsed.Port() != "" ||
		(parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" ||
		!chromiumExtensionIDPattern.MatchString(parsed.Host) {
		return "", false
	}
	return parsed.Host, true
}

func firefoxCaller(args []string, policy nativeCallerPolicy) (string, bool) {
	if len(args) != 2 {
		return "", false
	}
	manifestPath := cleanAbsolutePath(args[0])
	extensionID := strings.TrimSpace(args[1])
	if manifestPath == "" ||
		!strings.EqualFold(filepath.Ext(manifestPath), ".json") ||
		!firefoxExtensionIDPattern.MatchString(extensionID) {
		return "", false
	}
	manifest, ok := readNativeHostManifest(manifestPath)
	if !ok ||
		!manifestTargetsExecutable(manifest.Path, policy.executable) ||
		!containsExact(manifest.AllowedExtensions, extensionID) {
		return "", false
	}
	if _, allowed := policy.firefoxIDs[extensionID]; !allowed && !policy.allowDevelopment {
		return "", false
	}
	return "firefox-extension-id:" + extensionID, true
}

func nativeCallerDevelopmentAllowed() bool {
	return envBool("GRXFIRMA_NATIVEHOST_ALLOW_DEVELOPMENT_CALLER", false) &&
		logging.DebugAllowed()
}

func installedChromiumIDCandidates(executable string) []string {
	if executable == "" {
		return nil
	}
	dir := filepath.Dir(executable)
	return uniqueCleanPaths([]string{
		filepath.Join(dir, "extensions", "dipgra-extension-chromium.id"),
		filepath.Join(dir, "extensions", "chromium", "dipgra-extension-chromium.id"),
		filepath.Join(dir, "..", "extensions", "dipgra-extension-chromium.id"),
		filepath.Join(dir, "..", "extensions", "chromium", "dipgra-extension-chromium.id"),
		filepath.Join(dir, "..", "Extensions", "chromium", "dipgra-extension-chromium.id"),
	})
}

func nativeManifestCandidates(executable, home string) []string {
	var candidates []string
	names := []string{
		"com.grxfirma.native.json",
		"com.dipgra.grxfirma.json",
		"com.dipgra.portafirmas.json",
	}
	addDir := func(dir string) {
		if strings.TrimSpace(dir) == "" {
			return
		}
		for _, name := range names {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}

	if executable != "" {
		dir := filepath.Dir(executable)
		addDir(filepath.Join(dir, "manifests"))
	}
	configRoot := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configRoot == "" && strings.TrimSpace(home) != "" {
		configRoot = filepath.Join(home, ".config")
	}
	for _, relative := range []string{
		"google-chrome/NativeMessagingHosts",
		"google-chrome-for-testing/NativeMessagingHosts",
		"chromium/NativeMessagingHosts",
		"microsoft-edge/NativeMessagingHosts",
		"BraveSoftware/Brave-Browser/NativeMessagingHosts",
		"vivaldi/NativeMessagingHosts",
		"vivaldi-snapshot/NativeMessagingHosts",
		"opera/NativeMessagingHosts",
	} {
		addDir(filepath.Join(configRoot, filepath.FromSlash(relative)))
	}
	if home != "" {
		addDir(filepath.Join(home, ".mozilla", "native-messaging-hosts"))
		addDir(filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts"))
		for _, relative := range []string{
			"Google/Chrome/NativeMessagingHosts",
			"Chromium/NativeMessagingHosts",
			"Microsoft Edge/NativeMessagingHosts",
			"BraveSoftware/Brave-Browser/NativeMessagingHosts",
		} {
			addDir(filepath.Join(home, "Library", "Application Support", filepath.FromSlash(relative)))
		}
	}
	return uniqueCleanPaths(candidates)
}

func readSingleChromiumID(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > 256 {
		return "", false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(raw))
	return id, chromiumExtensionIDPattern.MatchString(id)
}

func readNativeHostManifest(path string) (nativeHostManifest, bool) {
	if cleanAbsolutePath(path) == "" {
		return nativeHostManifest{}, false
	}
	info, err := os.Lstat(path) // #nosec G703 -- only an absolute path is inspected; its opened file identity is checked below.
	if err != nil || !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > maxNativeManifestBytes {
		return nativeHostManifest{}, false
	}
	file, err := os.Open(path) // #nosec G703 -- opened file must match the previously inspected regular file.
	if err != nil {
		return nativeHostManifest{}, false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nativeHostManifest{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxNativeManifestBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxNativeManifestBytes {
		return nativeHostManifest{}, false
	}
	var manifest nativeHostManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nativeHostManifest{}, false
	}
	if _, ok := nativeHostManifestNames[strings.TrimSpace(manifest.Name)]; !ok ||
		manifest.Type != "stdio" ||
		cleanAbsolutePath(manifest.Path) == "" {
		return nativeHostManifest{}, false
	}
	return manifest, true
}

func manifestTargetsExecutable(target, executable string) bool {
	target = cleanAbsolutePath(target)
	executable = cleanAbsolutePath(executable)
	if target == "" || executable == "" {
		return false
	}
	if sameResolvedPath(target, executable) {
		return true
	}
	if !strings.EqualFold(filepath.Base(target), "browser-bridge.sh") {
		return false
	}
	return sameResolvedPath(
		filepath.Join(filepath.Dir(target), "grxfirma-nativehost"),
		executable,
	)
}

func sameResolvedPath(a, b string) bool {
	a = cleanAbsolutePath(a)
	b = cleanAbsolutePath(b)
	if a == "" || b == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(a); err == nil {
		a = filepath.Clean(resolved)
	}
	if resolved, err := filepath.EvalSymlinks(b); err == nil {
		b = filepath.Clean(resolved)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func cleanAbsolutePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func uniqueCleanPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}
