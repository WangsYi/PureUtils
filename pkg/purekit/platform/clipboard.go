package platform

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/bits"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"pureutils/pkg/purekit/core"
)

var (
	clipboardFormatsOnce sync.Once
	cfHTML               uint32
	cfRTF                uint32
	cfPNG                uint32
	cfImagePNG           uint32
	cfImageBMP           uint32
	cfJFIF               uint32
	cfImageJPEG          uint32
	cfGIF                uint32
	cfImageGIF           uint32
)

func initClipboardFormats() {
	clipboardFormatsOnce.Do(func() {
		cfHTML = registerFormat("HTML Format")
		cfRTF = registerFormat("Rich Text Format")
		cfPNG = registerFormat("PNG")
		cfImagePNG = registerFormat("image/png")
		cfImageBMP = registerFormat("image/bmp")
		cfJFIF = registerFormat("JFIF")
		cfImageJPEG = registerFormat("image/jpeg")
		cfGIF = registerFormat("GIF")
		cfImageGIF = registerFormat("image/gif")
	})
}

func registerFormat(name string) uint32 {
	r, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(Utf16Ptr(name))))
	return uint32(r)
}

// Clipboard reads and writes the Windows system clipboard.
type Clipboard struct {
	owner HWND
}

func NewClipboard(owner HWND) *Clipboard {
	initClipboardFormats()
	return &Clipboard{owner: owner}
}

func (c *Clipboard) open() bool {
	delays := []int{10, 15, 20, 30, 40, 50, 60, 80, 100, 120}
	for attempt := 0; attempt <= len(delays); attempt++ {
		r, _, _ := procOpenClipboard.Call(uintptr(c.owner))
		if r != 0 {
			return true
		}
		if attempt < len(delays) {
			timeSleep(delays[attempt])
		}
	}
	return false
}

func (c *Clipboard) close() {
	procCloseClipboard.Call()
}

func (c *Clipboard) Read() (*core.ClipboardContent, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !c.open() {
		return nil, fmt.Errorf("OpenClipboard failed")
	}
	defer c.close()

	content := &core.ClipboardContent{
		Formats: c.formatNames(),
	}
	content.Text = readUnicodeText()
	content.HTML = readTextFormat(cfHTML)
	content.RTF = readTextFormat(cfRTF)
	content.Files = readFileDrop()
	content.ImagePNG = readImagePNG()
	if len(content.ImagePNG) == 0 && len(content.Files) == 0 &&
		strings.TrimSpace(content.Text) == "" && content.HTML != "" &&
		strings.TrimSpace(core.StripHTML(content.HTML)) == "" {
		content.ImagePNG = extractHTMLImagePNG(content.HTML)
	}
	if content.IsEmpty() && len(content.Formats) == 0 {
		return nil, nil
	}
	return content, nil
}

func (c *Clipboard) ReadImagePNG() ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !c.open() {
		return nil, fmt.Errorf("OpenClipboard failed")
	}
	defer c.close()
	return readImagePNG(), nil
}

