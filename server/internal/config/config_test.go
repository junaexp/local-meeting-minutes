package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSaveAndValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	c := Default()
	c.Prompt = "한국어 회의록\n원문만 사용"
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Prompt != c.Prompt || loaded.OutputDir != "" || loaded.Listen != "127.0.0.1:8791" {
		t.Fatalf("unexpected config: %#v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("config permissions too broad: %v", info.Mode())
	}
	c.Listen = "0.0.0.0:8791"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("expected local-only error, got %v", err)
	}
	c.Listen = "127.0.0.1:8791"
	c.OutputDir = "relative"
	if err := c.Validate(); err == nil {
		t.Fatal("relative output dir accepted")
	}
}

func TestResolveBinary(t *testing.T) {
	path, err := ResolveBinary("go")
	if err != nil || !filepath.IsAbs(path) {
		t.Fatalf("go binary resolution: %q %v", path, err)
	}
	if _, err := ResolveBinary(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing binary accepted")
	}
}
