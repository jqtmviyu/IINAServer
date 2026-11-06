package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIPCDirUsesApplicationCachePath(t *testing.T) {
	got := defaultIPCDir()
	if !strings.HasSuffix(got, filepath.Join("iinaserver", "ipc")) {
		t.Fatalf("unexpected IPC directory: %q", got)
	}
}
