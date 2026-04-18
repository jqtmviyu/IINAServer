package player

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsIdleStopError(t *testing.T) {
	client := &IPCClient{}
	_ = client
	if !isIdleStopErrorResult(errors.New("property missing"), true, nil) {
		t.Fatal("expected idle stop error")
	}
	if isIdleStopErrorResult(errors.New("property missing"), false, nil) {
		t.Fatal("unexpected idle stop error when not idle")
	}
	if isIdleStopErrorResult(errors.New("other error"), true, nil) {
		t.Fatal("unexpected idle stop error for other error")
	}
	if isIdleStopErrorResult(errors.New("property missing"), false, errors.New("read idle failed")) {
		t.Fatal("unexpected idle stop error when idle check fails")
	}
}

func TestIINABinCandidates(t *testing.T) {
	configured := "/custom/iina-cli"
	candidates := iinaBinCandidates(configured)
	if len(candidates) < 4 {
		t.Fatalf("unexpected candidate count: %d", len(candidates))
	}
	if candidates[0] != configured {
		t.Fatalf("configured path not first: %#v", candidates)
	}
	if candidates[len(candidates)-3] != "/Applications/IINA.app/Contents/MacOS/iina-cli" {
		t.Fatalf("unexpected app candidate order: %#v", candidates)
	}
	if candidates[len(candidates)-2] != "iina-cli" || candidates[len(candidates)-1] != "iina" {
		t.Fatalf("unexpected path lookup order: %#v", candidates)
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		want := filepath.Join(home, ".local", "bin", "iina-cli")
		found := false
		for _, candidate := range candidates {
			if candidate == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("home candidate missing: want %q candidates=%#v", want, candidates)
		}
	}
}
