package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const mediaProcessHelper = `package main
import (
  "fmt"
  "os"
  "strings"
  "time"
)
func main() {
  args := os.Args[1:]
  for _, arg := range args {
    if arg == "-progress" {
      fmt.Println("out_time=00:00:01.000000")
      fmt.Println("progress=continue")
      if strings.Contains(strings.Join(args, " "), "slow.mp4") { time.Sleep(10*time.Second) }
      if err := os.WriteFile(args[len(args)-1], []byte("wav"), 0600); err != nil { os.Exit(2) }
      fmt.Fprintln(os.Stderr, "ffmpeg warning")
      return
    }
  }
  for i, arg := range args {
    if arg == "-of" && i+1 < len(args) {
      if err := os.WriteFile(args[i+1]+".srt", []byte("1\n00:00:01,000 --> 00:00:02,000\nhello\n"), 0600); err != nil { os.Exit(2) }
      fmt.Println("[00:00:01.000 --> 00:00:02.000] hello")
      fmt.Fprintln(os.Stderr, "whisper ready")
      return
    }
  }
  os.Exit(3)
}
`

func buildMediaProcessHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "helper.go")
	if err := os.WriteFile(source, []byte(mediaProcessHelper), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "media-helper")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binary, source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build media process helper: %v\n%s", err, out)
	}
	return binary
}

func TestProcessTranscriberStreamsStagesAndWritesSRT(t *testing.T) {
	binary := buildMediaProcessHelper(t)
	base := filepath.Join(t.TempDir(), "transcript")
	var events []transcriptionEvent
	var eventsMu sync.Mutex
	err := (processTranscriber{}).Run(context.Background(), "meeting.mp4", binary, binary, "model.bin", base, func(event transcriptionEvent) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base + ".srt")
	if err != nil || !strings.Contains(string(data), "00:00:01,000") {
		t.Fatalf("missing SRT: %q %v", data, err)
	}
	var stages, logs, preview string
	for _, event := range events {
		stages += event.Stage + " "
		logs += event.Log + " "
		preview += event.Text + " "
	}
	if !strings.Contains(stages, "extracting") || !strings.Contains(stages, "transcribing") || !strings.Contains(logs, "음성 추출 위치") || !strings.Contains(logs, "whisper ready") || !strings.Contains(preview, "hello") {
		t.Fatalf("streamed events: %#v", events)
	}
}

func TestProcessTranscriberCancellationStopsChild(t *testing.T) {
	binary := buildMediaProcessHelper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	base := filepath.Join(t.TempDir(), "transcript")
	err := (processTranscriber{}).Run(ctx, "slow.mp4", binary, binary, "model.bin", base, func(transcriptionEvent) {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if _, err := os.Stat(base + ".srt"); !os.IsNotExist(err) {
		t.Fatalf("cancelled process left SRT: %v", err)
	}
}

// Opt in to a local integration run with the installed macOS speech tools and Whisper base model.
func TestProcessTranscriberWithInstalledTools(t *testing.T) {
	if os.Getenv("MEET_TO_MD_REAL_MEDIA_TEST") != "1" {
		t.Skip("set MEET_TO_MD_REAL_MEDIA_TEST=1 to run with installed media tools")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS speech fixture")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	whisper := filepath.Join(root, "whisper", "build", "bin", "whisper-cli")
	model := filepath.Join(root, "whisper", "models", "ggml-base.bin")
	for _, path := range []string{whisper, model} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	aiff := filepath.Join(dir, "speech.aiff")
	media := filepath.Join(dir, "speech.mp3")
	if out, err := exec.Command("say", "-o", aiff, "We decided to meet next Monday. Alice will prepare the agenda.").CombinedOutput(); err != nil {
		t.Fatalf("make speech fixture: %v\n%s", err, out)
	}
	if out, err := exec.Command(ffmpeg, "-nostdin", "-y", "-loglevel", "error", "-i", aiff, media).CombinedOutput(); err != nil {
		t.Fatalf("encode speech fixture: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	base := filepath.Join(dir, "transcript")
	var events []transcriptionEvent
	var eventsMu sync.Mutex
	err = (processTranscriber{}).Run(ctx, media, ffmpeg, whisper, model, base, func(event transcriptionEvent) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base + ".srt")
	if err != nil || !strings.Contains(string(data), " --> ") {
		t.Fatalf("real Whisper did not write timestamped SRT: %q %v; events: %#v", data, err, events)
	}
	var liveText, extractionProgress bool
	for _, event := range events {
		liveText = liveText || strings.Contains(event.Text, " --> ")
		extractionProgress = extractionProgress || strings.Contains(event.Log, "음성 추출 위치")
	}
	if !liveText || !extractionProgress {
		t.Fatalf("real process did not stream transcription and extraction progress: %#v", events)
	}
}
