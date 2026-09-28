package codexapp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestJSONRPCResponseAndNotification(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	c := &Client{in: writer, lines: make(chan []byte, 4)}
	go func() {
		buffer := make([]byte, 4096)
		n, _ := reader.Read(buffer)
		if !strings.Contains(string(buffer[:n]), `"method":"model/list"`) {
			t.Errorf("unexpected request: %s", buffer[:n])
		}
		c.lines <- []byte(`{"method":"model/verification","params":{"ok":true}}`)
		c.lines <- []byte(`{"id":1,"result":{"data":[{"model":"gpt-6-sol"}]}}`)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var result struct {
		Data []struct {
			Model string `json:"model"`
		} `json:"data"`
	}
	seen := false
	if err := c.call(ctx, "model/list", map[string]any{}, &result, func(e Event) { seen = e.Method == "model/verification" }); err != nil {
		t.Fatal(err)
	}
	if !seen || len(result.Data) != 1 || result.Data[0].Model != "gpt-6-sol" {
		t.Fatalf("response=%#v seen=%v", result, seen)
	}
}
func TestJSONRPCError(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	c := &Client{in: writer, lines: make(chan []byte, 1)}
	go func() {
		buf := make([]byte, 4096)
		_, _ = reader.Read(buf)
		c.lines <- []byte(`{"id":1,"error":{"code":-32602,"message":"bad effort"}}`)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var result json.RawMessage
	if err := c.call(ctx, "turn/start", nil, &result, nil); err == nil || !strings.Contains(err.Error(), "bad effort") {
		t.Fatalf("unexpected error: %v", err)
	}
}
