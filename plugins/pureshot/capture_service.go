package pureshot

import (
	"image"
	"os"
	"path/filepath"
	"time"

	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/platform"
	"pureutils/pkg/purekit/storage"
	"pureutils/plugins/pureshot/annotate"
)

// ShotResult is the output of a completed capture.
type ShotResult struct {
	PNG       []byte
	Width     int
	Height    int
	ImagePath string
	SavedPath string
	ShotID    int64
	Copied    bool
	ShouldPin bool
	PinImage  *image.RGBA
}

// CaptureService bakes, stores, saves and copies screenshots without any
// direct dependency on PureClip.
type CaptureService struct {
	cfgFn func() config.PureShotConfig
	clip  clipboard.Service
	repo  *Repo
	blobs *storage.BlobStore
	log   *core.Logger
}

// NewCaptureService creates the service.
func NewCaptureService(cfgFn func() config.PureShotConfig, clip clipboard.Service, repo *Repo, blobs *storage.BlobStore, log *core.Logger) *CaptureService {
	return &CaptureService{cfgFn: cfgFn, clip: clip, repo: repo, blobs: blobs, log: log}
}

func (c *CaptureService) cfg() config.PureShotConfig {
	if c != nil && c.cfgFn != nil {
		return c.cfgFn()
	}
	return config.DefaultPureShotConfig()
}

// CaptureMonitor freezes one display.
func (c *CaptureService) CaptureMonitor(m platform.Monitor, includeCursor bool) (*image.RGBA, error) {
	trace := core.BeginPerf()
	defer core.EndPerf("capture.monitor", trace)
	return platform.CaptureMonitor(m, includeCursor)
}

// Complete bakes the crop with annotations, stores it in PureShot's blob store,
// optionally saves and copies it via host.Clipboard(), and records the shot in shots.db.
// target is "copy", "save", "pin", or "done".
func (c *CaptureService) Complete(frame *image.RGBA, crop image.Rectangle,
	doc *annotate.Document, target string) (*ShotResult, error) {
	s := c.cfg()
	trace := core.BeginPerf()
	baked, err := annotate.Bake(frame, crop, doc)
	core.EndPerf("shot.bake", trace)
	if err != nil {
		return nil, err
	}
	pngData, err := annotate.EncodePNG(baked)
	if err != nil {
		return nil, err
	}
	width, height := baked.Bounds().Dx(), baked.Bounds().Dy()

	var rel string
	if c.blobs != nil {
		trace = core.BeginPerf()
		rel, _, err = c.blobs.SavePNG(pngData)
		core.EndPerf("shot.store", trace)
		if err != nil {
			return nil, err
		}
	}

	action := s.Capture.DefaultAction
	shouldSave := target == "save" || s.Capture.AutoSave ||
		(target == "done" && (action == config.CaptureActionSave || action == config.CaptureActionCopyAndSave))
	shouldCopy := target == "copy" || target == "clipboard" ||
		(target == "done" && (action == config.CaptureActionCopyAndSave || ((action == config.CaptureActionCopy || action == "") && s.Capture.CopyToClipboard))) ||
		(target == "pin" && s.Capture.CopyToClipboard)
	shouldPin := target == "pin" || (target == "done" && action == config.CaptureActionPin)

	var savedPath string
	if shouldSave {
		trace = core.BeginPerf()
		savedPath = c.saveToFolder(pngData)
		core.EndPerf("shot.autosave", trace)
	}

	copied := false
	if shouldCopy && c.clip != nil {
		trace = core.BeginPerf()
		copied = c.clip.WriteImage(pngData, PluginID) == nil
		core.EndPerf("shot.clipboard", trace)
	}

	annotationJSON := ""
	if doc != nil && !doc.IsEmpty() {
		annotationJSON, _ = doc.ToJSON()
	}
	var id int64
	if c.repo != nil {
		id, err = c.repo.InsertShot(ShotRecord{
			ImagePath:      rel,
			SavedPath:      savedPath,
			AnnotationJSON: annotationJSON,
			Width:          width,
			Height:         height,
			Target:         target,
			CreatedUTC:     time.Now().Unix(),
		})
		if err != nil && c.log != nil {
			c.log.Warnf("截图记录写入失败: %v", err)
		}
	}
	if c.log != nil {
		c.log.Infof("截图完成 %dx%d → %s 存盘=%s 剪贴板=%v 贴图=%v", width, height, rel, emptyDash(savedPath), copied, shouldPin)
	}
	return &ShotResult{
		PNG: pngData, Width: width, Height: height, ImagePath: rel,
		SavedPath: savedPath, ShotID: id, Copied: copied, ShouldPin: shouldPin, PinImage: baked,
	}, nil
}

func (c *CaptureService) saveToFolder(pngData []byte) string {
	s := c.cfg()
	folder := s.Capture.SaveFolder
	if folder == "" {
		folder = config.DefaultSaveFolder()
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		if c.log != nil {
			c.log.Errorf("创建截图目录失败: %v", err)
		}
		return ""
	}
	name := core.BuildFileName(s.Capture.FileNamePattern, ".png")
	path := core.UniquePath(folder, filepath.Join(folder, name))
	if err := os.WriteFile(path, pngData, 0o600); err != nil {
		if c.log != nil {
			c.log.Errorf("保存截图失败: %v", err)
		}
		return ""
	}
	return path
}

// SaveAs writes the PNG to an explicit path.
func (c *CaptureService) SaveAs(pngData []byte, path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, pngData, 0o600)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
