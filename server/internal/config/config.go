package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen       string `toml:"listen" json:"listen"`
	CodexBinary  string `toml:"codex_binary" json:"codexBinary"`
	OutputDir    string `toml:"output_dir" json:"outputDir"`
	WhisperModel string `toml:"whisper_model" json:"whisperModel"`
	FFmpegBinary string `toml:"ffmpeg_binary" json:"ffmpegBinary"`
	Prompt       string `toml:"prompt" json:"prompt"`
}

func Default() Config {
	return Config{Listen: "127.0.0.1:8791", CodexBinary: "codex", WhisperModel: "large-v3-turbo", FFmpegBinary: "ffmpeg"}
}

func Load(path string) (Config, error) {
	c := Default()
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if !strings.HasPrefix(c.Listen, "127.0.0.1:") && !strings.HasPrefix(c.Listen, "localhost:") && !strings.HasPrefix(c.Listen, "[::1]:") {
		return errors.New("listen must bind to localhost")
	}
	if strings.TrimSpace(c.Prompt) == "" {
		return errors.New("prompt must not be empty")
	}
	if c.WhisperModel != "base" && c.WhisperModel != "small" && c.WhisperModel != "large-v3-turbo" {
		return errors.New("whisper_model must be base, small, or large-v3-turbo")
	}
	if c.OutputDir != "" {
		if !filepath.IsAbs(c.OutputDir) {
			return errors.New("output_dir must be an absolute path")
		}
		info, err := os.Stat(c.OutputDir)
		if err != nil || !info.IsDir() {
			return errors.New("output_dir must be an existing directory")
		}
	}
	return nil
}

func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	return atomicWrite(path, buf.Bytes(), 0600)
}

func atomicWrite(path string, contents []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(contents); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func ResolveBinary(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "codex"
	}
	if path, err := exec.LookPath(value); err == nil {
		return filepath.Abs(path)
	}
	if strings.ContainsAny(value, `/\`) {
		return "", fmt.Errorf("Codex 실행 파일을 찾을 수 없습니다: %s", value)
	}
	home, _ := os.UserHomeDir()
	candidates := []string{}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/Applications/Codex.app/Contents/Resources/codex", filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"))
	}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("PROGRAMFILES")} {
			if base != "" {
				candidates = append(candidates, filepath.Join(base, "Programs", "Codex", "codex.exe"), filepath.Join(base, "Codex", "codex.exe"))
			}
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("codex 명령을 찾을 수 없습니다. 설정에서 CLI 경로를 지정하세요")
}
