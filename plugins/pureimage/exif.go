package pureimage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strings"
)

// ExifData holds orientation and camera exposure metadata extracted from an image.
type ExifData struct {
	Orientation int
	Make        string
	Model       string
	DateTime    string
	Taken       string
	Exposure    string
	FNumber     string
	ISO         string
	Focal       string
}

// ParseExif parses JPEG APP1 EXIF metadata from raw file bytes.
func ParseExif(data []byte) ExifData {
	e := ExifData{Orientation: 1}
	if len(data) < 12 || data[0] != 0xFF || data[1] != 0xD8 {
		return e
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		if marker == 0xD8 || marker == 0xD9 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		if i+4 > len(data) {
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if segLen < 2 || i+2+segLen > len(data) {
			break
		}
		if marker == 0xE1 && segLen >= 8 && string(data[i+4:i+10]) == "Exif\x00\x00" {
			parseTIFF(data[i+10:i+2+segLen], &e)
			break
		}
		i += 2 + segLen
	}
	return e
}

func parseTIFF(tiff []byte, e *ExifData) {
	if len(tiff) < 8 {
		return
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return
	}
	ifd0 := int(order.Uint32(tiff[4:8]))
	parseIFD(tiff, ifd0, order, e, true)
}

func parseIFD(tiff []byte, off int, order binary.ByteOrder, e *ExifData, follow bool) {
	if off <= 0 || off+2 > len(tiff) {
		return
	}
	count := int(order.Uint16(tiff[off : off+2]))
	if count < 0 || count > 512 || off+2+count*12 > len(tiff) {
		return
	}
	exifIFD := 0
	for i := 0; i < count; i++ {
		p := off + 2 + i*12
		tag := order.Uint16(tiff[p : p+2])
		typ := order.Uint16(tiff[p+2 : p+4])
		n := order.Uint32(tiff[p+4 : p+8])
		val := tiff[p+8 : p+12]

		switch tag {
		case 0x0112: // Orientation
			if s := readShort(tiff, val, order, typ, n); s >= 1 && s <= 8 {
				e.Orientation = int(s)
			}
		case 0x010F:
			e.Make = readASCII(tiff, val, order, typ, n)
		case 0x0110:
			e.Model = readASCII(tiff, val, order, typ, n)
		case 0x0132:
			e.DateTime = readASCII(tiff, val, order, typ, n)
		case 0x9003:
			e.Taken = readASCII(tiff, val, order, typ, n)
		case 0x829A:
			e.Exposure = readRational(tiff, val, order, typ, n)
		case 0x829D:
			e.FNumber = readRational(tiff, val, order, typ, n)
		case 0x8827:
			if iso := readShort(tiff, val, order, typ, n); iso > 0 {
				e.ISO = fmt.Sprintf("%d", iso)
			}
		case 0x920A:
			e.Focal = readRational(tiff, val, order, typ, n)
		case 0x8769: // Exif IFD pointer
			exifIFD = int(order.Uint32(val))
		}
	}
	if follow && exifIFD > 0 {
		parseIFD(tiff, exifIFD, order, e, false)
	}
}

func entryData(tiff []byte, val []byte, order binary.ByteOrder, typ uint16, n uint32) []byte {
	sizes := map[uint16]uint64{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}
	size := sizes[typ]
	if size == 0 || n == 0 {
		return nil
	}
	total := size * uint64(n)
	if total > uint64(len(tiff)) {
		if total <= 4 && int(total) <= len(val) {
			return val[:int(total)]
		}
		return nil
	}
	if total <= 4 {
		return val[:min(int(total), len(val))]
	}
	off := uint64(order.Uint32(val))
	if off == 0 || off+total > uint64(len(tiff)) {
		return nil
	}
	return tiff[off : off+total]
}

func readASCII(tiff []byte, val []byte, order binary.ByteOrder, typ uint16, n uint32) string {
	d := entryData(tiff, val, order, typ, n)
	if d == nil {
		return ""
	}
	if idx := bytes.IndexByte(d, 0); idx >= 0 {
		d = d[:idx]
	}
	return strings.TrimSpace(string(d))
}

func readShort(tiff []byte, val []byte, order binary.ByteOrder, typ uint16, n uint32) uint16 {
	d := entryData(tiff, val, order, typ, n)
	if len(d) < 2 {
		return 0
	}
	return order.Uint16(d[:2])
}

func readRational(tiff []byte, val []byte, order binary.ByteOrder, typ uint16, n uint32) string {
	d := entryData(tiff, val, order, typ, n)
	if len(d) < 8 {
		return ""
	}
	num := order.Uint32(d[:4])
	den := order.Uint32(d[4:8])
	if den == 0 || num == 0 {
		return ""
	}
	v := float64(num) / float64(den)
	if v >= 1 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("1/%d", int(1/v+0.5))
}

// ExifFromFields constructs ExifData from decoded container EXIF fields.
func ExifFromFields(make, model, date, taken string, exposure, fnumber float64, iso int, focal float64) ExifData {
	ex := ExifData{
		Orientation: 1,
		Make:        strings.TrimSpace(make),
		Model:       strings.TrimSpace(model),
		DateTime:    strings.TrimSpace(date),
		Taken:       strings.TrimSpace(taken),
	}
	if exposure > 0 {
		if exposure < 1 {
			ex.Exposure = fmt.Sprintf("1/%d", int(1/exposure+0.5))
		} else {
			ex.Exposure = fmt.Sprintf("%.1f", exposure)
		}
	}
	if fnumber > 0 {
		ex.FNumber = fmt.Sprintf("%.1f", fnumber)
	}
	if iso > 0 {
		ex.ISO = fmt.Sprintf("%d", iso)
	}
	if focal > 0 {
		ex.Focal = fmt.Sprintf("%.0f", focal)
	}
	return ex
}

// Camera returns a human-readable camera make and model string.
func (e ExifData) Camera() string {
	if e.Make != "" && e.Model != "" && strings.HasPrefix(strings.ToLower(e.Model), strings.ToLower(e.Make)) {
		return strings.TrimSpace(e.Model)
	}
	return strings.TrimSpace(strings.Join([]string{e.Make, e.Model}, " "))
}

// ExposureText formats shutter speed, aperture, ISO, and focal length.
func (e ExifData) ExposureText() string {
	var parts []string
	if e.Exposure != "" {
		parts = append(parts, e.Exposure+"s")
	}
	if e.FNumber != "" {
		parts = append(parts, "f/"+e.FNumber)
	}
	if e.ISO != "" {
		parts = append(parts, "ISO "+e.ISO)
	}
	if e.Focal != "" {
		parts = append(parts, e.Focal+"mm")
	}
	return strings.Join(parts, " · ")
}

// LargestEmbeddedJPEG finds the largest embedded JPEG stream (SOI..EOI) in a container or RAW file.
func LargestEmbeddedJPEG(data []byte) []byte {
	var best []byte
	for i := 0; i+3 < len(data); i++ {
		if data[i] != 0xFF || data[i+1] != 0xD8 || data[i+2] != 0xFF {
			continue
		}
		end := bytes.Index(data[i+2:], []byte{0xFF, 0xD9})
		if end < 0 {
			continue
		}
		end += i + 2 + 2
		if end-i > len(best) {
			best = data[i:end]
		}
		i = end - 1
	}
	return best
}

// ImageHasAlpha reports whether img contains any non-opaque pixels.
func ImageHasAlpha(img image.Image) bool {
	switch im := img.(type) {
	case *image.NRGBA:
		for i := 3; i < len(im.Pix); i += 4 {
			if im.Pix[i] != 0xFF {
				return true
			}
		}
		return false
	case *image.RGBA:
		for i := 3; i < len(im.Pix); i += 4 {
			if im.Pix[i] != 0xFF {
				return true
			}
		}
		return false
	case *image.Gray, *image.Gray16, *image.YCbCr, *image.CMYK:
		return false
	case *image.Paletted:
		for _, c := range im.Palette {
			_, _, _, a := c.RGBA()
			if a < 0xFFFF {
				return true
			}
		}
		return false
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a < 0xFFFF {
				return true
			}
		}
	}
	return false
}

