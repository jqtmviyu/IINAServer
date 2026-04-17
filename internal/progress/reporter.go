package progress

import (
	"fmt"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/review"
)

type Reporter struct {
	store            *review.Store
	plan             model.PlayPlan
	progressInterval time.Duration
	lastProgressAt   time.Time
	lastSample       *model.ProgressSample
	started          bool
	stopped          bool
}

func NewReporter(store *review.Store, plan model.PlayPlan, progressInterval time.Duration) *Reporter {
	if progressInterval <= 0 {
		progressInterval = 30 * time.Second
	}
	return &Reporter{
		store:            store,
		plan:             plan,
		progressInterval: progressInterval,
	}
}

func (r *Reporter) OnSample(sample model.ProgressSample) {
	r.lastSample = &sample
	if !r.plan.UploadEnabled {
		return
	}
	if !r.started {
		r.started = true
		r.lastProgressAt = sample.At
		r.record("start", sample)
		return
	}
	if sample.Paused {
		return
	}
	if sample.At.Sub(r.lastProgressAt) < r.progressInterval {
		return
	}
	r.lastProgressAt = sample.At
	r.record("playing", sample)
}

func (r *Reporter) OnStop() {
	if r.stopped || !r.plan.UploadEnabled {
		return
	}
	r.stopped = true
	sample := model.ProgressSample{At: time.Now(), MediaTitle: r.plan.MediaTitle}
	if r.lastSample != nil {
		sample = *r.lastSample
	}
	r.record("end", sample)
}

func (r *Reporter) record(kind string, sample model.ProgressSample) {
	url, body := buildUploadPayload(r.plan, kind, sample)
	record := model.UploadRecord{
		Type:   kind,
		Method: "POST",
		URL:    url,
		Query: map[string]string{
			"X-Emby-Token":       r.plan.APIKey,
			"X-Emby-Device-Id":   r.plan.DeviceID,
			"X-Emby-Device-Name": "IINAServer",
		},
		Headers:   r.plan.RequestHeaders,
		Body:      body,
		DryRun:    r.plan.DryRunUpload,
		Sent:      false,
		CreatedAt: time.Now(),
	}
	if !r.plan.DryRunUpload {
		record.Error = "live upload not enabled in local review build"
	}
	r.store.AddUpload(r.plan.SessionID, record)
}

func buildUploadPayload(plan model.PlayPlan, kind string, sample model.ProgressSample) (string, map[string]any) {
	urlPath := map[string]string{
		"start":   "/emby/Sessions/Playing",
		"playing": "/emby/Sessions/Playing/Progress",
		"end":     "/emby/Sessions/Playing/Stopped",
	}[kind]
	url := fmt.Sprintf("%s://%s%s", plan.Scheme, plan.Netloc, urlPath)
	body := map[string]any{
		"EventName":     "timeupdate",
		"ItemId":        plan.ItemID,
		"MediaSourceId": plan.MediaSourceID,
		"PlayMethod":    "DirectStream",
		"PlaySessionId": plan.PlaySessionID,
		"PositionTicks": int64(sample.PositionSeconds * 10000000),
		"RepeatMode":    "RepeatNone",
	}
	if kind == "start" && sample.PositionSeconds == 0 {
		body["PositionTicks"] = int64(plan.StartSeconds * 10000000)
	}
	if kind == "end" {
		delete(body, "EventName")
	}
	return url, body
}
