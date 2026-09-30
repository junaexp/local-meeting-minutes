package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"meet-to-md/server/internal/httpapi"
	"meet-to-md/server/internal/service"
)

func main() {
	configPath := flag.String("config", "", "config.toml path")
	flag.Parse()
	path := *configPath
	if path == "" {
		candidates := []string{"config.toml", filepath.Join("server", "config.toml")}
		if executable, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(executable), "config.toml"))
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		log.Fatal("server/config.toml을 찾을 수 없습니다. -config 경로를 지정하세요")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		log.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(path))
	s, err := service.New(root, path, nil)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); s.Run(ctx) }()
	server := &http.Server{Addr: s.Config().Listen, Handler: (httpapi.API{Service: s, Root: root, Context: ctx}).Router(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Meet to MD: http://%s", s.Config().Listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	// Wait for cancellation and the final history checkpoint before the process exits.
	stop()
	select {
	case <-workerDone:
	case <-time.After(10 * time.Second):
		log.Print("작업 기록 종료 대기 시간이 초과되었습니다")
	}
}
