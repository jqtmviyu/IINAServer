package progress

import (
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
	reporter := NewReporter(store, plan, 10*time.Second)
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
