package purezip

import (
	"github.com/egoist/mygo/ui"

	pureui "pureutils/pkg/purekit/ui"
)

// Icons used by the PureZip compression manager module (lucide-style strokes).
var (
	iconArchive     = pureui.SVGStroke(`<path d="M21 8v13H3V8"/><path d="M1 3h22v5H1z"/><path d="M10 12h4"/>`)
	iconExtract     = pureui.SVGStroke(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>`)
	iconCompress    = pureui.SVGStroke(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/>`)
	iconFilePlus    = pureui.SVGStroke(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" y1="18" x2="12" y2="12"/><line x1="9" y1="15" x2="15" y2="15"/>`)
	iconFolderOpen  = pureui.SVGStroke(`<path d="m6 14 1.45-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.55 6a2 2 0 0 1-1.94 1.5H4a2 2 0 0 1-2-2V5c0-1.1.9-2 2-2h3.93a2 2 0 0 1 1.66.9l.82 1.2a2 2 0 0 0 1.66.9H18a2 2 0 0 1 2 2v2"/>`)
	iconArrowUp     = pureui.SVGStroke(`<line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/>`)
	iconArrowLeft   = pureui.SVGStroke(`<line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/>`)
	iconArrowRight  = pureui.SVGStroke(`<line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/>`)
	iconLock        = pureui.SVGStroke(`<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>`)
	iconInfo        = pureui.SVGStroke(`<circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/>`)
	iconShieldCheck = pureui.SVGStroke(`<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="m9 12 2 2 4-4"/>`)
	iconChevronDown = pureui.SVGStroke(`<polyline points="6 9 12 15 18 9"/>`)
	iconFolderFill  = pureui.SVGFill(`<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2Z"/>`)
	iconFileCode    = pureui.SVGStroke(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><path d="m10 13-2 2 2 2"/><path d="m14 17 2-2-2-2"/>`)
	iconFileAudio   = pureui.SVGStroke(`<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`)
	iconFileVideo   = pureui.SVGStroke(`<rect x="2" y="4" width="20" height="16" rx="2"/><path d="m10 9 5 3-5 3z"/>`)
	iconFileExe     = pureui.SVGStroke(`<rect x="3" y="4" width="18" height="16" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><circle cx="6" cy="6.5" r=".6"/><circle cx="8.5" cy="6.5" r=".6"/>`)
	iconRefresh     = pureui.SVGStroke(`<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>`)
	iconKey         = pureui.SVGStroke(`<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6"/><path d="m15.5 7.5 3 3L22 7l-3-3"/>`)
	iconEyeOff      = pureui.SVGStroke(`<path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.53 13.53 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/><line x1="2" y1="2" x2="22" y2="22"/>`)
)

// zipKindIcon returns the SVG icon and accent color for a file category.
func zipKindIcon(cat zipFileCategory, pal pureui.Palette) (*ui.SVG, ui.Color) {
	switch cat {
	case zipCatFolder:
		return iconFolderFill, pal.KindFiles
	case zipCatImage:
		return pureui.IconImage, pal.KindImage
	case zipCatCode:
		return iconFileCode, pal.KindText
	case zipCatAudio:
		return iconFileAudio, ui.Hex("#EC4899")
	case zipCatVideo:
		return iconFileVideo, pal.KindRich
	case zipCatArchive:
		return iconArchive, ui.Hex("#F97316")
	case zipCatDoc:
		return pureui.IconFileText, pal.Accent
	case zipCatExe:
		return iconFileExe, pal.TextSecondary
	default:
		return pureui.IconFiles, pal.TextMuted
	}
}
