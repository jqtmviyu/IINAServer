package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jqtmviyu/iinaServer/internal/config"
	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/player"
	"github.com/jqtmviyu/iinaServer/internal/session"
)

func TestLaunchPlayerSelectsConfiguredBackend(t *testing.T) {
	dir := t.TempDir()
	iinaBin := filepath.Join(dir, "iina")
	mpvBin := filepath.Join(dir, "mpv")
	writeFakePlayer(t, iinaBin)
	writeFakePlayer(t, mpvBin)
	cfg := config.Config{
		IINABin: iinaBin,
		MPVBin:  mpvBin,
		IPCDir:  filepath.Join(dir, "ipc"),
	}
	opt := player.LaunchOptions{MediaURL: "https://emby.example/video"}

	for _, test := range []struct {
		name       string
		playerType string
		wantArg    string
	}{
		{name: "IINA", playerType: model.PlayerIINA, wantArg: "--mpv-input-ipc-server="},
		{name: "MPV", playerType: model.PlayerMPV, wantArg: "--input-ipc-server="},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := launchPlayer(context.Background(), cfg, test.playerType, opt)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Cmd.Wait()
			if result.Args[0] != map[string]string{model.PlayerIINA: iinaBin, model.PlayerMPV: mpvBin}[test.playerType] {
				t.Fatalf("unexpected executable: %q", result.Args[0])
			}
			if !containsArgPrefix(result.Args, test.wantArg) {
				t.Fatalf("expected %q in args: %#v", test.wantArg, result.Args)
			}
		})
	}
}

func TestReuseExistingPlayerRequiresSameBackend(t *testing.T) {
	sessions := session.NewManager()
	sessions.SetPlayer(&session.PlayerInstance{Player: model.PlayerIINA, SocketPath: "/unused.sock"})

	reused, _, err := reuseExistingPlayer(context.Background(), config.Config{}, model.PlayPlan{Player: model.PlayerMPV}, sessions)
	if err == nil {
		t.Fatal("expected backend mismatch")
	}
	if reused {
		t.Fatal("reused IINA for MPV request")
	}
}

func writeFakePlayer(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func containsArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}
