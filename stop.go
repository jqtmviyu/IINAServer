package main

import (
	"net/http"

	"github.com/jqtmviyu/iinaServer/internal/httpapi"
	"github.com/jqtmviyu/iinaServer/internal/review"
	"github.com/jqtmviyu/iinaServer/internal/session"
)

func newStopHandler(store *review.Store, sessions *session.Manager) httpapi.StopFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpapi.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		current := sessions.Current()
		if current == nil {
			httpapi.WriteError(w, http.StatusNotFound, "no active session")
			return
		}
		if current.Cancel != nil {
			current.Cancel()
		}
		sessions.StopCurrent()
		store.SetStatus(current.SessionID, "stopped")
		httpapi.WriteReview(w, mustReview(store, current.SessionID))
	}
}
