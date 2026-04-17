package emby

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jqtmviyu/iinaServer/internal/model"
)

func ParsePlayRequest(req model.EmbyPlayRequest, defaultDryRun bool) (model.PlayPlan, error) {
	if strings.EqualFold(strings.TrimSpace(req.MountDiskEnable), "true") {
		return model.PlayPlan{}, fmt.Errorf("mount disk mode not supported")
	}
	if req.PlaybackURL == "" {
		return model.PlayPlan{}, fmt.Errorf("missing playbackUrl")
	}
	if req.ApiClient.ServerAddress == "" {
		return model.PlayPlan{}, fmt.Errorf("missing ApiClient._serverAddress")
	}
	playbackURL, err := url.Parse(req.PlaybackURL)
	if err != nil {
		return model.PlayPlan{}, fmt.Errorf("invalid playbackUrl: %w", err)
	}
	serverURL, err := url.Parse(req.ApiClient.ServerAddress)
	if err != nil {
		return model.PlayPlan{}, fmt.Errorf("invalid ApiClient._serverAddress: %w", err)
	}
	if !strings.Contains(playbackURL.Path, "/emby/") {
		return model.PlayPlan{}, fmt.Errorf("only emby playbackUrl supported")
	}
	query := playbackURL.Query()
	itemID, err := itemIDFromPath(playbackURL.Path)
	if err != nil {
		return model.PlayPlan{}, err
	}
	mediaSource, mediaSourceID, err := pickMediaSource(req.PlaybackData.MediaSources, query.Get("MediaSourceId"))
	if err != nil {
		return model.PlayPlan{}, err
	}
	apiKey := firstNonEmpty(query.Get("X-Emby-Token"), query.Get("api_key"))
	deviceID := firstNonEmpty(query.Get("X-Emby-Device-Id"), req.ApiClient.DeviceID)
	streamName := streamNameForSource(req.ApiClient.ServerVersion, mediaSource)
	container := sourceContainer(mediaSource)
	streamURL := fmt.Sprintf("%s://%s/emby/videos/%s/%s%s?DeviceId=%s&MediaSourceId=%s&PlaySessionId=%s&api_key=%s&Static=true",
		serverURL.Scheme,
		serverURL.Host,
		itemID,
		streamName,
		container,
		url.QueryEscape(deviceID),
		url.QueryEscape(mediaSourceID),
		url.QueryEscape(req.PlaybackData.PlaySessionID),
		url.QueryEscape(apiKey),
	)
	startSeconds := 0
	if seek := query.Get("StartTimeTicks"); seek != "" {
		if ticks, err := strconv.ParseInt(seek, 10, 64); err == nil {
			startSeconds = int(ticks / 10000000)
		}
	}
	dryRun := defaultDryRun
	if req.Options.DryRunUpload != nil {
		dryRun = *req.Options.DryRunUpload
	}
	subtitleURL := buildSubtitleURL(serverURL, itemID, mediaSourceID, apiKey, mediaSource, query.Get("SubtitleStreamIndex"))
	if req.Options.DebugSubtitleURL != "" {
		subtitleURL = req.Options.DebugSubtitleURL
	} else if req.Options.DebugMediaURL != "" {
		subtitleURL = ""
	}
	effectiveMedia := streamURL
	if req.Options.DebugMediaURL != "" {
		effectiveMedia = req.Options.DebugMediaURL
	}
	plan := model.PlayPlan{
		Source:            "emby",
		Scheme:            serverURL.Scheme,
		Netloc:            serverURL.Host,
		ItemID:            itemID,
		MediaSourceID:     mediaSourceID,
		PlaySessionID:     req.PlaybackData.PlaySessionID,
		DeviceID:          deviceID,
		APIKey:            apiKey,
		StartSeconds:      startSeconds,
		MediaTitle:        buildMediaTitle(req.ExtraData.MainEpInfo, mediaSource.Path),
		StreamURL:         streamURL,
		EffectiveMediaURL: effectiveMedia,
		SubtitleURL:       subtitleURL,
		EffectiveSubtitle: subtitleURL,
		UploadEnabled:     true,
		DryRunUpload:      dryRun,
		DebugMediaURL:     req.Options.DebugMediaURL,
		DebugSubtitleURL:  req.Options.DebugSubtitleURL,
		RequestHeaders:    cloneHeaders(req.Request.Headers),
	}
	return plan, nil
}

func itemIDFromPath(path string) (string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if part == "Items" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("item id not found in playbackUrl")
}

