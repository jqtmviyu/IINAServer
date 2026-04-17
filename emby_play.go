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
			"reviewOnly":       req.Options.ReviewOnly,
			"dryRunUpload":     plan.DryRunUpload,
			"debugMediaURL":    plan.DebugMediaURL,
			"debugSubtitleURL": plan.DebugSubtitleURL,
			"playerMode":       map[bool]string{true: "reused", false: "launched"}[reused],
		})
		store.SetStatus(sessionID, "launching")
		reporter := progress.NewReporter(store, plan, cfg.DeviceName, &http.Client{Timeout: cfg.HTTPTimeout}, cfg.ProgressInterval)

		go observeSession(sessionCtx, store, sessions, sessionID, result.SocketPath, subtitleFile, cfg.PollInterval, reporter)

		httpapi.WriteReview(w, mustReview(store, sessionID))
	}
}

func launchOrReusePlan(ctx context.Context, cfg config.Config, sessionID string, plan model.PlayPlan, store *review.Store, sessions *session.Manager) (*player.LaunchResult, string, bool, error) {
	subtitleInput := strings.TrimSpace(plan.EffectiveSubtitle)
	subtitleFile := ""
	if subtitleInput != "" && (strings.HasPrefix(subtitleInput, "http://") || strings.HasPrefix(subtitleInput, "https://")) {
		resp, err := http.Get(subtitleInput)
		if err != nil {
			return nil, "", false, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", false, fmt.Errorf("subtitle response status=%d", resp.StatusCode)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", false, err
		}
		subtitleFile = filepath.Join(os.TempDir(), fmt.Sprintf("tmp_sub_%s.ass", sessionID))
		if err := os.WriteFile(subtitleFile, data, 0644); err != nil {
			return nil, "", false, err
		}
		plan.EffectiveSubtitle = subtitleFile
		store.SetPlan(sessionID, plan)
	} else {
		subtitleFile = subtitleInput
	}

	if reused, result, err := reuseExistingPlayer(ctx, cfg, plan, sessions); err == nil && reused {
		return result, subtitleFile, true, nil
	}

	playerCtx, playerCancel := context.WithCancel(context.Background())
	result, err := player.LaunchIINA(playerCtx, cfg, player.LaunchOptions{
		MediaURL:     plan.EffectiveMediaURL,
		SubtitleFile: subtitleInputOrFile(plan, subtitleFile),
		StartSeconds: plan.StartSeconds,
		MediaTitle:   plan.MediaTitle,
		SessionID:    sessionID,
	})
	if err != nil {
		playerCancel()
		if subtitleFile != "" && strings.Contains(subtitleFile, "tmp_sub_") {
			_ = os.Remove(subtitleFile)
		}
		return nil, "", false, err
	}
	sessions.SetPlayer(&session.PlayerInstance{
		Args:       append([]string(nil), result.Args...),
		Cancel:     playerCancel,
		PID:        result.PID,
		SocketPath: result.SocketPath,
	})
	return result, subtitleFile, false, nil
}

func reuseExistingPlayer(ctx context.Context, cfg config.Config, plan model.PlayPlan, sessions *session.Manager) (bool, *player.LaunchResult, error) {
	_ = cfg
	inst := sessions.CurrentPlayer()
	if inst == nil || strings.TrimSpace(inst.SocketPath) == "" {
		return false, nil, fmt.Errorf("no reusable player")
	}
	client := player.NewIPCClient(inst.SocketPath, 2*time.Second)
	if err := client.WaitUntilReady(ctx); err != nil {
		sessions.ClearPlayer()
		return false, nil, err
	}
	if err := waitForReusableWindow(ctx, client); err != nil {
		cleanupBrokenPlayer(client, sessions)
		return false, nil, err
	}
	if err := client.LoadFile(plan.EffectiveMediaURL); err != nil {
		sessions.ClearPlayer()
		return false, nil, err
	}
	if err := client.SetProperty("force-media-title", plan.MediaTitle); err != nil && plan.MediaTitle != "" {
		return false, nil, err
	}
	if err := client.AddSubtitle(plan.EffectiveSubtitle); err != nil {
		return false, nil, err
	}
	if plan.StartSeconds > 0 {
		if err := waitForSeekableMedia(ctx, client); err != nil {
			return false, nil, err
		}
		if err := client.SeekAbsolute(plan.StartSeconds); err != nil {
			return false, nil, err
		}
	}
	if err := client.SetProperty("pause", false); err != nil {
		return false, nil, err
	}
	if err := waitForReusableWindow(ctx, client); err != nil {
		cleanupBrokenPlayer(client, sessions)
		return false, nil, err
	}
	if err := player.ActivateIINA(); err != nil {
		fmt.Println("activate iina:", err)
	}
	return true, &player.LaunchResult{
		Args:       append([]string(nil), inst.Args...),
		SocketPath: inst.SocketPath,
		PID:        inst.PID,
	}, nil
}

func waitForReusableWindow(ctx context.Context, client *player.IPCClient) error {
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	lastErr := fmt.Errorf("player window not configured")
	for {
		configured, err := client.WindowConfigured()
		if err == nil {
			if configured {
				return nil
			}
			lastErr = fmt.Errorf("player window not configured")
		} else {
			windowID, windowErr := client.WindowID()
			if windowErr == nil && windowID > 0 {
				return nil
			}
			lastErr = err
		}
		select {
		case <-waitCtx.Done():
			return lastErr
		case <-ticker.C:
		}
	}
}

func waitForSeekableMedia(ctx context.Context, client *player.IPCClient) error {
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	lastErr := fmt.Errorf("media not seekable")
	for {
		seekable, err := client.Seekable()
		if err == nil {
			if seekable {
				return nil
			}
			lastErr = fmt.Errorf("media not seekable")
		} else {
			lastErr = err
		}
		select {
		case <-waitCtx.Done():
			return lastErr
		case <-ticker.C:
		}
	}
}

func cleanupBrokenPlayer(client *player.IPCClient, sessions *session.Manager) {
	if client != nil {
		_ = client.Quit()
	}
	sessions.ClearPlayer()
}

func waitForPlayerExit(result *player.LaunchResult, sessions *session.Manager) {
	if result == nil || result.Cmd == nil {
		return
	}
	_ = result.Cmd.Wait()
	current := sessions.CurrentPlayer()
	if current != nil && current.SocketPath == result.SocketPath {
		sessions.ClearPlayer()
	}
	_ = os.Remove(result.SocketPath)
}

func observeSession(ctx context.Context, store *review.Store, sessions *session.Manager, sessionID string, socketPath string, subtitleFile string, interval time.Duration, reporter *progress.Reporter) {
	defer func() {
		reporter.OnStop()
		store.SetStatus(sessionID, "stopped")
		sessions.ClearIfCurrent(sessionID)
		if subtitleFile != "" && strings.Contains(subtitleFile, "tmp_sub_") {
			_ = os.Remove(subtitleFile)
		}
	}()
	if err := player.WaitForFirstSample(ctx, socketPath); err != nil {
		store.AddError(sessionID, err.Error())
		store.SetStatus(sessionID, "ipc_wait_failed")
		return
	}
	store.SetStatus(sessionID, "ipc_ready")
	client := player.NewIPCClient(socketPath, 5*time.Second)
	observeErr := client.Observe(ctx, player.ObserveOptions{
		Interval:      interval,
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
}

func subtitleInputOrFile(plan model.PlayPlan, subtitleFile string) string {
	if subtitleFile != "" {
		return subtitleFile
	}
	return plan.EffectiveSubtitle
}
