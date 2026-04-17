package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/config"
)

type LaunchOptions struct {
	MediaURL     string
	SubtitleFile string
	StartSeconds int
	MediaTitle   string
	SessionID    string
}

type LaunchResult struct {
	Cmd        *exec.Cmd
	Args       []string
	SocketPath string
	PID        int
}

func LaunchIINA(ctx context.Context, cfg config.Config, opt LaunchOptions) (*LaunchResult, error) {
	bin, err := resolveIINABin(cfg.IINABin)
	if err != nil {
		return nil, err
	}
	socketPath := filepath.Join(cfg.IPCDir, fmt.Sprintf("iinaserver-%s.sock", opt.SessionID))
	_ = os.Remove(socketPath)
	args := []string{bin, "--no-stdin", opt.MediaURL}
	args = append(args, "--mpv-input-ipc-server="+socketPath)
	if opt.StartSeconds > 0 {
		args = append(args, fmt.Sprintf("--mpv-start=%d", opt.StartSeconds))
	}
	if opt.MediaTitle != "" {
		args = append(args, "--mpv-force-media-title="+opt.MediaTitle)
	}
	if opt.SubtitleFile != "" {
		args = append(args, "--mpv-sub-files="+opt.SubtitleFile)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &LaunchResult{
		Cmd:        cmd,
		Args:       append([]string(nil), args...),
		SocketPath: socketPath,
		PID:        cmd.Process.Pid,
	}, nil
}

func resolveIINABin(configured string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(configured) != "" {
		candidates = append(candidates, strings.TrimSpace(configured))
	}
	candidates = append(candidates,
		"/Users/nuc/.local/bin/iina-cli",
		"/Applications/IINA.app/Contents/MacOS/iina-cli",
		"iina-cli",
		"iina",
	)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "/") {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("iina executable not found")
}

func WaitForFirstSample(ctx context.Context, socketPath string) error {
	client := NewIPCClient(socketPath, 12*time.Second)
	if err := client.WaitUntilReady(ctx); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return client.Observe(waitCtx, ObserveOptions{
		Interval:   1 * time.Second,
		WaitForAny: true,
	})
}
