package player

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMPVArgs(t *testing.T) {
	got := mpvArgs("/usr/local/bin/mpv", "/tmp/mpv.sock", LaunchOptions{
		MediaURL:     "https://emby.example/stream?api_key=token",
		SubtitleFile: "/tmp/subtitle.ass",
		StartSeconds: 42,
		MediaTitle:   "Demo",
	})
	want := []string{
		"/usr/local/bin/mpv",
		"--input-ipc-server=/tmp/mpv.sock",
		"--idle=yes",
		"--force-window=yes",
		"--keep-open=no",
		"--no-terminal",
		"--osc=yes",
		"--start=42",
		"--force-media-title=Demo",
		"--sub-file=/tmp/subtitle.ass",
		"--",
		"https://emby.example/stream?api_key=token",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEnsureIPCDirUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipc")
	if err := ensureIPCDir(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Fatalf("unexpected directory permissions: %#o", got)
	}
}

func TestResolveMPVBinUsesConfiguredPath(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mpv")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := resolveMPVBin(bin)
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("unexpected executable: %q", got)
	}
}
