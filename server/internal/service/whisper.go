package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	model := s.cfg.WhisperModel
	s.whisperInstall = InstallProgress{Model: model, Stage: "starting", Message: "설치 준비 중", Log: "설치 시작 · 모델 " + model + "\n"}
	s.broadcastLocked()
	s.mu.Unlock()
	log.Printf("Whisper 설치 시작: %s", model)
	go func() {
		err := s.installWhisper(ctx, model)
		s.mu.Lock()
		s.whisperInstalling = false
		if err != nil {
			s.whisperError = err.Error()
			s.whisperInstall.Stage = "failed"
			s.whisperInstall.Message = "설치 실패"
			s.whisperInstall.Log = tail(s.whisperInstall.Log+"설치 실패: "+err.Error()+"\n", 20000)
		} else {
			s.whisperInstall.Stage = "completed"
			s.whisperInstall.Message = "설치 완료"
			s.whisperInstall.Log = tail(s.whisperInstall.Log+"설치 완료\n", 20000)
		}
		s.broadcastLocked()
		s.mu.Unlock()
		if err != nil {
			log.Printf("Whisper 설치 실패: %v", err)
		} else {
			log.Printf("Whisper 설치 완료: %s", model)
		}
	}()
	return nil
}
func (s *Service) installWhisper(ctx context.Context, model string) error {
	base := filepath.Join(s.root, "whisper")
	modelPath := filepath.Join(base, "models", "ggml-"+model+".bin")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0700); err != nil {
		return err
	}
	if info, err := os.Stat(modelPath); err != nil || info.Size() < 1000000 {
		s.setInstallStage("model_download", "Whisper "+model+" 모델 다운로드 중")
		url := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-" + model + ".bin"
		maxBytes := int64(600 * 1024 * 1024)
		if model == "large-v3-turbo" {
			maxBytes = 2 * 1024 * 1024 * 1024
		}
		if err := downloadWithProgress(ctx, url, modelPath, maxBytes, s.setInstallDownload); err != nil {
			return fmt.Errorf("Whisper 모델 다운로드: %w", err)
		}
		if info, err := os.Stat(modelPath); err != nil || info.Size() < 1000000 {
			return errors.New("다운로드한 Whisper 모델 파일이 너무 작습니다")
		}
	} else {
		s.appendInstallLog("설치된 Whisper 모델을 사용합니다.")
	}
	if bin := s.whisperBinary(); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			s.appendInstallLog("설치된 Whisper 실행 파일을 사용합니다.")
			return nil
		}
	}
	if runtime.GOOS == "windows" {
		return installWindowsWhisper(ctx, base, s)
	}
	if runtime.GOOS == "darwin" {
		return installMacWhisper(ctx, base, s)
	}
	return errors.New("Whisper 자동 설치는 macOS 및 Windows에서 지원합니다")
}

func (s *Service) setInstallStage(stage, message string) {
	s.mu.Lock()
	s.whisperInstall.Stage = stage
	s.whisperInstall.Message = message
	s.whisperInstall.Downloaded = 0
	s.whisperInstall.Total = 0
	s.whisperInstall.Log = tail(s.whisperInstall.Log+message+"\n", 20000)
	s.broadcastLocked()
	s.mu.Unlock()
	log.Printf("Whisper 설치: %s", message)
}

func (s *Service) appendInstallLog(line string) {
	s.mu.Lock()
	s.whisperInstall.Log = tail(s.whisperInstall.Log+line+"\n", 20000)
	if time.Since(s.lastInstallEvent) >= 200*time.Millisecond {
		s.broadcastLocked()
		s.lastInstallEvent = time.Now()
	}
	s.mu.Unlock()
	log.Printf("Whisper 설치: %s", line)
}

func (s *Service) setInstallDownload(received, total int64) {
	s.mu.Lock()
	previous := s.whisperInstall.Downloaded
	s.whisperInstall.Downloaded = received
	s.whisperInstall.Total = total
	if total > 0 && previous*10/total != received*10/total {
		log.Printf("Whisper 설치 다운로드: %d%%", received*100/total)
	}
	if time.Since(s.lastInstallEvent) >= 250*time.Millisecond || (total > 0 && received == total) {
		s.broadcastLocked()
		s.lastInstallEvent = time.Now()
	}
	s.mu.Unlock()
}

func (s *Service) installExtractProgress() func(int, int) {
	lastDecile := -1
	return func(done, total int) {
		if total == 0 {
			return
		}
		decile := done * 10 / total
		if decile != lastDecile {
			lastDecile = decile
			s.appendInstallLog(fmt.Sprintf("압축 해제 %d/%d개", done, total))
		}
	}
}

