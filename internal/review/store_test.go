package review

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jqtmviyu/iinaServer/internal/model"
)

func TestReviewResponsesRedactEmbyCredentials(t *testing.T) {
	store := NewStore(2)
	store.Create("s1", "emby")
	store.SetPlan("s1", model.PlayPlan{
		APIKey:            "secret-token",
		StreamURL:         "https://emby.example/stream?api_key=secret-token",
		EffectiveMediaURL: "https://emby.example/stream?X-Emby-Token=secret-token",
		RequestHeaders:    map[string]string{"Authorization": "Bearer secret-token", "Referer": "https://emby.example/web/"},
	})
	store.SetLaunch("s1", model.PlayerMPV, []string{"mpv", "https://emby.example/stream?api_key=secret-token"}, "/tmp/mpv.sock", 123)
	store.AddUpload("s1", model.UploadRecord{
		URL:   "https://emby.example/emby/Sessions/Playing",
		Query: map[string]string{"X-Emby-Token": "secret-token", "X-Emby-Device-Id": "device-1"},
	})

	response, ok := store.Get("s1")
	if !ok {
		t.Fatal("missing review")
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-token") {
		t.Fatalf("review leaked credentials: %s", data)
	}
	if response.Player != model.PlayerMPV {
		t.Fatalf("unexpected player: %q", response.Player)
	}
	if got := response.Uploads[0].Query["X-Emby-Device-Id"]; got != "device-1" {
		t.Fatalf("non-secret query was removed: %q", got)
	}
	if store.byID["s1"].Plan.APIKey != "secret-token" {
		t.Fatal("redacting response changed the stored plan")
	}
}
