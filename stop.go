package main

import (
	"context"
	"net/http"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/httpapi"
	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/player"
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
		inst := sessions.CurrentPlayer()
		if inst != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client := player.NewIPCClient(inst.SocketPath, 2*time.Second)
			if err := client.WaitUntilReady(stopCtx); err != nil {
				sessions.ClearPlayer()
			} else if err := client.StopPlayback(); err != nil {
				sessions.ClearPlayer()
				store.AddError(current.SessionID, err.Error())
				httpapi.WriteError(w, http.StatusInternalServerError, "无法停止播放器")
				return
			} else {
				sessions.ClearPlayer()
			}
		}
		sessions.StopCurrent()
		store.SetStatus(current.SessionID, "stopped")
		httpapi.WriteReview(w, mustReview(store, current.SessionID))
	}
}

func mustReview(store *review.Store, sessionID string) *model.SessionReview {
	if store != nil {
		if review, ok := store.Get(sessionID); ok {
			return review
		}
	}
	return &model.SessionReview{SessionID: sessionID}
}
