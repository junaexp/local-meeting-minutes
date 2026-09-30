package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

func jobActive(job Job) bool {
	return job.Status == "queued" || job.Status == "preparing" || job.Status == "running"
}
func newID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Enqueue is idempotent for sources already queued or running. Finished sources
// can be explicitly rerun and receive new, non-overwriting output filenames.
func (s *Service) Enqueue(paths []string, model, effort string) ([]Job, error) {
	files, err := Expand(paths)
	if err != nil {
		return nil, err
	}
	if !validModelEffort(model, effort) {
		return nil, errors.New("지원하지 않는 모델 또는 추론 수준입니다")
	}
	s.changesMu.Lock()
	defer s.changesMu.Unlock()
	s.mu.Lock()
	previous := append([]Job{}, s.jobs...)
	selected := make([]Job, 0, len(files))
	active := make(map[string]Job)
	for _, j := range s.jobs {
		if j.Kind != "transcription" && jobActive(j) {
			active[j.Path] = j
		}
	}
	for _, path := range files {
		if j, ok := active[path]; ok {
			selected = append(selected, j)
			continue
		}
		if len(s.jobs) >= 1000 {
			s.jobs = previous
			s.mu.Unlock()
			return nil, errors.New("작업 기록이 가득 찼습니다")
		}
		id, err := newID()
		if err != nil {
			s.jobs = previous
			s.mu.Unlock()
			return nil, err
		}
		j := Job{ID: id, Kind: "minutes", Path: path, Name: filepath.Base(path), Status: "queued", Stage: "queued", Phase: "대기 중", Model: model, Effort: effort, TranscriptionModel: s.cfg.WhisperModel, Prompt: s.cfg.Prompt, CreatedAt: time.Now().UTC()}
		s.jobs = append(s.jobs, j)
		selected = append(selected, j)
		active[path] = j
	}
	s.revision++
	s.mu.Unlock()
	if err := s.commitMembership(previous); err != nil {
		return nil, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return selected, nil
}
func (s *Service) EnqueueTranscription(path string) (Job, error) {
	real, err := validateFile(path)
	if err != nil {
		return Job{}, err
	}
	if IsText(real) {
		return Job{}, errors.New("오디오 또는 영상 파일만 전사할 수 있습니다")
	}
	s.changesMu.Lock()
	defer s.changesMu.Unlock()
	s.mu.Lock()
	for _, j := range s.jobs {
		if j.Kind == "transcription" && j.Path == real && j.TranscriptionModel == s.cfg.WhisperModel && jobActive(j) {
			s.mu.Unlock()
			return j, nil
		}
	}
	if len(s.jobs) >= 1000 {
		s.mu.Unlock()
		return Job{}, errors.New("작업 기록이 가득 찼습니다")
	}
	id, err := newID()
	if err != nil {
		s.mu.Unlock()
		return Job{}, err
	}
	previous := append([]Job{}, s.jobs...)
	j := Job{ID: id, Kind: "transcription", Path: real, Name: filepath.Base(real), Status: "queued", Stage: "queued", Phase: "전사 대기 중", TranscriptionModel: s.cfg.WhisperModel, CreatedAt: time.Now().UTC()}
	s.jobs = append(s.jobs, j)
	s.revision++
	s.mu.Unlock()
	if err := s.commitMembership(previous); err != nil {
		return Job{}, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return j, nil
}
func validModelEffort(model, effort string) bool {
	if model != "gpt-6-astra" && model != "gpt-6-sol" && model != "gpt-5.6-sol" {
		return false
	}
	return effort == "low" || effort == "medium" || effort == "high" || effort == "xhigh"
}
func (s *Service) commitMembership(previous []Job) error {
	if err := s.persist(); err != nil {
		s.restoreMembership(previous)
		return err
	}
	s.mu.Lock()
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}
func (s *Service) Delete(id string) error {
	s.changesMu.Lock()
	defer s.changesMu.Unlock()
	s.mu.Lock()
	previous := append([]Job{}, s.jobs...)
	for i, j := range s.jobs {
		if j.ID != id {
			continue
		}
		if j.Status == "running" || j.Status == "preparing" {
			s.mu.Unlock()
			return errors.New("실행 중인 작업은 취소한 뒤 삭제하세요")
		}
		s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
		s.revision++
		s.mu.Unlock()
		return s.commitMembership(previous)
	}
	s.mu.Unlock()
	return os.ErrNotExist
}
func (s *Service) DeleteFinished() (int, error) {
	s.changesMu.Lock()
	defer s.changesMu.Unlock()
	s.mu.Lock()
	previous := append([]Job{}, s.jobs...)
	kept := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		if jobActive(j) {
			kept = append(kept, j)
		}
	}
	deleted := len(s.jobs) - len(kept)
	if deleted == 0 {
		s.mu.Unlock()
		return 0, nil
	}
	s.jobs = kept
	s.revision++
	s.mu.Unlock()
	if err := s.commitMembership(previous); err != nil {
		return 0, err
	}
	return deleted, nil
}

// Cancellation of a running process must remain responsive during a slow checkpoint.
func (s *Service) Cancel(id string) error {
	s.mu.RLock()
	for _, j := range s.jobs {
		if j.ID == id && (j.Status == "running" || j.Status == "preparing") {
			cancel := s.activeCancel
			s.mu.RUnlock()
			if cancel != nil {
				cancel()
			}
			return nil
		}
	}
	s.mu.RUnlock()
	s.changesMu.Lock()
	defer s.changesMu.Unlock()
	s.mu.Lock()
	for i := range s.jobs {
		j := &s.jobs[i]
		if j.ID != id {
			continue
		}
		if j.Status == "running" || j.Status == "preparing" {
			cancel := s.activeCancel
			s.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			return nil
		}
		if j.Status != "queued" {
			s.mu.Unlock()
			return errors.New("취소할 수 없는 상태입니다")
		}
		j.Status = "cancelled"
		j.Stage = "cancelled"
		j.Phase = "취소됨"
		now := time.Now().UTC()
		j.CompletedAt = &now
		s.revision++
		s.mu.Unlock()
		err := s.persist()
		s.mu.Lock()
		s.broadcastLocked()
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return os.ErrNotExist
}

// Run is the single worker. It owns the checkpoint loop and waits for its final write.
func (s *Service) Run(ctx context.Context) {
	checkpointCtx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go s.checkpointLoop(checkpointCtx, done)
	defer func() { stop(); <-done }()
	for {
		if ctx.Err() != nil {
			return
		}
		s.changesMu.Lock()
		s.mu.Lock()
		idx := -1
		for i := range s.jobs {
			if s.jobs[i].Status == "queued" {
				idx = i
				break
			}
		}
		if idx < 0 {
			s.mu.Unlock()
			s.changesMu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
			continue
		}
		now := time.Now().UTC()
		s.jobs[idx].Status = "preparing"
		s.jobs[idx].Phase = "입력 준비 중"
		if s.jobs[idx].Kind == "transcription" {
			s.jobs[idx].Phase = "전사 준비 중"
		}
		s.jobs[idx].StartedAt = &now
		job := s.jobs[idx]
		s.revision++
		jobCtx, cancel := context.WithCancel(ctx)
		s.activeCancel = cancel
		s.mu.Unlock()
		s.persist()
		s.mu.Lock()
		s.broadcastLocked()
		s.mu.Unlock()
		s.changesMu.Unlock()
		if job.Kind == "transcription" {
			s.processTranscription(jobCtx, job)
		} else {
			s.process(jobCtx, job)
		}
		cancel()
		s.mu.Lock()
		s.activeCancel = nil
		s.mu.Unlock()
	}
}
