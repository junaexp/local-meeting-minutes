// Package service owns sequential work, durable history, and local tool integration.
package service

import (
	"context"
	"encoding/json"
	"meet-to-md/server/internal/codexapp"
	"meet-to-md/server/internal/config"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Service has three lock boundaries:
// mu protects memory only; never hold it for disk I/O or process execution.
// changesMu orders durable edits; persistMu orders snapshot writes outside mu.
// Active cancellation intentionally bypasses changesMu so a slow disk cannot delay it.
type Service struct {
	mu                sync.RWMutex
	changesMu         sync.Mutex
	persistMu         sync.Mutex
	cfg               config.Config
	configPath        string
	root              string
	storePath         string
	runner            codexapp.Runner
	transcriber       mediaTranscriber
	jobs              []Job
	subs              map[chan Snapshot]struct{}
	wake              chan struct{}
	activeCancel      context.CancelFunc
	whisperInstalling bool
	whisperError      string
	whisperInstall    InstallProgress
	lastInstallEvent  time.Time
	lastLiveEvent     time.Time
	liveDirty         bool
	revision          uint64
	savedRevision     uint64
	storageError      string
	codexReady        bool
	whisperReady      bool
	readinessRevision uint64
	// writeHistory is replaceable in tests to exercise slow disks and failed writes.
	writeHistory func(string, []Job) error
}

func New(root, configPath string, runner codexapp.Runner) (*Service, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if runner == nil {
		runner = codexapp.ProcessRunner{}
	}
	s := &Service{root: root, configPath: configPath, cfg: cfg, storePath: filepath.Join(root, "server", "data", "jobs.json"), runner: runner, transcriber: processTranscriber{}, jobs: []Job{}, subs: map[chan Snapshot]struct{}{}, wake: make(chan struct{}, 1), writeHistory: writeHistoryFile}
	if data, err := os.ReadFile(s.storePath); err == nil {
		if err := json.Unmarshal(data, &s.jobs); err != nil {
			return nil, err
		}
		for i := range s.jobs {
			if s.jobs[i].Status == "running" || s.jobs[i].Status == "preparing" {
				s.jobs[i].Status = "interrupted"
				s.jobs[i].Stage = "interrupted"
				s.jobs[i].Error = "서버가 재시작되어 작업이 중단되었습니다. 다시 등록해 주세요."
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	s.revision = 1
	s.refreshReadiness()
	if err := s.persist(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Config() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}
func (s *Service) UpdateConfig(next config.Config) error {
	if err := next.Validate(); err != nil {
		return err
	}
	s.changesMu.Lock()
	defer s.changesMu.Unlock()
	if err := config.Save(s.configPath, next); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = next
	s.mu.Unlock()
	s.refreshReadiness()
	s.mu.Lock()
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}

// Readiness is cached between configuration, installation, and explicit rescan events.
// A generation guard prevents a slow rescan from replacing a newer result.
func (s *Service) refreshReadiness() {
	s.mu.Lock()
	s.readinessRevision++
	version, cfg := s.readinessRevision, s.cfg
	s.mu.Unlock()
	_, codexErr := config.ResolveBinary(cfg.CodexBinary)
	model, modelErr := os.Stat(filepath.Join(s.root, "whisper", "models", "ggml-"+cfg.WhisperModel+".bin"))
	binary, binaryErr := os.Stat(s.whisperBinary())
	ready := modelErr == nil && !model.IsDir() && model.Size() >= 1000000 && binaryErr == nil && !binary.IsDir()
	s.mu.Lock()
	if version == s.readinessRevision && cfg == s.cfg {
		s.codexReady, s.whisperReady = codexErr == nil, ready
	}
	s.mu.Unlock()
}
