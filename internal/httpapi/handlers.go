package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/review"
)

type DirectPlayFunc func(http.ResponseWriter, *http.Request)
type EmbyPlayFunc func(http.ResponseWriter, *http.Request)
type StopFunc func(http.ResponseWriter, *http.Request)

type Handler struct {
	Store      *review.Store
	DirectPlay DirectPlayFunc
	EmbyPlay   EmbyPlayFunc
	Stop       StopFunc
}

func (h Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.withLocalOrigin(h.Healthz, false))
	mux.HandleFunc("/v1/review/sessions/latest", h.withLocalOrigin(h.GetLatestReview, false))
	mux.HandleFunc("/v1/review/sessions/", h.withLocalOrigin(h.GetSessionReview, false))
	if h.DirectPlay != nil {
		mux.HandleFunc("/play", h.withLocalOrigin(http.HandlerFunc(h.DirectPlay), true))
	}
	if h.EmbyPlay != nil {
		mux.HandleFunc("/v1/emby/play", h.withLocalOrigin(http.HandlerFunc(h.EmbyPlay), true))
	}
	if h.Stop != nil {
		mux.HandleFunc("/v1/session/stop", h.withLocalOrigin(http.HandlerFunc(h.Stop), true))
	}
}

func (h Handler) withLocalOrigin(next http.HandlerFunc, control bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "local host required", http.StatusForbidden)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			normalized, ok := normalizeOrigin(origin)
			if !ok {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", normalized)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if strings.EqualFold(r.Header.Get("Access-Control-Request-Private-Network"), "true") {
				w.Header().Set("Access-Control-Allow-Private-Network", "true")
			}
			w.Header().Add("Vary", "Origin")
		} else if control && isUntrustedBrowserRequest(r) {
			http.Error(w, "browser origin required", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func isUntrustedBrowserRequest(r *http.Request) bool {
	fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if fetchSite != "" && fetchSite != "none" && fetchSite != "same-origin" {
		return true
	}
	return strings.TrimSpace(r.Referer()) != ""
}

func isLoopbackHost(hostPort string) bool {
	host := hostPort
	if parsedHost, _, err := net.SplitHostPort(hostPort); err == nil {
		host = parsedHost
	}
	if strings.EqualFold(strings.Trim(host, "[]"), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func normalizeOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (strings.ToLower(parsed.Scheme) != "http" && strings.ToLower(parsed.Scheme) != "https") || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}

func (h Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h Handler) GetLatestReview(w http.ResponseWriter, _ *http.Request) {
	if h.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "review store unavailable"})
		return
	}
	review, ok := h.Store.Latest()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no session review found"})
		return
	}
	writeJSON(w, http.StatusOK, review)
}

func (h Handler) GetSessionReview(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "review store unavailable"})
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/v1/review/sessions/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	review, ok := h.Store.Get(sessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session review not found"})
		return
	}
	writeJSON(w, http.StatusOK, review)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func WriteReview(w http.ResponseWriter, review *model.SessionReview) {
	writeJSON(w, http.StatusOK, review)
}
