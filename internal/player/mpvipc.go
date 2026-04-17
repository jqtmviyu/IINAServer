package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/model"
)

type IPCClient struct {
	socketPath string
	timeout    time.Duration
}

type ObserveOptions struct {
	Interval      time.Duration
	OnSample      func(model.ProgressSample)
	WaitForAny    bool
	StopOnIPCExit bool
}

type ipcCommand struct {
	Command []any `json:"command"`
}

type ipcResponse struct {
	Data  any    `json:"data"`
	Error string `json:"error"`
}

func NewIPCClient(socketPath string, timeout time.Duration) *IPCClient {
	return &IPCClient{socketPath: socketPath, timeout: timeout}
}

func (c *IPCClient) WaitUntilReady(ctx context.Context) error {
	deadline := time.NewTimer(c.timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(c.socketPath); err == nil {
			conn, err := net.DialTimeout("unix", c.socketPath, 500*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("ipc socket not ready: %s", c.socketPath)
		case <-ticker.C:
		}
	}
}

func (c *IPCClient) Observe(ctx context.Context, opts ObserveOptions) error {
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		sample, err := c.ReadSample()
		if err == nil {
			if opts.OnSample != nil {
				opts.OnSample(sample)
			}
			if opts.WaitForAny {
				return nil
			}
		} else if !opts.WaitForAny {
			if opts.StopOnIPCExit && isIPCStoppedError(err) {
				return nil
			}
			if isIdleStopError(c, err) {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *IPCClient) ReadSample() (model.ProgressSample, error) {
	position, err := c.getFloat("time-pos")
	if err != nil {
		return model.ProgressSample{}, err
	}
	duration, _ := c.getFloat("duration")
	paused, _ := c.getBool("pause")
	title, _ := c.getString("media-title")
	return model.ProgressSample{
		At:              time.Now(),
		PositionSeconds: position,
		DurationSeconds: duration,
		Paused:          paused,
		MediaTitle:      title,
	}, nil
}

func (c *IPCClient) Command(command ...any) error {
	_, err := c.call(command)
	return err
}

func (c *IPCClient) LoadFile(mediaURL string) error {
	return c.Command("loadfile", mediaURL, "replace")
}

func (c *IPCClient) SetProperty(name string, value any) error {
	return c.Command("set_property", name, value)
}

func (c *IPCClient) SeekAbsolute(seconds int) error {
	return c.Command("seek", seconds, "absolute+exact")
}

func (c *IPCClient) StopPlayback() error {
	return c.Command("stop")
}

func (c *IPCClient) AddSubtitle(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return c.Command("sub-add", path, "select")
}

func (c *IPCClient) Quit() error {
	return c.Command("quit")
}

func (c *IPCClient) WindowConfigured() (bool, error) {
	return c.getBool("vo-configured")
}

func (c *IPCClient) WindowID() (int64, error) {
	return c.getInt("window-id")
}

func (c *IPCClient) IdleActive() (bool, error) {
	return c.getBool("idle-active")
}

func (c *IPCClient) Seekable() (bool, error) {
	return c.getBool("seekable")
}

func isIPCStoppedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if errors.Is(netErr.Err, os.ErrNotExist) {
			return true
		}
	}
	return strings.Contains(err.Error(), "connect: connection refused") || strings.Contains(err.Error(), "no such file or directory")
}

func isIdleStopError(client *IPCClient, err error) bool {
	if client == nil {
		return false
	}
	idle, idleErr := client.IdleActive()
	return isIdleStopErrorResult(err, idle, idleErr)
}

func isIdleStopErrorResult(err error, idle bool, idleErr error) bool {
	if err == nil || !strings.Contains(err.Error(), "property missing") {
		return false
	}
	return idleErr == nil && idle
}

func (c *IPCClient) getFloat(name string) (float64, error) {
	resp, err := c.call([]any{"get_property", name})
	if err != nil {
		return 0, err
	}
	switch val := resp.Data.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case nil:
		return 0, errors.New("property missing")
	default:
		return 0, fmt.Errorf("unexpected float type %T", val)
	}
}

func (c *IPCClient) getInt(name string) (int64, error) {
	resp, err := c.call([]any{"get_property", name})
	if err != nil {
		return 0, err
	}
	switch val := resp.Data.(type) {
	case float64:
		return int64(val), nil
	case int:
		return int64(val), nil
	case int64:
		return val, nil
	case nil:
		return 0, errors.New("property missing")
	default:
		return 0, fmt.Errorf("unexpected int type %T", val)
	}
}

func (c *IPCClient) getBool(name string) (bool, error) {
	resp, err := c.call([]any{"get_property", name})
	if err != nil {
		return false, err
	}
	if val, ok := resp.Data.(bool); ok {
		return val, nil
	}
	return false, fmt.Errorf("unexpected bool type %T", resp.Data)
}

func (c *IPCClient) getString(name string) (string, error) {
	resp, err := c.call([]any{"get_property", name})
	if err != nil {
		return "", err
	}
	if val, ok := resp.Data.(string); ok {
		return val, nil
	}
	return "", fmt.Errorf("unexpected string type %T", resp.Data)
}

func (c *IPCClient) call(command []any) (ipcResponse, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		logIPCFailure(c.socketPath, command, err)
		return ipcResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	payload, err := json.Marshal(ipcCommand{Command: command})
	if err != nil {
		logIPCFailure(c.socketPath, command, err)
		return ipcResponse{}, err
	}
	payload = append(payload, '\n')
	if _, err = conn.Write(payload); err != nil {
		logIPCFailure(c.socketPath, command, err)
		return ipcResponse{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		logIPCFailure(c.socketPath, command, err)
		return ipcResponse{}, err
	}
	var resp ipcResponse
	if err = json.Unmarshal(line, &resp); err != nil {
		logIPCFailure(c.socketPath, command, err)
		return ipcResponse{}, err
	}
	if resp.Error != "" && resp.Error != "success" {
		err = fmt.Errorf("ipc error: %s", resp.Error)
		logIPCFailure(c.socketPath, command, err)
		return ipcResponse{}, err
	}
	return resp, nil
}

func logIPCFailure(socketPath string, command []any, err error) {
	fmt.Printf("event=ipc.command.fail socket_path=%q ipc_command=%q ipc_args=%q err=%q\n", socketPath, commandName(command), commandArgsSummary(command), err)
}

func commandName(command []any) string {
	if len(command) == 0 {
		return ""
	}
	name, _ := command[0].(string)
	return name
}

func commandArgsSummary(command []any) string {
	if len(command) <= 1 {
		return ""
	}
	parts := make([]string, 0, len(command)-1)
	for _, arg := range command[1:] {
		switch typed := arg.(type) {
		case string:
			value := typed
			if len(value) > 120 {
				value = value[:117] + "..."
			}
			parts = append(parts, value)
		default:
			parts = append(parts, fmt.Sprintf("%v", typed))
		}
	}
	return strings.Join(parts, " | ")
}
