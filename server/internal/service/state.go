package service

import (
	"os"
	"time"
)

// Snapshot returns a complete in-memory copy for internal callers.
func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}
func (s *Service) UISnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uiSnapshotLocked()
}
func (s *Service) snapshotLocked() Snapshot {
	return Snapshot{Jobs: append([]Job{}, s.jobs...), CodexReady: s.codexReady, WhisperReady: s.whisperReady, WhisperInstalling: s.whisperInstalling, WhisperError: s.whisperError, WhisperInstall: s.whisperInstall, StorageError: s.storageError}
}

// Broadcasts carry summaries plus bounded live output. Prompts are fetched with job details.
func (s *Service) uiSnapshotLocked() Snapshot {
	snapshot := Snapshot{CodexReady: s.codexReady, WhisperReady: s.whisperReady, WhisperInstalling: s.whisperInstalling, WhisperError: s.whisperError, WhisperInstall: s.whisperInstall, StorageError: s.storageError}
	jobs := make([]Job, 0, len(s.jobs))
	finished := 0
	for i := len(s.jobs) - 1; i >= 0; i-- {
		job := s.jobs[i]
		job.Prompt = ""
		if !jobActive(job) {
			if finished >= 50 {
				continue
			}
			finished++
			job.Result, job.RecentOutput, job.TranscriptionLog, job.TranscriptPreview = "", "", "", ""
		}
		jobs = append(jobs, job)
	}
	for i, j := 0, len(jobs)-1; i < j; i, j = i+1, j-1 {
		jobs[i], jobs[j] = jobs[j], jobs[i]
	}
	snapshot.Jobs = jobs
	return snapshot
}
func (s *Service) Job(id string) (Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, job := range s.jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return Job{}, os.ErrNotExist
}
func (s *Service) Subscribe() (<-chan Snapshot, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan Snapshot, 1)
	s.subs[ch] = struct{}{}
	ch <- s.uiSnapshotLocked()
	return ch, func() { s.mu.Lock(); defer s.mu.Unlock(); delete(s.subs, ch); close(ch) }
}

// Each subscriber keeps only the latest snapshot, bounding memory for slow clients.
func (s *Service) broadcastLocked() {
	s.liveDirty = false
	if len(s.subs) == 0 {
		return
	}
	snapshot := s.uiSnapshotLocked()
	for ch := range s.subs {
		select {
		case ch <- snapshot:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- snapshot:
			default:
			}
		}
	}
}
func (s *Service) mutate(id string, fn func(*Job)) {
	s.changesMu.Lock()
	defer s.changesMu.Unlock()
	s.mu.Lock()
	found := false
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			fn(&s.jobs[i])
			s.revision++
			found = true
			break
		}
	}
	s.mu.Unlock()
	if !found {
		return
	}
	// persist records and broadcasts errors; a failed checkpoint remains dirty for retry.
	s.persist()
	s.mu.Lock()
	s.broadcastLocked()
	s.mu.Unlock()
}

// High-frequency output never performs disk I/O. The checkpoint loop coalesces it.
func (s *Service) mutateLive(id string, fn func(*Job)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID != id {
			continue
		}
		fn(&s.jobs[i])
		s.revision++
		s.liveDirty = true
		if time.Since(s.lastLiveEvent) >= 200*time.Millisecond {
			s.broadcastLocked()
			s.lastLiveEvent = time.Now()
		}
		return
	}
}
