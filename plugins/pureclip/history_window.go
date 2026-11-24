package pureclip

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"

	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/platform"
	pureui "pureutils/pkg/purekit/ui"
)

const (
	compactHistoryWidth         = 396
	compactHistoryHeight        = 520
	historyWidth                = 840
	historyHeight               = 600
	defaultSplitWidth           = float32(390)
	maxThumbCacheItems          = 120
	maxPreviewCacheItems        = 12
	minSnapshotItems            = 500
	typingPreviewDebounce       = 150 * time.Millisecond
	deepSearchDebounce          = 120 * time.Millisecond
	compactPreviewTextWidth     = 368
	compactPreviewMinWidth      = 300
	compactPreviewMaxWidth      = 420
	compactPreviewMinHeight     = 168
	compactPreviewMaxHeight     = 420
	compactPreviewImageMinH     = 210
	compactPreviewImageMaxH     = 408
	compactTextPreviewThreshold = 26.0
)

type indexedRecord struct {
	rec       Record
	previewLC string
	bodyLC    string
	processLC string
}

type cachedPreview struct {
	bmp *ui.Bitmap
	w   float32
	h   float32
}

type compactPreviewKey struct {
	active   bool
	recordID int64
	hash     string
	favorite bool
	bodyLen  int
	imgPath  string
}

type historyView struct {
	plugin *Plugin
	win    *mygo.Window

	compact     bool
	query       string
	filter      int
	allItems    []indexedRecord
	mayHaveMore bool
	searchGen   uint64
	searchTimer *time.Timer
	items       []Record
	list        ui.ListState
	search      ui.Handle
	splitWidth  float32

	selected     int
	status       string
	pendingToast string
	pinned       bool
	confirmClear bool

	suppressBlur       bool
	closed             bool
	focusSearch        bool
	previousForeground platform.HWND

	previewID        int64
	previewBitmap    *ui.Bitmap
	previewImgW      float32
	previewImgH      float32
	thumbCache       map[int64]*ui.Bitmap
	thumbLoading     map[int64]bool
	previewCache     map[int64]cachedPreview
	previewLoading   map[int64]bool
	previewLoadingID int64

	lastPreviewKey     compactPreviewKey
	previewSyncQueued  atomic.Bool
	previewSyncSeq     uint64
	previewSyncTimer   *time.Timer
	typingDelayPreview bool

	pending   string
	pendingID int64
}

func newHistoryView(a *Plugin) *historyView {
	h := &historyView{
		plugin:         a,
		selected:       -1,
		focusSearch:    true,
		splitWidth:     defaultSplitWidth,
		thumbCache:     map[int64]*ui.Bitmap{},
		thumbLoading:   map[int64]bool{},
		previewCache:   map[int64]cachedPreview{},
		previewLoading: map[int64]bool{},
	}
	h.Reload()
	return h
}

func newCompactHistoryView(a *Plugin) *historyView {
	h := &historyView{
		plugin:         a,
		compact:        true,
		selected:       -1,
		focusSearch:    true,
		splitWidth:     defaultSplitWidth,
		thumbCache:     map[int64]*ui.Bitmap{},
		thumbLoading:   map[int64]bool{},
		previewCache:   map[int64]cachedPreview{},
		previewLoading: map[int64]bool{},
	}
	h.Reload()
	return h
}

func (h *historyView) toggleCompact() {
	h.compact = !h.compact
	h.focusSearch = true
	h.typingDelayPreview = false
	h.lastPreviewKey = compactPreviewKey{}
	if !h.compact && h.plugin != nil {
		h.plugin.hideHistoryPreview()
	}
	if h.win != nil && h.plugin != nil {
		if h.compact {
			h.plugin.resizePanel(h.win, compactHistoryWidth, compactHistoryHeight)
		} else {
			h.plugin.resizePanel(h.win, historyWidth, historyHeight)
		}
	}
	if h.compact {
		h.schedulePreviewSync()
	}
	h.invalidate()
}

func (h *historyView) selectedRecord() *Record {
	if h.selected < 0 || h.selected >= len(h.items) {
		return nil
	}
	return &h.items[h.selected]
}

func textDisplayUnits(s string) float64 {
	var units float64
	for _, r := range s {
		if r < 128 {
			units += 0.55
		} else {
			units += 1.0
		}
	}
	return units
}

func shouldShowCompactPreview(rec *Record) bool {
	if rec == nil {
		return false
	}
	switch rec.Kind {
	case core.KindImage:
		return true
	case core.KindText, core.KindRichText:
		body := rec.Body
		if body == "" {
			body = rec.Preview
		}
		trimmed := strings.TrimSpace(body)
		if rec.BodyTruncated || strings.ContainsAny(trimmed, "\r\n") {
			return true
		}
		if textDisplayUnits(trimmed) > compactTextPreviewThreshold {
			return true
		}
		return rec.Body != "" && strings.TrimSpace(rec.Body) != strings.TrimSpace(rec.Preview)
	case core.KindFiles:
		payload := DecodePayload(rec.PayloadJSON)
		if len(payload.Files) > 1 {
			return true
		}
		if len(payload.Files) == 1 && textDisplayUnits(payload.Files[0]) > compactTextPreviewThreshold {
			return true
		}
		return firstImageFileFromPayload(rec.PayloadJSON) != ""
	default:
		return false
	}
}

func previewPopoverSize(rec *Record) (int, int) {
	if rec == nil {
		return compactPreviewTextWidth, 240
	}
	switch rec.Kind {
	case core.KindImage:
		if rec.Width > 0 && rec.Height > 0 {
			scale := 1.0
			if sx := 372.0 / float64(rec.Width); sx < scale {
				scale = sx
			}
			if sy := 296.0 / float64(rec.Height); sy < scale {
				scale = sy
			}
			stageW := int(float64(rec.Width)*scale) + 28
			stageH := int(float64(rec.Height)*scale) + 28
			w := clampInt(stageW+20, compactPreviewMinWidth, compactPreviewMaxWidth)
			h := clampInt(stageH+88, compactPreviewImageMinH, compactPreviewImageMaxH)
			return w, h
		}
		return 360, 260
	case core.KindFiles:
		n := len(DecodePayload(rec.PayloadJSON).Files)
		h := clampInt(96+n*54, 164, 360)
		return compactPreviewTextWidth, h
	default:
		body := rec.Body
		if body == "" {
			body = rec.Preview
		}
		lines, _, _ := pureui.WrapTextLines(body, 12, float64(compactPreviewTextWidth-48), 24)
		textH := len(lines)*18 + 26
		h := clampInt(textH+88, compactPreviewMinHeight, compactPreviewMaxHeight)
		return compactPreviewTextWidth, h
	}
}

func (h *historyView) compactPreviewKey() compactPreviewKey {
	if h.closed || !h.compact || h.confirmClear {
		return compactPreviewKey{}
	}
	rec := h.selectedRecord()
	if !shouldShowCompactPreview(rec) {
		return compactPreviewKey{}
	}
	return compactPreviewKey{
		active:   true,
		recordID: rec.ID,
		hash:     rec.Hash,
		favorite: rec.Favorite,
		bodyLen:  len(rec.Body),
		imgPath:  rec.ImagePath,
	}
}

func (h *historyView) schedulePreviewSync() {
	if h.plugin == nil || h.plugin.headless() || h.win == nil || h.closed {
		return
	}
	delay := h.typingDelayPreview
	h.typingDelayPreview = false
	if h.previewSyncTimer != nil {
		h.previewSyncTimer.Stop()
		h.previewSyncTimer = nil
	}
	if delay {
		h.previewSyncSeq++
		seq := h.previewSyncSeq
		h.previewSyncTimer = time.AfterFunc(typingPreviewDebounce, func() {
			mygo.RunOnMain(func() {
				if h.closed || h.plugin == nil || h.plugin.view != h || h.previewSyncSeq != seq {
					return
				}
				h.plugin.syncHistoryPreview(h)
			})
		})
		return
	}
	if h.previewSyncQueued.Swap(true) {
		return
	}
	h.previewSyncSeq++
	seq := h.previewSyncSeq
	go func() {
		mygo.RunOnMain(func() {
			h.previewSyncQueued.Store(false)
			if h.closed || h.plugin == nil || h.plugin.view != h || h.previewSyncSeq != seq {
				return
			}
			h.plugin.syncHistoryPreview(h)
		})
	}()
}

// Reload refreshes the in-memory history snapshot from SQLite and applies
// the active search/filter criteria.
func (h *historyView) Reload() {
	limit := h.plugin.Config().History.ListLimit
	if limit < minSnapshotItems {
		limit = minSnapshotItems
	}
	rows, err := h.plugin.Service().Search("", nil, nil, limit)
	if err != nil {
		h.status = "读取历史失败: " + err.Error()
		h.allItems = nil
		h.mayHaveMore = false
		h.items = nil
		h.selected = -1
		return
	}
	h.allItems = buildIndexedRecords(rows)
	h.mayHaveMore = len(rows) >= limit
	h.applyFilter(false)
}

