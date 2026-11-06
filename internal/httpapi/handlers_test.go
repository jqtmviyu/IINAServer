package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalOriginReflectsBrowserOrigin(t *testing.T) {
	mux := http.NewServeMux()
	Handler{}.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/healthz", nil)
	request.Header.Set("Origin", "https://emby.example")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://emby.example" {
		t.Fatalf("unexpected allow-origin: %q", got)
	}
}

func TestLocalOriginRejectsInvalidOrigin(t *testing.T) {
	mux := http.NewServeMux()
	Handler{}.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/healthz", nil)
	request.Header.Set("Origin", "null")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow-origin: %q", got)
	}
}

func TestLocalOriginRejectsNonLoopbackHost(t *testing.T) {
	mux := http.NewServeMux()
	Handler{}.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "http://evil.example/healthz", nil)
	request.Header.Set("Origin", "https://emby.example")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestControlRouteRejectsCrossSiteRequestWithoutOrigin(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	Handler{
		DirectPlay: func(w http.ResponseWriter, _ *http.Request) {
			called = true
		},
	}.Register(mux)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/play?video=https://media.example/video", nil)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if called {
		t.Fatal("cross-site request reached the player handler")
	}
}

func TestLocalOriginHandlesPreflight(t *testing.T) {
	mux := http.NewServeMux()
	Handler{}.Register(mux)
	request := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:8080/healthz", nil)
	request.Header.Set("Origin", "https://emby.example")
	request.Header.Set("Access-Control-Request-Private-Network", "true")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" {
		t.Fatalf("unexpected allow-methods: %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Fatalf("unexpected private-network allowance: %q", got)
	}
}