type downloadWriter struct {
	io.Writer
	received int64
	total    int64
	progress func(int64, int64)
}

func (w *downloadWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.received += int64(n)
	if w.progress != nil {
		w.progress(w.received, w.total)
	}
	return n, err
}

func downloadWithProgress(ctx context.Context, url, target string, maxBytes int64, progress func(int64, int64)) error {
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
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	if progress != nil {
		progress(0, total)
	}
	writer := &downloadWriter{Writer: f, total: total, progress: progress}
	n, err := io.Copy(writer, io.LimitReader(resp.Body, maxBytes+1))
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
	return extractZipWithProgress(archive, dest, stripRoot, nil)
}

func extractZipWithProgress(archive, dest string, stripRoot bool, progress func(int, int)) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	const maxEntrySize = 200 * 1024 * 1024
	const maxExtractedSize = 1024 * 1024 * 1024
	var extracted int64
	for index, item := range r.File {
		name := filepath.FromSlash(item.Name)
		if stripRoot {
			parts := strings.SplitN(name, string(filepath.Separator), 2)
			if len(parts) != 2 {
				if progress != nil {
					progress(index+1, len(r.File))
				}
				continue
			}
			name = parts[1]
		}
		if name == "" {
			if progress != nil {
				progress(index+1, len(r.File))
			}
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
			if progress != nil {
				progress(index+1, len(r.File))
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
		if progress != nil {
			progress(index+1, len(r.File))
		}
	}
	return nil
}
func installWindowsWhisper(ctx context.Context, base string, s *Service) error {
	asset := "whisper-bin-x64.zip"
	tag := whisperInstallerVersion()
	if runtime.GOARCH == "arm64" {
		asset = "whisper-bin-win-cpu-arm64.zip"
	} else if runtime.GOARCH != "amd64" {
		return errors.New("이 Windows CPU 아키텍처는 지원하지 않습니다")
	}
	zipPath := filepath.Join(base, "whisper-windows.zip")
	defer os.Remove(zipPath)
	s.setInstallStage("binary_download", "Whisper 실행 파일 다운로드 중")
	if err := downloadWithProgress(ctx, "https://github.com/ggml-org/whisper.cpp/releases/download/"+tag+"/"+asset, zipPath, 200*1024*1024, s.setInstallDownload); err != nil {
		return err
	}
	s.setInstallStage("extracting", "Whisper 실행 파일 압축 해제 중")
	if err := extractZipWithProgress(zipPath, filepath.Join(base, "bin"), false, s.installExtractProgress()); err != nil {
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
func installMacWhisper(ctx context.Context, base string, s *Service) error {
	cmake, err := exec.LookPath("cmake")
	if err != nil {
		return errors.New("macOS에서는 Whisper 빌드에 CMake가 필요합니다. CMake 설치 후 다시 시도하세요")
	}
	archive := filepath.Join(base, "whisper-source.zip")
	defer os.Remove(archive)
	s.setInstallStage("source_download", "Whisper 소스 다운로드 중")
	if err := downloadWithProgress(ctx, "https://github.com/ggml-org/whisper.cpp/archive/refs/tags/"+whisperVersion+".zip", archive, 100*1024*1024, s.setInstallDownload); err != nil {
		return err
	}
	s.setInstallStage("extracting", "Whisper 소스 압축 해제 중")
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		return err
	}
	if err := extractZipWithProgress(archive, source, true, s.installExtractProgress()); err != nil {
		return err
	}
	build := filepath.Join(base, "build")
	for index, args := range [][]string{{"-S", source, "-B", build, "-DCMAKE_BUILD_TYPE=Release"}, {"--build", build, "--config", "Release", "-j", "4"}} {
		if index == 0 {
			s.setInstallStage("configuring", "macOS 빌드 설정 중")
		} else {
			s.setInstallStage("building", "macOS 빌드 중")
		}
		cmd := exec.CommandContext(ctx, cmake, args...)
		var mu sync.Mutex
		var output string
		record := func(line string) {
			mu.Lock()
			output = tail(output+line+"\n", 2000)
			mu.Unlock()
			s.appendInstallLog(line)
		}
		err := runStreamingCommand(cmd, record, record)
		if err != nil {
			return fmt.Errorf("Whisper 빌드 실패: %w: %s", err, output)
		}
	}
	if _, err := os.Stat(filepath.Join(build, "bin", "whisper-cli")); err != nil {
		return errors.New("Whisper 빌드 후 실행 파일을 찾지 못했습니다")
	}
	return nil
}
