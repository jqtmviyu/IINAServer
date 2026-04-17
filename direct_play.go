package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/config"
	"github.com/jqtmviyu/iinaServer/internal/httpapi"
	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/player"
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

		if subtitleURL != "" {
			resp, err := http.Get(subtitleURL)
			if err != nil {
				store.AddError(sessionID, err.Error())
				httpapi.WriteError(w, http.StatusInternalServerError, "无法下载字幕")
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				err = fmt.Errorf("subtitle response status=%d", resp.StatusCode)
				store.AddError(sessionID, err.Error())
				httpapi.WriteError(w, http.StatusBadGateway, "字幕下载失败")
				return
			}
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				store.AddError(sessionID, err.Error())
				httpapi.WriteError(w, http.StatusInternalServerError, "无法读取字幕数据")
				return
			}
			subtitleFile := filepath.Join(os.TempDir(), fmt.Sprintf("tmp_sub_%s.ass", sessionID))
			if err := os.WriteFile(subtitleFile, data, 0644); err != nil {
				store.AddError(sessionID, err.Error())
				httpapi.WriteError(w, http.StatusInternalServerError, "无法保存字幕文件")
				return
			}
			plan.EffectiveSubtitle = subtitleFile
			store.SetPlan(sessionID, plan)
		}

		ctx, cancel := context.WithCancel(context.Background())
		prev := sessions.Replace(&session.ActiveSession{SessionID: sessionID, Cancel: cancel})
		if prev != nil && prev.Cancel != nil {
			prev.Cancel()
		}

		result, err := player.LaunchIINA(ctx, cfg, player.LaunchOptions{
			MediaURL:     plan.EffectiveMediaURL,
			SubtitleFile: plan.EffectiveSubtitle,
			MediaTitle:   filepath.Base(plan.EffectiveMediaURL),
			SessionID:    sessionID,
		})
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
				if strings.Contains(plan.EffectiveSubtitle, "tmp_sub_") {
					_ = os.Remove(plan.EffectiveSubtitle)
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

func mustReview(store *review.Store, sessionID string) *model.SessionReview {
	review, ok := store.Get(sessionID)
	if !ok {
		return &model.SessionReview{SessionID: sessionID, Status: "missing"}
	}
	return review
}
