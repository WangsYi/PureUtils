//go:build !nopurevideo

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/purevideo"
)

const purevideoCompiled = true

func builtinPureVideo() (PluginFactory, bool) {
	return PluginFactory{
		ID:       purevideo.PluginID,
		ModuleID: core.ModuleVideo,
		Name:     i18n.S("PureVideo（极速视频浏览）", "PureVideo (Fast Video Player)"),
		Desc:     i18n.S("内置 libmpv 硬件解码，支持 MP4 / MKV / MOV / HEVC / FLV 与外挂字幕", "Built-in libmpv hardware decoding for 21 video formats & external subtitles"),
		New:      func() purekit.Plugin { return purevideo.NewPlugin() },
	}, true
}
