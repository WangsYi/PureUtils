package purekit

import (
	"context"
	"fmt"
	"image"

	"pureutils/pkg/purekit/core"
)

const (
	// CapClipboardHistory is registered by PureClip to expose recent history
	// items, pending ingestion flush, and change notifications.
	CapClipboardHistory = "clipboard.history"
	// CapImagePreview is registered by PureImage to open image files/buffers
	// and advertise supported formats.
	CapImagePreview = "image.preview"
	// CapImagePin is registered by PureShot to create floating desktop pins.
	CapImagePin = "image.pin"
	// CapVideoPreview is registered by PureVideo to open local videos and
	// advertise supported video extensions.
	CapVideoPreview = "video.preview"
)

// ClipboardHistoryRequest is the typed request envelope for CapClipboardHistory.
type ClipboardHistoryRequest struct {
	Action    string       `json:"action"` // "flush" | "list_recent" | "load_content" | "subscribe_changed" | "status"
	Limit     int          `json:"limit,omitempty"`
	EntryID   int64        `json:"entryId,omitempty"`
	OnChanged func()       `json:"-"`
}

// HistoryEntry summarizes one clipboard history entry without leaking SQLite details.
type HistoryEntry struct {
	ID            int64              `json:"id"`
	Kind          core.ClipboardKind `json:"kind"`
	Preview       string             `json:"preview"`
	SourceProcess string             `json:"sourceProcess,omitempty"`
}

// ClipboardHistoryResponse is returned by CapClipboardHistory handlers.
type ClipboardHistoryResponse struct {
	Entries     []HistoryEntry         `json:"entries,omitempty"`
	Content     *core.ClipboardContent `json:"content,omitempty"`
	Active      bool                   `json:"active"`
	Monitoring  bool                   `json:"monitoring"`
	Unsubscribe func()                 `json:"-"`
}

// ImagePreviewRequest is the typed request envelope for CapImagePreview.
type ImagePreviewRequest struct {
	Action string      `json:"action"` // "open_path" | "open_image" | "supported_exts" | "toggle_quick_preview"
	Path   string      `json:"path,omitempty"`
	Title  string      `json:"title,omitempty"`
	Image  image.Image `json:"-"`
}

// ImagePreviewResponse is returned by CapImagePreview handlers.
type ImagePreviewResponse struct {
	Extensions []string `json:"extensions,omitempty"`
}

// ImagePinRequest is the typed request envelope for CapImagePin.
type ImagePinRequest struct {
	Action  string                 `json:"action"` // "pin_image" | "pin_text" | "pin_content"
	Image   image.Image            `json:"-"`
	Text    string                 `json:"text,omitempty"`
	Content *core.ClipboardContent `json:"-"`
	Cascade int                    `json:"cascade,omitempty"`
}

// CallClipboardHistoryFlush flushes debounced clipboard ingestion if PureClip is active.
func CallClipboardHistoryFlush(ctx context.Context, b *Bus) error {
	_, err := b.Call(ctx, CapClipboardHistory, ClipboardHistoryRequest{Action: "flush"})
	return err
}

// CallClipboardHistoryListRecent returns up to limit recent entries and active status.
func CallClipboardHistoryListRecent(ctx context.Context, b *Bus, limit int) (ClipboardHistoryResponse, error) {
	raw, err := b.Call(ctx, CapClipboardHistory, ClipboardHistoryRequest{Action: "list_recent", Limit: limit})
	if err != nil {
		return ClipboardHistoryResponse{}, err
	}
	res, ok := raw.(ClipboardHistoryResponse)
	if !ok {
		return ClipboardHistoryResponse{}, fmt.Errorf("purekit: unexpected %s response type %T", CapClipboardHistory, raw)
	}
	return res, nil
}

// CallClipboardHistoryLoadContent loads a replayable ClipboardContent by EntryID.
func CallClipboardHistoryLoadContent(ctx context.Context, b *Bus, entryID int64) (*core.ClipboardContent, error) {
	raw, err := b.Call(ctx, CapClipboardHistory, ClipboardHistoryRequest{Action: "load_content", EntryID: entryID})
	if err != nil {
		return nil, err
	}
	res, ok := raw.(ClipboardHistoryResponse)
	if !ok {
		return nil, fmt.Errorf("purekit: unexpected %s response type %T", CapClipboardHistory, raw)
	}
	return res.Content, nil
}

