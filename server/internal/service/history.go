package service

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// persist serializes writers and snapshots under mu, then releases mu before encoding,
// fsync and rename. A newer revision remains dirty when output arrives during a write.
func (s *Service) persist() error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	s.mu.RLock()
	revision := s.revision
	if revision == s.savedRevision && s.storageError == "" {
		s.mu.RUnlock()
		return nil
	}
	jobs := append([]Job{}, s.jobs...)
	s.mu.RUnlock()
	err := s.writeHistory(s.storePath, jobs)
	s.mu.Lock()
	previous := s.storageError
	if err != nil {
		s.storageError = "작업 기록을 저장하지 못했습니다. 저장 공간과 폴더 권한을 확인해 주세요."
	} else {
		s.savedRevision = revision
		s.storageError = ""
	}
	if previous != s.storageError {
		s.broadcastLocked()
	}
	s.mu.Unlock()
	if err != nil && previous == "" {
		log.Printf("작업 기록 저장 실패: %v", err)
	}
	return err
}

// writeHistoryFile preserves the previous valid history until the new file is complete.
func writeHistoryFile(path string, jobs []Job) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(jobs)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".jobs-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Flush the trailing throttled event even when a process goes quiet, and checkpoint
// dirty logs every two seconds. The owner waits for the final flush on shutdown.
func (s *Service) checkpointLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	events := time.NewTicker(200 * time.Millisecond)
	defer events.Stop()
	writes := time.NewTicker(2 * time.Second)
	defer writes.Stop()
	for {
		select {
		case <-ctx.Done():
			s.persist()
			return
		case <-writes.C:
			s.persist()
		case <-events.C:
			s.mu.Lock()
			if s.liveDirty {
				s.broadcastLocked()
				s.lastLiveEvent = time.Now()
			}
			s.mu.Unlock()
		}
	}
}

// restoreMembership rolls back a failed API edit without losing live output
// that arrived while its disk write was in progress. Caller holds changesMu.
func (s *Service) restoreMembership(previous []Job) {
	s.mu.Lock()
	current := make(map[string]Job, len(s.jobs))
	for _, j := range s.jobs {
		current[j.ID] = j
	}
	for i, j := range previous {
		if latest, ok := current[j.ID]; ok {
			previous[i] = latest
		}
	}
	s.jobs = previous
	s.revision++
	s.broadcastLocked()
	s.mu.Unlock()
}
