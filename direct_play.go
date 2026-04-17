package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/config"
	"github.com/jqtmviyu/iinaServer/internal/httpapi"
	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/progress"
	"github.com/jqtmviyu/iinaServer/internal/review"
	"github.com/jqtmviyu/iinaServer/internal/session"
)

func newDirectPlayHandler(cfg config.Config, store *review.Store, sessions *session.Manager) httpapi.DirectPlayFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		videoURL := strings.TrimSpace(r.URL.Query().Get("video"))
		subtitleURL := strings.TrimSpace(r.URL.Query().Get("subtitle"))
		fmt.Println("videoURL:", videoURL)
		fmt.Println("subtitleURL", subtitleURL)
		if videoURL == "" {
			httpapi.WriteError(w, http.StatusBadRequest, "缺少视频 URL")
			return
		}

		sessionID := fmt.Sprintf("direct-%d", time.Now().UnixNano())
		store.Create(sessionID, "direct-play")
		plan := model.PlayPlan{
			SessionID:         sessionID,
			Source:            "direct-play",
			StreamURL:         videoURL,
			EffectiveMediaURL: videoURL,
			SubtitleURL:       subtitleURL,
			EffectiveSubtitle: subtitleURL,
			UploadEnabled:     false,
			DryRunUpload:      true,
		}
		store.SetPlan(sessionID, plan)
		store.SetStatus(sessionID, "review")

		sessionCtx, sessionCancel := context.WithCancel(context.Background())
		prev := sessions.Replace(&session.ActiveSession{SessionID: sessionID, Cancel: sessionCancel})
		if prev != nil && prev.Cancel != nil {
			prev.Cancel()
		}

		result, subtitleFile, reused, err := launchOrReusePlan(sessionCtx, cfg, sessionID, plan, store, sessions)
		if err != nil {
			sessionCancel()
			store.AddError(sessionID, err.Error())
			store.SetStatus(sessionID, "launch_failed")
			httpapi.WriteError(w, http.StatusInternalServerError, "无法启动播放器")
			return
		}
		store.SetLaunch(sessionID, result.Args, result.SocketPath, result.PID)
		store.SetMetadata(sessionID, map[string]any{
			"playerMode": map[bool]string{true: "reused", false: "launched"}[reused],
		})
		store.SetStatus(sessionID, "launching")
		reporter := progress.NewReporter(store, plan, cfg.ProgressInterval)

		go observeSession(sessionCtx, store, sessions, sessionID, result.SocketPath, subtitleFile, cfg.PollInterval, reporter)

		httpapi.WriteReview(w, mustReview(store, sessionID))
	}
}
