package emby

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jqtmviyu/iinaServer/internal/model"
)

func TestParsePlayRequest(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "emby_play_request_episode.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	var req model.EmbyPlayRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("unmarshal testdata: %v", err)
	}
	plan, err := ParsePlayRequest(req, true)
	if err != nil {
		t.Fatalf("ParsePlayRequest error: %v", err)
	}
	if plan.ItemID != "item123" {
		t.Fatalf("unexpected item id: %s", plan.ItemID)
	}
	if plan.MediaSourceID != "ms-1" {
		t.Fatalf("unexpected media source id: %s", plan.MediaSourceID)
	}
	if plan.StartSeconds != 45 {
		t.Fatalf("unexpected start seconds: %d", plan.StartSeconds)
	}
	wantStream := "http://127.0.0.1:8096/emby/videos/item123/original.mkv?DeviceId=device-123&MediaSourceId=ms-1&PlaySessionId=play-456&api_key=token-abc&Static=true"
	if plan.StreamURL != wantStream {
		t.Fatalf("unexpected stream url: %s", plan.StreamURL)
	}
	wantSub := "http://127.0.0.1:8096/emby/videos/item123/ms-1/Subtitles/2/0/Stream.ass?api_key=token-abc"
	if plan.SubtitleURL != wantSub {
		t.Fatalf("unexpected subtitle url: %s", plan.SubtitleURL)
	}
	if plan.MediaTitle != "Demo Show S1:E1 - Pilot  |  S01E01.mkv" {
		t.Fatalf("unexpected media title: %s", plan.MediaTitle)
	}
}