func buildIndexedRecords(rows []Record) []indexedRecord {
	out := make([]indexedRecord, len(rows))
	for i, r := range rows {
		out[i] = indexedRecord{
			rec:       r,
			previewLC: strings.ToLower(r.Preview),
			bodyLC:    strings.ToLower(r.Body),
			processLC: strings.ToLower(r.SourceProcess),
		}
	}
	return out
}

func matchIndexedRecord(ir *indexedRecord, tokens []string) bool {
	for _, tok := range tokens {
		if !strings.Contains(ir.previewLC, tok) &&
			!strings.Contains(ir.bodyLC, tok) &&
			!strings.Contains(ir.processLC, tok) {
			return false
		}
	}
	return true
}

func (h *historyView) applyFilter(fromTyping bool) {
	selectedID := int64(0)
	if h.selected >= 0 && h.selected < len(h.items) {
		selectedID = h.items[h.selected].ID
	}

	q := strings.ToLower(strings.TrimSpace(h.query))
	tokens := strings.Fields(q)
	filtered := make([]Record, 0, len(h.allItems))
	for i := range h.allItems {
		ir := &h.allItems[i]
		switch h.filter {
		case 1:
			if ir.rec.Kind != core.KindText && ir.rec.Kind != core.KindRichText {
				continue
			}
		case 2:
			if ir.rec.Kind != core.KindImage {
				continue
			}
		case 3:
			if ir.rec.Kind != core.KindFiles {
				continue
			}
		case 4:
			if !ir.rec.Favorite {
				continue
			}
		}
		if len(tokens) > 0 && !matchIndexedRecord(ir, tokens) {
			continue
		}
		filtered = append(filtered, ir.rec)
	}

	h.items = filtered
	h.selected = -1
	if selectedID != 0 {
		for i := range filtered {
			if filtered[i].ID == selectedID {
				h.selected = i
				break
			}
		}
	}
	if h.selected < 0 && len(filtered) > 0 {
		h.selected = 0
	}

	newSelectedID := int64(0)
	if h.selected >= 0 && h.selected < len(filtered) {
		newSelectedID = filtered[h.selected].ID
	}
	if newSelectedID != h.previewID {
		h.previewID = 0
		h.previewBitmap = nil
		h.previewImgW = 0
		h.previewImgH = 0
		h.previewLoadingID = 0
	}

	h.typingDelayPreview = fromTyping && h.compact
	if h.status == "" || strings.HasPrefix(h.status, "读取历史失败") {
		h.status = "就绪"
	}

	h.searchGen++
	if h.searchTimer != nil {
		h.searchTimer.Stop()
		h.searchTimer = nil
	}
	if h.mayHaveMore && q != "" {
		h.scheduleDeepSearch(h.query, h.filter, h.searchGen)
	}
}

func (h *historyView) currentFilterParams() (*core.ClipboardKind, *bool) {
	var kind *core.ClipboardKind
	var favorite *bool
	switch h.filter {
	case 1:
		k := core.KindText
		kind = &k
	case 2:
		k := core.KindImage
		kind = &k
	case 3:
		k := core.KindFiles
		kind = &k
	case 4:
		f := true
		favorite = &f
	}
	return kind, favorite
}

func (h *historyView) scheduleDeepSearch(query string, filter int, gen uint64) {
	if h.plugin == nil || h.plugin.headless() || h.closed {
		return
	}
	if h.searchTimer != nil {
		h.searchTimer.Stop()
	}
	kind, favorite := h.currentFilterParams()
	limit := h.plugin.Config().History.ListLimit
	if limit <= 0 {
		limit = 200
	}
	h.searchTimer = time.AfterFunc(deepSearchDebounce, func() {
		items, err := h.plugin.Service().Search(query, kind, favorite, limit)
		if err != nil {
			return
		}
		mygo.RunOnMain(func() {
			if h.closed || h.searchGen != gen || h.query != query || h.filter != filter {
				return
			}
			seen := make(map[int64]bool, len(h.allItems))
			for i := range h.allItems {
				seen[h.allItems[i].rec.ID] = true
			}
			var extra []Record
			for _, it := range items {
				if !seen[it.ID] {
					extra = append(extra, it)
				}
			}
			if len(extra) == 0 {
				return
			}
			h.items = append(h.items, extra...)
			if h.selected < 0 && len(h.items) > 0 {
				h.selected = 0
			}
			h.invalidate()
		})
	})
}

func (h *historyView) stopTimers() {
	h.searchGen++
	if h.searchTimer != nil {
		h.searchTimer.Stop()
		h.searchTimer = nil
	}
	h.previewSyncSeq++
	if h.previewSyncTimer != nil {
		h.previewSyncTimer.Stop()
		h.previewSyncTimer = nil
	}
}

func (h *historyView) clearImageCaches() {
	h.thumbCache = map[int64]*ui.Bitmap{}
	h.thumbLoading = map[int64]bool{}
	h.previewCache = map[int64]cachedPreview{}
	h.previewLoading = map[int64]bool{}
	h.previewID = 0
	h.previewBitmap = nil
	h.previewImgW = 0
	h.previewImgH = 0
	h.previewLoadingID = 0
}

func (h *historyView) putThumbCache(id int64, bmp *ui.Bitmap) {
	if h.thumbCache == nil {
		h.thumbCache = map[int64]*ui.Bitmap{}
	}
	if len(h.thumbCache) >= maxThumbCacheItems {
		for k := range h.thumbCache {
			if k == id {
				continue
			}
			delete(h.thumbCache, k)
			if len(h.thumbCache) < maxThumbCacheItems/2 {
				break
			}
		}
	}
	h.thumbCache[id] = bmp
}

func (h *historyView) putPreviewCache(id int64, cp cachedPreview) {
	if h.previewCache == nil {
		h.previewCache = map[int64]cachedPreview{}
	}
	if len(h.previewCache) >= maxPreviewCacheItems {
		for k := range h.previewCache {
			if k == id {
				continue
			}
			delete(h.previewCache, k)
			if len(h.previewCache) < maxPreviewCacheItems/2 {
				break
			}
		}
	}
	h.previewCache[id] = cp
}

func (h *historyView) view(c *ui.Context) {
	if h.closed {
		return
	}
	pal := pureui.ApplyTheme(c)
	if h.splitWidth <= 0 {
		h.splitWidth = defaultSplitWidth
	}
	if h.pending != "" {
		h.applyPending(c)
	}
	if h.pendingToast != "" {
		c.Toast(h.pendingToast)
		h.pendingToast = ""
	}
	if h.selected >= len(h.items) {
		h.selected = len(h.items) - 1
	}
	if h.selected < -1 {
		h.selected = -1
	}
	if !h.confirmClear && c.Shortcut(ui.Cmd, ui.KeyF) {
		h.search.Focus()
	}
	if !h.confirmClear && c.Shortcut(0, ui.KeyTab) {
		h.toggleCompact()
	}

	if h.compact {
		h.compactView(c, pal)
	} else {
		ui.Column(c).Fill().Background(pal.Bg).Radius(14).Border(1, pal.BorderStrong).InsetShadow(0, 1, 0, 0, pal.Specular).Clip().Children(func() {
			h.searchHeader(c, pal)
			h.filterRow(c, pal)
			ui.Box(c).Grow(1).MinHeight(0).Children(func() {
				if len(h.items) == 0 {
					h.emptyBody(c, pal)
					return
				}
				ui.Split(c, &h.splitWidth, func() {
					h.listPane(c, pal)
				}, func() {
					h.previewPane(c, pal)
				}).Fill()
			})
			h.footer(c, pal)
		})
	}

	h.handleShortcuts(c)

	if switchIdx := ui.AlertDialog(c, &h.confirmClear,
		"清空非收藏历史？",
		"收藏项会被保留，其余记录删除后无法撤销。",
		"取消", "清空",
	); switchIdx == 1 {
		if err := h.plugin.Service().Clear(); err != nil {
			h.status = "清空失败: " + err.Error()
		} else {
			h.status = "已清空非收藏记录"
			c.Toast("已清空非收藏记录")
		}
		h.clearImageCaches()
		h.Reload()
		h.invalidate()
	}

	if key := h.compactPreviewKey(); key != h.lastPreviewKey {
		h.lastPreviewKey = key
		h.schedulePreviewSync()
	}

	if !h.confirmClear && c.Shortcut(0, ui.KeyEscape) {
		h.closeWindow()
	}
}

