package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"meet-to-md/server/internal/config"
	"meet-to-md/server/internal/service"
)

func testAPI(t *testing.T) (http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "server")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Prompt = "회의록"
	path := filepath.Join(dir, "config.toml")
	if err := config.Save(path, c); err != nil {
		t.Fatal(err)
	}
	s, err := service.New(root, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return (API{Service: s, Root: root, Context: context.Background()}).Router(), root
}
func serve(handler http.Handler, method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	r.Host = "127.0.0.1:8791"
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestEmptyStateUsesArray(t *testing.T) {
	h, root := testAPI(t)
	got := serve(h, "GET", "/api/state", nil, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("state: %d %s", got.Code, got.Body.String())
	}
	var state struct {
		Jobs json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if string(state.Jobs) != "[]" {
		t.Fatalf("empty jobs must be an array, got %s", state.Jobs)
	}
	stored, err := os.ReadFile(filepath.Join(root, "server", "data", "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(stored)) != "[]" {
		t.Fatalf("empty job store must be an array, got %s", stored)
	}
}

func TestEnvironmentReportsWhisperModelPath(t *testing.T) {
	h, root := testAPI(t)
	got := serve(h, "GET", "/api/environment", nil, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("environment: %d %s", got.Code, got.Body.String())
	}
	var environment struct {
		Whisper struct {
			Model      string `json:"model"`
			ModelPath  string `json:"modelPath"`
			ModelReady bool   `json:"modelReady"`
		} `json:"whisper"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &environment); err != nil {
		t.Fatal(err)
	}
	if environment.Whisper.Model != "large-v3-turbo" || environment.Whisper.ModelReady {
		t.Fatalf("unexpected Whisper status: %+v", environment.Whisper)
	}
	want := filepath.Join(root, "whisper", "models", "ggml-large-v3-turbo.bin")
	if environment.Whisper.ModelPath != want {
		t.Fatalf("model path: got %q, want %q", environment.Whisper.ModelPath, want)
	}
}

func TestLocalBoundaryAndFileFlow(t *testing.T) {
	h, root := testAPI(t)
	file := filepath.Join(root, "meeting.srt")
	_ = os.WriteFile(file, []byte("원문 테스트"), 0600)
	if got := serve(h, "GET", "/api/preview?path="+file, nil, nil); got.Code != 200 || !strings.Contains(got.Body.String(), "원문 테스트") {
		t.Fatalf("preview: %d %s", got.Code, got.Body.String())
	}
	if got := serve(h, "GET", "/api/browse?path="+root, nil, nil); got.Code != 200 || !strings.Contains(got.Body.String(), "meeting.srt") {
		t.Fatalf("browse: %d %s", got.Code, got.Body.String())
	}
	data, _ := json.Marshal(map[string]any{"paths": []string{file}})
	if got := serve(h, "POST", "/api/expand", data, nil); got.Code != 403 {
		t.Fatalf("CSRF header not required: %d", got.Code)
	}
	if got := serve(h, "POST", "/api/expand", data, map[string]string{"X-Meet-To-MD": "1", "Origin": "https://foreign.example"}); got.Code != 403 {
		t.Fatalf("foreign origin accepted: %d", got.Code)
	}
	if got := serve(h, "POST", "/api/expand", data, map[string]string{"X-Meet-To-MD": "1"}); got.Code != 200 || !strings.Contains(got.Body.String(), "meeting.srt") {
		t.Fatalf("expand: %d %s", got.Code, got.Body.String())
	}
	create, _ := json.Marshal(map[string]any{"paths": []string{file}, "model": "gpt-5.6-sol", "effort": "medium"})
	if got := serve(h, "POST", "/api/jobs", create, map[string]string{"X-Meet-To-MD": "1"}); got.Code != 201 || !strings.Contains(got.Body.String(), "queued") {
		t.Fatalf("create: %d %s", got.Code, got.Body.String())
	}
	if got := serve(h, "GET", "/api/state", nil, nil); got.Code != 200 || !strings.Contains(got.Body.String(), "meeting.srt") {
		t.Fatalf("state: %d %s", got.Code, got.Body.String())
	}
}

func TestInvalidConfigAndBrowserPath(t *testing.T) {
	h, _ := testAPI(t)
	if got := serve(h, "GET", "/api/browse?path=relative", nil, nil); got.Code != 400 {
		t.Fatalf("relative browse accepted: %d", got.Code)
	}
	data := []byte(`{"listen":"0.0.0.0:8791","codexBinary":"codex","prompt":"회의록","outputDir":"","whisperModel":"base","ffmpegBinary":"ffmpeg"}`)
	if got := serve(h, "PUT", "/api/config", data, map[string]string{"X-Meet-To-MD": "1"}); got.Code != 400 {
		t.Fatalf("nonlocal listen accepted: %d", got.Code)
	}
}
