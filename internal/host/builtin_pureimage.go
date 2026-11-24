//go:build !nopureimage && !nofreeimage

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/plugins/pureimage"
)

const pureimageCompiled = true

func builtinPureImage() (PluginFactory, bool) {
	return PluginFactory{
		ID:       pureimage.PluginID,
		ModuleID: core.ModuleViewer,
		Name:     "PureImage（看图与空格预览）",
		Desc:     "支持 JPEG Turbo/HEIC/AVIF/JXL/RAW、动图与资源管理器空格预览",
		New:      func() purekit.Plugin { return pureimage.NewPlugin() },
	}, true
}
