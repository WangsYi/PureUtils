//go:build !nopureshot

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/pureshot"
)

const pureshotCompiled = true

func builtinPureShot() (PluginFactory, bool) {
	return PluginFactory{
		ID:       pureshot.PluginID,
		ModuleID: core.ModuleCapture,
		Name:     i18n.S("PureShot（截图、标注与悬浮贴图）", "PureShot (Capture, Annotate & Pins)"),
		Desc:     i18n.S("区域/整屏截图、9 种标注与置顶悬浮贴图", "Region/full-screen capture, 9 annotations & always-on-top pins"),
		New:      func() purekit.Plugin { return pureshot.NewPlugin() },
	}, true
}