func pickMediaSource(sources []model.MediaSource, wanted string) (model.MediaSource, string, error) {
	if len(sources) == 0 {
		return model.MediaSource{}, "", fmt.Errorf("missing playbackData.MediaSources")
	}
	if wanted != "" && wanted != "undefined" {
		for _, source := range sources {
			if source.ID == wanted {
				return source, source.ID, nil
			}
		}
	}
	return sources[0], sources[0].ID, nil
}

func streamNameForSource(version string, source model.MediaSource) string {
	if strings.EqualFold(source.VideoType, "BluRay") {
		return "main"
	}
	if compareVersion(version, "4.8.0.40") >= 0 {
		return "original"
	}
	return "stream"
}

func sourceContainer(source model.MediaSource) string {
	if strings.EqualFold(source.Container, "bluray") {
		return ".m2ts"
	}
	if strings.EqualFold(source.VideoType, "BluRay") {
		return ".m3u8"
	}
	if ext := filepath.Ext(source.Path); ext != "" {
		return ext
	}
	if source.Container == "" {
		return ""
	}
	if strings.HasPrefix(source.Container, ".") {
		return source.Container
	}
	return "." + source.Container
}

func buildSubtitleURL(serverURL *url.URL, itemID string, mediaSourceID string, apiKey string, source model.MediaSource, subtitleIndex string) string {
	picked := pickSubtitle(source.MediaStreams, subtitleIndex)
	if picked == nil {
		return ""
	}
	if picked.DeliveryURL != "" && !strings.EqualFold(picked.Codec, "sup") {
		if strings.HasPrefix(picked.DeliveryURL, "http://") || strings.HasPrefix(picked.DeliveryURL, "https://") {
			return picked.DeliveryURL
		}
		return fmt.Sprintf("%s://%s%s", serverURL.Scheme, serverURL.Host, picked.DeliveryURL)
	}
	codec := picked.Codec
	if codec == "" {
		codec = "srt"
	}
	return fmt.Sprintf("%s://%s/emby/videos/%s/%s/Subtitles/%d/0/Stream.%s?api_key=%s",
		serverURL.Scheme,
		serverURL.Host,
		itemID,
		mediaSourceID,
		picked.Index,
		codec,
		url.QueryEscape(apiKey),
	)
}

func pickSubtitle(streams []model.MediaStream, subtitleIndex string) *model.MediaStream {
	if subtitleIndex != "" {
		if idx, err := strconv.Atoi(subtitleIndex); err == nil {
			for _, stream := range streams {
				if stream.Type == "Subtitle" && stream.IsExternal && stream.Index == idx {
					copy := stream
					return &copy
				}
			}
		}
	}
	for _, stream := range streams {
		if stream.Type == "Subtitle" && stream.IsExternal && stream.IsDefault {
			copy := stream
			return &copy
		}
	}
	for _, stream := range streams {
		if stream.Type == "Subtitle" && stream.IsExternal {
			copy := stream
			return &copy
		}
	}
	return nil
}

func buildMediaTitle(info model.MainEpInfo, sourcePath string) string {
	basename := filepath.Base(sourcePath)
	var title string
	if info.SeasonID == nil {
		if info.ProductionYear > 0 {
			title = fmt.Sprintf("%s (%d)", info.Name, info.ProductionYear)
		} else {
			title = info.Name
		}
	} else if info.ParentIndexNumber == nil || info.IndexNumber == nil {
		title = strings.TrimSpace(info.SeriesName + " - " + info.Name)
	} else if info.IndexNumberEnd == nil {
		title = fmt.Sprintf("%s S%d:E%d - %s", info.SeriesName, *info.ParentIndexNumber, *info.IndexNumber, info.Name)
	} else {
		title = fmt.Sprintf("%s S%d:E%d-%d - %s", info.SeriesName, *info.ParentIndexNumber, *info.IndexNumber, *info.IndexNumberEnd, info.Name)
	}
	if title == "" {
		return basename
	}
	if basename == "" {
		return title
	}
	return title + "  |  " + basename
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compareVersion(left string, right string) int {
	ls := splitVersion(left)
	rs := splitVersion(right)
	maxLen := len(ls)
	if len(rs) > maxLen {
		maxLen = len(rs)
	}
	for i := 0; i < maxLen; i++ {
		var l, r int
		if i < len(ls) {
			l = ls[i]
		}
		if i < len(rs) {
			r = rs[i]
		}
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	}
	return 0
}

func splitVersion(value string) []int {
	parts := strings.Split(strings.TrimSpace(value), ".")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			result = append(result, 0)
			continue
		}
		result = append(result, n)
	}
	return result
}
