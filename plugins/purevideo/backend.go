package purevideo

import (
	"image"
	"time"
)

const (
	// EngineMPV uses the bundled or user-managed libmpv-2.dll backend.
	EngineMPV = "mpv"
)

// MediaTrack describes an audio, video, or subtitle stream exposed by the backend.
type MediaTrack struct {
	ID       int    `json:"id"`
	Type     string `json:"type"` // "video", "audio", "sub"
	Title    string `json:"title"`
	Lang     string `json:"lang"`
	Codec    string `json:"codec"`
	Selected bool   `json:"selected"`
	External bool   `json:"external"`
}

// MediaInfo holds stream codec and signal metadata for the active video.
type MediaInfo struct {
	VideoCodec    string
	AudioCodec    string
	Width         int
	Height        int
	FPS           float64
	VideoBitrate  int64
	AudioRate     int
	AudioChannels int
}

// PlaybackState represents the player's current transport state.
type PlaybackState int

const (
	StateEmpty PlaybackState = iota
	StateLoading
	StatePlaying
	StatePaused
	StateStopped
	StateError
)

func (s PlaybackState) String() string {
	switch s {
	case StateLoading:
		return "loading"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	case StateStopped:
		return "stopped"
	case StateError:
		return "error"
	default:
		return "empty"
	}
}

// BackendCallbacks receives asynchronous engine notifications (possibly on background threads).
type BackendCallbacks struct {
	OnReady func()
	OnState func(PlaybackState)
	OnEnded func()
	OnError func(code uint32, msg string)
}

// PlaybackBackend abstracts a native video decoding/rendering backend bound to a Win32 child HWND.
type PlaybackBackend interface {
	Kind() string
	Open(path string, autoPlay bool) error
	Play() error
	Pause() error
	Stop() error
	Seek(pos time.Duration) error
	FrameStep() error
	FrameBackStep() error
	SetVolume(vol float32) error
	SetMute(mute bool) error
	SetRate(rate float32) error
	Position() time.Duration
	Duration() time.Duration
	NativeSize() (int, int)
	State() PlaybackState
	Resize(w, h int)
	Repaint()
	Close()

	Tracks() []MediaTrack
	SetAudioTrack(id int) error
	SetSubtitleTrack(id int) error
	AddSubtitle(path string) error
	SubtitleVisible() bool
	SetSubtitleVisible(v bool) error
	AudioDelay() float64
	SetAudioDelay(sec float64) error
	SubtitleDelay() float64
	SetSubtitleDelay(sec float64) error
	SetAspectRatio(ratio string) error
	SetRotate(deg int) error
	SetVideoFlip(hflip, vflip bool) error
	SetZoom(zoom float64) error
	SetDeinterlace(on bool) error
	SetABLoop(a, b float64) error
	SetHWDec(enabled bool) error
	CaptureFrame() (image.Image, error)
	MediaInfo() MediaInfo
}
