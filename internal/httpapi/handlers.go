package httpapi

import (
	"encoding/json"
	"net/http"
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
	mux.HandleFunc("/healthz", h.Healthz)
	mux.HandleFunc("/v1/review/sessions/latest", h.GetLatestReview)
	mux.HandleFunc("/v1/review/sessions/", h.GetSessionReview)
	if h.DirectPlay != nil {
		mux.HandleFunc("/play", h.DirectPlay)
	}
	if h.EmbyPlay != nil {
		mux.HandleFunc("/v1/emby/play", h.EmbyPlay)
	}
	if h.Stop != nil {
		mux.HandleFunc("/v1/session/stop", h.Stop)
	}
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func WriteReview(w http.ResponseWriter, review *model.SessionReview) {
	writeJSON(w, http.StatusOK, review)
}
