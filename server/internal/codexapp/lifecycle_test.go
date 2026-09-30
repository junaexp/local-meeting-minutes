package codexapp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// The test binary is a portable fake child process; no installed Codex or account is used.
func TestOutputFloodChild(t *testing.T) {
	if os.Getenv("MEET_TO_MD_FLOOD_CHILD") != "1" {
		return
	}
	for i := 0; i < 10000; i++ {
		fmt.Println(strings.Repeat("x", 1024))
	}
	os.Exit(0)
}
func TestCancellationUnblocksFullOutputChannel(t *testing.T) {
	if os.Getenv("MEET_TO_MD_FLOOD_CHILD") == "1" {
		return
	}
	t.Setenv("MEET_TO_MD_FLOOD_CHILD", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := start(ctx, executable)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(3 * time.Second)
	for len(c.lines) < cap(c.lines) {
		select {
		case <-deadline:
			c.close()
			t.Fatal("child did not fill output channel")
		case <-time.After(time.Millisecond):
		}
	}
	done := make(chan struct{})
	go func() { c.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reader remained blocked after cancellation")
	}
}
