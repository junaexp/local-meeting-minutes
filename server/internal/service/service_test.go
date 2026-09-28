package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"meet-to-md/server/internal/codexapp"
	"meet-to-md/server/internal/config"
)

type fakeRunner struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeRunner) ListModels(context.Context, string) ([]codexapp.Model, error) {
	return []codexapp.Model{{ID: "gpt-5.6-sol", Model: "gpt-5.6-sol"}}, nil
}
func (f *fakeRunner) Run(_ context.Context, _ string, model, effort, prompt, cwd string, notify func(codexapp.Event)) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, filepath.Base(cwd)+":"+model+":"+effort)
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
