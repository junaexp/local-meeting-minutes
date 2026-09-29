package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"meet-to-md/server/internal/codexapp"
	"meet-to-md/server/internal/config"
)

type fakeRunner struct {
	mu      sync.Mutex
	calls   []string
	prompts []string
}

func (f *fakeRunner) ListModels(context.Context, string) ([]codexapp.Model, error) {
	return []codexapp.Model{{ID: "gpt-5.6-sol", Model: "gpt-5.6-sol"}}, nil
}
func (f *fakeRunner) Run(_ context.Context, _ string, model, effort, prompt, cwd string, notify func(codexapp.Event)) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, filepath.Base(cwd)+":"+model+":"+effort)
	f.prompts = append(f.prompts, prompt)
	f.mu.Unlock()
	if !strings.Contains(prompt, "<transcript>") || !strings.Contains(prompt, "</transcript>") {
		return "", os.ErrInvalid
	}
	if notify != nil {
		payload, _ := json.Marshal(map[string]string{"delta": "# 결과"})
		notify(codexapp.Event{Method: "item/agentMessage/delta", Params: payload})
	}
	return "# 회의록\n\n결정 사항 없음", nil
}

type fakeTranscriber struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	block   chan struct{}
}

func (f *fakeTranscriber) Run(ctx context.Context, _, _, _, _, outputBase string, emit func(transcriptionEvent)) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	emit(transcriptionEvent{Stage: "extracting", Log: "음성 추출 위치 00:00:01"})
	emit(transcriptionEvent{Stage: "transcribing", Log: "Whisper 전사 시작", Text: "[00:00:01.000 --> 00:00:02.000] 안녕하세요"})
	if f.started != nil {
		close(f.started)
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return os.WriteFile(outputBase+".srt", []byte("1\n00:00:01,000 --> 00:00:02,000\n안녕하세요\n"), 0600)
}

