package pureclip

import (
	"bytes"
	"image/png"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"pureutils/pkg/purekit/clipboard"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/storage"
)

// HistoryService owns clipboard history ingestion (via purekit/clipboard.Service
// subscriptions), deduplication, retention, and replay.
type HistoryService struct {
	repo        *Repo
	blobs       *storage.BlobStore
	legacyBlobs *storage.BlobStore
	clip        clipboard.Service
	cfgFn       func() config.PureClipConfig
	log         *core.Logger
	onChange    func()

	monitoring       atomic.Bool
	mu               sync.Mutex
	writesSincePrune int
}

// NewHistoryService creates the service.
func NewHistoryService(
	repo *Repo,
	blobs *storage.BlobStore,
	legacyBlobs *storage.BlobStore,
	clip clipboard.Service,
	cfgFn func() config.PureClipConfig,
	log *core.Logger,
	onChange func(),
) *HistoryService {
	return &HistoryService{
		repo:        repo,
		blobs:       blobs,
		legacyBlobs: legacyBlobs,
		clip:        clip,
		cfgFn:       cfgFn,
		log:         log,
		onChange:    onChange,
	}
}

func (h *HistoryService) cfg() config.PureClipConfig {
	if h != nil && h.cfgFn != nil {
		return h.cfgFn()
	}
	return config.DefaultPureClipConfig()
}

// StartMonitoring activates clipboard history ingestion and starts the shared
// clipboard watcher if needed.
func (h *HistoryService) StartMonitoring() error {
	if h.clip != nil && !h.clip.Monitoring() {
		if err := h.clip.Start(); err != nil {
			return err
		}
	}
	h.monitoring.Store(true)
	if h.log != nil {
		h.log.Infof("PureClip 剪贴板监控已启动")
	}
	go h.Prune()
	return nil
}

// StopMonitoring pauses clipboard history ingestion for PureClip.
func (h *HistoryService) StopMonitoring() {
	h.monitoring.Store(false)
	if h.log != nil {
		h.log.Infof("PureClip 剪贴板监控已停止")
	}
}

// ToggleMonitoring flips monitoring and reports the new state.
func (h *HistoryService) ToggleMonitoring() bool {
	if h.monitoring.Load() {
		h.StopMonitoring()
		return false
	}
	if err := h.StartMonitoring(); err != nil {
		if h.log != nil {
			h.log.Errorf("恢复剪贴板监控失败: %v", err)
		}
		return false
	}
	return true
}

// Monitoring reports whether clipboard events are recorded into history.
func (h *HistoryService) Monitoring() bool {
	return h != nil && h.monitoring.Load()
}

// HandleClipboardEvent processes one event emitted by host.Clipboard().Subscribe.
func (h *HistoryService) HandleClipboardEvent(ev clipboard.Event) *Record {
	if h == nil || h.repo == nil || ev.Content == nil || ev.Content.IsEmpty() || ev.Ephemeral {
		return nil
	}
	// Ignore PureClip's own write-back replay.
	if ev.SelfWrite && ev.Source == PluginID {
		return nil
	}
	cfg := h.cfg()
	if !h.monitoring.Load() || !cfg.History.Enabled {
		return nil
	}

	content := ev.Content
	kind := content.PrimaryKind()
	if kind == core.KindImage && !cfg.History.CaptureImages {
		return nil
	}
	if kind == core.KindFiles && !cfg.History.CaptureFiles {
		return nil
	}
	if kind == core.KindOther {
		return nil
	}

	sourceProc := resolveEventSourceProcess(ev)
	if !ev.SelfWrite && sourceProc != "" && core.IsIgnoredProcess(cfg.History.IgnoredProcesses, sourceProc) {
		if h.log != nil {
			h.log.Debugf("跳过敏感进程 %s", sourceProc)
		}
		return nil
	}

	trace := core.BeginPerf()
	defer core.EndPerf("history.ingest", trace)

	hash := core.HashContent(content)
	now := ev.Timestamp
	if now.IsZero() {
		now = time.Now()
	}
	body, truncated := core.BodyOf(content, cfg.History.MaxInlineChars)
	preview := core.PreviewOf(content)

	var imagePath string
	var imageCreated bool
	var width, height int
	if kind == core.KindImage && len(content.ImagePNG) > 0 {
		if icfg, err := png.DecodeConfig(bytes.NewReader(content.ImagePNG)); err == nil {
			width, height = icfg.Width, icfg.Height
		}
		if h.blobs != nil {
			p, created, err := h.blobs.SavePNG(content.ImagePNG)
			if err != nil {
				if h.log != nil {
					h.log.Warnf("保存剪贴板图片失败: %v", err)
				}
			} else {
				imagePath, imageCreated = p, created
			}
		}
	}

	rec := Record{
		Kind:          kind,
		Preview:       preview,
		Body:          body,
		Hash:          hash,
		ByteSize:      core.EstimateSize(content, body),
		Width:         width,
		Height:        height,
		ImagePath:     imagePath,
		PayloadJSON:   EncodePayload(Payload{HTML: content.HTML, RTF: content.RTF, Files: content.Files}),
		CreatedUTC:    now.Unix(),
		LastUsedUTC:   now.Unix(),
		UseCount:      1,
		SourceProcess: sourceProc,
		BodyTruncated: truncated,
	}
	out, err := h.repo.InsertOrTouch(rec)
	if err != nil {
		if h.log != nil {
			h.log.Errorf("剪贴板入库失败: %v", err)
		}
		if imagePath != "" && imageCreated && h.blobs != nil {
			_ = h.blobs.Delete([]string{imagePath})
		}
		return nil
	}
	if out.WasDuplicate && imagePath != "" && imageCreated && out.Record.ImagePath != imagePath && h.blobs != nil {
		_ = h.blobs.Delete([]string{imagePath})
	}
	if !out.WasDuplicate {
		h.mu.Lock()
		h.writesSincePrune++
		prune := h.writesSincePrune%200 == 0
		h.mu.Unlock()
		if prune {
			go h.Prune()
		}
	}
	if h.onChange != nil {
		h.onChange()
	}
	if h.log != nil {
		h.log.Infof("记录剪贴板 #%d %s dup=%v \"%s\" proc=%s",
			out.Record.ID, out.Record.Kind, out.WasDuplicate, firstLine(out.Record.Preview), sourceProc)
	}
	return &out.Record
}

