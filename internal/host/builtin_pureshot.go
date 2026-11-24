//go:build !nopureshot

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/plugins/pureshot"
)

const pureshotCompiled = true

func builtinPureShot() (PluginFactory, bool) {
	return PluginFactory{
		ID:       pureshot.PluginID,
		ModuleID: core.ModuleCapture,
		Name:     "PureShot（截图、标注与悬浮贴图）",
		Desc:     "区域/整屏截图、9 种标注与置顶悬浮贴图",
		New:      func() purekit.Plugin { return pureshot.NewPlugin() },
	}, true
}
