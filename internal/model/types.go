package model

import "time"

type EmbyPlayRequest struct {
	ApiClient       ApiClientInfo `json:"ApiClient"`
	PlaybackURL     string        `json:"playbackUrl"`
	PlaybackData    PlaybackData  `json:"playbackData"`
	Request         RequestMeta   `json:"request"`
	ExtraData       ExtraData     `json:"extraData"`
	MountDiskEnable string        `json:"mountDiskEnable"`
	Options         PlayOptions   `json:"options"`
}

type ApiClientInfo struct {
	ServerAddress string `json:"_serverAddress"`
	ServerVersion string `json:"_serverVersion"`
	DeviceID      string `json:"_deviceId"`
}

type PlaybackData struct {
	PlaySessionID string        `json:"PlaySessionId"`
	MediaSources  []MediaSource `json:"MediaSources"`
}

type MediaSource struct {
	ID           string        `json:"Id"`
	Name         string        `json:"Name"`
	Path         string        `json:"Path"`
	Container    string        `json:"Container"`
	VideoType    string        `json:"VideoType"`
	RunTimeTicks int64         `json:"RunTimeTicks"`
	MediaStreams []MediaStream `json:"MediaStreams"`
}

type MediaStream struct {
	Index        int    `json:"Index"`
	Type         string `json:"Type"`
	IsExternal   bool   `json:"IsExternal"`
	Codec        string `json:"Codec"`
	DeliveryURL  string `json:"DeliveryUrl"`
	Title        string `json:"Title"`
	DisplayTitle string `json:"DisplayTitle"`
	IsDefault    bool   `json:"IsDefault"`
}

type RequestMeta struct {
	Headers map[string]string `json:"headers"`
}

type ExtraData struct {
	MainEpInfo MainEpInfo `json:"mainEpInfo"`
}

type MainEpInfo struct {
	Name              string    `json:"Name"`
	SeriesName        string    `json:"SeriesName"`
	ProductionYear    int       `json:"ProductionYear"`
	SeasonID          *string   `json:"SeasonId"`
	ParentIndexNumber *int      `json:"ParentIndexNumber"`
	IndexNumber       *int      `json:"IndexNumber"`
	IndexNumberEnd    *int      `json:"IndexNumberEnd"`
	Type              string    `json:"Type"`
	Path              string    `json:"Path"`
	Chapters          []Chapter `json:"Chapters"`
}

type Chapter struct {
	MarkerType         string `json:"MarkerType"`
	StartPositionTicks int64  `json:"StartPositionTicks"`
}

type PlayOptions struct {
	ReviewOnly       bool   `json:"reviewOnly"`
	DryRunUpload     *bool  `json:"dryRunUpload,omitempty"`
	DebugMediaURL    string `json:"debugMediaURL,omitempty"`
	DebugSubtitleURL string `json:"debugSubtitleURL,omitempty"`
}

type PlayPlan struct {
	SessionID         string            `json:"sessionId"`
	Source            string            `json:"source"`
	Scheme            string            `json:"scheme,omitempty"`
	Netloc            string            `json:"netloc,omitempty"`
	ItemID            string            `json:"itemId,omitempty"`
	MediaSourceID     string            `json:"mediaSourceId,omitempty"`
	PlaySessionID     string            `json:"playSessionId,omitempty"`
	DeviceID          string            `json:"deviceId,omitempty"`
	APIKey            string            `json:"apiKey,omitempty"`
	StartSeconds      int               `json:"startSeconds,omitempty"`
	MediaTitle        string            `json:"mediaTitle,omitempty"`
	StreamURL         string            `json:"streamURL,omitempty"`
	EffectiveMediaURL string            `json:"effectiveMediaURL,omitempty"`
	SubtitleURL       string            `json:"subtitleURL,omitempty"`
	EffectiveSubtitle string            `json:"effectiveSubtitle,omitempty"`
	UploadEnabled     bool              `json:"uploadEnabled"`
	DryRunUpload      bool              `json:"dryRunUpload"`
	DebugMediaURL     string            `json:"debugMediaURL,omitempty"`
	DebugSubtitleURL  string            `json:"debugSubtitleURL,omitempty"`
	RequestHeaders    map[string]string `json:"requestHeaders,omitempty"`
}

type ProgressSample struct {
	At              time.Time `json:"at"`
	PositionSeconds float64   `json:"positionSeconds"`
	DurationSeconds float64   `json:"durationSeconds"`
	Paused          bool      `json:"paused"`
	MediaTitle      string    `json:"mediaTitle"`
}

type UploadRecord struct {
	Type      string            `json:"type"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Query     map[string]string `json:"query,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      map[string]any    `json:"body,omitempty"`
	DryRun    bool              `json:"dryRun"`
	Sent      bool              `json:"sent"`
	CreatedAt time.Time         `json:"createdAt"`
	Error     string            `json:"error,omitempty"`
}

type SessionReview struct {
	SessionID string           `json:"sessionId"`
	Source    string           `json:"source"`
	Status    string           `json:"status"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Plan      *PlayPlan        `json:"plan,omitempty"`
	IINAArgs  []string         `json:"iinaArgs,omitempty"`
	IPCSocket string           `json:"ipcSocket,omitempty"`
	PID       int              `json:"pid,omitempty"`
	Samples   []ProgressSample `json:"samples,omitempty"`
	Uploads   []UploadRecord   `json:"uploads,omitempty"`
	Errors    []string         `json:"errors,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
}
