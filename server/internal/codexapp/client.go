package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// The Codex app-server speaks one JSON-RPC message per stdio line.
type Client struct {
	cmd      *exec.Cmd
	in       io.WriteCloser
	lines    chan []byte
	errLines chan string
	writeMu  sync.Mutex
	nextID   int
	cancel   context.CancelFunc
	readers  sync.WaitGroup
	readErr  error
}

type Model struct {
	ID                        string `json:"id"`
	Model                     string `json:"model"`
	DisplayName               string `json:"displayName"`
	SupportedReasoningEfforts []struct {
		ReasoningEffort string `json:"reasoningEffort"`
	} `json:"supportedReasoningEfforts"`
}

type Event struct {
	Method string
	Params json.RawMessage
}

type Runner interface {
	Run(context.Context, string, string, string, string, string, func(Event)) (string, error)
	ListModels(context.Context, string) ([]Model, error)
}

type ProcessRunner struct{}

func start(ctx context.Context, binary string) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)
	started := false
	defer func() {
		if !started {
			cancel()
		}
	}()
	cmd := exec.CommandContext(ctx, binary, "app-server", "--listen", "stdio://")
	cmd.Env = os.Environ()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{cmd: cmd, in: in, lines: make(chan []byte, 128), errLines: make(chan string, 64), cancel: cancel}
	started = true
	c.readers.Add(2)
	go func() {
		defer c.readers.Done()
		defer close(c.lines)
		s := bufio.NewScanner(out)
		s.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for s.Scan() {
			// Cancellation must also unblock a full channel after its consumer exits.
			select {
			case c.lines <- append([]byte(nil), s.Bytes()...):
			case <-ctx.Done():
				return
			}
		}
		c.readErr = s.Err()
	}()
	go func() {
		defer c.readers.Done()
		defer close(c.errLines)
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			select {
			case c.errLines <- s.Text():
			default:
			}
		}
	}()
	return c, nil
}

func (c *Client) close() {
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.in.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	c.readers.Wait()
}

// Called only after lines is closed, which also publishes the scanner error.
func (c *Client) readFailure() error {
	if c.readErr != nil {
		return fmt.Errorf("Codex 출력 읽기 실패: %w", c.readErr)
	}
	return errors.New("Codex app-server 연결이 종료되었습니다")
}

func (c *Client) send(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return json.NewEncoder(c.in).Encode(v)
}

func (c *Client) call(ctx context.Context, method string, params any, result any, notify func(Event)) error {
	c.nextID++
	id := c.nextID
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-c.lines:
			if !ok {
				return c.readFailure()
			}
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			if msg.Method != "" {
				if len(msg.ID) > 0 && string(msg.ID) != "null" {
					_ = c.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID), "error": map[string]any{"code": -32601, "message": "Interactive approval is unavailable in read-only mode"}})
				} else if notify != nil {
					notify(Event{Method: msg.Method, Params: msg.Params})
				}
				continue
			}
			var responseID int
			if json.Unmarshal(msg.ID, &responseID) != nil || responseID != id {
				continue
			}
			if msg.Error != nil {
				return fmt.Errorf("%s: %s (%d)", method, msg.Error.Message, msg.Error.Code)
			}
			if result != nil {
				return json.Unmarshal(msg.Result, result)
			}
			return nil
		}
	}
}

func (c *Client) initialize(ctx context.Context) error {
	var response json.RawMessage
	if err := c.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "meet-to-md", "title": "Meet to MD", "version": "0.1.0"}}, &response, nil); err != nil {
		return err
	}
	return c.send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
}

func (ProcessRunner) ListModels(ctx context.Context, binary string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, err := start(ctx, binary)
	if err != nil {
		return nil, err
	}
	defer c.close()
	if err := c.initialize(ctx); err != nil {
		return nil, err
	}
	all := []Model{}
	cursor := ""
	for {
		var response struct {
			Data       []Model `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100, "includeHidden": true}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.call(ctx, "model/list", params, &response, nil); err != nil {
			return nil, err
		}
		all = append(all, response.Data...)
		if response.NextCursor == nil || *response.NextCursor == "" || *response.NextCursor == cursor {
			break
		}
		cursor = *response.NextCursor
		if len(all) > 500 {
			return nil, errors.New("모델 목록 페이지가 너무 많습니다")
		}
	}
	return all, nil
}

func (ProcessRunner) Run(ctx context.Context, binary, model, effort, prompt, cwd string, notify func(Event)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	c, err := start(ctx, binary)
	if err != nil {
		return "", err
	}
	defer c.close()
	if err := c.initialize(ctx); err != nil {
		return "", err
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := c.call(ctx, "thread/start", map[string]any{"cwd": cwd, "ephemeral": true, "serviceName": "meet-to-md", "approvalPolicy": "never", "sandbox": "read-only", "model": model}, &thread, notify); err != nil {
		return "", err
	}
	if thread.Thread.ID == "" {
		return "", errors.New("Codex 스레드 ID가 없습니다")
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	params := map[string]any{"threadId": thread.Thread.ID, "cwd": cwd, "model": model, "effort": effort, "approvalPolicy": "never", "sandboxPolicy": map[string]any{"type": "readOnly"}, "input": []map[string]string{{"type": "text", "text": prompt}}}
	if err := c.call(ctx, "turn/start", params, &turn, notify); err != nil {
		return "", err
	}
	if turn.Turn.ID == "" {
		return "", errors.New("Codex 턴 ID가 없습니다")
	}
	var text string
	for {
		select {
		case <-ctx.Done():
			return text, ctx.Err()
		case line, ok := <-c.lines:
			if !ok {
				return text, c.readFailure()
			}
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(line, &msg) != nil {
				continue
			}
			if len(msg.ID) > 0 && string(msg.ID) != "null" {
				_ = c.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID), "error": map[string]any{"code": -32601, "message": "Interactive approval is unavailable"}})
				continue
			}
			e := Event{Method: msg.Method, Params: msg.Params}
			if notify != nil {
				notify(e)
			}
			switch msg.Method {
			case "item/agentMessage/delta":
				var delta struct {
					Delta string `json:"delta"`
				}
				_ = json.Unmarshal(msg.Params, &delta)
				text += delta.Delta
			case "item/completed":
				var item struct {
					Item struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"item"`
				}
				_ = json.Unmarshal(msg.Params, &item)
				if item.Item.Type == "agentMessage" && item.Item.Text != "" {
					text = item.Item.Text
				}
			case "turn/completed":
				var done struct {
					Turn struct {
						Status string `json:"status"`
						Error  *struct {
							Message string `json:"message"`
						} `json:"error"`
					} `json:"turn"`
				}
				_ = json.Unmarshal(msg.Params, &done)
				if done.Turn.Status != "completed" {
					if done.Turn.Error != nil {
						return text, errors.New(done.Turn.Error.Message)
					}
					return text, fmt.Errorf("Codex 작업 상태: %s", done.Turn.Status)
				}
				if text == "" {
					return "", errors.New("Codex가 빈 결과를 반환했습니다")
				}
				return text, nil
			}
		}
	}
}
