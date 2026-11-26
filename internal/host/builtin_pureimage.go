//go:build !nopureimage && !nofreeimage

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/pureimage"
)

const pureimageCompiled = true

func builtinPureImage() (PluginFactory, bool) {
	return PluginFactory{
		ID:       pureimage.PluginID,
		ModuleID: core.ModuleViewer,
		Name:     i18n.S("PureImage（看图与空格预览）", "PureImage (Image Viewer & Space Preview)"),
		Desc:     i18n.S("支持 JPEG Turbo/HEIC/AVIF/JXL/RAW、动图与资源管理器空格预览", "JPEG Turbo/HEIC/AVIF/JXL/RAW, animations & Explorer space preview"),
		New:      func() purekit.Plugin { return pureimage.NewPlugin() },
	}, true
}