func (h *historyView) handleShortcuts(c *ui.Context) {
	if h.closed || h.confirmClear {
		return
	}
	inSearch := h.search.FocusWithin(c)
	inList := h.list.Focused(c)
	if inSearch {
		if c.Shortcut(0, ui.KeyUp) {
			h.moveSelection(-1)
		}
		if c.Shortcut(0, ui.KeyDown) {
			h.moveSelection(1)
		}
	}
	if h.selected < 0 || h.selected >= len(h.items) {
		return
	}
	rec := h.items[h.selected]

	quickKeys := [...]ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
	for i, key := range quickKeys {
		if i < len(h.items) && (c.Shortcut(ui.Cmd, key) || (inList && !inSearch && h.list.Shortcut(c, 0, key))) {
			h.selected = i
			if h.items[i].ID != h.previewID {
				h.previewID = 0
				h.previewBitmap = nil
				h.previewImgW = 0
				h.previewImgH = 0
				h.previewLoadingID = 0
			}
			h.list.ScrollTo(i, ui.Center)
			h.pending, h.pendingID = "paste", h.items[i].ID
			h.invalidate()
			return
		}
	}

	switch {
	case c.Shortcut(ui.Cmd, ui.KeyP), inList && !inSearch && h.list.Shortcut(c, 0, ui.KeyP):
		h.pending, h.pendingID = "pin", rec.ID
	case c.Shortcut(ui.Cmd, ui.KeyDelete), (inList || (inSearch && h.query == "")) && c.Shortcut(0, ui.KeyDelete):
		h.pending, h.pendingID = "delete", rec.ID
	case (inList || (inSearch && h.query == "")) && c.Shortcut(ui.Cmd, ui.KeyC):
		h.pending, h.pendingID = "copy", rec.ID
	case inList && !inSearch && h.list.Shortcut(c, 0, ui.KeyF):
		h.pending, h.pendingID = "favorite", rec.ID
	case !inSearch && !inList && c.Shortcut(0, ui.KeyEnter):
		h.pending, h.pendingID = "paste", rec.ID
	}
	if h.pending != "" {
		h.invalidate()
	}
}

func (h *historyView) openSettings() {
	h.suppressBlur = true
	if h.plugin != nil && h.plugin.host != nil {
		h.plugin.host.OpenSettings(PluginID)
	}
	time.AfterFunc(400*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			h.suppressBlur = false
			h.invalidate()
		})
	})
}

func (h *historyView) toggleMonitoringWithToast() {
	on := h.plugin.ToggleMonitoring()
	if on {
		h.status = "剪贴板监控已恢复"
		h.pendingToast = "剪贴板监控已恢复"
	} else {
		h.status = "剪贴板监控已暂停"
		h.pendingToast = "剪贴板监控已暂停"
	}
	h.invalidate()
}

func (h *historyView) searchHeader(c *ui.Context, pal pureui.Palette) {
	height := float32(48)
	placeholder := "搜索剪贴板历史、文件名或来源…"
	if h.compact {
		height = 46
		placeholder = "搜索剪贴板历史…"
	}
	ui.Row(c).Height(height).Padding(8, 12).Gap(7).AlignItems(ui.Center).Background(pal.BgElevated).DragWindow().Children(func() {
		field := ui.SearchField(c, &h.query).Placeholder(placeholder).Grow(1).MinWidth(120).Bind(&h.search)
		if h.focusSearch {
			h.search.Focus()
			h.focusSearch = false
		}
		if field.Changed() {
			h.applyFilter(h.query != "")
		}
		if field.Submitted() && h.selected >= 0 && h.selected < len(h.items) {
			h.pending, h.pendingID = "paste", h.items[h.selected].ID
			h.invalidate()
		}

		if h.query != "" {
			if pureui.IconButton(c, pureui.IconXCircle, "清空搜索", "清空搜索关键字", false, false, pal).Size(24, 24).Radius(6).Clicked() {
				h.query = ""
				h.focusSearch = true
				h.applyFilter(false)
				h.invalidate()
			}
		}

		monitoring := h.plugin.Monitoring()
		dotColor := pal.Success
		monLabel := "监听中"
		monTip := "正在监听剪贴板（点击暂停）"
		if !monitoring {
			dotColor = pal.Warning
			monLabel = "已暂停"
			monTip = "剪贴板监听已暂停（点击恢复）"
		}
		monBtn := ui.ButtonBase(c).Size(24, 24).Radius(6).Center().Label(monLabel).Tooltip(monTip)
		if monBtn.Pressed() {
			monBtn.Background(pal.CardActive)
		} else if monBtn.Hovered() {
			monBtn.Background(pal.CardHover)
		} else {
			monBtn.Background(ui.Transparent)
		}
		monBtn.Children(func() {
			ui.Box(c).Size(7, 7).Radius(3.5).Background(dotColor)
		})
		monBtn.OnClick(h.toggleMonitoringWithToast)

		viewTip := "展开双栏预览 (Tab)"
		if !h.compact {
			viewTip = "收起为精简浮窗 (Tab)"
		}
		if pureui.IconButton(c, pureui.IconPanelRight, "切换视图", viewTip, !h.compact, false, pal).Size(26, 26).Radius(6).Clicked() {
			h.toggleCompact()
		}
		if !h.compact || h.pinned {
			if pureui.IconButton(c, pureui.IconPin, "钉住", "钉住面板（失焦不自动关闭）", h.pinned, false, pal).Size(26, 26).Radius(6).Clicked() {
				h.pinned = !h.pinned
				h.invalidate()
			}
		}

		moreBtn := pureui.IconButton(c, pureui.IconMoreHorizontal, "更多操作", "更多操作", false, false, pal).Size(26, 26).Radius(6)
		moreBtn.Menu(func(m *ui.Menu) {
			if m.Item("钉住面板").Checked(h.pinned).Chosen() {
				h.pinned = !h.pinned
				h.invalidate()
			}
			monMenuLabel := "暂停剪贴板监听"
			if !monitoring {
				monMenuLabel = "恢复剪贴板监听"
			}
			if m.Item(monMenuLabel).Chosen() {
				h.toggleMonitoringWithToast()
			}
			m.Separator()
			if m.Item("清空非收藏历史…").Chosen() {
				h.confirmClear = true
				h.invalidate()
			}
			if m.Item("打开 PureClip 设置…").Chosen() {
				h.openSettings()
			}
			m.Separator()
			if m.Item("关闭 (Esc)").Chosen() {
				h.closeWindow()
			}
		})
	})
	ui.Box(c).Height(1).FillWidth().Background(pal.Border)
}

func (h *historyView) filterRow(c *ui.Context, pal pureui.Palette) {
	filters := [...]string{"全部", "文本", "图片", "文件", "收藏"}
	ui.Row(c).Height(34).Padding(0, 12).Gap(4).AlignItems(ui.Center).Background(pal.Bg).Children(func() {
		for i, label := range filters {
			active := h.filter == i
			btn := ui.ButtonBase(c).Padding(3, 9).Radius(7).Center().Label(label)
			fg := pal.TextMuted
			bg := ui.Transparent
			weight := 500
			if active {
				weight = 600
				bg = pal.AccentSoft
				fg = pal.Accent
				if pal.Dark {
					fg = pal.AccentHover
				}
			} else if btn.Hovered() {
				bg = pal.CardHover
				fg = pal.Text
			}
			btn.Background(bg)
			btn.Children(func() {
				ui.Text(c, label).FontSize(11).FontWeight(weight).SingleLine().TextColor(fg)
			})
			if btn.Clicked() {
				h.filter = i
				h.focusSearch = true
				h.applyFilter(false)
				h.invalidate()
			}
		}
		ui.Spacer(c)
		countText := fmt.Sprintf("%d 条", len(h.items))
		if h.compact {
			countText = fmt.Sprintf("%d", len(h.items))
		}
		ui.Text(c, countText).FontSize(10.5).SingleLine().TextColor(pal.TextMuted.Alpha(0.75))
	})
	ui.Box(c).Height(1).FillWidth().Background(pal.Border)
}

func (h *historyView) emptyBody(c *ui.Context, pal pureui.Palette) {
	if h.query != "" || h.filter != 0 {
		pureui.EmptyState(c, pureui.IconSearch, "没有匹配的记录", "尝试更换搜索关键字，或重置当前类型筛选条件。", pal, func() {
			btn := ui.ButtonBase(c).Height(34).Padding(0, 16).Radius(9).Center().Label("重置筛选").
				InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.22)).
				Shadow(0, 4, 12, 0, pal.Accent.Alpha(0.28))
			if btn.Pressed() {
				btn.Background(pal.AccentPressed)
			} else if btn.Hovered() {
				btn.Gradient(pal.AccentHover, pal.Accent, 135)
			} else {
				btn.Gradient(pal.Accent, pal.AccentEnd, 135)
			}
			btn.Children(func() {
				ui.Text(c, "重置筛选").FontSize(12).FontWeight(600).SingleLine().TextColor(pal.AccentText)
			})
			btn.OnClick(func() {
				h.query = ""
				h.filter = 0
				h.applyFilter(false)
				h.invalidate()
			})
		})
		return
	}
	pureui.EmptyState(c, pureui.IconClipboard, "剪贴板历史为空", "复制文本、代码、图片或文件后，这里会自动收录并智能去重。", pal, func() {
		tips := []struct{ k, v string }{
			{"Ctrl+Shift+V", "呼出面板"},
			{"1-9", "快捷粘贴"},
			{"F", "星标保留"},
		}
		ui.Row(c).Gap(8).Wrap().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			for _, tip := range tips {
				ui.Row(c).Padding(5, 10).Gap(6).Radius(8).Background(pal.CardBg).Border(1, pal.Border).AlignItems(ui.Center).Children(func() {
					pureui.Keycap(c, tip.k, pal)
					ui.Text(c, tip.v).FontSize(11).FontWeight(500).SingleLine().TextColor(pal.TextSecondary)
				})
			}
		})
	})
}

