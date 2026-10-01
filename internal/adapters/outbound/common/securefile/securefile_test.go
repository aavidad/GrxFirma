// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile_test

import (
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

func TestReadFileRegular(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := securefile.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("ReadFile = %q", got)
	}
}

func TestReadFileRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(target, []byte(`{"secret":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := securefile.ReadFile(link); err == nil {
		t.Fatal("ReadFile accepted a final-component symlink")
	}
}

func TestReadFileRejectsDirectory(t *testing.T) {
	if _, err := securefile.ReadFile(t.TempDir()); err == nil {
		t.Fatal("ReadFile accepted a directory")
	}
}

func TestOpenAppendCreatesPrivateRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	file, err := securefile.OpenAppend(path, 0o600)
	if err != nil {
		t.Fatalf("OpenAppend: %v", err)
	}
	if _, err := file.Write([]byte("first\n")); err != nil {
		t.Fatalf("Write(first): %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(first): %v", err)
	}
	file, err = securefile.OpenAppend(path, 0o600)
	if err != nil {
		t.Fatalf("OpenAppend(second): %v", err)
	}
	if _, err := file.Write([]byte("second\n")); err != nil {
		t.Fatalf("Write(second): %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(second): %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Fatalf("append content = %q", data)
	}
}

func TestOpenAppendRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	link := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(target, []byte("untouched\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := securefile.OpenAppend(link, 0o600); err == nil {
		t.Fatal("OpenAppend accepted a final-component symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target): %v", err)
	}
	if string(data) != "untouched\n" {
		t.Fatalf("symlink target changed to %q", data)
	}
}

func TestReadFilePreservesNotExistClassification(t *testing.T) {
	_, err := securefile.ReadFile(filepath.Join(t.TempDir(), "missing.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("ReadFile error = %v; os.IsNotExist must remain true", err)
	}
}

func TestReadFileLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.bin")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := securefile.ReadFileLimit(path, 4); err == nil {
		t.Fatal("ReadFileLimit accepted an oversized file")
	}
	got, err := securefile.ReadFileLimit(path, 5)
	if err != nil {
		t.Fatalf("ReadFileLimit: %v", err)
	}
	if string(got) != "12345" {
		t.Fatalf("ReadFileLimit = %q", got)
	}
}

func TestWriteFileAtomicReplacesRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("old-long-content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := securefile.WriteFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteFileAtomicRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := securefile.WriteFileAtomic(link, []byte("overwrite"), 0o600); err == nil {
		t.Fatal("WriteFileAtomic accepted a final-component symlink")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(got) != "untouched" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestOpenDirRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked-dir")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := securefile.OpenDir(link); err == nil {
		t.Fatal("OpenDir accepted a final-component symlink")
	}
}

func TestRemoveFileRemovesRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.bin")
	if err := os.WriteFile(path, []byte("ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := securefile.RemoveFile(path); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("Lstat after RemoveFile = %v; want not exist", err)
	}
}

func TestRemoveFileRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.bin")
	link := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := securefile.RemoveFile(link); err == nil {
		t.Fatal("RemoveFile accepted a final-component symlink")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(got) != "untouched" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestRemoveFilePreservesNotExistClassification(t *testing.T) {
	err := securefile.RemoveFile(filepath.Join(t.TempDir(), "missing.bin"))
	if !os.IsNotExist(err) {
		t.Fatalf("RemoveFile error = %v; os.IsNotExist must remain true", err)
	}
}
