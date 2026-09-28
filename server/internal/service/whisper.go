package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const whisperVersion = "v1.8.3"

func whisperInstallerVersion() string {
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		return "b5130"
	}
	return whisperVersion
}

func (s *Service) whisperBinary() string {
	base := filepath.Join(s.root, "whisper")
	if runtime.GOOS == "windows" {
		var found string
		_ = filepath.WalkDir(filepath.Join(base, "bin"), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.EqualFold(d.Name(), "whisper-cli.exe") {
				found = path
				return filepath.SkipAll
			}
			return nil
		})
		return found
	}
	return filepath.Join(base, "build", "bin", "whisper-cli")
}
func (s *Service) whisperReadyLocked() bool {
	if info, err := os.Stat(filepath.Join(s.root, "whisper", "models", "ggml-"+s.cfg.WhisperModel+".bin")); err != nil || info.Size() < 1000000 {
		return false
	}
	bin := s.whisperBinary()
	if bin == "" {
		return false
	}
	info, err := os.Stat(bin)
	return err == nil && !info.IsDir()
}
func (s *Service) StartWhisperInstall(ctx context.Context) error {
	s.mu.Lock()
	if s.whisperInstalling {
		s.mu.Unlock()
		return errors.New("이미 설치 중입니다")
	}
	s.whisperInstalling = true
	s.whisperError = ""
	s.broadcastLocked()
	s.mu.Unlock()
	go func() {
		err := s.installWhisper(ctx)
		s.mu.Lock()
		s.whisperInstalling = false
		if err != nil {
			s.whisperError = err.Error()
		}
		s.broadcastLocked()
		s.mu.Unlock()
	}()
	return nil
}
func (s *Service) installWhisper(ctx context.Context) error {
	s.mu.RLock()
	model := s.cfg.WhisperModel
	s.mu.RUnlock()
	base := filepath.Join(s.root, "whisper")
	modelPath := filepath.Join(base, "models", "ggml-"+model+".bin")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0700); err != nil {
		return err
	}
	if info, err := os.Stat(modelPath); err != nil || info.Size() < 1000000 {
		url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-" + model + ".bin"
		maxBytes := int64(600 * 1024 * 1024)
		if model == "large-v3-turbo" {
			maxBytes = 2 * 1024 * 1024 * 1024
		}
		if err := download(ctx, url, modelPath, maxBytes); err != nil {
			return fmt.Errorf("Whisper 모델 다운로드: %w", err)
		}
	}
	if bin := s.whisperBinary(); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return nil
		}
	}
	if runtime.GOOS == "windows" {
		return installWindowsWhisper(ctx, base)
	}
	if runtime.GOOS == "darwin" {
		return installMacWhisper(ctx, base)
	}
	return errors.New("Whisper 자동 설치는 macOS 및 Windows에서 지원합니다")
}

func download(ctx context.Context, url, target string, maxBytes int64) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return errors.New("다운로드 크기 제한 초과")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		f.Close()
		return err
	}
	if n > maxBytes {
		f.Close()
		return errors.New("다운로드 크기 제한 초과")
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), target)
}
func extractZip(archive, dest string, stripRoot bool) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	const maxEntrySize = 200 * 1024 * 1024
	const maxExtractedSize = 1024 * 1024 * 1024
	var extracted int64
	for _, item := range r.File {
		name := filepath.FromSlash(item.Name)
		if stripRoot {
			parts := strings.SplitN(name, string(filepath.Separator), 2)
			if len(parts) != 2 {
				continue
			}
			name = parts[1]
		}
		if name == "" {
			continue
		}
		clean := filepath.Clean(name)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("압축 파일에 잘못된 경로가 있습니다")
		}
		path := filepath.Join(dest, clean)
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
			continue
		}
		if item.UncompressedSize64 > maxEntrySize || int64(item.UncompressedSize64) > maxExtractedSize-extracted {
			return errors.New("압축 파일의 압축 해제 크기 제한을 초과했습니다")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		in, err := item.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
		if err != nil {
			in.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, maxEntrySize+1))
		closeErr := out.Close()
		in.Close()
		if copyErr != nil {
			return copyErr
		}
		if n > maxEntrySize || n > maxExtractedSize-extracted {
			return errors.New("압축 파일의 압축 해제 크기 제한을 초과했습니다")
		}
		extracted += n
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func installWindowsWhisper(ctx context.Context, base string) error {
	asset := "whisper-bin-x64.zip"
	tag := whisperInstallerVersion()
	if runtime.GOARCH == "arm64" {
		asset = "whisper-bin-win-cpu-arm64.zip"
	} else if runtime.GOARCH != "amd64" {
		return errors.New("이 Windows CPU 아키텍처는 지원하지 않습니다")
	}
	zipPath := filepath.Join(base, "whisper-windows.zip")
	defer os.Remove(zipPath)
	if err := download(ctx, "https://github.com/ggml-org/whisper.cpp/releases/download/"+tag+"/"+asset, zipPath, 200*1024*1024); err != nil {
		return err
	}
	if err := extractZip(zipPath, filepath.Join(base, "bin"), false); err != nil {
		return err
	}
	found := false
	_ = filepath.WalkDir(filepath.Join(base, "bin"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(d.Name(), "whisper-cli.exe") {
			found = true
		}
		return nil
	})
	if !found {
		return errors.New("Whisper 압축 파일에 whisper-cli.exe가 없습니다")
	}
	return nil
}
func installMacWhisper(ctx context.Context, base string) error {
	cmake, err := exec.LookPath("cmake")
	if err != nil {
		return errors.New("macOS에서는 Whisper 빌드에 CMake가 필요합니다. CMake 설치 후 다시 시도하세요")
	}
	archive := filepath.Join(base, "whisper-source.zip")
	defer os.Remove(archive)
	if err := download(ctx, "https://github.com/ggml-org/whisper.cpp/archive/refs/tags/"+whisperVersion+".zip", archive, 100*1024*1024); err != nil {
		return err
	}
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		return err
	}
	if err := extractZip(archive, source, true); err != nil {
		return err
	}
	build := filepath.Join(base, "build")
	for _, args := range [][]string{{"-S", source, "-B", build, "-DCMAKE_BUILD_TYPE=Release"}, {"--build", build, "--config", "Release", "-j", "4"}} {
		cmd := exec.CommandContext(ctx, cmake, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Whisper 빌드 실패: %s", tail(string(out), 2000))
		}
	}
	if _, err := os.Stat(filepath.Join(build, "bin", "whisper-cli")); err != nil {
		return errors.New("Whisper 빌드 후 실행 파일을 찾지 못했습니다")
	}
	return nil
}
