package purevideo

import "time"

const (
	// EngineMPV uses the bundled or user-managed libmpv-2.dll backend.
	EngineMPV = "mpv"
)

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
}