func setupMedia(t *testing.T) (string, *Service, *fakeRunner, *fakeTranscriber) {
	t.Helper()
	root, s, runner := setup(t)
	bin := filepath.Join(root, "whisper", "build", "bin", "whisper-cli")
	if runtime.GOOS == "windows" {
		bin = filepath.Join(root, "whisper", "whisper-cli.exe")
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("test binary"), 0700); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(root, "whisper", "models", "ggml-large-v3-turbo.bin")
	if err := os.MkdirAll(filepath.Dir(model), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model, make([]byte, 1000000), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	cfg.FFmpegBinary = executable
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	transcriber := &fakeTranscriber{}
	s.transcriber = transcriber
	media := filepath.Join(root, "meeting.mp4")
	if err := os.WriteFile(media, []byte("original media"), 0600); err != nil {
		t.Fatal(err)
	}
	return media, s, runner, transcriber
}
func setup(t *testing.T) (string, *Service, *fakeRunner) {
	t.Helper()
	root := t.TempDir()
	server := filepath.Join(root, "server")
	if err := os.MkdirAll(server, 0700); err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Prompt = "회의록 생성"
	path := filepath.Join(server, "config.toml")
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{}
	s, err := New(root, path, f)
	if err != nil {
		t.Fatal(err)
	}
	return root, s, f
}
func waitDone(t *testing.T, s *Service, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		snap := s.Snapshot()
		if len(snap.Jobs) == n {
			done := true
			for _, j := range snap.Jobs {
				if j.Status != "completed" {
					done = false
				}
			}
			if done {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("jobs did not complete: %#v", snap.Jobs)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestExpandQueueAndSingleMarkdownPerFile(t *testing.T) {
	root, s, f := setup(t)
	folder := filepath.Join(root, "meeting")
	_ = os.MkdirAll(folder, 0700)
	for _, name := range []string{"b.vtt", "a.srt", "skip.pdf"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("안녕하세요"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := Expand([]string{folder, filepath.Join(folder, "a.srt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || filepath.Base(paths[0]) != "a.srt" || filepath.Base(paths[1]) != "b.vtt" {
		t.Fatalf("bad expansion/order: %v", paths)
	}
	jobs, err := s.Enqueue([]string{filepath.Join(folder, "b.vtt"), filepath.Join(folder, "a.srt")}, "gpt-5.6-sol", "xhigh")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].Name != "b.vtt" {
		t.Fatalf("enqueue order: %#v", jobs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitDone(t, s, 2)
	snap := s.Snapshot()
	realFolder, err := filepath.EvalSymlinks(folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range snap.Jobs {
		if job.OutputPath != filepath.Join(realFolder, strings.TrimSuffix(job.Name, filepath.Ext(job.Name))+"_회의록.md") {
			t.Errorf("wrong output path: %s", job.OutputPath)
		}
		b, err := os.ReadFile(job.OutputPath)
		if err != nil || !strings.Contains(string(b), "# 회의록") {
			t.Errorf("output unreadable: %v", err)
		}
		if job.Result == "" {
			t.Error("live result absent")
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 2 {
		t.Fatalf("runner calls: %v", f.calls)
	}
	// A repeated source must get a new name and never overwrite the prior result.
	newPath, err := saveMinutes(filepath.Join(folder, "a.srt"), "", "another")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(newPath, "a_회의록_2.md") {
		t.Fatalf("collision path: %s", newPath)
	}
}

func TestPersistenceAndCancellation(t *testing.T) {
	root, s, _ := setup(t)
	file := filepath.Join(root, "meeting.txt")
	_ = os.WriteFile(file, []byte("테스트"), 0600)
	created, err := s.Enqueue([]string{file}, "gpt-5.6-sol", "low")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(created[0].ID); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Jobs[0].Status != "cancelled" {
		t.Fatal("queued job was not cancelled")
	}
	if err := s.Delete(created[0].ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := New(root, filepath.Join(root, "server", "config.toml"), &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Snapshot().Jobs) != 0 {
		t.Fatal("deleted job was persisted")
	}
	if _, err := s.Enqueue([]string{file}, "unknown", "low"); err == nil {
		t.Fatal("invalid model accepted")
	}
}

func TestPreviewAndTraversalValidation(t *testing.T) {
	root, s, _ := setup(t)
	path := filepath.Join(root, "sample.vtt")
	_ = os.WriteFile(path, []byte("WEBVTT\n00:00:01.000 --> 00:00:02.000\n발언"), 0600)
	preview, err := s.Preview(path)
	if err != nil || !strings.Contains(preview, "발언") {
		t.Fatalf("preview: %q %v", preview, err)
	}
	if _, err := s.Preview("../relative.vtt"); err == nil {
		t.Fatal("relative path accepted")
	}
	if _, err := Expand([]string{root, root, root, root}); err != nil {
		t.Fatal(err)
	}
}

func TestManualTranscriptionIsReusedAndInvalidated(t *testing.T) {
	media, s, runner, transcriber := setupMedia(t)
	manualJob, err := s.EnqueueTranscription(media)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitDone(t, s, 1)
	preview, ready, err := s.PreviewState(media)
	if err != nil || !ready || !strings.Contains(preview, "00:00:01,000") {
		t.Fatalf("cached preview: %q %v %v", preview, ready, err)
	}
	reopened, err := New(s.root, s.configPath, runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, persisted, err := reopened.PreviewState(media); err != nil || !persisted {
		t.Fatalf("transcript did not survive service restart: %v %v", persisted, err)
	}
	if !strings.Contains(s.Snapshot().Jobs[0].TranscriptionLog, "Whisper 전사 시작") {
		t.Fatal("transcription log was not retained")
	}
	if _, err := s.Enqueue([]string{media}, "gpt-5.6-sol", "low"); err != nil {
		t.Fatal(err)
	}
	waitDone(t, s, 2)
	transcriber.mu.Lock()
	calls := transcriber.calls
	transcriber.mu.Unlock()
	if calls != 1 {
		t.Fatalf("cached transcription was not reused: %d runs", calls)
	}
	runner.mu.Lock()
	prompt := runner.prompts[0]
	runner.mu.Unlock()
	if !strings.Contains(prompt, "00:00:01,000") {
		t.Fatal("timestamped SRT was not sent to Codex")
	}
	if err := os.WriteFile(media, []byte("changed media contents"), 0600); err != nil {
		t.Fatal(err)
	}
	_, ready, err = s.PreviewState(media)
	if err != nil || ready {
		t.Fatalf("stale transcription was reused: %v %v", ready, err)
	}
	if oldTranscript, err := s.JobTranscript(manualJob.ID); err != nil || !strings.Contains(oldTranscript, "안녕하세요") {
		t.Fatalf("completed job lost its transcript after source changed: %q %v", oldTranscript, err)
	}
	if _, err := s.EnqueueTranscription(media); err != nil {
		t.Fatal(err)
	}
	waitDone(t, s, 3)
	transcriber.mu.Lock()
	calls = transcriber.calls
	transcriber.mu.Unlock()
	if calls != 2 {
		t.Fatalf("changed media was not transcribed again: %d runs", calls)
	}
	baseModel := filepath.Join(s.root, "whisper", "models", "ggml-base.bin")
	if err := os.WriteFile(baseModel, make([]byte, 1000000), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	cfg.WhisperModel = "base"
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	_, ready, err = s.PreviewState(media)
	if err != nil || ready {
		t.Fatalf("old model cache was reused: %v %v", ready, err)
	}
	if _, err := s.EnqueueTranscription(media); err != nil {
		t.Fatal(err)
	}
	waitDone(t, s, 4)
	transcriber.mu.Lock()
	calls = transcriber.calls
	transcriber.mu.Unlock()
	if calls != 3 {
		t.Fatalf("new model did not transcribe: %d runs", calls)
	}
}

func TestAutomaticMediaTranscriptionStillCreatesMinutes(t *testing.T) {
	media, s, runner, transcriber := setupMedia(t)
	if _, err := s.Enqueue([]string{media}, "gpt-5.6-sol", "low"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitDone(t, s, 1)
	job := s.Snapshot().Jobs[0]
	if job.Kind != "minutes" || job.OutputPath == "" || job.TranscriptPath == "" || job.TranscriptKey == "" || !strings.Contains(job.TranscriptionLog, "Whisper 전사 시작") {
		t.Fatalf("automatic media job: %+v", job)
	}
	if data, err := os.ReadFile(job.TranscriptPath); err != nil || !strings.Contains(string(data), "00:00:01,000") {
		t.Fatalf("missing exported SRT: %q %v", data, err)
	}
	if filepath.Dir(job.TranscriptPath) != filepath.Dir(job.OutputPath) {
		t.Fatalf("SRT and Markdown use different output folders: %+v", job)
	}
	transcriber.mu.Lock()
	calls := transcriber.calls
	transcriber.mu.Unlock()
	runner.mu.Lock()
	prompts := len(runner.prompts)
	runner.mu.Unlock()
	if calls != 1 || prompts != 1 {
		t.Fatalf("auto flow ran %d transcriptions and %d Codex turns", calls, prompts)
	}
}

func TestVideoMinutesExportPairsWithoutOverwriting(t *testing.T) {
	media, s, _, transcriber := setupMedia(t)
	outputDir := t.TempDir()
	cfg := s.Config()
	cfg.OutputDir = outputDir
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(outputDir, "meeting_회의록.md")
	if err := os.WriteFile(previous, []byte("기존 회의록"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	for index, suffix := range []string{"_2", "_3"} {
		if _, err := s.Enqueue([]string{media}, "gpt-5.6-sol", "low"); err != nil {
			t.Fatal(err)
		}
		waitDone(t, s, index+1)
		job := s.Snapshot().Jobs[index]
		wantSRT := filepath.Join(outputDir, "meeting_전사"+suffix+".srt")
		wantMD := filepath.Join(outputDir, "meeting_회의록"+suffix+".md")
		if job.TranscriptPath != wantSRT || job.OutputPath != wantMD {
			t.Fatalf("wrong paired paths: SRT %q, MD %q", job.TranscriptPath, job.OutputPath)
		}
		if data, err := os.ReadFile(wantSRT); err != nil || !strings.Contains(string(data), "안녕하세요") {
			t.Fatalf("SRT: %q %v", data, err)
		}
		if data, err := os.ReadFile(wantMD); err != nil || !strings.Contains(string(data), "회의록") {
			t.Fatalf("Markdown: %q %v", data, err)
		}
	}
	if data, err := os.ReadFile(previous); err != nil || string(data) != "기존 회의록" {
		t.Fatalf("overwrote previous result: %q %v", data, err)
	}
	transcriber.mu.Lock()
	calls := transcriber.calls
	transcriber.mu.Unlock()
	if calls != 1 {
		t.Fatalf("cached SRT was not reused: %d transcriptions", calls)
	}
}

func TestVideoTranscriptRemainsWhenMinutesFail(t *testing.T) {
	media, s, _, _ := setupMedia(t)
	cfg := s.Config()
	cfg.CodexBinary = filepath.Join(t.TempDir(), "missing-codex")
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enqueue([]string{media}, "gpt-5.6-sol", "low"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	deadline := time.After(5 * time.Second)
	var job Job
	for {
		job = s.Snapshot().Jobs[0]
		if job.Status == "failed" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("job did not fail: %+v", job)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if job.TranscriptPath == "" || job.OutputPath != "" || !strings.Contains(job.Phase, "SRT 저장됨") {
		t.Fatalf("unexpected result paths: %+v", job)
	}
	if data, err := os.ReadFile(job.TranscriptPath); err != nil || !strings.Contains(string(data), "안녕하세요") {
		t.Fatalf("SRT did not survive: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(media), "meeting_회의록.md")); !os.IsNotExist(err) {
		t.Fatalf("Markdown was unexpectedly saved: %v", err)
	}
}

func TestQueuedMinutesWaitsForManualTranscription(t *testing.T) {
	media, s, _, transcriber := setupMedia(t)
	if _, err := s.EnqueueTranscription(media); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enqueue([]string{media}, "gpt-5.6-sol", "low"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitDone(t, s, 2)
	transcriber.mu.Lock()
	calls := transcriber.calls
	transcriber.mu.Unlock()
	if calls != 1 {
		t.Fatalf("queued minute job repeated transcription: %d", calls)
	}
}

func TestCancellingTranscriptionStopsWithoutCache(t *testing.T) {
	media, s, _, transcriber := setupMedia(t)
	transcriber.started = make(chan struct{})
	transcriber.block = make(chan struct{})
	job, err := s.EnqueueTranscription(media)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go s.Run(ctx)
	select {
	case <-transcriber.started:
	case <-time.After(5 * time.Second):
		t.Fatal("transcription did not start")
	}
	live := s.Snapshot().Jobs[0]
	if live.Stage != "transcribing" || !strings.Contains(live.TranscriptionLog, "Whisper 전사 시작") || !strings.Contains(live.TranscriptPreview, "안녕하세요") {
		t.Fatalf("live transcription state missing: %+v", live)
	}
	if err := s.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for s.Snapshot().Jobs[0].Status != "cancelled" {
		select {
		case <-deadline:
			t.Fatal("transcription was not cancelled")
		case <-time.After(10 * time.Millisecond):
		}
	}
	_, ready, err := s.PreviewState(media)
	if err != nil || ready {
		t.Fatalf("cancelled transcription left cache: %v %v", ready, err)
	}
}
