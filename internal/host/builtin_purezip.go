//go:build !nopurezip

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/purezip"
)

const purezipCompiled = true

func builtinPureZip() (PluginFactory, bool) {
	return PluginFactory{
		ID:       purezip.PluginID,
		ModuleID: core.ModuleZip,
		Name:     i18n.S("PureZip（压缩与解压管理）", "PureZip (Archive Manager)"),
		Desc:     i18n.S("浏览/智能解压/多线程压缩/加密/右键菜单与文件关联", "Browse, smart extract, multi-threaded compress, encrypt & shell integration"),
		New:      func() purekit.Plugin { return purezip.NewPlugin() },
	}, true
}
