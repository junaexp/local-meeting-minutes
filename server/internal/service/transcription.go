package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const maxTranscriptBytes = 2 * 1024 * 1024

type transcriptionEvent struct {
	Stage string
	Log   string
	Text  string
}

type mediaTranscriber interface {
	Run(context.Context, string, string, string, string, string, func(transcriptionEvent)) error
}

type processTranscriber struct{}

func (processTranscriber) Run(ctx context.Context, source, ffmpeg, whisper, model, outputBase string, emit func(transcriptionEvent)) error {
	wav := filepath.Join(filepath.Dir(outputBase), "audio.wav")
	emit(transcriptionEvent{Stage: "extracting", Log: "FFmpeg 음성 추출 시작"})
	var ffmpegError string
	progressTime := ""
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-y", "-nostats", "-loglevel", "warning", "-stats_period", "1", "-progress", "pipe:1", "-i", source, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav)
	err := runStreamingCommand(cmd, func(line string) {
		if strings.HasPrefix(line, "out_time=") {
			progressTime = strings.TrimPrefix(line, "out_time=")
		}
		if strings.HasPrefix(line, "progress=") && progressTime != "" {
			emit(transcriptionEvent{Log: "음성 추출 위치 " + progressTime})
		}
	}, func(line string) {
		ffmpegError = tail(ffmpegError+line+"\n", 3000)
		emit(transcriptionEvent{Log: "FFmpeg: " + line})
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("ffmpeg 변환 실패: %w: %s", err, strings.TrimSpace(ffmpegError))
	}
	emit(transcriptionEvent{Stage: "transcribing", Log: "Whisper 전사 시작"})
	var whisperError string
	cmd = exec.CommandContext(ctx, whisper, "-m", model, "-f", wav, "-l", "auto", "-pp", "-osrt", "-of", outputBase)
	err = runStreamingCommand(cmd, func(line string) {
		if strings.HasPrefix(line, "[") && strings.Contains(line, " --> ") && strings.Contains(line, "]") {
			emit(transcriptionEvent{Text: line})
		} else if strings.TrimSpace(line) != "" {
			emit(transcriptionEvent{Log: "Whisper: " + line})
		}
	}, func(line string) {
		whisperError = tail(whisperError+line+"\n", 3000)
		emit(transcriptionEvent{Log: "Whisper: " + line})
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("Whisper 전사 실패: %w: %s", err, strings.TrimSpace(whisperError))
	}
	return nil
}

func runStreamingCommand(cmd *exec.Cmd, stdout, stderr func(string)) error {
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errOut, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var readers sync.WaitGroup
	readErrors := make(chan error, 2)
	read := func(pipe io.Reader, emit func(string)) {
		defer readers.Done()
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			emit(strings.TrimSuffix(scanner.Text(), "\r"))
		}
		if err := scanner.Err(); err != nil {
			readErrors <- err
		}
	}
	readers.Add(2)
	go read(out, stdout)
	go read(errOut, stderr)
	readers.Wait()
	waitErr := cmd.Wait()
	close(readErrors)
	for readErr := range readErrors {
		if waitErr == nil {
			waitErr = readErr
		}
	}
	return waitErr
}

func (s *Service) processTranscription(ctx context.Context, job Job) {
	s.mutate(job.ID, func(next *Job) { next.Status = "running"; next.Phase = "전사 시작" })
	transcript, err := s.transcribeMedia(ctx, job)
	if err != nil {
		s.fail(job.ID, err)
		return
	}
	if err := ctx.Err(); err != nil {
		s.fail(job.ID, err)
		return
	}
	path, err := saveStandaloneTranscript(job.Path, s.Config().OutputDir, transcript)
	if err != nil {
		s.fail(job.ID, fmt.Errorf("SRT 저장 실패: %w", err))
		return
	}
	if err := ctx.Err(); err != nil {
		s.mutate(job.ID, func(next *Job) { next.TranscriptPath = path })
		s.fail(job.ID, err)
		return
	}
	s.mutate(job.ID, func(next *Job) {
		now := time.Now().UTC()
		next.Status = "completed"
		next.Stage = "completed"
		next.Phase = "전사 완료"
		next.TranscriptPath = path
		next.TranscriptPreview = ""
		next.CompletedAt = &now
	})
}

func (s *Service) transcribeMedia(ctx context.Context, job Job) (string, error) {
	real, err := validateFile(job.Path)
	if err != nil {
		return "", err
	}
	if IsText(real) {
		return "", errors.New("오디오 또는 영상 파일만 전사할 수 있습니다")
	}
	s.mu.RLock()
	ffmpeg := s.cfg.FFmpegBinary
	model := job.TranscriptionModel
	if model == "" {
		model = s.cfg.WhisperModel
	}
	s.mu.RUnlock()
	key, err := transcriptKey(real, model)
	if err != nil {
		return "", err
	}
	if cached, ready, err := s.readTranscript(key); err != nil {
		return "", err
	} else if ready {
		s.mutate(job.ID, func(next *Job) { next.TranscriptKey = key })
		s.recordTranscription(job.ID, transcriptionEvent{Stage: "transcribed", Log: "보관된 전사문을 재사용합니다."})
		return cached, nil
	}
	bin := s.whisperBinary()
	modelPath := filepath.Join(s.root, "whisper", "models", "ggml-"+model+".bin")
	if stat, err := os.Stat(bin); err != nil || stat.IsDir() {
		return "", errors.New("Whisper 실행 파일이 없습니다. 설치 버튼을 눌러주세요")
	}
	if stat, err := os.Stat(modelPath); err != nil || stat.IsDir() || stat.Size() < 1000000 {
		return "", errors.New("Whisper 모델이 없습니다. 설치 버튼을 눌러주세요")
	}
	if _, err := exec.LookPath(ffmpeg); err != nil {
		return "", errors.New("ffmpeg를 설치하거나 설정에서 경로를 지정해 주세요")
	}
	work, err := os.MkdirTemp("", "meet-to-md-audio-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	outputBase := filepath.Join(work, "transcript")
	err = s.transcriber.Run(ctx, real, ffmpeg, bin, modelPath, outputBase, func(event transcriptionEvent) {
		s.recordTranscription(job.ID, event)
	})
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	file, err := os.Open(outputBase + ".srt")
	if err != nil {
		return "", fmt.Errorf("Whisper SRT 결과를 읽을 수 없습니다: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTranscriptBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxTranscriptBytes {
		return "", errors.New("전사 결과가 2MB를 초과합니다")
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", errors.New("Whisper가 빈 전사문을 반환했습니다")
	}
	currentKey, err := transcriptKey(real, model)
	if err != nil {
		return "", err
	}
	if currentKey != key {
		return "", errors.New("전사 중 원본 파일이 변경되었습니다. 다시 전사해 주세요")
	}
	if err := s.saveTranscript(key, data); err != nil {
		return "", err
	}
	s.mutate(job.ID, func(next *Job) { next.TranscriptKey = key })
	s.recordTranscription(job.ID, transcriptionEvent{Stage: "transcribed", Log: "전사 완료 · SRT 전사문을 보관했습니다."})
	return string(data), nil
}

func (s *Service) JobTranscript(id string) (string, error) {
	s.mu.RLock()
	key := ""
	for _, job := range s.jobs {
		if job.ID == id {
			key = job.TranscriptKey
			break
		}
	}
	s.mu.RUnlock()
	if key == "" {
		return "", os.ErrNotExist
	}
	transcript, ready, err := s.readTranscript(key)
	if err != nil {
		return "", err
	}
	if !ready {
		return "", os.ErrNotExist
	}
	return transcript, nil
}

func (s *Service) recordTranscription(id string, event transcriptionEvent) {
	if event.Stage != "" {
		phase := map[string]string{"extracting": "FFmpeg 음성 추출 중", "transcribing": "Whisper 전사 중", "transcribed": "전사 완료"}[event.Stage]
		s.mutate(id, func(job *Job) { job.Stage = event.Stage; job.Phase = phase })
	}
	if event.Log == "" && event.Text == "" {
		return
	}
	s.mutateLive(id, func(job *Job) {
		if event.Log != "" {
			job.TranscriptionLog = tail(job.TranscriptionLog+time.Now().Format("15:04:05")+"  "+event.Log+"\n", 30000)
		}
		if event.Text != "" {
			job.TranscriptPreview = tail(job.TranscriptPreview+event.Text+"\n", 64*1024)
		}
	})
}

func transcriptKey(path, model string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d\x00%s", path, info.Size(), info.ModTime().UnixNano(), model)))
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) cachedTranscript(path, model string) (string, bool, error) {
	key, err := transcriptKey(path, model)
	if err != nil {
		return "", false, err
	}
	return s.readTranscript(key)
}

func (s *Service) readTranscript(key string) (string, bool, error) {
	if decoded, err := hex.DecodeString(key); err != nil || len(decoded) != sha256.Size {
		return "", false, errors.New("올바르지 않은 전사문 키입니다")
	}
	path := filepath.Join(s.root, "server", "data", "transcripts", key+".srt")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTranscriptBytes+1))
	if err != nil {
		return "", false, err
	}
	if len(data) > maxTranscriptBytes || strings.TrimSpace(string(data)) == "" {
		return "", false, nil
	}
	return string(data), true, nil
}

func (s *Service) saveTranscript(key string, data []byte) error {
	dir := filepath.Join(s.root, "server", "data", "transcripts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".transcript-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, key+".srt"))
}

func headPreview(value string, n int) string {
	if len(value) <= n {
		return value
	}
	for n > 0 && !utf8.ValidString(value[:n]) {
		n--
	}
	return value[:n]
}

func tailPreview(value string, n int) string {
	if len(value) <= n {
		return value
	}
	start := len(value) - n
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	if line := strings.IndexByte(value[start:], '\n'); line >= 0 {
		start += line + 1
	}
	return value[start:]
}
