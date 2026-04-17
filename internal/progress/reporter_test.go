package progress

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jqtmviyu/iinaServer/internal/model"
	"github.com/jqtmviyu/iinaServer/internal/review"
)

func TestReporterDryRunRecordsStartProgressStop(t *testing.T) {
	store := review.NewStore(5)
	store.Create("s1", "emby")
	plan := model.PlayPlan{
		SessionID:     "s1",
		Source:        "emby",
		Scheme:        "http",
		Netloc:        "127.0.0.1:8096",
		ItemID:        "item123",
		MediaSourceID: "ms-1",
		PlaySessionID: "play-456",
		DeviceID:      "device-123",
		APIKey:        "token-abc",
		StartSeconds:  3,
		MediaTitle:    "demo",
		UploadEnabled: true,
		DryRunUpload:  true,
	}
	reporter := NewReporter(store, plan, "IINAServer", nil, 10*time.Second)
	base := time.Unix(100, 0)
	reporter.OnSample(model.ProgressSample{At: base, PositionSeconds: 5, MediaTitle: "demo"})
	reporter.OnSample(model.ProgressSample{At: base.Add(5 * time.Second), PositionSeconds: 9, MediaTitle: "demo"})
	reporter.OnSample(model.ProgressSample{At: base.Add(12 * time.Second), PositionSeconds: 17, MediaTitle: "demo"})
	reporter.OnStop()
	reviewData, ok := store.Get("s1")
	if !ok {
		t.Fatal("missing review")
	}
	if len(reviewData.Uploads) != 3 {
		t.Fatalf("unexpected upload count: %d", len(reviewData.Uploads))
	}
	if reviewData.Uploads[0].Type != "start" || reviewData.Uploads[1].Type != "playing" || reviewData.Uploads[2].Type != "end" {
		t.Fatalf("unexpected upload types: %#v", reviewData.Uploads)
	}
	if reviewData.Uploads[1].Body["PositionTicks"].(int64) != int64(17*10000000) {
		t.Fatalf("unexpected progress ticks: %v", reviewData.Uploads[1].Body["PositionTicks"])
	}
}

func TestReporterLiveUploadMarksSent(t *testing.T) {
	requests := make([]struct {
		path        string
		query       string
		contentType string
		body        string
		referer     string
	}, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		requests = append(requests, struct {
			path        string
			query       string
			contentType string
			body        string
			referer     string
		}{
			path:        r.URL.Path,
			query:       r.URL.RawQuery,
			contentType: r.Header.Get("Content-Type"),
			body:        string(data),
			referer:     r.Header.Get("Referer"),
		})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store := review.NewStore(5)
	store.Create("s1", "emby")
	plan := livePlanForServer(server.URL)
	reporter := NewReporter(store, plan, "MyIINA", server.Client(), 10*time.Second)
	base := time.Unix(100, 0)
	reporter.OnSample(model.ProgressSample{At: base, PositionSeconds: 0, MediaTitle: "demo"})
	reporter.OnSample(model.ProgressSample{At: base.Add(12 * time.Second), PositionSeconds: 17, MediaTitle: "demo"})
	reporter.OnStop()

	reviewData, ok := store.Get("s1")
	if !ok {
		t.Fatal("missing review")
	}
	if len(reviewData.Uploads) != 3 {
		t.Fatalf("unexpected upload count: %d", len(reviewData.Uploads))
	}
	for i, upload := range reviewData.Uploads {
		if !upload.Sent {
			t.Fatalf("upload %d not sent: %#v", i, upload)
		}
		if upload.Error != "" {
			t.Fatalf("upload %d unexpected error: %s", i, upload.Error)
		}
	}
	if len(requests) != 3 {
		t.Fatalf("unexpected request count: %d", len(requests))
	}
	if requests[0].path != "/emby/Sessions/Playing" || requests[1].path != "/emby/Sessions/Playing/Progress" || requests[2].path != "/emby/Sessions/Playing/Stopped" {
		t.Fatalf("unexpected request paths: %#v", requests)
	}
	if requests[0].contentType != "application/json" {
		t.Fatalf("unexpected content-type: %s", requests[0].contentType)
	}
	if requests[0].referer != "http://emby.local/item/1" {
		t.Fatalf("unexpected referer: %s", requests[0].referer)
	}
	wantQuery := "X-Emby-Device-Id=device-123&X-Emby-Device-Name=MyIINA&X-Emby-Token=token-abc"
	if requests[0].query != wantQuery {
		t.Fatalf("unexpected query: %s", requests[0].query)
	}
	if requests[0].body != `{"EventName":"timeupdate","ItemId":"item123","MediaSourceId":"ms-1","PlayMethod":"DirectStream","PlaySessionId":"play-456","PositionTicks":30000000,"RepeatMode":"RepeatNone"}` {
		t.Fatalf("unexpected start body: %s", requests[0].body)
	}
	if requests[2].body != `{"ItemId":"item123","MediaSourceId":"ms-1","PlayMethod":"DirectStream","PlaySessionId":"play-456","PositionTicks":170000000,"RepeatMode":"RepeatNone"}` {
		t.Fatalf("unexpected end body: %s", requests[2].body)
	}
}

func TestReporterLiveUploadRecordsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	store := review.NewStore(5)
	store.Create("s1", "emby")
	plan := livePlanForServer(server.URL)
	reporter := NewReporter(store, plan, "MyIINA", server.Client(), 10*time.Second)
	reporter.OnSample(model.ProgressSample{At: time.Unix(100, 0), PositionSeconds: 5, MediaTitle: "demo"})

	reviewData, ok := store.Get("s1")
	if !ok {
		t.Fatal("missing review")
	}
	if len(reviewData.Uploads) != 1 {
		t.Fatalf("unexpected upload count: %d", len(reviewData.Uploads))
	}
	if reviewData.Uploads[0].Sent {
		t.Fatalf("unexpected sent upload: %#v", reviewData.Uploads[0])
	}
	if reviewData.Uploads[0].Error != "status=502 body=bad gateway\n" {
		t.Fatalf("unexpected error: %s", reviewData.Uploads[0].Error)
	}
}

func livePlanForServer(serverURL string) model.PlayPlan {
	return model.PlayPlan{
		SessionID:     "s1",
		Source:        "emby",
		Scheme:        "http",
		Netloc:        serverURL[len("http://"):],
		ItemID:        "item123",
		MediaSourceID: "ms-1",
		PlaySessionID: "play-456",
		DeviceID:      "device-123",
		APIKey:        "token-abc",
		StartSeconds:  3,
		MediaTitle:    "demo",
		UploadEnabled: true,
		DryRunUpload:  false,
		RequestHeaders: map[string]string{
			"Referer": "http://emby.local/item/1",
		},
	}
}
