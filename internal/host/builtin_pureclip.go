//go:build !nopureclip

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/pureclip"
)

const pureclipCompiled = true

func builtinPureClip() (PluginFactory, bool) {
	return PluginFactory{
		ID:       pureclip.PluginID,
		ModuleID: core.ModuleHistory,
		Name:     i18n.S("PureClip（剪贴板历史）", "PureClip (Clipboard History)"),
		Desc:     i18n.S("自动监听文本/富文本/图片/文件并持久化检索", "Auto-capture text/rich text/images/files with instant search"),
		New:      func() purekit.Plugin { return pureclip.NewPlugin() },
	}, true
}
