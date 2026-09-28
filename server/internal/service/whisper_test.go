package service

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func makeArchive(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("binary")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestExtractZipRejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	if err := extractZip(makeArchive(t, "../outside"), dest, false); err == nil {
		t.Fatal("zip traversal accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "outside")); !os.IsNotExist(err) {
		t.Fatal("archive wrote outside destination")
	}
}
func TestExtractZipPreservesBinaryDirectory(t *testing.T) {
	dest := t.TempDir()
	if err := extractZip(makeArchive(t, "bundle/bin/whisper-cli.exe"), dest, false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "bundle", "bin", "whisper-cli.exe")); err != nil || string(b) != "binary" {
		t.Fatalf("extracted file: %s %v", b, err)
	}
}
