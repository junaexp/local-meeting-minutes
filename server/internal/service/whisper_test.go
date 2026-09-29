package service

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeArchive(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("binary")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestExtractZipRejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	if err := extractZip(makeArchive(t, "../outside"), dest, false); err == nil {
		t.Fatal("zip traversal accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "outside")); !os.IsNotExist(err) {
		t.Fatal("archive wrote outside destination")
	}
}
func TestExtractZipPreservesBinaryDirectory(t *testing.T) {
	dest := t.TempDir()
	if err := extractZip(makeArchive(t, "bundle/bin/whisper-cli.exe"), dest, false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "bundle", "bin", "whisper-cli.exe")); err != nil || string(b) != "binary" {
		t.Fatalf("extracted file: %s %v", b, err)
	}
}

func TestDownloadReportsBytesAndWritesCompletedFile(t *testing.T) {
	content := bytes.Repeat([]byte("meeting"), 50000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "350000")
		_, _ = w.Write(content)
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "model.bin")
	var lastReceived, lastTotal int64
	var calls int
	err := downloadWithProgress(context.Background(), server.URL, target, int64(len(content)+1), func(received, total int64) {
		if received < lastReceived {
			t.Fatalf("download progress moved backward: %d to %d", lastReceived, received)
		}
		lastReceived, lastTotal = received, total
		calls++
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 || lastReceived != int64(len(content)) || lastTotal != int64(len(content)) {
		t.Fatalf("wrong progress: calls=%d received=%d total=%d", calls, lastReceived, lastTotal)
	}
	if data, err := os.ReadFile(target); err != nil || !bytes.Equal(data, content) {
		t.Fatalf("downloaded file mismatch: %v", err)
	}
}

func TestWhisperInstallReportsCompletedModel(t *testing.T) {
	_, service, _, _ := setupMedia(t)
	if err := service.StartWhisperInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	for {
		state := service.Snapshot()
		if !state.WhisperInstalling {
			if state.WhisperInstall.Stage != "completed" || state.WhisperInstall.Model != "large-v3-turbo" || !state.WhisperReady {
				t.Fatalf("wrong installation result: %+v", state)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("installation did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestWhisperInstallReportsFailureAndAllowsRetry(t *testing.T) {
	_, service, _ := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.StartWhisperInstall(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	for {
		state := service.Snapshot()
		if !state.WhisperInstalling {
			if state.WhisperInstall.Stage != "failed" || state.WhisperError == "" || state.WhisperInstall.Model != "large-v3-turbo" {
				t.Fatalf("wrong installation failure: %+v", state)
			}
			if err := service.StartWhisperInstall(ctx); err != nil {
				t.Fatalf("retry was blocked: %v", err)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("installation did not fail")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for service.Snapshot().WhisperInstalling {
		select {
		case <-deadline:
			t.Fatal("retry did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
