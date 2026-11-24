package purekit

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"image"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"pureutils/pkg/purekit/core"
)

func TestBusRegisterCallAndTypedContracts(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	if _, err := bus.Call(ctx, CapImagePreview, ImagePreviewRequest{Action: "open_path"}); !errors.Is(err, ErrCapabilityNotFound) {
		t.Fatalf("missing capability err = %v, want ErrCapabilityNotFound", err)
	}

	var openedPath string
	var openedImg image.Image
	if err := bus.Register(CapImagePreview, func(_ context.Context, raw any) (any, error) {
		req := raw.(ImagePreviewRequest)
		switch req.Action {
		case "open_path":
			openedPath = req.Path
			return ImagePreviewResponse{}, nil
		case "open_image":
			openedImg = req.Image
			return ImagePreviewResponse{}, nil
		case "supported_exts":
			return ImagePreviewResponse{Extensions: []string{".png", ".jpg"}}, nil
		case "toggle_quick_preview":
			return ImagePreviewResponse{}, nil
		default:
			return nil, errors.New("unknown action")
		}
	}); err != nil {
		t.Fatalf("Register image.preview: %v", err)
	}

	if err := bus.Register(CapImagePreview, func(context.Context, any) (any, error) { return nil, nil }); !errors.Is(err, ErrCapabilityExists) {
		t.Fatalf("duplicate Register err = %v, want ErrCapabilityExists", err)
	}

	var pinnedText string
	if err := bus.Register(CapImagePin, func(_ context.Context, raw any) (any, error) {
		req := raw.(ImagePinRequest)
		pinnedText = req.Text
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := bus.Register(CapClipboardHistory, func(_ context.Context, raw any) (any, error) {
		req := raw.(ClipboardHistoryRequest)
		switch req.Action {
		case "flush":
			return ClipboardHistoryResponse{Active: true, Monitoring: true}, nil
		case "list_recent":
			return ClipboardHistoryResponse{
				Active:     true,
				Monitoring: true,
				Entries:    []HistoryEntry{{ID: 42, Kind: core.KindText, Preview: "hello"}},
			}, nil
		case "load_content":
			return ClipboardHistoryResponse{
				Active:  true,
				Content: &core.ClipboardContent{Text: "hello"},
			}, nil
		default:
			return ClipboardHistoryResponse{}, nil
		}
	}); err != nil {
		t.Fatal(err)
	}

	if !bus.Has(CapImagePreview) || len(bus.List()) != 3 {
		t.Fatalf("bus.List = %v", bus.List())
	}

	if err := CallImagePreviewPath(ctx, bus, `D:\a.png`); err != nil || openedPath != `D:\a.png` {
		t.Fatalf("CallImagePreviewPath failed: opened=%q err=%v", openedPath, err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := CallImagePreviewImage(ctx, bus, "t", img); err != nil || openedImg == nil {
		t.Fatalf("CallImagePreviewImage failed: err=%v", err)
	}
	exts, err := CallImagePreviewSupportedExts(ctx, bus)
	if err != nil || len(exts) != 2 {
		t.Fatalf("CallImagePreviewSupportedExts = %v, err=%v", exts, err)
	}
	if err := CallImagePinText(ctx, bus, "pin-me", 1); err != nil || pinnedText != "pin-me" {
		t.Fatalf("CallImagePinText = %q, err=%v", pinnedText, err)
	}
	if err := CallClipboardHistoryFlush(ctx, bus); err != nil {
		t.Fatalf("CallClipboardHistoryFlush: %v", err)
	}
	listRes, err := CallClipboardHistoryListRecent(ctx, bus, 10)
	if err != nil || len(listRes.Entries) != 1 || listRes.Entries[0].ID != 42 {
		t.Fatalf("CallClipboardHistoryListRecent = %+v, err=%v", listRes, err)
	}
	loaded, err := CallClipboardHistoryLoadContent(ctx, bus, 42)
	if err != nil || loaded == nil || loaded.Text != "hello" {
		t.Fatalf("CallClipboardHistoryLoadContent = %+v, err=%v", loaded, err)
	}

	// Concurrent calls must remain race-free.
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = CallImagePreviewSupportedExts(ctx, bus)
		}()
	}
	wg.Wait()

	bus.Unregister(CapImagePreview)
	if bus.Has(CapImagePreview) {
		t.Fatal("expected image.preview unregistered")
	}
}

func TestPurekitDependencyBoundaries(t *testing.T) {
	fset := token.NewFileSet()
	root := "."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".go" {
			return err
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p == "pureutils" ||
				strings.HasPrefix(p, "pureutils/internal/") ||
				strings.HasPrefix(p, "pureutils/plugins/") ||
				strings.HasPrefix(p, "shotclip/") ||
				p == "shotclip" {
				t.Errorf("%s has forbidden reverse/host/plugin import %q", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
}