// TransformedDims computes image dimensions after EXIF orientation and user quarter rotation.
func TransformedDims(w, h, orientation, rotate int) (int, int) {
	if orientation >= 5 && orientation <= 8 {
		w, h = h, w
	}
	if ((rotate%180)+180)%180 != 0 {
		w, h = h, w
	}
	return w, h
}

// FitWithin returns scaled dimensions keeping aspect ratio with the longest edge equal to maxDim.
func FitWithin(w, h, maxDim int) (int, int) {
	if w <= 0 || h <= 0 || maxDim <= 0 {
		return max(1, w), max(1, h)
	}
	if w >= h {
		return maxDim, max(1, int(float64(h)*float64(maxDim)/float64(w)))
	}
	return max(1, int(float64(w)*float64(maxDim)/float64(h))), maxDim
}

// TransformImage applies EXIF orientation, quarter-turn rotation, and horizontal flip.
func TransformImage(src image.Image, orientation, rotate int, flip bool) image.Image {
	img := OrientImage(src, orientation)
	switch ((rotate % 360) + 360) % 360 {
	case 90:
		img = Rotate90(img)
	case 180:
		img = Rotate90(Rotate90(img))
	case 270:
		img = Rotate90(Rotate90(Rotate90(img)))
	}
	if flip {
		img = FlipHorizontal(img)
	}
	return img
}

// Rotate90 rotates src 90 degrees clockwise.
func Rotate90(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < w; y++ {
		for x := 0; x < h; x++ {
			r, g, bl, a := src.At(b.Min.X+y, b.Min.Y+h-1-x).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

// FlipHorizontal mirrors src horizontally.
func FlipHorizontal(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := src.At(b.Min.X+w-1-x, b.Min.Y+y).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

// OrientImage applies EXIF orientation (values 2..8).
func OrientImage(src image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))

	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch orientation {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			r, g, bl, a := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}
