package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlowCheckpointDoesNotBlockStateLiveOutputOrActiveCancel(t *testing.T) {
	_, s, _ := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.mu.Lock()
	s.jobs = []Job{{ID: "live", Status: "running", Kind: "transcription"}}
	s.activeCancel = cancel
	s.revision++
	s.mu.Unlock()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s.writeHistory = func(_ string, _ []Job) error { close(started); <-release; return nil }
	defer func() { close(release); <-done }()
	go func() { defer close(done); s.persist() }()
	<-started
	responsive := make(chan struct{})
	go func() {
		defer close(responsive)
		s.mutateLive("live", func(j *Job) { j.TranscriptionLog = "still streaming" })
		if got := s.UISnapshot().Jobs[0].TranscriptionLog; got != "still streaming" {
			t.Errorf("live state = %q", got)
		}
		if err := s.Cancel("live"); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-responsive:
	case <-time.After(time.Second):
		t.Fatal("disk write blocked state or cancellation")
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("active process was not cancelled")
	}
}

func TestCheckpointFailureVisibleAndNewerOutputRemainsDirty(t *testing.T) {
	_, s, _ := setup(t)
	s.mu.Lock()
	s.jobs = []Job{{ID: "live", Status: "running"}}
	s.revision++
	s.mu.Unlock()
	s.writeHistory = func(string, []Job) error { return errors.New("synthetic disk failure") }
	if err := s.persist(); err == nil {
		t.Fatal("expected failure")
	}
	if s.UISnapshot().StorageError == "" {
		t.Fatal("storage failure hidden")
	}
	s.writeHistory = func(path string, jobs []Job) error {
		s.mutateLive("live", func(j *Job) { j.RecentOutput = "new output during fsync" })
		return writeHistoryFile(path, jobs)
	}
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
	s.mu.RLock()
	dirty := s.savedRevision < s.revision
	s.mu.RUnlock()
	if !dirty {
		t.Fatal("output that arrived during write was incorrectly marked saved")
	}
	s.writeHistory = writeHistoryFile
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
	if s.UISnapshot().StorageError != "" {
		t.Fatal("recovery warning did not clear")
	}
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		t.Fatal(err)
	}
	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		t.Fatal(err)
	}
	if jobs[0].RecentOutput != "new output during fsync" {
		t.Fatal("latest output lost")
	}
}
func TestHistoryDeleteRollbackPreservesLiveOutput(t *testing.T) {
	_, s, _ := setup(t)
	s.mu.Lock()
	s.jobs = []Job{{ID: "done", Status: "completed"}, {ID: "live", Status: "running"}}
	s.revision++
	s.mu.Unlock()
	s.writeHistory = func(string, []Job) error {
		s.mutateLive("live", func(j *Job) { j.RecentOutput = "concurrent output" })
		return errors.New("synthetic write failure")
	}
	if _, err := s.DeleteFinished(); err == nil {
		t.Fatal("expected delete failure")
	}
	jobs := s.Snapshot().Jobs
	if len(jobs) != 2 || jobs[1].RecentOutput != "concurrent output" {
		t.Fatalf("rollback lost state: %+v", jobs)
	}
}
func TestDuplicateMinutesAndExplicitRerun(t *testing.T) {
	root, s, _ := setup(t)
	path := filepath.Join(root, "duplicate.txt")
	if err := os.WriteFile(path, []byte("synthetic meeting"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := s.Enqueue([]string{path}, "gpt-5.6-sol", "low")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Enqueue([]string{path}, "gpt-6-sol", "high")
	if err != nil {
		t.Fatal(err)
	}
	if first[0].ID != second[0].ID || len(s.Snapshot().Jobs) != 1 {
		t.Fatal("duplicate active work registered")
	}
	s.mutate(first[0].ID, func(j *Job) { j.Status = "completed" })
	third, err := s.Enqueue([]string{path}, "gpt-5.6-sol", "low")
	if err != nil {
		t.Fatal(err)
	}
	if third[0].ID == first[0].ID || len(s.Snapshot().Jobs) != 2 {
		t.Fatal("explicit rerun not registered")
	}
}
func TestSnapshotUsesCachedReadinessAndOmitsAllPrompts(t *testing.T) {
	root, s, _ := setup(t)
	path := filepath.Join(root, "synthetic-codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	cfg.CodexBinary = path
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if !s.UISnapshot().CodexReady {
		t.Fatal("configured binary not detected")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !s.UISnapshot().CodexReady {
		t.Fatal("snapshot unexpectedly rescanned filesystem")
	}
	s.Environment()
	if s.UISnapshot().CodexReady {
		t.Fatal("rescan did not invalidate readiness")
	}
	s.mu.Lock()
	s.jobs = []Job{{ID: "queued", Status: "queued", Prompt: "private prompt"}, {ID: "live", Status: "running", Prompt: "private prompt"}}
	s.mu.Unlock()
	for _, j := range s.UISnapshot().Jobs {
		if j.Prompt != "" {
			t.Fatal("prompt leaked into repeated snapshots")
		}
	}
}
func TestCheckpointLoopFlushesTrailingEventAndFinalHistory(t *testing.T) {
	_, s, _ := setup(t)
	s.mu.Lock()
	s.jobs = []Job{{ID: "live", Status: "running"}}
	s.lastLiveEvent = time.Now()
	s.mu.Unlock()
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()
	<-events
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go s.checkpointLoop(ctx, done)
	s.mutateLive("live", func(j *Job) { j.RecentOutput = "last fragment" })
	select {
	case snapshot := <-events:
		if snapshot.Jobs[0].RecentOutput != "last fragment" {
			t.Fatal("missing tail")
		}
	case <-time.After(time.Second):
		t.Fatal("tail event never flushed")
	}
	cancel()
	<-done
	stored, err := os.ReadFile(s.storePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "last fragment") {
		t.Fatal("shutdown lost pending output")
	}
}
func BenchmarkUISnapshotDense(b *testing.B) {
	s := &Service{subs: map[chan Snapshot]struct{}{}}
	for i := 0; i < 1000; i++ {
		status := "completed"
		if i >= 800 {
			status = "queued"
		}
		s.jobs = append(s.jobs, Job{ID: fmt.Sprint(i), Status: status, Prompt: strings.Repeat("p", 8192), Result: strings.Repeat("r", 120000), RecentOutput: strings.Repeat("l", 30000)})
	}
	// Real queued jobs have no output.
	for i := 800; i < 1000; i++ {
		s.jobs[i].Result = ""
		s.jobs[i].RecentOutput = ""
	}
	s.jobs[999].Status = "running"
	s.jobs[999].Result = strings.Repeat("r", 120000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(s.UISnapshot()); err != nil {
			b.Fatal(err)
		}
	}
}
