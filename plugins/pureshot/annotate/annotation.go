package annotate

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"pureutils/pkg/purekit/core"
)

// Tool is one of the capture annotation tools.
type Tool string

const (
	ToolRect      Tool = "rect"
	ToolEllipse   Tool = "ellipse"
	ToolLine      Tool = "line"
	ToolArrow     Tool = "arrow"
	ToolPen       Tool = "pen"
	ToolHighlight Tool = "highlight"
	ToolText      Tool = "text"
	ToolMosaic    Tool = "mosaic"
	ToolBadge     Tool = "badge"
)

// Tools is the toolbar order.
var Tools = []Tool{ToolRect, ToolEllipse, ToolLine, ToolArrow, ToolPen, ToolHighlight, ToolText, ToolMosaic, ToolBadge}

// ToolLabel returns the Chinese label shown in the capture toolbar.
func ToolLabel(t Tool) string {
	switch t {
	case ToolRect:
		return "矩形"
	case ToolEllipse:
		return "椭圆"
	case ToolLine:
		return "直线"
	case ToolArrow:
		return "箭头"
	case ToolPen:
		return "画笔"
	case ToolHighlight:
		return "高亮"
	case ToolText:
		return "文字"
	case ToolMosaic:
		return "马赛克"
	case ToolBadge:
		return "序号"
	}
	return string(t)
}

// Item is one annotation. Coordinates are in the cropped image's pixel
// space, with the origin at its top-left corner. One struct is used for all
// tools so the document round-trips through JSON without a polymorphic
// decoder.
type Item struct {
	ID        int          `json:"id"`
	Tool      Tool         `json:"tool"`
	Color     core.ARGB    `json:"color"`
	Thickness float64      `json:"thickness"`
	Start     core.Point   `json:"start,omitempty"`
	End       core.Point   `json:"end,omitempty"`
	Points    []core.Point `json:"points,omitempty"`
	Filled    bool         `json:"filled,omitempty"`
	FillColor core.ARGB    `json:"fillColor,omitempty"`

	Origin        core.Point `json:"origin,omitempty"`
	Text          string     `json:"text,omitempty"`
	FontSize      float64    `json:"fontSize,omitempty"`
	HasBackground bool       `json:"hasBackground,omitempty"`
	Background    core.ARGB  `json:"background,omitempty"`
	Measured      core.Size  `json:"measured,omitempty"`

	Region    core.Rect `json:"region,omitempty"`
	BlockSize float64   `json:"blockSize,omitempty"`

	Center     core.Point `json:"center,omitempty"`
	Number     int        `json:"number,omitempty"`
	Radius     float64    `json:"radius,omitempty"`
	TextColor  core.ARGB  `json:"textColor,omitempty"`
	HeadLength float64    `json:"headLength,omitempty"`
}

// Clone returns a deep copy of the item.
func (it *Item) Clone() *Item {
	if it == nil {
		return nil
	}
	c := *it
	if it.Points != nil {
		c.Points = append([]core.Point(nil), it.Points...)
	}
	return &c
}

// Bounds returns the item's bounding box, stroke included.
func (it *Item) Bounds() core.Rect {
	if it == nil {
		return core.Rect{}
	}
	switch it.Tool {
	case ToolRect, ToolEllipse:
		return core.RectFromPoints(it.Start, it.End).Inflate(maxf(it.Thickness/2, 1))
	case ToolLine:
		return core.RectFromPoints(it.Start, it.End).Inflate(maxf(it.Thickness/2, 1))
	case ToolArrow:
		return core.RectFromPoints(it.Start, it.End).Inflate(maxf(it.Thickness, it.ResolveHeadLength()*ArrowHeadSpreadFactor))
	case ToolHighlight:
		return core.RectFromPoints(it.Start, it.End).Inflate(it.Thickness / 2)
	case ToolPen:
		if len(it.Points) == 0 {
			return core.Rect{}
		}
		minX, minY := math.MaxFloat64, math.MaxFloat64
		maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
		for _, p := range it.Points {
			minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
			maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
		}
		return core.Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}.Inflate(it.Thickness / 2)
	case ToolText:
		w := it.Measured.W
		if w <= 0 {
			w = math.Max(40, float64(len([]rune(it.Text)))*it.FontSize*0.6)
		}
		h := it.Measured.H
		if h <= 0 {
			h = it.FontSize * 1.4
		}
		return core.Rect{X: it.Origin.X, Y: it.Origin.Y, W: w, H: h}
	case ToolMosaic:
		return it.Region
	case ToolBadge:
		return core.Rect{X: it.Center.X - it.Radius, Y: it.Center.Y - it.Radius,
			W: it.Radius * 2, H: it.Radius * 2}
	}
	return core.Rect{}
}

// ResolveHeadLength returns the arrow head length, deriving it from the
// stroke width when unset.
func (it *Item) ResolveHeadLength() float64 {
	if it.HeadLength > 0 {
		return it.HeadLength
	}
	return math.Max(14, it.Thickness*3.6)
}

// ArrowHeadSpreadFactor is the spread of the arrow head across the shaft.
const ArrowHeadSpreadFactor = 0.42