func (h *historyView) listPane(c *ui.Context, pal pureui.Palette) {
	h.list.Key = func(i int) any { return h.items[i].ID }
	h.list.Label = func(i int) string { return rowTitle(h.items[i]) }
	h.list.Selected = &h.selected

	selectedBg := pal.AccentSoft
	listTheme := *c.Theme()
	listTheme.Accent = selectedBg
	listTheme.AccentText = pal.Text
	listTheme.SurfaceHover = pal.CardHover
	listTheme.Radius = 8

	ui.Column(c).Fill().Background(pal.Bg).Padding(6).Children(func() {
		ui.Themed(c, &listTheme).Fill().Background(ui.Transparent).Children(func() {
			list := ui.List(c, &h.list, len(h.items)).Gap(2).Rows(func(row ui.ListRow) {
				h.renderRow(row, pal, selectedBg)
			}).Grow(1)
			if list.Changed() {
				if rec := h.selectedRecord(); rec == nil || rec.ID != h.previewID {
					h.previewID = 0
					h.previewBitmap = nil
					h.previewImgW = 0
					h.previewImgH = 0
					h.previewLoadingID = 0
				}
			}
			if list.Submitted() {
				if h.selected >= 0 && h.selected < len(h.items) {
					h.pending = "paste"
					h.pendingID = h.items[h.selected].ID
					h.invalidate()
				}
			}
		})
	})
}

func (h *historyView) rowContextMenu(r ui.Element, rec Record) {
	r.ContextMenu(func(m *ui.Menu) {
		if m.Item("粘贴").Chosen() {
			h.pending, h.pendingID = "paste", rec.ID
		}
		if m.Item("复制到剪贴板").Chosen() {
			h.pending, h.pendingID = "copy", rec.ID
		}
		if m.Item("收藏").Checked(rec.Favorite).Chosen() {
			h.pending, h.pendingID = "favorite", rec.ID
		}
		if h.plugin.canPin() {
			if m.Item("贴成图片").Chosen() {
				h.pending, h.pendingID = "pin", rec.ID
			}
		}
		if h.plugin.canPreviewImage() && (rec.Kind == core.KindImage || (rec.Kind == core.KindFiles && firstImageFileFromPayload(rec.PayloadJSON) != "")) {
			if m.Item("用 PureImage 查看").Chosen() {
				h.pending, h.pendingID = "view", rec.ID
			}
		}
		if rec.Kind == core.KindFiles {
			m.Separator()
			if m.Item("复制路径").Chosen() {
				h.pending, h.pendingID = "copy-path", rec.ID
			}
			if m.Item("打开所在文件夹").Chosen() {
				h.pending, h.pendingID = "reveal", rec.ID
			}
		}
		m.Separator()
		if m.Item("删除").Chosen() {
			h.pending, h.pendingID = "delete", rec.ID
		}
	})
}

func (h *historyView) compactView(c *ui.Context, pal pureui.Palette) {
	ui.Column(c).Fill().Background(pal.Bg).Radius(12).Border(1, pal.BorderStrong).InsetShadow(0, 1, 0, 0, pal.Specular).Clip().Children(func() {
		h.searchHeader(c, pal)
		h.filterRow(c, pal)
		ui.Box(c).Grow(1).MinHeight(0).Children(func() {
			if len(h.items) == 0 {
				h.emptyBody(c, pal)
				return
			}
			h.listPane(c, pal)
		})
		h.footer(c, pal)
	})
}

func rowTitle(rec Record) string {
	if rec.Kind == core.KindImage {
		sizeStr := compactBytes(rec.ByteSize)
		if rec.Width > 0 && rec.Height > 0 {
			return fmt.Sprintf("PNG %d×%d · %s", rec.Width, rec.Height, sizeStr)
		}
		return "PNG 图片 · " + sizeStr
	}
	return rec.Preview
}

func rowMeta(rec Record) string {
	parts := make([]string, 0, 3)
	switch rec.Kind {
	case core.KindFiles:
		parts = append(parts, "文件")
	case core.KindRichText:
		parts = append(parts, "富文本")
	}
	parts = append(parts, timeAgo(rec.LastUsedUTC))
	if rec.SourceProcess != "" {
		parts = append(parts, rec.SourceProcess)
	}
	return strings.Join(parts, " · ")
}

func (h *historyView) renderRow(row ui.ListRow, pal pureui.Palette, selectedBg ui.Color) {
	rec := h.items[row.Index]
	c := row.Context
	selected := row.Selected()

	r := ui.Row(c).Padding(6, 9).Gap(9).Radius(8).AlignItems(ui.Center)
	bg := ui.Transparent
	if selected {
		bg = selectedBg
		r.Border(1, pal.AccentBorder)
	} else if r.Hovered() {
		bg = pal.CardHover
	}
	r.Background(bg)
	h.rowContextMenu(r, rec)

	r.Children(func() {
		switch rec.Kind {
		case core.KindImage:
			if thumb := h.loadThumbnail(&rec); thumb != nil {
				ui.Box(c).Size(34, 26).Radius(5).Border(1, pal.Border).Clip().Shrink(0).Children(func() {
					ui.Image(c, thumb).Fit(ui.Cover).Size(34, 26)
				})
			} else {
				ui.Box(c).Size(34, 26).Radius(5).Background(pal.BgInset).Border(1, pal.Border).Center().Shrink(0).Children(func() {
					ui.Icon(c, pureui.IconImage).Size(14, 14).TextColor(pal.TextMuted)
				})
			}
		case core.KindFiles:
			ui.Box(c).Size(18, 18).Center().Shrink(0).Children(func() {
				ui.Icon(c, pureui.IconFiles).Size(14, 14).TextColor(pal.KindFiles)
			})
		default:
			dotCol := pal.TextMuted.Alpha(0.42)
			if rec.Kind == core.KindRichText {
				dotCol = pal.KindRich.Alpha(0.75)
			}
			if selected {
				dotCol = pal.Accent
			}
			ui.Box(c).Size(12, 12).Center().Shrink(0).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(dotCol)
			})
		}

		titleWeight := 500
		if selected {
			titleWeight = 600
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			preview := ui.Text(c, rowTitle(rec)).FontSize(12).FontWeight(titleWeight).SingleLine().TextColor(pal.Text)
			if preview.RowDoubleClicked() || r.RowDoubleClicked() {
				h.pending, h.pendingID = "paste", rec.ID
				h.invalidate()
			}
			ui.Text(c, rowMeta(rec)).FontSize(10).SingleLine().TextColor(pal.TextMuted.Alpha(0.82))
		})

		if rec.Favorite {
			ui.Icon(c, pureui.IconStarFilled).Size(11, 11).TextColor(pal.Favorite)
		}
		if row.Index < 9 {
			idxCol := pal.TextMuted.Alpha(0.45)
			idxW := 400
			if selected {
				idxCol = pal.Accent
				idxW = 600
			}
			ui.Row(c).Width(14).Justify(ui.End).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, fmt.Sprintf("%d", row.Index+1)).FontSize(10.5).FontWeight(idxW).SingleLine().TextColor(idxCol)
			})
		}
	})
}

func softActionButton(c *ui.Context, icon *ui.SVG, label, tooltip string, active, danger bool, size float32, pal pureui.Palette) ui.Element {
	b := ui.ButtonBase(c).Size(size, size).Radius(8).Center().Label(label)
	if tooltip != "" {
		b.Tooltip(tooltip)
	}
	bg := pal.CardBg
	fg := pal.TextSecondary
	pressed := b.Pressed()
	switch {
	case active:
		bg = pal.WarningSoft
		fg = pal.Favorite
		if pressed {
			bg = pal.Favorite.Alpha(0.28)
		} else if b.Hovered() {
			bg = pal.Favorite.Alpha(0.22)
		}
	case danger:
		if pressed {
			bg = pal.Danger.Alpha(0.24)
			fg = pal.Danger
		} else if b.Hovered() {
			bg = pal.DangerSoft
			fg = pal.Danger
		}
	default:
		if pressed {
			bg = pal.CardActive
			fg = pal.Text
		} else if b.Hovered() {
			bg = pal.CardHover
			fg = pal.Text
		}
	}
	b.Background(bg)
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(fg)
	})
	return b
}

