package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jqtmviyu/iinaServer/internal/config"
)

func LaunchMPV(ctx context.Context, cfg config.Config, opt LaunchOptions) (*LaunchResult, error) {
	bin, err := resolveMPVBin(cfg.MPVBin)
	if err != nil {
		return nil, err
	}
	if err := ensureIPCDir(cfg.IPCDir); err != nil {
		return nil, err
	}
	socketPath := sessionSocketPath(cfg, "mpv", opt.SessionID)
	_ = os.Remove(socketPath)
	args := mpvArgs(bin, socketPath, opt)
	cmd := exec.CommandContext(ctx, bin, args[1:]...)
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

func sessionSocketPath(cfg config.Config, player string, sessionID string) string {
	safeID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.TrimSpace(sessionID))
	if safeID == "" {
		safeID = fmt.Sprintf("%d", os.Getpid())
	}
	return filepath.Join(cfg.IPCDir, player+"-"+safeID+".sock")
}

func mpvArgs(bin string, socketPath string, opt LaunchOptions) []string {
	args := []string{
		bin,
		"--input-ipc-server=" + socketPath,
		"--idle=yes",
		"--force-window=yes",
		"--keep-open=no",
		"--no-terminal",
		"--osc=yes",
	}
	if opt.StartSeconds > 0 {
		args = append(args, fmt.Sprintf("--start=%d", opt.StartSeconds))
	}
	if opt.MediaTitle != "" {
		args = append(args, "--force-media-title="+opt.MediaTitle)
	}
	if opt.SubtitleFile != "" {
		args = append(args, "--sub-file="+opt.SubtitleFile)
	}
	return append(args, "--", opt.MediaURL)
}

func ensureIPCDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func resolveMPVBin(configured string) (string, error) {
	candidates := []string{strings.TrimSpace(configured)}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", "mpv"))
	}
	candidates = append(candidates, "mpv")
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
	return "", fmt.Errorf("mpv executable not found")
}