// CallClipboardHistorySubscribeChanged subscribes to PureClip history updates.
func CallClipboardHistorySubscribeChanged(ctx context.Context, b *Bus, fn func()) (func(), error) {
	raw, err := b.Call(ctx, CapClipboardHistory, ClipboardHistoryRequest{Action: "subscribe_changed", OnChanged: fn})
	if err != nil {
		return nil, err
	}
	res, ok := raw.(ClipboardHistoryResponse)
	if !ok {
		return nil, fmt.Errorf("purekit: unexpected %s response type %T", CapClipboardHistory, raw)
	}
	if res.Unsubscribe == nil {
		return func() {}, nil
	}
	return res.Unsubscribe, nil
}

// CallImagePreviewPath opens a file or folder path in PureImage.
func CallImagePreviewPath(ctx context.Context, b *Bus, path string) error {
	_, err := b.Call(ctx, CapImagePreview, ImagePreviewRequest{Action: "open_path", Path: path})
	return err
}

// CallImagePreviewImage opens an in-memory image in PureImage.
func CallImagePreviewImage(ctx context.Context, b *Bus, title string, img image.Image) error {
	_, err := b.Call(ctx, CapImagePreview, ImagePreviewRequest{Action: "open_image", Title: title, Image: img})
	return err
}

// CallImagePreviewSupportedExts queries supported image extensions from PureImage.
func CallImagePreviewSupportedExts(ctx context.Context, b *Bus) ([]string, error) {
	raw, err := b.Call(ctx, CapImagePreview, ImagePreviewRequest{Action: "supported_exts"})
	if err != nil {
		return nil, err
	}
	res, ok := raw.(ImagePreviewResponse)
	if !ok {
		return nil, fmt.Errorf("purekit: unexpected %s response type %T", CapImagePreview, raw)
	}
	return res.Extensions, nil
}

// CallImagePreviewToggleQuickPreview asks PureImage to toggle the floating quick preview window.
func CallImagePreviewToggleQuickPreview(ctx context.Context, b *Bus) error {
	_, err := b.Call(ctx, CapImagePreview, ImagePreviewRequest{Action: "toggle_quick_preview"})
	return err
}

// CallImagePinImage asks PureShot to pin an image on the desktop.
func CallImagePinImage(ctx context.Context, b *Bus, img image.Image, cascade int) error {
	_, err := b.Call(ctx, CapImagePin, ImagePinRequest{Action: "pin_image", Image: img, Cascade: cascade})
	return err
}

// CallImagePinText asks PureShot to pin a text card on the desktop.
func CallImagePinText(ctx context.Context, b *Bus, text string, cascade int) error {
	_, err := b.Call(ctx, CapImagePin, ImagePinRequest{Action: "pin_text", Text: text, Cascade: cascade})
	return err
}

// CallImagePinContent asks PureShot to pin ClipboardContent (image or text).
func CallImagePinContent(ctx context.Context, b *Bus, content *core.ClipboardContent, cascade int) error {
	_, err := b.Call(ctx, CapImagePin, ImagePinRequest{Action: "pin_content", Content: content, Cascade: cascade})
	return err
}

// VideoPreviewRequest is the typed request envelope for CapVideoPreview.
type VideoPreviewRequest struct {
	Action string `json:"action"` // "open_path" | "supported_exts"
	Path   string `json:"path,omitempty"`
}

// VideoPreviewResponse is returned by CapVideoPreview handlers.
type VideoPreviewResponse struct {
	Extensions []string `json:"extensions,omitempty"`
}

// CallVideoPreviewPath opens a video file or folder path in PureVideo.
func CallVideoPreviewPath(ctx context.Context, b *Bus, path string) error {
	_, err := b.Call(ctx, CapVideoPreview, VideoPreviewRequest{Action: "open_path", Path: path})
	return err
}

// CallVideoPreviewSupportedExts queries supported video extensions from PureVideo.
func CallVideoPreviewSupportedExts(ctx context.Context, b *Bus) ([]string, error) {
	raw, err := b.Call(ctx, CapVideoPreview, VideoPreviewRequest{Action: "supported_exts"})
	if err != nil {
		return nil, err
	}
	res, ok := raw.(VideoPreviewResponse)
	if !ok {
		return nil, fmt.Errorf("purekit: unexpected %s response type %T", CapVideoPreview, raw)
	}
	return res.Extensions, nil
}
