//go:build !nopuremonitor

package host

import (
	"pureutils/pkg/purekit"
	"pureutils/pkg/purekit/core"
	"pureutils/pkg/purekit/i18n"
	"pureutils/plugins/puremonitor"
)

const puremonitorCompiled = true

func builtinPureMonitor() (PluginFactory, bool) {
	return PluginFactory{
		ID:       puremonitor.PluginID,
		ModuleID: core.ModuleMonitor,
		Name:     i18n.S("PureMonitor（硬件与状态监控）", "PureMonitor (System & Hardware Monitor)"),
		Desc:     i18n.S("任务栏与可收起悬浮窗监控、硬件配置详情、游戏帧率 HUD 与可选 PawnIO 温度扩展", "Taskbar & collapsible floating monitor, hardware specs, game FPS HUD & optional PawnIO sensor extension"),
		New:      func() purekit.Plugin { return puremonitor.NewPlugin() },
	}, true
}

// RunMonitorHelper runs the elevated telemetry/ETW helper mode if args contain --monitor-helper.
func RunMonitorHelper(args []string) (bool, int) {
	return puremonitor.RunHelperCLI(args)
}
