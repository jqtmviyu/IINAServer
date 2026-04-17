package player

import (
	"errors"
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