func (h *historyView) previewPane(c *ui.Context, pal pureui.Palette) {
	ui.Column(c).Fill().Background(pal.SidebarBg).Padding(12, 14).Gap(10).Children(func() {
		if h.selected < 0 || h.selected >= len(h.items) {
			pureui.EmptyState(c, pureui.IconFileText, "选择一条记录查看详情", "在左侧列表点击或使用 ↑↓ 键切换记录。", pal, nil)
			return
		}
		rec := &h.items[h.selected]

		var leftMeta string
		var payload Payload
		var bodyStr string

		// 1. Clean preview stage
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			switch rec.Kind {
			case core.KindImage:
				h.renderPreviewImage(c, rec, pal)
				if rec.Width > 0 && rec.Height > 0 {
					leftMeta = fmt.Sprintf("%d×%d · %s", rec.Width, rec.Height, humanBytes(rec.ByteSize))
				} else {
					leftMeta = humanBytes(rec.ByteSize)
				}

			case core.KindFiles:
				payload = DecodePayload(rec.PayloadJSON)
				leftMeta = fmt.Sprintf("%d 个文件", len(payload.Files))
				ui.Box(c).Fill().Padding(10, 12).Radius(10).Background(pal.CardBg).Border(1, pal.Border).Children(func() {
					ui.Scroll(c).Fill().Children(func() {
						ui.Column(c).Gap(6).Children(func() {
							for _, p := range payload.Files {
								exists := fileExists(p)
								base := filepath.Base(p)
								dir := filepath.Dir(p)
								ui.Row(c).Padding(8, 10).Gap(9).Radius(8).Background(pal.BgInset).AlignItems(ui.Center).Children(func() {
									iconCol := pal.KindFiles
									if !exists {
										iconCol = pal.Danger
									}
									ui.Icon(c, pureui.IconFiles).Size(14, 14).TextColor(iconCol)
									ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
										ui.Text(c, base).FontSize(12).FontWeight(500).SingleLine().TextColor(pal.Text)
										ui.Text(c, dir).FontSize(10).SingleLine().TextColor(pal.TextMuted)
									})
									if !exists {
										pureui.PillBadge(c, "已失效", pal.Danger, pal.DangerSoft)
									}
								})
							}
						})
					})
				})

			default:
				bodyStr = rec.Body
				if bodyStr == "" {
					bodyStr = rec.Preview
				}
				lines := strings.Count(bodyStr, "\n") + 1
				chars := utf8.RuneCountInString(bodyStr)
				leftMeta = fmt.Sprintf("%d 字符 · %d 行", chars, lines)
				ui.Box(c).Fill().Padding(12, 14).Radius(10).Background(pal.CardBg).Border(1, pal.Border).Children(func() {
					ui.Scroll(c).Fill().Children(func() {
						ui.Text(c, bodyStr).FontSize(12.5).LineHeight(1.55).Selectable().TextColor(pal.Text)
					})
				})
			}
		})

		// 2. Compact single-line metadata
		ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Text(c, leftMeta).FontSize(11).SingleLine().TextColor(pal.TextMuted)
			if rec.BodyTruncated {
				pureui.PillBadge(c, "已截断", pal.Warning, pal.WarningSoft)
			}
			if rec.IsLegacy() {
				pureui.PillBadge(c, "旧库只读", pal.Warning, pal.WarningSoft)
			}
			ui.Spacer(c)
			rightMeta := timeAgo(rec.LastUsedUTC)
			if rec.SourceProcess != "" {
				rightMeta = rec.SourceProcess + " · " + rightMeta
			}
			ui.Text(c, rightMeta).FontSize(11).SingleLine().TextColor(pal.TextMuted)
		})

		// 3. Clean action bar
		ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
			pasteBtn := ui.ButtonBase(c).Height(32).Grow(1).Radius(8).Padding(0, 14).Gap(6).Center().
				Label("粘贴").Tooltip("粘贴到原窗口 (Enter)")
			if pasteBtn.Pressed() {
				pasteBtn.Background(pal.AccentPressed)
			} else if pasteBtn.Hovered() {
				pasteBtn.Gradient(pal.AccentHover, pal.Accent, 135)
			} else {
				pasteBtn.Gradient(pal.Accent, pal.AccentEnd, 135)
			}
			pasteBtn.Children(func() {
				ui.Text(c, "粘贴 ↵").FontSize(12).FontWeight(600).SingleLine().TextColor(pal.AccentText)
			})
			pasteBtn.OnClick(func() {
				h.pending, h.pendingID = "paste", rec.ID
				h.invalidate()
			})

			if softActionButton(c, pureui.IconCopy, "复制", "复制到剪贴板 (Ctrl+C)", false, false, 32, pal).Clicked() {
				h.pending, h.pendingID = "copy", rec.ID
				h.invalidate()
			}
			favIcon := pureui.IconStar
			if rec.Favorite {
				favIcon = pureui.IconStarFilled
			}
			if softActionButton(c, favIcon, "收藏", "收藏 / 取消收藏 (F)", rec.Favorite, false, 32, pal).Clicked() {
				h.pending, h.pendingID = "favorite", rec.ID
				h.invalidate()
			}
			if h.plugin.canPin() && softActionButton(c, pureui.IconPin, "贴图", "贴到桌面悬浮显示 (P)", false, false, 32, pal).Clicked() {
				h.pending, h.pendingID = "pin", rec.ID
				h.invalidate()
			}
			if rec.Kind == core.KindImage && h.plugin.canPreviewImage() {
				if softActionButton(c, pureui.IconEye, "用 PureImage 查看", "在 PureImage 中大图查看", false, false, 32, pal).Clicked() {
					h.pending, h.pendingID = "view", rec.ID
					h.invalidate()
				}
			}
			if rec.Kind == core.KindFiles {
				if h.plugin.canPreviewImage() && firstImageFileFromPayload(rec.PayloadJSON) != "" {
					if softActionButton(c, pureui.IconEye, "用 PureImage 查看", "在 PureImage 中查看图片文件", false, false, 32, pal).Clicked() {
						h.pending, h.pendingID = "view", rec.ID
						h.invalidate()
					}
				}
				if softActionButton(c, pureui.IconExternalLink, "复制路径", "复制文件路径列表", false, false, 32, pal).Clicked() {
					h.pending, h.pendingID = "copy-path", rec.ID
					h.invalidate()
				}
				if softActionButton(c, pureui.IconFolder, "打开所在文件夹", "在资源管理器中显示", false, false, 32, pal).Clicked() {
					h.pending, h.pendingID = "reveal", rec.ID
					h.invalidate()
				}
			}
			if softActionButton(c, pureui.IconTrash, "删除", "删除该记录 (Delete)", false, true, 32, pal).Clicked() {
				h.pending, h.pendingID = "delete", rec.ID
				h.invalidate()
			}
		})
	})
}

func (h *historyView) drawImageStage(c *ui.Context, rec *Record, pal pureui.Palette, maxH float32) {
	box := ui.Box(c).Grow(1).MinHeight(110)
	if maxH > 0 {
		box.MaxHeight(maxH)
	}
	box.Radius(9).Border(1, pal.Border).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		const tile = float32(12)
		c1 := ui.RGB(18, 22, 32)
		c2 := ui.RGB(24, 29, 42)
		if !pal.Dark {
			c1 = ui.RGB(237, 241, 247)
			c2 = ui.RGB(226, 232, 242)
		}
		p.Fill(r, c1, 0)
		for rowIdx := 0; float32(rowIdx)*tile < r.H; rowIdx++ {
			for colIdx := 0; float32(colIdx)*tile < r.W; colIdx++ {
				if (rowIdx+colIdx)%2 == 1 {
					tx := r.X + float32(colIdx)*tile
					ty := r.Y + float32(rowIdx)*tile
					p.Fill(ui.Rect{X: tx, Y: ty, W: minf(tile, r.X+r.W-tx), H: minf(tile, r.Y+r.H-ty)}, c2, 0)
				}
			}
		}

		iw, ih := h.previewImgW, h.previewImgH
		if iw <= 0 {
			iw = float32(rec.Width)
		}
		if ih <= 0 {
			ih = float32(rec.Height)
		}
		if iw > 0 && ih > 0 {
			const pad = float32(14)
			availW := maxf(24, r.W-pad*2)
			availH := maxf(24, r.H-pad*2)
			scale := minf(1, minf(availW/iw, availH/ih))
			dw, dh := iw*scale, ih*scale
			imgRect := ui.Rect{
				X: r.X + (r.W-dw)/2,
				Y: r.Y + (r.H-dh)/2,
				W: dw,
				H: dh,
			}
			shadowCol := ui.RGBA(0, 0, 0, 0.36)
			if !pal.Dark {
				shadowCol = ui.RGBA(15, 23, 42, 0.18)
			}
			p.Shadow(imgRect, 6, 0, 8, 22, 0, shadowCol)
			p.Clip(imgRect, 6, func() {
				p.Image(h.previewBitmap, imgRect, ui.FillBox)
			})
			p.Stroke(imgRect, pal.BorderStrong, 6, 1)
		}
	})
}