func (c *Clipboard) Write(content *core.ClipboardContent) error {
	if content == nil || content.IsEmpty() {
		return fmt.Errorf("clipboard: empty content")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !c.open() {
		return fmt.Errorf("OpenClipboard failed")
	}
	defer c.close()
	r, _, err := procEmptyClipboard.Call()
	if r == 0 {
		return fmt.Errorf("EmptyClipboard: %w", err)
	}
	wrote := false
	if len(content.ImagePNG) > 0 {
		if setData(cfPNG, content.ImagePNG) {
			wrote = true
		}
		if dib, err := pngToDIB(content.ImagePNG); err == nil && setData(CF_DIB, dib) {
			wrote = true
		}
	}
	if content.Text != "" && setData(CF_UNICODETEXT, utf16Bytes(content.Text)) {
		wrote = true
	}
	if content.HTML != "" && setData(cfHTML, utf8Bytes(content.HTML)) {
		wrote = true
	}
	if content.RTF != "" && setData(cfRTF, utf8Bytes(content.RTF)) {
		wrote = true
	}
	if len(content.Files) > 0 && setData(CF_HDROP, buildDropFiles(content.Files)) {
		wrote = true
	}
	if !wrote {
		return fmt.Errorf("clipboard: no representation was accepted")
	}
	return nil
}

func (c *Clipboard) WriteImagePNG(pngData []byte) error {
	return c.Write(&core.ClipboardContent{ImagePNG: pngData})
}

func (c *Clipboard) WriteText(text string) error {
	return c.Write(&core.ClipboardContent{Text: text})
}

func (c *Clipboard) formatNames() []string {
	var out []string
	var format uint32
	for {
		r, _, _ := procEnumClipboardFormats.Call(uintptr(format))
		if r == 0 {
			break
		}
		format = uint32(r)
		out = append(out, describeFormat(format))
	}
	return out
}

func describeFormat(format uint32) string {
	switch format {
	case CF_UNICODETEXT:
		return "CF_UNICODETEXT"
	case CF_DIB:
		return "CF_DIB"
	case CF_DIBV5:
		return "CF_DIBV5"
	case CF_BITMAP:
		return "CF_BITMAP"
	case CF_HDROP:
		return "CF_HDROP"
	}
	buf := make([]uint16, 256)
	r, _, _ := procGetClipboardFormatNameW.Call(uintptr(format), uintptr(unsafe.Pointer(&buf[0])), 256)
	if r == 0 {
		return fmt.Sprintf("CF_%X", format)
	}
	return UTF16ToString(buf)
}

func readUnicodeText() string {
	r, _, _ := procIsClipboardFormatAvailable.Call(CF_UNICODETEXT)
	if r == 0 {
		return ""
	}
	h, _, _ := procGetClipboardData.Call(CF_UNICODETEXT)
	if h == 0 {
		return ""
	}
	size, _, _ := procGlobalSize.Call(h)
	if size < 2 {
		return ""
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)
	n := int(size)/2 - 1
	if n < 0 {
		n = 0
	}
	buf := unsafe.Slice((*uint16)(unsafe.Pointer(p)), n)
	return UTF16ToString(buf)
}

func readTextFormat(format uint32) string {
	if format == 0 {
		return ""
	}
	r, _, _ := procIsClipboardFormatAvailable.Call(uintptr(format))
	if r == 0 {
		return ""
	}
	h, _, _ := procGetClipboardData.Call(uintptr(format))
	if h == 0 {
		return ""
	}
	size, _, _ := procGlobalSize.Call(h)
	if size == 0 {
		return ""
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)
	data := unsafe.Slice((*byte)(unsafe.Pointer(p)), int(size))
	return string(bytes.TrimRight(data, "\x00"))
}

func readFileDrop() []string {
	r, _, _ := procIsClipboardFormatAvailable.Call(CF_HDROP)
	if r == 0 {
		return nil
	}
	h, _, _ := procGetClipboardData.Call(CF_HDROP)
	if h == 0 {
		return nil
	}
	count, _, _ := procDragQueryFileW.Call(h, 0xFFFFFFFF, 0, 0)
	if count == 0 {
		return nil
	}
	out := make([]string, 0, int(count))
	for i := uintptr(0); i < count; i++ {
		buf := make([]uint16, 32768)
		n, _, _ := procDragQueryFileW.Call(h, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n > 0 {
			out = append(out, UTF16ToString(buf[:n]))
		}
	}
	return out
}

func readImagePNG() []byte {
	initClipboardFormats()
	for _, fmtID := range []uint32{cfPNG, cfImagePNG} {
		if fmtID != 0 {
			if b := readRaw(fmtID); len(b) > 0 {
				if _, err := png.Decode(bytes.NewReader(b)); err == nil {
					return b
				}
			}
		}
	}
	if b := readRaw(CF_DIBV5); len(b) >= 40 {
		if img, err := dibToRGBA(b); err == nil {
			if out := encodePNG(img); len(out) > 0 {
				return out
			}
		}
	}
	if b := readRaw(CF_DIB); len(b) >= 40 {
		if img, err := dibToRGBA(b); err == nil {
			if out := encodePNG(img); len(out) > 0 {
				return out
			}
		}
	}
	if img := readBitmapRGBA(); img != nil {
		if out := encodePNG(img); len(out) > 0 {
			return out
		}
	}
	if cfImageBMP != 0 {
		if b := readRaw(cfImageBMP); len(b) >= 40 {
			dib := b
			if len(dib) > 14 && dib[0] == 'B' && dib[1] == 'M' {
				dib = dib[14:]
			}
			if img, err := dibToRGBA(dib); err == nil {
				if out := encodePNG(img); len(out) > 0 {
					return out
				}
			}
		}
	}
	for _, fmtID := range []uint32{cfJFIF, cfImageJPEG} {
		if fmtID != 0 {
			if b := readRaw(fmtID); len(b) > 0 {
				if img, err := jpeg.Decode(bytes.NewReader(b)); err == nil {
					if out := encodePNG(img); len(out) > 0 {
						return out
					}
				}
			}
		}
	}
	for _, fmtID := range []uint32{cfGIF, cfImageGIF} {
		if fmtID != 0 {
			if b := readRaw(fmtID); len(b) > 0 {
				if img, err := gif.Decode(bytes.NewReader(b)); err == nil {
					if out := encodePNG(img); len(out) > 0 {
						return out
					}
				}
			}
		}
	}
	return nil
}

func DecodeImageBytes(data []byte) (image.Image, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}
	if len(data) > 14 && data[0] == 'B' && data[1] == 'M' {
		if img, err := dibToRGBA(data[14:]); err == nil {
			return img, nil
		}
	}
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	if img, err := dibToRGBA(data); err == nil {
		return img, nil
	}
	return nil, fmt.Errorf("unsupported image format")
}

func readBitmapRGBA() *image.RGBA {
	r, _, _ := procIsClipboardFormatAvailable.Call(CF_BITMAP)
	if r == 0 {
		return nil
	}
	hbmp, _, _ := procGetClipboardData.Call(CF_BITMAP)
	if hbmp == 0 {
		return nil
	}
	var bm Bitmap
	got, _, _ := procGetObjectW.Call(hbmp, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	if got == 0 || bm.BmWidth <= 0 || bm.BmHeight == 0 {
		return nil
	}
	w := int(bm.BmWidth)
	h := int(bm.BmHeight)
	if h < 0 {
		h = -h
	}
	if w > 32768 || h > 32768 {
		return nil
	}
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil
	}
	defer procReleaseDC.Call(0, screenDC)

	var bmi BitmapInfo
	bmi.Header.BiSize = uint32(unsafe.Sizeof(bmi.Header))
	bmi.Header.BiWidth = int32(w)
	bmi.Header.BiHeight = -int32(h)
	bmi.Header.BiPlanes = 1
	bmi.Header.BiBitCount = 32
	bmi.Header.BiCompression = BI_RGB
	bitsBuf := make([]byte, w*h*4)
	lines, _, _ := procGetDIBits.Call(screenDC, hbmp, 0, uintptr(h),
		ptr(unsafe.Pointer(&bitsBuf[0])), ptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS)
	runtime.KeepAlive(bitsBuf)
	if lines == 0 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for i := 0; i < w*h; i++ {
		b, g, rr, a := bitsBuf[i*4], bitsBuf[i*4+1], bitsBuf[i*4+2], bitsBuf[i*4+3]
		img.Pix[i*4+0] = rr
		img.Pix[i*4+1] = g
		img.Pix[i*4+2] = b
		img.Pix[i*4+3] = a
		if a != 0 {
			anyAlpha = true
		}
	}
	if !anyAlpha {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	return img
}

func extractHTMLImagePNG(rawHTML string) []byte {
	lower := strings.ToLower(rawHTML)
	idx := strings.Index(lower, "<img")
	if idx < 0 {
		return nil
	}
	tagEnd := strings.IndexByte(rawHTML[idx:], '>')
	if tagEnd < 0 {
		return nil
	}
	tag := rawHTML[idx : idx+tagEnd+1]
	src := extractAttr(tag, "src")
	if src == "" {
		return nil
	}
	srcLower := strings.ToLower(src)
	const maxBytes = 25 << 20
	if strings.HasPrefix(srcLower, "data:image/") {
		comma := strings.IndexByte(src, ',')
		if comma <= 0 {
			return nil
		}
		meta := strings.ToLower(src[:comma])
		if !strings.Contains(meta, ";base64") {
			return nil
		}
		raw, err := base64.StdEncoding.DecodeString(src[comma+1:])
		if err != nil || len(raw) == 0 || len(raw) > maxBytes {
			return nil
		}
		if _, err := png.Decode(bytes.NewReader(raw)); err == nil {
			return raw
		}
		if img, err := DecodeImageBytes(raw); err == nil {
			return encodePNG(img)
		}
		return nil
	}
	if strings.HasPrefix(srcLower, "file://") {
		u, err := url.Parse(src)
		if err != nil {
			return nil
		}
		path := filepath.FromSlash(strings.TrimPrefix(u.Path, "/"))
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".bmp":
		default:
			return nil
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxBytes {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if ext == ".png" {
			if _, err := png.Decode(bytes.NewReader(raw)); err == nil {
				return raw
			}
		}
		if img, err := DecodeImageBytes(raw); err == nil {
			return encodePNG(img)
		}
	}
	return nil
}

func extractAttr(tag, name string) string {
	lower := strings.ToLower(tag)
	pos := 0
	for {
		i := strings.Index(lower[pos:], name)
		if i < 0 {
			return ""
		}
		start := pos + i
		if start > 0 && lower[start-1] != ' ' && lower[start-1] != '\t' && lower[start-1] != '\r' && lower[start-1] != '\n' {
			pos = start + len(name)
			continue
		}
		j := start + len(name)
		for j < len(tag) && (tag[j] == ' ' || tag[j] == '\t') {
			j++
		}
		if j >= len(tag) || tag[j] != '=' {
			pos = j
			continue
		}
		j++
		for j < len(tag) && (tag[j] == ' ' || tag[j] == '\t') {
			j++
		}
		if j >= len(tag) {
			return ""
		}
		quote := tag[j]
		if quote == '"' || quote == '\'' {
			end := strings.IndexByte(tag[j+1:], quote)
			if end < 0 {
				return ""
			}
			return strings.TrimSpace(tag[j+1 : j+1+end])
		}
		end := j
		for end < len(tag) && tag[end] != ' ' && tag[end] != '>' && tag[end] != '\t' {
			end++
		}
		return strings.TrimSpace(tag[j:end])
	}
}

func readRaw(format uint32) []byte {
	r, _, _ := procIsClipboardFormatAvailable.Call(uintptr(format))
	if r == 0 {
		return nil
	}
	h, _, _ := procGetClipboardData.Call(uintptr(format))
	if h == 0 {
		return nil
	}
	size, _, _ := procGlobalSize.Call(h)
	if size == 0 {
		return nil
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(h)
	data := unsafe.Slice((*byte)(unsafe.Pointer(p)), int(size))
	out := make([]byte, len(data))
	copy(out, data)
	return out
}

func setData(format uint32, data []byte) bool {
	if len(data) == 0 {
		return false
	}
	h, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE|GMEM_ZEROINIT, uintptr(len(data)))
	if h == 0 {
		return false
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return false
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data))
	copy(dst, data)
	procGlobalUnlock.Call(h)
	r, _, _ := procSetClipboardData.Call(uintptr(format), h)
	if r == 0 {
		procGlobalFree.Call(h)
		return false
	}
	return true
}

func utf16Bytes(s string) []byte {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return nil
	}
	b := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return b
}

func utf8Bytes(s string) []byte {
	return append([]byte(s), 0)
}

func buildDropFiles(files []string) []byte {
	if len(files) == 0 {
		return nil
	}
	headerSize := int(unsafe.Sizeof(DropFiles{}))
	var names []uint16
	for _, f := range files {
		u, err := syscall.UTF16FromString(f)
		if err != nil {
			continue
		}
		names = append(names, u[:len(u)-1]...)
		names = append(names, 0)
	}
	names = append(names, 0)
	payload := make([]byte, headerSize+len(names)*2)
	header := DropFiles{PFiles: uint32(headerSize), FWide: 1}
	*(*DropFiles)(unsafe.Pointer(&payload[0])) = header
	for i, v := range names {
		binary.LittleEndian.PutUint16(payload[headerSize+i*2:], v)
	}
	return payload
}

func pngToDIB(pngData []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := w * 4
	payload := make([]byte, 40+stride*h)
	put := func(off int, v uint32) { binary.LittleEndian.PutUint32(payload[off:], v) }
	put(0, 40)
	put(4, uint32(w))
	put(8, uint32(h))
	binary.LittleEndian.PutUint16(payload[12:], 1)
	binary.LittleEndian.PutUint16(payload[14:], 32)
	put(16, 0)
	put(20, uint32(stride*h))
	for y := 0; y < h; y++ {
		srcY := h - 1 - y
		for x := 0; x < w; x++ {
			r, g, bb, _ := img.At(b.Min.X+x, b.Min.Y+srcY).RGBA()
			off := 40 + y*stride + x*4
			payload[off+0] = uint8(bb >> 8)
			payload[off+1] = uint8(g >> 8)
			payload[off+2] = uint8(r >> 8)
			payload[off+3] = 255
		}
	}
	return payload, nil
}

func dibToRGBA(dib []byte) (*image.RGBA, error) {
	if len(dib) < 40 {
		return nil, fmt.Errorf("DIB too short")
	}
	headerSize := int(binary.LittleEndian.Uint32(dib[0:4]))
	width := int(int32(binary.LittleEndian.Uint32(dib[4:8])))
	rawHeight := int(int32(binary.LittleEndian.Uint32(dib[8:12])))
	bitCount := int(binary.LittleEndian.Uint16(dib[14:16]))
	compression := binary.LittleEndian.Uint32(dib[16:20])
	clrUsed := int(binary.LittleEndian.Uint32(dib[32:36]))
	if width <= 0 || rawHeight == 0 || width > 32768 {
		return nil, fmt.Errorf("DIB has invalid dimensions")
	}
	height := rawHeight
	topDown := false
	if height < 0 {
		height = -height
		topDown = true
	}
	if height > 32768 {
		return nil, fmt.Errorf("DIB height too large")
	}
	if headerSize < 40 || headerSize > len(dib) {
		return nil, fmt.Errorf("DIB header size %d is invalid", headerSize)
	}

	var rMask, gMask, bMask, aMask uint32
	switch bitCount {
	case 32:
		rMask, gMask, bMask, aMask = 0x00FF0000, 0x0000FF00, 0x000000FF, 0xFF000000
	case 16:
		rMask, gMask, bMask, aMask = 0x7C00, 0x03E0, 0x001F, 0
	}

	extra := 0
	if compression == BI_BITFIELDS || compression == BI_ALPHABITFIELDS {
		if headerSize == 40 {
			if len(dib) < 40+12 {
				return nil, fmt.Errorf("DIB missing bitfield masks")
			}
			rMask = binary.LittleEndian.Uint32(dib[40:44])
			gMask = binary.LittleEndian.Uint32(dib[44:48])
			bMask = binary.LittleEndian.Uint32(dib[48:52])
			aMask = 0
			extra = 12
			if compression == BI_ALPHABITFIELDS && len(dib) >= 40+16 {
				aMask = binary.LittleEndian.Uint32(dib[52:56])
				extra = 16
			}
		} else if headerSize >= 52 {
			rMask = binary.LittleEndian.Uint32(dib[40:44])
			gMask = binary.LittleEndian.Uint32(dib[44:48])
			bMask = binary.LittleEndian.Uint32(dib[48:52])
			aMask = 0
			if headerSize >= 56 {
				aMask = binary.LittleEndian.Uint32(dib[52:56])
			}
		}
	}

	paletteCount := 0
	if clrUsed > 0 {
		paletteCount = clrUsed
	} else if bitCount > 0 && bitCount <= 8 {
		paletteCount = 1 << bitCount
	}
	paletteOffset := headerSize + extra
	if paletteCount < 0 || uint64(paletteOffset)+uint64(paletteCount)*4 > uint64(len(dib)) {
		return nil, fmt.Errorf("DIB palette exceeds buffer")
	}
	pixelOffset := paletteOffset + paletteCount*4
	if headerSize >= 124 && compression != BI_PNG && compression != BI_JPEG {
		profData := int(binary.LittleEndian.Uint32(dib[112:116]))
		profSize := int(binary.LittleEndian.Uint32(dib[116:120]))
		if profSize > 0 && profData == pixelOffset && uint64(profData)+uint64(profSize) < uint64(len(dib)) {
			pixelOffset += profSize
		}
	}

	if compression == BI_PNG || compression == BI_JPEG {
		if pixelOffset >= len(dib) {
			return nil, fmt.Errorf("DIB compressed payload missing")
		}
		var dec image.Image
		var err error
		if compression == BI_PNG {
			dec, err = png.Decode(bytes.NewReader(dib[pixelOffset:]))
		} else {
			dec, err = jpeg.Decode(bytes.NewReader(dib[pixelOffset:]))
		}
		if err != nil {
			return nil, err
		}
		rgba := image.NewRGBA(dec.Bounds())
		draw.Draw(rgba, rgba.Bounds(), dec, dec.Bounds().Min, draw.Src)
		return rgba, nil
	}

	if compression != BI_RGB && compression != BI_BITFIELDS && compression != BI_ALPHABITFIELDS {
		return nil, fmt.Errorf("DIB compression %d is unsupported", compression)
	}
	switch bitCount {
	case 32, 24, 16, 8, 4, 1:
	default:
		return nil, fmt.Errorf("DIB bit count %d is unsupported", bitCount)
	}

	stride := ((width*bitCount + 31) / 32) * 4
	if uint64(pixelOffset)+uint64(stride)*uint64(height) > uint64(len(dib)) {
		return nil, fmt.Errorf("DIB is truncated")
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	anyAlpha := false
	hasExplicitAlpha := bitCount == 32 && aMask != 0
	for row := 0; row < height; row++ {
		srcRow := row
		if !topDown {
			srcRow = height - 1 - row
		}
		src := pixelOffset + srcRow*stride
		switch bitCount {
		case 32:
			for x := 0; x < width; x++ {
				px := binary.LittleEndian.Uint32(dib[src+x*4 : src+x*4+4])
				di := row*img.Stride + x*4
				img.Pix[di+0] = extractChannel(px, rMask)
				img.Pix[di+1] = extractChannel(px, gMask)
				img.Pix[di+2] = extractChannel(px, bMask)
				a := uint8(255)
				if hasExplicitAlpha {
					a = extractChannel(px, aMask)
					if a != 0 {
						anyAlpha = true
					}
				}
				img.Pix[di+3] = a
			}
		case 24:
			for x := 0; x < width; x++ {
				off := src + x*3
				di := row*img.Stride + x*4
				img.Pix[di+0] = dib[off+2]
				img.Pix[di+1] = dib[off+1]
				img.Pix[di+2] = dib[off+0]
				img.Pix[di+3] = 255
			}
		case 16:
			for x := 0; x < width; x++ {
				px := uint32(binary.LittleEndian.Uint16(dib[src+x*2 : src+x*2+2]))
				di := row*img.Stride + x*4
				img.Pix[di+0] = extractChannel(px, rMask)
				img.Pix[di+1] = extractChannel(px, gMask)
				img.Pix[di+2] = extractChannel(px, bMask)
				a := uint8(255)
				if aMask != 0 {
					a = extractChannel(px, aMask)
					if a != 0 {
						anyAlpha = true
					}
				}
				img.Pix[di+3] = a
			}
		case 8:
			for x := 0; x < width; x++ {
				idx := int(dib[src+x])
				setIndexedPixel(img, x, row, dib, paletteOffset, paletteCount, idx)
			}
		case 4:
			for x := 0; x < width; x++ {
				b := dib[src+x/2]
				var idx int
				if x%2 == 0 {
					idx = int(b >> 4)
				} else {
					idx = int(b & 0x0F)
				}
				setIndexedPixel(img, x, row, dib, paletteOffset, paletteCount, idx)
			}
		case 1:
			for x := 0; x < width; x++ {
				b := dib[src+x/8]
				idx := int((b >> uint(7-(x%8))) & 1)
				setIndexedPixel(img, x, row, dib, paletteOffset, paletteCount, idx)
			}
		}
	}
	if (hasExplicitAlpha || (bitCount == 16 && aMask != 0)) && !anyAlpha {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
	}
	return img, nil
}

func extractChannel(val, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := bits.TrailingZeros32(mask)
	width := bits.OnesCount32(mask)
	v := (val & mask) >> shift
	if width >= 8 {
		return uint8(v >> (width - 8))
	}
	maxVal := uint32((1 << width) - 1)
	if maxVal == 0 {
		return 0
	}
	return uint8((v*255 + maxVal/2) / maxVal)
}

func setIndexedPixel(img *image.RGBA, x, y int, dib []byte, palOff, palCount, idx int) {
	di := y*img.Stride + x*4
	if idx < 0 || idx >= palCount {
		img.Pix[di+3] = 255
		return
	}
	off := palOff + idx*4
	img.Pix[di+0] = dib[off+2]
	img.Pix[di+1] = dib[off+1]
	img.Pix[di+2] = dib[off+0]
	img.Pix[di+3] = 255
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

func timeSleep(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// ClipboardListener receives WM_CLIPBOARDUPDATE on its own message loop.
type ClipboardListener struct {
	win      *MessageWindow
	mu       sync.Mutex
	running  bool
	onUpdate func()
}

func NewClipboardListener() (*ClipboardListener, error) {
	w, err := NewMessageWindow("clipboard")
	if err != nil {
		return nil, err
	}
	l := &ClipboardListener{win: w}
	w.OnMessage(WM_CLIPBOARDUPDATE, func(_, _ uintptr) uintptr {
		l.mu.Lock()
		fn := l.onUpdate
		l.mu.Unlock()
		if fn != nil {
			fn()
		}
		return 0
	})
	return l, nil
}

func (l *ClipboardListener) Handle() HWND {
	if l == nil || l.win == nil {
		return 0
	}
	return l.win.HWND
}

func (l *ClipboardListener) OnUpdate(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onUpdate = fn
}

func (l *ClipboardListener) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return nil
	}
	r, _, err := procAddClipboardFormatListener.Call(uintptr(l.win.HWND))
	if r == 0 {
		return fmt.Errorf("AddClipboardFormatListener: %w", err)
	}
	l.running = true
	return nil
}

func (l *ClipboardListener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.running {
		return
	}
	procRemoveClipboardFormatListener.Call(uintptr(l.win.HWND))
	l.running = false
}

func (l *ClipboardListener) Close() {
	if l == nil {
		return
	}
	l.Stop()
	l.win.Close()
}
