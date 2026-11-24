//go:build !nopurezip

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/plugins/purezip"
)

const purezipCompiled = true

func builtinPureZip() (PluginFactory, bool) {
	return PluginFactory{
		ID:       purezip.PluginID,
		ModuleID: core.ModuleZip,
		Name:     "PureZip（压缩与解压管理）",
		Desc:     "浏览/智能解压/多线程压缩/加密/右键菜单与文件关联",
		New:      func() purekit.Plugin { return purezip.NewPlugin() },
	}, true
}