func (h *historyView) previewPopoverView(c *ui.Context) {
	if h.closed {
		return
	}
	pal := pureui.ApplyTheme(c)
	rec := h.selectedRecord()
	if rec == nil {
		return
	}
	_, kindLabel, kindFg, _ := pureui.KindVisual(rec.Kind, pal)

	var bodyStr string
	var payload Payload
	switch rec.Kind {
	case core.KindFiles:
		payload = DecodePayload(rec.PayloadJSON)
	case core.KindImage:
	default:
		bodyStr = rec.Body
		if bodyStr == "" {
			bodyStr = rec.Preview
		}
	}

	ui.Column(c).Fill().Background(pal.Bg).Radius(12).Border(1, pal.BorderStrong).InsetShadow(0, 1, 0, 0, pal.Specular).Clip().Children(func() {
		// Header
		ui.Row(c).Height(34).Padding(0, 12).Gap(6).AlignItems(ui.Center).Background(pal.BgElevated).Children(func() {
			ui.Text(c, kindLabel).FontSize(10.5).FontWeight(600).SingleLine().TextColor(kindFg)
			ui.Text(c, timeAgo(rec.LastUsedUTC)).FontSize(10.5).SingleLine().TextColor(pal.TextMuted)
			ui.Spacer(c)
			if rec.SourceProcess != "" {
				ui.Text(c, rec.SourceProcess).FontSize(10.5).SingleLine().TextColor(pal.TextMuted)
			}
			if rec.IsLegacy() {
				pureui.PillBadge(c, "旧库只读", pal.Warning, pal.WarningSoft)
			}
			if rec.Favorite {
				ui.Icon(c, pureui.IconStarFilled).Size(12, 12).TextColor(pal.Favorite)
			}
		})

		ui.Box(c).Height(1).FillWidth().Background(pal.Border)

		// Body
		ui.Column(c).Grow(1).MinHeight(0).Padding(10).Background(pal.Bg).Children(func() {
			switch rec.Kind {
			case core.KindImage:
				h.renderPreviewImage(c, rec, pal)

			case core.KindFiles:
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Gap(6).Children(func() {
						for _, p := range payload.Files {
							exists := fileExists(p)
							base := filepath.Base(p)
							dir := filepath.Dir(p)
							ui.Row(c).Padding(8, 10).Gap(8).Radius(8).Background(pal.BgInset).AlignItems(ui.Center).Children(func() {
								iconCol := pal.KindFiles
								if !exists {
									iconCol = pal.Danger
								}
								ui.Icon(c, pureui.IconFiles).Size(13, 13).TextColor(iconCol)
								ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
									ui.Text(c, base).FontSize(11.5).FontWeight(500).SingleLine().TextColor(pal.Text)
									ui.Text(c, dir).FontSize(9.5).SingleLine().TextColor(pal.TextMuted)
								})
								if !exists {
									pureui.PillBadge(c, "已失效", pal.Danger, pal.DangerSoft)
								}
							})
						}
					})
				})

			default:
				ui.Box(c).Grow(1).MinHeight(0).Padding(10, 12).Radius(9).Background(pal.BgInset).Border(1, pal.Border).Children(func() {
					ui.Scroll(c).Fill().Children(func() {
						ui.Text(c, bodyStr).FontSize(12).LineHeight(1.50).TextColor(pal.Text)
					})
				})
			}
		})

		ui.Box(c).Height(1).FillWidth().Background(pal.Border)

		// Micro Footer
		ui.Row(c).Height(26).Padding(0, 12).Gap(6).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
			switch rec.Kind {
			case core.KindImage:
				ui.Text(c, fmt.Sprintf("%d × %d px · %s", rec.Width, rec.Height, humanBytes(rec.ByteSize))).
					FontSize(10).SingleLine().TextColor(pal.TextMuted)
			case core.KindFiles:
				ui.Text(c, fmt.Sprintf("共 %d 个文件", len(payload.Files))).
					FontSize(10).SingleLine().TextColor(pal.TextMuted)
			default:
				lines := strings.Count(bodyStr, "\n") + 1
				chars := utf8.RuneCountInString(bodyStr)
				ui.Text(c, fmt.Sprintf("%d 字符 · %d 行", chars, lines)).
					FontSize(10).SingleLine().TextColor(pal.TextMuted)
				if rec.BodyTruncated {
					pureui.PillBadge(c, "已截断", pal.Warning, pal.WarningSoft)
				}
			}
			ui.Spacer(c)
			ui.Text(c, "Tab 完整视图").FontSize(10).SingleLine().TextColor(pal.TextMuted.Alpha(0.72))
		})
	})
}

func (h *historyView) moveSelection(delta int) {
	if len(h.items) == 0 {
		return
	}
	if h.selected < 0 {
		h.selected = 0
		return
	}
	h.selected += delta
	if h.selected < 0 {
		h.selected = 0
	}
	if h.selected >= len(h.items) {
		h.selected = len(h.items) - 1
	}
	if rec := h.selectedRecord(); rec == nil || rec.ID != h.previewID {
		h.previewID = 0
		h.previewBitmap = nil
		h.previewImgW = 0
		h.previewImgH = 0
		h.previewLoadingID = 0
	}
	h.list.ScrollTo(h.selected, ui.Center)
	h.invalidate()
}

func (h *historyView) footer(c *ui.Context, pal pureui.Palette) {
	ui.Box(c).Height(1).FillWidth().Background(pal.Border)
	ui.Row(c).Height(26).Padding(0, 12).Gap(8).AlignItems(ui.Center).Background(pal.SidebarBg).Children(func() {
		hasAlert := strings.Contains(h.status, "失败") ||
			strings.Contains(h.status, "暂停") ||
			strings.Contains(h.status, "只读") ||
			strings.Contains(h.status, "失效")
		if hasAlert {
			dotCol := pal.Warning
			if strings.Contains(h.status, "失败") || strings.Contains(h.status, "失效") {
				dotCol = pal.Danger
			}
			ui.Row(c).Gap(5).AlignItems(ui.Center).Grow(1).MinWidth(0).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(dotCol).Shrink(0)
				ui.Text(c, h.status).FontSize(10).SingleLine().TextColor(pal.TextMuted)
			})
		} else {
			hints := "↵ 粘贴   Ctrl+1-9 快选   Ctrl+P 贴图   Del 删除"
			if h.compact {
				hints = "↵ 粘贴   Ctrl+1-9 快选   Tab 展开"
			}
			ui.Text(c, hints).FontSize(10).SingleLine().TextColor(pal.TextMuted.Alpha(0.75))
			ui.Spacer(c)
		}

		rightHint := "Tab 收起"
		if h.compact {
			rightHint = "Esc"
		}
		ui.Text(c, rightHint).FontSize(10).SingleLine().TextColor(pal.TextMuted.Alpha(0.70))
	})
}

func (h *historyView) decodeScaledImage(rec *Record, maxW, maxH int, scaler xdraw.Scaler) (image.Image, error) {
	if rec == nil || rec.ImagePath == "" || h.plugin == nil || h.plugin.Service() == nil {
		return nil, errors.New("missing image")
	}
	data, err := h.plugin.Service().ReadImage(rec)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return scaleDownWith(img, maxW, maxH, scaler), nil
}

func (h *historyView) loadThumbnail(rec *Record) *ui.Bitmap {
	if bmp, ok := h.thumbCache[rec.ID]; ok {
		return bmp
	}
	if rec.ImagePath == "" || h.plugin == nil || h.plugin.Service() == nil {
		return nil
	}
	if h.plugin.headless() {
		img, err := h.decodeScaledImage(rec, 68, 52, xdraw.ApproxBiLinear)
		if err != nil {
			return nil
		}
		bmp := ui.NewBitmap(img)
		h.putThumbCache(rec.ID, bmp)
		return bmp
	}
	if h.thumbLoading[rec.ID] {
		return nil
	}
	if h.thumbLoading == nil {
		h.thumbLoading = map[int64]bool{}
	}
	h.thumbLoading[rec.ID] = true
	id := rec.ID
	recCopy := *rec
	go func() {
		img, err := h.decodeScaledImage(&recCopy, 68, 52, xdraw.ApproxBiLinear)
		var bmp *ui.Bitmap
		if err == nil && img != nil {
			bmp = ui.NewBitmap(img)
		}
		mygo.RunOnMain(func() {
			delete(h.thumbLoading, id)
			if h.closed {
				return
			}
			h.putThumbCache(id, bmp)
			if bmp != nil {
				h.invalidate()
			}
		})
	}()
	return nil
}

