//go:build !nopurevideo

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/plugins/purevideo"
)

const purevideoCompiled = true

func builtinPureVideo() (PluginFactory, bool) {
	return PluginFactory{
		ID:       purevideo.PluginID,
		ModuleID: core.ModuleVideo,
		Name:     "PureVideo（极速视频浏览）",
		Desc:     "内置 libmpv 硬件解码，支持 MP4 / MKV / MOV / HEVC / FLV 与外挂字幕",
		New:      func() purekit.Plugin { return purevideo.NewPlugin() },
	}, true
}
