package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/config"
	"github.com/jqtmviyu/iinaServer/internal/emby"
	"github.com/jqtmviyu/iinaServer/internal/httpapi"
	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/player"
	"github.com/jqtmviyu/iinaServer/internal/progress"
	"github.com/jqtmviyu/iinaServer/internal/review"
	"github.com/jqtmviyu/iinaServer/internal/session"
)

func newEmbyPlayHandler(cfg config.Config, store *review.Store, sessions *session.Manager) httpapi.EmbyPlayFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpapi.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		defer r.Body.Close()
		var req model.EmbyPlayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		plan, err := emby.ParsePlayRequest(req, cfg.DefaultDryRunUpload())
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		sessionID := fmt.Sprintf("emby-%d", time.Now().UnixNano())
		plan.SessionID = sessionID
		store.Create(sessionID, "emby")
		store.SetPlan(sessionID, plan)
		store.SetMetadata(sessionID, map[string]any{
			"reviewOnly":       req.Options.ReviewOnly,
			"dryRunUpload":     plan.DryRunUpload,
			"debugMediaURL":    plan.DebugMediaURL,
			"debugSubtitleURL": plan.DebugSubtitleURL,
		})
		store.SetStatus(sessionID, "parsed")
		if req.Options.ReviewOnly {
			store.SetStatus(sessionID, "review_only")
			httpapi.WriteReview(w, mustReview(store, sessionID))
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		prev := sessions.Replace(&session.ActiveSession{SessionID: sessionID, Cancel: cancel})
		if prev != nil && prev.Cancel != nil {
			prev.Cancel()
		}

		result, subtitleFile, err := launchPlan(ctx, cfg, sessionID, plan, store)
		if err != nil {
			cancel()
			store.AddError(sessionID, err.Error())
			store.SetStatus(sessionID, "launch_failed")
			httpapi.WriteError(w, http.StatusInternalServerError, "无法启动播放器")
			return
		}
		store.SetLaunch(sessionID, result.Args, result.SocketPath, result.PID)
		store.SetStatus(sessionID, "launching")
		reporter := progress.NewReporter(store, plan, cfg.ProgressInterval)

		go func() {
			defer func() {
				reporter.OnStop()
				store.SetStatus(sessionID, "stopped")
				sessions.ClearIfCurrent(sessionID)
				_ = os.Remove(result.SocketPath)
				if subtitleFile != "" {
					_ = os.Remove(subtitleFile)
				}
			}()
			if err := player.WaitForFirstSample(ctx, result.SocketPath); err != nil {
				store.AddError(sessionID, err.Error())
				store.SetStatus(sessionID, "ipc_wait_failed")
				return
			}
			store.SetStatus(sessionID, "ipc_ready")
			client := player.NewIPCClient(result.SocketPath, 5*time.Second)
			observeErr := client.Observe(ctx, player.ObserveOptions{
				Interval:      cfg.PollInterval,
				StopOnIPCExit: true,
				OnSample: func(sample model.ProgressSample) {
					store.AddSample(sessionID, sample)
					reporter.OnSample(sample)
					if sample.Paused {
						store.SetStatus(sessionID, "paused")
						return
					}
					store.SetStatus(sessionID, "playing")
				},
			})
			if observeErr != nil && ctx.Err() == nil {
				store.AddError(sessionID, observeErr.Error())
			}
		}()

		httpapi.WriteReview(w, mustReview(store, sessionID))
	}
}

func launchPlan(ctx context.Context, cfg config.Config, sessionID string, plan model.PlayPlan, store *review.Store) (*player.LaunchResult, string, error) {
	subtitleInput := strings.TrimSpace(plan.EffectiveSubtitle)
	subtitleFile := ""
	if subtitleInput != "" && (strings.HasPrefix(subtitleInput, "http://") || strings.HasPrefix(subtitleInput, "https://")) {
		resp, err := http.Get(subtitleInput)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", fmt.Errorf("subtitle response status=%d", resp.StatusCode)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}
		subtitleFile = filepath.Join(os.TempDir(), fmt.Sprintf("tmp_sub_%s.ass", sessionID))
		if err := os.WriteFile(subtitleFile, data, 0644); err != nil {
			return nil, "", err
		}
		plan.EffectiveSubtitle = subtitleFile
		store.SetPlan(sessionID, plan)
	} else {
		subtitleFile = subtitleInput
	}
	result, err := player.LaunchIINA(ctx, cfg, player.LaunchOptions{
		MediaURL:     plan.EffectiveMediaURL,
		SubtitleFile: subtitleInputOrFile(plan, subtitleFile),
		StartSeconds: plan.StartSeconds,
		MediaTitle:   plan.MediaTitle,
		SessionID:    sessionID,
	})
	if err != nil {
		if subtitleFile != "" && strings.Contains(subtitleFile, "tmp_sub_") {
			_ = os.Remove(subtitleFile)
		}
		return nil, "", err
	}
	return result, subtitleFile, nil
}

func subtitleInputOrFile(plan model.PlayPlan, subtitleFile string) string {
	if subtitleFile != "" {
		return subtitleFile
	}
	return plan.EffectiveSubtitle
}