func (h *historyView) loadPreview(rec *Record) {
	if cached, ok := h.previewCache[rec.ID]; ok {
		h.previewID = rec.ID
		h.previewBitmap = cached.bmp
		h.previewImgW = cached.w
		h.previewImgH = cached.h
		h.previewLoadingID = 0
		return
	}
	h.previewID = rec.ID
	h.previewBitmap = nil
	h.previewImgW = 0
	h.previewImgH = 0
	if rec.ImagePath == "" || h.plugin == nil || h.plugin.Service() == nil {
		h.previewLoadingID = 0
		return
	}
	if h.plugin.headless() {
		scaled, err := h.decodeScaledImage(rec, 640, 480, xdraw.BiLinear)
		if err != nil {
			h.putPreviewCache(rec.ID, cachedPreview{})
			h.previewLoadingID = 0
			return
		}
		cp := cachedPreview{
			bmp: ui.NewBitmap(scaled),
			w:   float32(scaled.Bounds().Dx()),
			h:   float32(scaled.Bounds().Dy()),
		}
		h.previewBitmap = cp.bmp
		h.previewImgW = cp.w
		h.previewImgH = cp.h
		h.previewLoadingID = 0
		h.putPreviewCache(rec.ID, cp)
		return
	}
	h.previewLoadingID = rec.ID
	if h.previewLoading[rec.ID] {
		return
	}
	if h.previewLoading == nil {
		h.previewLoading = map[int64]bool{}
	}
	h.previewLoading[rec.ID] = true
	id := rec.ID
	recCopy := *rec
	go func() {
		scaled, err := h.decodeScaledImage(&recCopy, 640, 480, xdraw.BiLinear)
		var cp cachedPreview
		if err == nil && scaled != nil {
			cp = cachedPreview{
				bmp: ui.NewBitmap(scaled),
				w:   float32(scaled.Bounds().Dx()),
				h:   float32(scaled.Bounds().Dy()),
			}
		}
		mygo.RunOnMain(func() {
			delete(h.previewLoading, id)
			if h.closed {
				return
			}
			h.putPreviewCache(id, cp)
			if h.previewLoadingID == id {
				h.previewLoadingID = 0
			}
			if h.previewID == id {
				h.previewBitmap = cp.bmp
				h.previewImgW = cp.w
				h.previewImgH = cp.h
				h.invalidate()
				if h.plugin != nil && h.plugin.previewWin != nil && h.plugin.previewState.visible {
					h.plugin.previewWin.Invalidate()
				}
			}
		})
	}()
}

func (h *historyView) renderPreviewImage(c *ui.Context, rec *Record, pal pureui.Palette) {
	if h.previewID != rec.ID {
		h.loadPreview(rec)
	}
	if h.previewBitmap != nil {
		h.drawImageStage(c, rec, pal, 0)
		return
	}
	if h.previewLoading[rec.ID] || h.previewLoadingID == rec.ID {
		ui.Box(c).Fill().Radius(10).Background(pal.BgInset).Border(1, pal.Border).Center().Children(func() {
			ui.Text(c, "加载图片预览…").FontSize(11).TextColor(pal.TextMuted)
		})
		return
	}
	pureui.EmptyState(c, pureui.IconAlertCircle, "图片文件缺失", "原始内容文件可能已被外部删除或移动。", pal, nil)
}

func (h *historyView) applyPending(c *ui.Context) {
	action, id := h.pending, h.pendingID
	h.pending = ""
	switch action {
	case "paste":
		h.paste(id)
	case "copy":
		h.copyToClipboard(id)
	case "favorite":
		if _, err := h.plugin.Service().ToggleFavorite(id); err != nil {
			if errors.Is(err, ErrLegacyReadOnly) {
				h.status = "旧版历史记录为只读，无法收藏或修改"
			} else {
				h.status = "收藏失败: " + err.Error()
			}
		} else {
			h.status = "已更新收藏状态"
		}
		h.Reload()
	case "delete":
		if err := h.plugin.Service().Delete(id); err != nil {
			if errors.Is(err, ErrLegacyReadOnly) {
				h.status = "旧版历史记录为只读，无法删除"
			} else {
				h.status = "删除失败: " + err.Error()
			}
		} else {
			h.status = "已删除记录"
			c.Toast("已删除记录")
		}
		delete(h.thumbCache, id)
		delete(h.thumbLoading, id)
		delete(h.previewCache, id)
		delete(h.previewLoading, id)
		if h.previewID == id {
			h.previewID = 0
			h.previewBitmap = nil
			h.previewImgW = 0
			h.previewImgH = 0
			h.previewLoadingID = 0
		}
		h.Reload()
	case "pin":
		h.pinRecord(id)
	case "view":
		h.viewInFreeImage(id)
	case "copy-path":
		h.copyPaths(id)
	case "reveal":
		h.reveal(id)
	}
	h.invalidate()
}

func (h *historyView) paste(id int64) {
	h.suppressBlur = true
	fg := h.previousForeground
	settings := h.plugin.Config()
	go func() {
		outcome := h.plugin.Service().Restore(id)
		if outcome.Wrote && settings.History.PasteOnEnter {
			if fg != 0 {
				_ = platform.FocusWindow(fg)
			}
			time.Sleep(80 * time.Millisecond)
			_ = platform.SendPaste()
		}
		mygo.RunOnMain(func() {
			if h.closed {
				return
			}
			if len(outcome.MissingFiles) > 0 {
				h.status = fmt.Sprintf("已粘贴，但 %d 个文件地址已失效", len(outcome.MissingFiles))
				h.suppressBlur = false
				h.invalidate()
				return
			}
			if !outcome.Wrote {
				h.status = "写回剪贴板失败"
				h.suppressBlur = false
				h.invalidate()
				return
			}
			h.closeWindow()
		})
	}()
}

func (h *historyView) copyToClipboard(id int64) {
	go func() {
		outcome := h.plugin.Service().Restore(id)
		mygo.RunOnMain(func() {
			if h.closed {
				return
			}
			if len(outcome.MissingFiles) > 0 {
				h.status = fmt.Sprintf("已复制，但 %d 个文件地址已失效", len(outcome.MissingFiles))
			} else if outcome.Wrote {
				h.status = "已复制到剪贴板"
				h.pendingToast = "已复制到剪贴板"
			} else {
				h.status = "写回剪贴板失败"
			}
			h.Reload()
			h.invalidate()
		})
	}()
}

func (h *historyView) pinRecord(id int64) {
	if !h.plugin.canPin() {
		h.status = "PureShot 贴图插件未启用"
		return
	}
	content, err := h.plugin.Service().LoadContent(id)
	if err != nil || content == nil || content.IsEmpty() {
		h.status = "这条记录没有可贴的内容"
		return
	}
	if err := purekit.CallImagePinContent(context.Background(), h.plugin.host.Bus(), content, 0); err != nil {
		h.status = "这条记录无法贴图显示"
		return
	}
	if content.PrimaryKind() == core.KindImage {
		h.status = "已贴成图片"
		h.pendingToast = "已贴成悬浮图片"
	} else {
		h.status = "已贴到桌面"
		h.pendingToast = "已贴到桌面悬浮显示"
	}
}

func (h *historyView) viewInFreeImage(id int64) {
	if !h.plugin.canPreviewImage() {
		h.status = "PureImage 看图插件未启用"
		return
	}
	rec, err := h.plugin.Service().Get(id)
	if err != nil || rec == nil {
		h.status = "记录不存在"
		return
	}
	bus := h.plugin.host.Bus()
	switch rec.Kind {
	case core.KindImage:
		if fullPath := h.plugin.Service().ResolveImagePath(rec); fullPath != "" && fileExists(fullPath) {
			if err := purekit.CallImagePreviewPath(context.Background(), bus, fullPath); err == nil {
				h.status = "已在 PureImage 中打开"
				return
			}
		}
		content, err := h.plugin.Service().LoadContent(id)
		if err != nil || content == nil || len(content.ImagePNG) == 0 {
			h.status = "这条记录没有可用的图片"
			return
		}
		img, err := png.Decode(bytes.NewReader(content.ImagePNG))
		if err != nil {
			h.status = "图片解码失败: " + err.Error()
			return
		}
		_ = purekit.CallImagePreviewImage(context.Background(), bus, "剪贴板图片", img)
		h.status = "已在 PureImage 中打开"
	case core.KindFiles:
		p := firstImageFileFromPayload(rec.PayloadJSON)
		if p == "" {
			h.status = "列表里没有可查看的本地图片文件"
			return
		}
		_ = purekit.CallImagePreviewPath(context.Background(), bus, p)
		h.status = "已在 PureImage 中打开"
	}
}

var viewerSupportedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jpe": true, ".jfif": true,
	".png": true, ".gif": true, ".webp": true, ".bmp": true,
	".tif": true, ".tiff": true,
	".heic": true, ".heif": true, ".hif": true,
	".avif": true, ".avifs": true, ".jxl": true,
	".cr2": true, ".cr3": true, ".crw": true,
	".nef": true, ".nrw": true, ".arw": true, ".srf": true, ".sr2": true,
	".orf": true, ".raf": true, ".rw2": true, ".dng": true, ".pef": true,
	".srw": true, ".x3f": true, ".3fr": true, ".fff": true, ".iiq": true,
	".mos": true, ".mrw": true, ".raw": true, ".rwl": true, ".kdc": true, ".dcr": true,
}