func resolveEventSourceProcess(ev clipboard.Event) string {
	if !ev.SelfWrite {
		return ev.SourceProcess
	}
	switch ev.Source {
	case "pureshot":
		return "PureShot"
	case "pureimage":
		return "PureImage"
	case "purezip":
		return "PureZip"
	default:
		if ev.SourceProcess != "" {
			return ev.SourceProcess
		}
		return ev.Source
	}
}

// Search lists history rows (including read-only legacy fallback rows).
func (h *HistoryService) Search(query string, kind *core.ClipboardKind, favoriteOnly *bool, limit int) ([]Record, error) {
	return h.repo.Search(query, kind, favoriteOnly, limit)
}

// Get returns one record by ID.
func (h *HistoryService) Get(id int64) (*Record, error) { return h.repo.Get(id) }

// ReadImage reads the PNG bytes referenced by rec from the writable or read-only
// legacy blob store.
func (h *HistoryService) ReadImage(rec *Record) ([]byte, error) {
	if rec == nil || rec.ImagePath == "" {
		return nil, os.ErrNotExist
	}
	stores := []*storage.BlobStore{h.blobs, h.legacyBlobs}
	if rec.IsLegacy() {
		stores = []*storage.BlobStore{h.legacyBlobs, h.blobs}
	}
	var lastErr error = os.ErrNotExist
	for _, bs := range stores {
		if bs == nil {
			continue
		}
		if data, err := bs.Read(rec.ImagePath); err == nil {
			return data, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

// ResolveImagePath locates the on-disk file for rec.ImagePath across writable
// and read-only legacy stores.
func (h *HistoryService) ResolveImagePath(rec *Record) string {
	if rec == nil || rec.ImagePath == "" {
		return ""
	}
	stores := []*storage.BlobStore{h.blobs, h.legacyBlobs}
	if rec.IsLegacy() {
		stores = []*storage.BlobStore{h.legacyBlobs, h.blobs}
	}
	for _, bs := range stores {
		if bs == nil {
			continue
		}
		if bs.Exists(rec.ImagePath) {
			return bs.Path(rec.ImagePath)
		}
	}
	return ""
}

// LoadContent reconstructs all representations of entry id for replay.
func (h *HistoryService) LoadContent(id int64) (*core.ClipboardContent, error) {
	rec, err := h.repo.Get(id)
	if err != nil || rec == nil {
		return nil, err
	}
	payload := DecodePayload(rec.PayloadJSON)
	content := &core.ClipboardContent{
		Text:  rec.Body,
		HTML:  payload.HTML,
		RTF:   payload.RTF,
		Files: payload.Files,
	}
	if rec.ImagePath != "" {
		pngData, err := h.ReadImage(rec)
		if err != nil {
			if h.log != nil {
				h.log.Warnf("历史图片文件缺失: %s", rec.ImagePath)
			}
		} else {
			content.ImagePNG = pngData
		}
	}
	if content.IsEmpty() {
		return nil, nil
	}
	return content, nil
}

// Restore writes a history row back to the system clipboard, verifying file
// addresses first.
func (h *HistoryService) Restore(id int64) RestoreOutcome {
	content, err := h.LoadContent(id)
	if err != nil {
		if h.log != nil {
			h.log.Errorf("读取历史 #%d 失败: %v", id, err)
		}
		return RestoreOutcome{}
	}
	if content == nil {
		return RestoreOutcome{}
	}
	var missing []string
	kind := content.PrimaryKind()
	if (kind == core.KindFiles || kind == core.KindImage) && len(content.Files) > 0 {
		live, miss := core.FileAddresses(content.Files)
		if kind == core.KindFiles {
			missing = miss
			if len(live) == 0 {
				if h.log != nil {
					h.log.Warnf("历史 #%d 的 %d 个地址都不在原位置，不写剪贴板", id, len(content.Files))
				}
				return RestoreOutcome{Wrote: false, MissingFiles: missing}
			}
		}
		content.Files = live
	}
	if h.clip != nil {
		trace := core.BeginPerf()
		err = h.clip.Write(content, clipboard.WriteOptions{Source: PluginID})
		core.EndPerf("clipboard.write", trace)
		if err != nil {
			if h.log != nil {
				h.log.Warnf("写回剪贴板失败 id=%d: %v", id, err)
			}
			return RestoreOutcome{Wrote: false, MissingFiles: missing}
		}
	}
	if err := h.repo.Touch(id); err != nil && h.log != nil {
		h.log.Warnf("更新历史使用次数失败: %v", err)
	}
	if h.onChange != nil {
		h.onChange()
	}
	return RestoreOutcome{Wrote: true, MissingFiles: missing}
}

// ToggleFavorite flips a writable row's favorite flag.
func (h *HistoryService) ToggleFavorite(id int64) (bool, error) {
	rec, err := h.repo.Get(id)
	if err != nil || rec == nil {
		return false, err
	}
	if rec.IsLegacy() {
		return false, ErrLegacyReadOnly
	}
	changed, err := h.repo.SetFavorite(id, !rec.Favorite)
	if err == nil && changed && h.onChange != nil {
		h.onChange()
	}
	return changed, err
}

// Delete removes a writable row and its blob.
func (h *HistoryService) Delete(id int64) error {
	res, err := h.repo.Delete(id)
	if err != nil {
		return err
	}
	if h.blobs != nil {
		_ = h.blobs.Delete(res.RemovedImages)
	}
	if h.onChange != nil {
		h.onChange()
	}
	return nil
}

// Clear removes every non-favorite row in writable clip.db (never touches legacy data).
func (h *HistoryService) Clear() error {
	res, err := h.repo.ClearAll()
	if err != nil {
		return err
	}
	if h.blobs != nil {
		_ = h.blobs.Delete(res.RemovedImages)
	}
	if h.onChange != nil {
		h.onChange()
	}
	if h.log != nil {
		h.log.Infof("清空历史：移除 %d 条", res.RemovedEntries)
	}
	return nil
}

// Prune applies the retention policy on writable clip.db.
func (h *HistoryService) Prune() {
	cfg := h.cfg()
	res, err := h.repo.Trim(cfg.History.MaxEntries, cfg.History.MaxAgeDays)
	if err != nil {
		if h.log != nil {
			h.log.Warnf("历史清理失败: %v", err)
		}
		return
	}
	if h.blobs != nil {
		_ = h.blobs.Delete(res.RemovedImages)
	}
	if res.RemovedEntries > 0 {
		if h.log != nil {
			h.log.Infof("保留策略清理 %d 条", res.RemovedEntries)
		}
		if h.onChange != nil {
			h.onChange()
		}
	}
}

// CollectOrphans deletes blob files inside PureClip's writable store that no
// writable row references. Never touches legacy images directories.
func (h *HistoryService) CollectOrphans() (int, error) {
	if h.blobs == nil {
		return 0, nil
	}
	referenced, err := h.repo.AllImagePaths()
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, p := range referenced {
		seen[p] = true
	}
	all, err := h.blobs.Enumerate()
	if err != nil {
		return 0, err
	}
	var orphans []string
	for _, p := range all {
		if !seen[p] {
			orphans = append(orphans, p)
		}
	}
	_ = h.blobs.Delete(orphans)
	if len(orphans) > 0 && h.log != nil {
		h.log.Infof("回收孤儿图片 %d 张", len(orphans))
	}
	return len(orphans), nil
}

func firstLine(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
