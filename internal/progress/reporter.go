package progress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/review"
)

type Reporter struct {
	store            *review.Store
	plan             model.PlayPlan
	deviceName       string
	httpClient       *http.Client
	progressInterval time.Duration
	lastProgressAt   time.Time
	lastSample       *model.ProgressSample
	started          bool
	stopped          bool
}

func NewReporter(store *review.Store, plan model.PlayPlan, deviceName string, httpClient *http.Client, progressInterval time.Duration) *Reporter {
	if progressInterval <= 0 {
		progressInterval = 30 * time.Second
	}
	if deviceName == "" {
		deviceName = "IINAServer"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Reporter{
		store:            store,
		plan:             plan,
		deviceName:       deviceName,
		httpClient:       httpClient,
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
			"X-Emby-Device-Name": r.deviceName,
		},
		Headers:   cloneHeaders(r.plan.RequestHeaders),
		Body:      body,
		DryRun:    r.plan.DryRunUpload,
		Sent:      false,
		CreatedAt: time.Now(),
	}
	if !r.plan.DryRunUpload {
		record.Sent, record.Error = r.send(record)
	}
	r.store.AddUpload(r.plan.SessionID, record)
}

func (r *Reporter) send(record model.UploadRecord) (bool, string) {
	payload, err := json.Marshal(record.Body)
	if err != nil {
		return false, err.Error()
	}
	requestURL, err := buildRequestURL(record.URL, record.Query)
	if err != nil {
		return false, err.Error()
	}
	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return false, err.Error()
	}
	for key, value := range record.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return false, fmt.Sprintf("status=%d read body: %v", resp.StatusCode, err)
	}
	if len(body) == 0 {
		return false, fmt.Sprintf("status=%d", resp.StatusCode)
	}
	return false, fmt.Sprintf("status=%d body=%s", resp.StatusCode, string(body))
}

func buildRequestURL(rawURL string, query map[string]string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	for key, value := range query {
		if value == "" {
			continue
		}
		values.Set(key, value)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func cloneHeaders(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