func firstImageFileFromPayload(payloadJSON string) string {
	payload := DecodePayload(payloadJSON)
	for _, p := range payload.Files {
		if viewerSupportedExts[strings.ToLower(filepath.Ext(p))] && fileExists(p) {
			return p
		}
	}
	return ""
}

func (h *historyView) copyPaths(id int64) {
	rec, err := h.plugin.Service().Get(id)
	if err != nil || rec == nil {
		h.status = "记录不存在"
		return
	}
	payload := DecodePayload(rec.PayloadJSON)
	text := strings.Join(payload.Files, "\n")
	if text == "" {
		h.status = "没有文件路径"
		return
	}
	if err := h.plugin.host.Clipboard().WriteText(text, PluginID); err != nil {
		h.status = "复制路径失败: " + err.Error()
		return
	}
	h.status = "文件路径已复制"
	h.pendingToast = "文件路径已复制"
}

func (h *historyView) reveal(id int64) {
	rec, err := h.plugin.Service().Get(id)
	if err != nil || rec == nil {
		h.status = "记录不存在"
		return
	}
	payload := DecodePayload(rec.PayloadJSON)
	for _, p := range payload.Files {
		if fileExists(p) {
			if err := platform.RevealInFolder(p); err != nil {
				h.status = "打开文件夹失败: " + err.Error()
			} else {
				h.status = "已在资源管理器中打开"
			}
			return
		}
	}
	h.status = "文件地址都已失效"
}

func (h *historyView) invalidate() {
	if h.win != nil {
		h.win.Invalidate()
	}
}

func (h *historyView) closeWindow() {
	h.closed = true
	h.stopTimers()
	if h.plugin != nil {
		h.plugin.closeHistoryPreview()
	}
	if h.win != nil {
		h.win.Close()
	}
}

func timeAgo(unix int64) string {
	d := time.Since(time.Unix(unix, 0))
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	default:
		return time.Unix(unix, 0).Format("2006-01-02")
	}
}

func humanBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func compactBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		kb := (n + 512) / 1024
		if kb < 1 {
			kb = 1
		}
		return fmt.Sprintf("%d KB", kb)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func scaleDown(src image.Image, maxW, maxH int) image.Image {
	return scaleDownWith(src, maxW, maxH, xdraw.ApproxBiLinear)
}

func scaleDownWith(src image.Image, maxW, maxH int, scaler xdraw.Scaler) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return src
	}
	scale := float64(maxW) / float64(w)
	if float64(maxH)/float64(h) < scale {
		scale = float64(maxH) / float64(h)
	}
	dw, dh := int(float64(w)*scale), int(float64(h)*scale)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	if scaler == nil {
		scaler = xdraw.ApproxBiLinear
	}
	scaler.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

type historyPreviewState struct {
	visible bool
	width   int
	height  int
}

// ShowHistory opens or reopens the compact Ditto-style clipboard popup near
// the cursor. Users can expand it in place to the full two-column inspector.
func (a *Plugin) ShowHistory() {
	if a.headless() {
		return
	}
	if a.win != nil {
		h := a.view
		if h == nil {
			return
		}
		h.compact = true
		h.query = ""
		h.Reload()
		h.suppressBlur = false
		if !a.win.IsVisible() {
			h.previousForeground = platform.ForegroundWindow()
		}
		a.win.Show()
		a.placePanel(a.win, compactHistoryWidth, compactHistoryHeight)
		a.win.Focus()
		a.win.SetAlwaysOnTop(true)
		h.focusSearch = true
		h.typingDelayPreview = false
		h.lastPreviewKey = compactPreviewKey{}
		if a.previewWin != nil {
			a.previewWin.SetAnchor(mygo.Rectangle{X: 0, Y: 0, Width: compactHistoryWidth, Height: compactHistoryHeight})
		}
		h.schedulePreviewSync()
		a.win.Invalidate()
		return
	}

	h := newCompactHistoryView(a)
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             "PureClip 剪贴板历史",
		Width:             compactHistoryWidth,
		Height:            compactHistoryHeight,
		Hidden:            true,
		Frameless:         true,
		Panel:             true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		CornerRadius:      12,
		BackgroundColor:   "#00000000",
		Content:           ui.View(h.view),
	})
	h.win = win
	win.OnBlur(func() {
		if h.pinned || h.suppressBlur || h.confirmClear || h.closed {
			return
		}
		if a.previewWin != nil && a.previewWin.IsFocused() {
			return
		}
		win.Close()
	})
	win.OnHide(func() {
		a.hideHistoryPreview()
	})
	win.OnClose(func(_ *mygo.CloseEvent) {
		h.closed = true
		h.stopTimers()
		a.closeHistoryPreview()
		if a.win == win {
			a.win = nil
			a.view = nil
		}
		a.mu.RLock()
		host := a.host
		a.mu.RUnlock()
		if host != nil {
			host.NotifyWindowClosed()
		}
	})
	a.win = win
	a.view = h
	h.previousForeground = platform.ForegroundWindow()
	h.Reload()
	a.placePanel(win, compactHistoryWidth, compactHistoryHeight)
	win.Show()
	win.Focus()
	win.SetAlwaysOnTop(true)
	win.Update(func() {
		h.focusSearch = true
		win.Invalidate()
		h.schedulePreviewSync()
	})
}

func (a *Plugin) hideHistoryPreview() {
	if a.headless() || a.previewWin == nil {
		a.previewState.visible = false
		return
	}
	if a.previewWin.IsVisible() {
		a.previewWin.Hide()
	}
	a.previewState.visible = false
}

func (a *Plugin) closeHistoryPreview() {
	if a.headless() {
		return
	}
	if w := a.previewWin; w != nil {
		a.previewWin = nil
		a.previewState = historyPreviewState{}
		w.Close()
	}
}

func (a *Plugin) syncHistoryPreview(h *historyView) {
	if a.headless() || a.win == nil || h == nil || h.closed || !h.compact || h.confirmClear || !a.win.IsVisible() {
		a.hideHistoryPreview()
		return
	}
	rec := h.selectedRecord()
	if !shouldShowCompactPreview(rec) {
		a.hideHistoryPreview()
		return
	}
	if rec.Kind == core.KindImage && h.previewID != rec.ID {
		h.loadPreview(rec)
	}
	pw, ph := previewPopoverSize(rec)
	anchor := mygo.Rectangle{X: 0, Y: 0, Width: compactHistoryWidth, Height: compactHistoryHeight}
	if a.previewWin == nil {
		fw := mygo.NewFlyout(mygo.FlyoutOptions{
			Parent:       a.win,
			Anchor:       anchor,
			Placement:    mygo.PlacementRightStart,
			Gap:          8,
			Width:        pw,
			Height:       ph,
			Focusable:    false,
			Hidden:       true,
			Shadow:       false,
			CornerRadius: 12,
			Content:      ui.View(h.previewPopoverView),
		})
		fw.OnClose(func(_ *mygo.CloseEvent) {
			if a.previewWin == fw {
				a.previewWin = nil
				a.previewState = historyPreviewState{}
			}
		})
		a.previewWin = fw
		a.previewState = historyPreviewState{width: pw, height: ph}
	} else if a.previewState.width != pw || a.previewState.height != ph {
		a.previewWin.SetSize(pw, ph)
		a.previewState.width = pw
		a.previewState.height = ph
	}
	if !a.previewState.visible || !a.previewWin.IsVisible() {
		a.previewWin.SetAnchor(anchor)
		a.previewWin.ShowInactive()
		a.previewWin.SetAlwaysOnTop(true)
		a.previewState.visible = true
	}
	a.previewWin.Invalidate()
}

func (a *Plugin) resizePanel(win *mygo.Window, w, h int) {
	if win == nil {
		return
	}
	b := win.Bounds()
	d := mygo.Screen.DisplayNearestPoint(mygo.Point{X: b.X, Y: b.Y})
	wa := d.WorkArea
	x := clampInt(b.X, wa.X+8, wa.X+wa.Width-w-8)
	y := clampInt(b.Y, wa.Y+8, wa.Y+wa.Height-h-8)
	win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
}

func (a *Plugin) placePanel(win *mygo.Window, w, h int) {
	p := mygo.Screen.CursorScreenPoint()
	d := mygo.Screen.DisplayNearestPoint(p)
	wa := d.WorkArea
	x := int(p.X) + 12
	y := int(p.Y) + 12
	if x+w > wa.X+wa.Width {
		x = int(p.X) - w - 12
	}
	if y+h > wa.Y+wa.Height {
		y = int(p.Y) - h - 12
	}
	x = clampInt(x, wa.X+8, wa.X+wa.Width-w-8)
	y = clampInt(y, wa.Y+8, wa.Y+wa.Height-h-8)
	win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: w, Height: h})
}

func clampInt(v, lo, hi int) int {
	if lo > hi {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}