// ArrowShape is the triangle and shaft endpoint of an arrow.
type ArrowShape struct {
	Tip   core.Point
	Base  core.Point
	Left  core.Point
	Right core.Point
}

// ArrowGeometry computes the same arrow the preview and the baker draw.
func ArrowGeometry(start, end core.Point, headLength float64) ArrowShape {
	dx, dy := end.X-start.X, end.Y-start.Y
	length := math.Hypot(dx, dy)
	if length < 0.5 {
		return ArrowShape{Tip: end, Base: end, Left: end, Right: end}
	}
	ux, uy := dx/length, dy/length
	head := math.Min(headLength, length)
	base := core.Point{X: end.X - ux*head, Y: end.Y - uy*head}
	spread := head * ArrowHeadSpreadFactor
	px, py := -uy, ux
	return ArrowShape{
		Tip:   end,
		Base:  base,
		Left:  core.Point{X: base.X + px*spread, Y: base.Y + py*spread},
		Right: core.Point{X: base.X - px*spread, Y: base.Y - py*spread},
	}
}

// Document is one capture's annotation document. Coordinates are in the
// cropped image's pixels. Undo and redo keep full snapshots, which is cheap
// enough at this scale and cannot desynchronise incremental edits.
type Document struct {
	Width, Height int
	Items         []*Item
	BadgeCounter  int
	NextID        int

	undo [][]*Item
	redo [][]*Item
	mu   sync.Mutex
}

// NewDocument creates an empty document for a cropped image.
func NewDocument(width, height int) *Document {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return &Document{Width: width, Height: height, NextID: 1}
}

// Add appends an item and records an undo point.
func (d *Document) Add(it *Item) *Item {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pushLocked()
	it.ID = d.NextID
	d.NextID++
	d.Items = append(d.Items, it)
	return it
}

// AddWithoutHistory appends an item without recording an undo point. It is
// used for a draft that is still being drawn; the caller records the undo
// point when the drag ends.
func (d *Document) AddWithoutHistory(it *Item) *Item {
	d.mu.Lock()
	defer d.mu.Unlock()
	it.ID = d.NextID
	d.NextID++
	d.Items = append(d.Items, it)
	return it
}

// BeginChange records an undo point before a multi-step edit.
func (d *Document) BeginChange() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pushLocked()
}

// RemoveLast removes the last item and records an undo point.
func (d *Document) RemoveLast() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.Items) == 0 {
		return
	}
	d.pushLocked()
	d.Items = d.Items[:len(d.Items)-1]
}

// Clear removes all items and records an undo point.
func (d *Document) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.Items) == 0 {
		return
	}
	d.pushLocked()
	d.Items = nil
}

// CanUndo reports whether an undo point exists.
func (d *Document) CanUndo() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.undo) > 0
}

// CanRedo reports whether a redo point exists.
func (d *Document) CanRedo() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.redo) > 0
}

// Undo restores the previous snapshot.
func (d *Document) Undo() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.undo) == 0 {
		return
	}
	d.redo = append(d.redo, cloneItems(d.Items))
	last := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	d.Items = cloneItems(last)
}

// Redo restores the snapshot undone last.
func (d *Document) Redo() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.redo) == 0 {
		return
	}
	d.undo = append(d.undo, cloneItems(d.Items))
	last := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	d.Items = cloneItems(last)
}

// NextBadge returns the next step number for the badge tool.
func (d *Document) NextBadge() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.BadgeCounter++
	return d.BadgeCounter
}

// IsEmpty reports whether there are no items.
func (d *Document) IsEmpty() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.Items) == 0
}

// Snapshot returns a copy of the items safe to render outside the lock.
func (d *Document) Snapshot() []*Item {
	d.mu.Lock()
	defer d.mu.Unlock()
	return cloneItems(d.Items)
}

func (d *Document) pushLocked() {
	d.undo = append(d.undo, cloneItems(d.Items))
	if len(d.undo) > 64 {
		d.undo = d.undo[1:]
	}
	d.redo = nil
}

func cloneItems(items []*Item) []*Item {
	out := make([]*Item, len(items))
	for i, it := range items {
		out[i] = it.Clone()
	}
	return out
}

// persisted is the JSON shape of a document.
type persisted struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Badge  int     `json:"badge"`
	Items  []*Item `json:"items"`
}

// ToJSON encodes the document.
func (d *Document) ToJSON() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b, err := json.Marshal(persisted{Width: d.Width, Height: d.Height, Badge: d.BadgeCounter, Items: d.Items})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// FromJSON decodes a document.
func FromJSON(data string) (*Document, error) {
	var p persisted
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return nil, fmt.Errorf("annotation document: %w", err)
	}
	if p.Width <= 0 || p.Height <= 0 {
		return nil, fmt.Errorf("annotation document: invalid size %dx%d", p.Width, p.Height)
	}
	d := NewDocument(p.Width, p.Height)
	d.BadgeCounter = p.Badge
	for _, it := range p.Items {
		if it == nil {
			continue
		}
		if it.Tool == "" {
			return nil, fmt.Errorf("annotation document: item %d has no tool", it.ID)
		}
		d.Items = append(d.Items, it)
		if it.ID >= d.NextID {
			d.NextID = it.ID + 1
		}
	}
	return d, nil
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
