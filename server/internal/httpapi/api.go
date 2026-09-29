package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"meet-to-md/server/internal/config"
	"meet-to-md/server/internal/service"
)

type API struct {
	Service *service.Service
	Root    string
	Context context.Context
}

func (a API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(localOnly)
	r.Route("/api", func(r chi.Router) {
		r.Get("/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, a.Service.UISnapshot()) })
		r.Get("/events", a.events)
		r.Get("/config", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, a.Service.Config()) })
		r.Get("/environment", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, a.Service.Environment()) })
		r.Put("/config", a.updateConfig)
		r.Get("/browse", a.browse)
		r.Post("/expand", a.expand)
		r.Get("/preview", a.preview)
		r.Post("/import", a.importFile)
		r.Post("/jobs", a.createJobs)
		r.Delete("/jobs", a.deleteFinishedJobs)
		r.Post("/transcriptions", a.createTranscription)
		r.Delete("/jobs/{id}", a.deleteJob)
		r.Get("/jobs/{id}", a.job)
		r.Post("/jobs/{id}/cancel", a.cancelJob)
		r.Get("/jobs/{id}/transcript", a.jobTranscript)
		r.Get("/models", a.models)
		r.Post("/codex/test", a.testCodex)
		r.Post("/whisper/install", a.installWhisper)
	})
	dist := filepath.Join(a.Root, "ui", "dist")
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dist, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		index := filepath.Join(dist, "index.html")
		if _, err := os.Stat(index); err == nil {
			http.ServeFile(w, r, index)
			return
		}
		http.Error(w, "UI 빌드가 없습니다. ui에서 npm run build를 실행하세요.", http.StatusNotFound)
	})
	return r
}
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if strings.HasPrefix(host, "[") {
			host = strings.TrimPrefix(strings.SplitN(host, "]", 2)[0], "[")
		} else {
			host = strings.Split(host, ":")[0]
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			http.Error(w, "localhost only", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin != "" && !strings.HasPrefix(origin, "http://localhost:") && !strings.HasPrefix(origin, "http://127.0.0.1:") && !strings.HasPrefix(origin, "http://[::1]:") {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
			if r.Header.Get("X-Meet-To-MD") != "1" {
				http.Error(w, "missing local request header", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func bad(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	d := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func (a API) updateConfig(w http.ResponseWriter, r *http.Request) {
	var c config.Config
	if err := decode(r, &c); err != nil {
		bad(w, 400, err)
		return
	}
	if err := a.Service.UpdateConfig(c); err != nil {
		bad(w, 400, err)
		return
	}
	writeJSON(w, 200, a.Service.Config())
}
func (a API) browse(w http.ResponseWriter, r *http.Request) {
	path, items, err := service.Browse(r.URL.Query().Get("path"))
	if err != nil {
		bad(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"path": path, "parent": filepath.Dir(path), "entries": items})
}

func (a API) expand(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths []string `json:"paths"`
	}
	if err := decode(r, &in); err != nil {
		bad(w, 400, err)
		return
	}
	paths, err := service.Expand(in.Paths)
	if err != nil {
		bad(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"paths": paths})
}
func (a API) preview(w http.ResponseWriter, r *http.Request) {
	text, transcribed, err := a.Service.PreviewState(r.URL.Query().Get("path"))
	if err != nil {
		bad(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"text": text, "transcribed": transcribed})
}
func (a API) importFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1000*1024*1024+1024*1024)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		bad(w, 400, err)
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["files"]
	if len(files) == 0 || len(files) > 100 {
		bad(w, 400, errors.New("1~100개 파일을 선택해 주세요"))
		return
	}
	paths := []string{}
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			bad(w, 400, err)
			return
		}
		path, err := a.Service.Import(header.Filename, file)
		file.Close()
		if err != nil {
			bad(w, 400, err)
			return
		}
		paths = append(paths, path)
	}
	writeJSON(w, 201, map[string]any{"paths": paths})
}
func (a API) createJobs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths  []string `json:"paths"`
		Model  string   `json:"model"`
		Effort string   `json:"effort"`
	}
	if err := decode(r, &in); err != nil {
		bad(w, 400, err)
		return
	}
	jobs, err := a.Service.Enqueue(in.Paths, in.Model, in.Effort)
	if err != nil {
		bad(w, 400, err)
		return
	}
	writeJSON(w, 201, map[string]any{"jobs": jobs})
}
func (a API) createTranscription(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
	}
	if err := decode(r, &in); err != nil {
		bad(w, 400, err)
		return
	}
	job, err := a.Service.EnqueueTranscription(in.Path)
	if err != nil {
		bad(w, 400, err)
		return
	}
	writeJSON(w, 201, map[string]any{"job": job})
}
func (a API) jobTranscript(w http.ResponseWriter, r *http.Request) {
	text, err := a.Service.JobTranscript(chi.URLParam(r, "id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		bad(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}
func (a API) deleteJob(w http.ResponseWriter, r *http.Request) {
	if err := a.Service.Delete(chi.URLParam(r, "id")); err != nil {
		status := 409
		if errors.Is(err, os.ErrNotExist) {
			status = 404
		}
		bad(w, status, err)
		return
	}
	w.WriteHeader(204)
}
func (a API) deleteFinishedJobs(w http.ResponseWriter, r *http.Request) {
	deleted, err := a.Service.DeleteFinished()
	if err != nil {
		bad(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "state": a.Service.UISnapshot()})
}
func (a API) cancelJob(w http.ResponseWriter, r *http.Request) {
	if err := a.Service.Cancel(chi.URLParam(r, "id")); err != nil {
		status := 409
		if errors.Is(err, os.ErrNotExist) {
			status = 404
		}
		bad(w, status, err)
		return
	}
	writeJSON(w, 200, a.Service.UISnapshot())
}
func (a API) job(w http.ResponseWriter, r *http.Request) {
	job, err := a.Service.Job(chi.URLParam(r, "id"))
	if err != nil {
		bad(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
func (a API) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		bad(w, 500, errors.New("streaming unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	events, unsubscribe := a.Service.Subscribe()
	defer unsubscribe()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case snapshot := <-events:
			data, _ := json.Marshal(snapshot)
			if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(w, "event: heartbeat\ndata: {}\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
func (a API) models(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	models, err := a.Service.Models(ctx)
	if err != nil {
		bad(w, 503, err)
		return
	}
	writeJSON(w, 200, map[string]any{"models": models})
}
func (a API) testCodex(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Model string `json:"model"`
	}
	if err := decode(r, &in); err != nil {
		bad(w, 400, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, models, err := a.Service.TestCodex(ctx, in.Model)
	if err != nil {
		bad(w, 503, err)
		return
	}
	writeJSON(w, 200, map[string]any{"result": result, "models": models})
}
func (a API) installWhisper(w http.ResponseWriter, r *http.Request) {
	if err := a.Service.StartWhisperInstall(a.Context); err != nil {
		bad(w, 409, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "installing"})
}
