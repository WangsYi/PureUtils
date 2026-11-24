//go:build !nopureclip

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/plugins/pureclip"
)

const pureclipCompiled = true

func builtinPureClip() (PluginFactory, bool) {
	return PluginFactory{
		ID:       pureclip.PluginID,
		ModuleID: core.ModuleHistory,
		Name:     "PureClip（剪贴板历史）",
		Desc:     "自动监听文本/富文本/图片/文件并持久化检索",
		New:      func() purekit.Plugin { return pureclip.NewPlugin() },
	}, true
}
