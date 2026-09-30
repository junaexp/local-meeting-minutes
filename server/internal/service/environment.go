package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"meet-to-md/server/internal/config"
)

type BinaryEnvironment struct {
	Path    string `json:"path"`
	Ready   bool   `json:"ready"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type WhisperEnvironment struct {
	Model            string `json:"model"`
	ModelPath        string `json:"modelPath"`
	ModelReady       bool   `json:"modelReady"`
	BinaryPath       string `json:"binaryPath"`
	BinaryReady      bool   `json:"binaryReady"`
	InstallerVersion string `json:"installerVersion"`
}

type Environment struct {
	Codex   BinaryEnvironment  `json:"codex"`
	Whisper WhisperEnvironment `json:"whisper"`
	FFmpeg  BinaryEnvironment  `json:"ffmpeg"`
}

// Environment reports the currently saved configuration, not unsaved UI edits.
func (s *Service) Environment() Environment {
	s.refreshReadiness()
	s.mu.Lock()
	s.broadcastLocked()
	s.mu.Unlock()
	s.mu.RLock()
	cfg := s.cfg
	root := s.root
	s.mu.RUnlock()

	info := Environment{}
	if path, err := config.ResolveBinary(cfg.CodexBinary); err == nil {
		info.Codex = BinaryEnvironment{Path: path, Ready: true}
	} else {
		info.Codex.Error = err.Error()
	}

	info.Whisper.Model = cfg.WhisperModel
	info.Whisper.ModelPath = filepath.Join(root, "whisper", "models", "ggml-"+cfg.WhisperModel+".bin")
	info.Whisper.InstallerVersion = whisperInstallerVersion()
	if file, err := os.Stat(info.Whisper.ModelPath); err == nil && !file.IsDir() && file.Size() >= 1000000 {
		info.Whisper.ModelReady = true
	}
	if path := s.whisperBinary(); path != "" {
		if file, err := os.Stat(path); err == nil && !file.IsDir() {
			info.Whisper.BinaryPath = path
			info.Whisper.BinaryReady = true
		}
	}

	path, err := exec.LookPath(cfg.FFmpegBinary)
	if err != nil {
		info.FFmpeg.Error = "ffmpeg 실행 파일을 찾을 수 없습니다."
		return info
	}
	path, err = filepath.Abs(path)
	if err != nil {
		info.FFmpeg.Error = "ffmpeg 실행 파일 경로를 확인할 수 없습니다."
		return info
	}
	info.FFmpeg.Path = path
	info.FFmpeg.Ready = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-version").Output()
	if err != nil {
		info.FFmpeg.Error = "ffmpeg 버전을 확인할 수 없습니다."
		return info
	}
	info.FFmpeg.Version = ffmpegVersion(string(out))
	if info.FFmpeg.Version == "" {
		info.FFmpeg.Error = "ffmpeg 버전 정보를 읽을 수 없습니다."
	}
	return info
}

func ffmpegVersion(output string) string {
	firstLine := strings.SplitN(output, "\n", 2)[0]
	fields := strings.Fields(firstLine)
	if len(fields) >= 3 && fields[0] == "ffmpeg" && fields[1] == "version" {
		return fields[2]
	}
	return ""
}
