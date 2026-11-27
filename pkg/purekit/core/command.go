package core

import (
	"strconv"
	"strings"

	"pureutils/pkg/purekit/i18n"
)

// Command is an action that can be triggered by a hotkey, the tray, the
// launcher, the command line or the UI.
type Command string

const (
	CmdCaptureRegion         Command = "capture-region"
	CmdCaptureMonitor        Command = "capture-monitor"
	CmdPinClipboard          Command = "pin-clipboard"
	CmdShowHistory           Command = "show-history"
	CmdTogglePins            Command = "toggle-pins"
	CmdPinsClickable         Command = "pins-clickable"
	CmdTogglePinsPassthrough Command = "toggle-pins-passthrough"
	CmdShowSettings          Command = "show-settings"
	CmdOpenLauncher          Command = "open-launcher"
	CmdToggleMonitoring      Command = "toggle-monitoring"
	CmdOpenViewer            Command = "open-viewer"
	CmdToggleSpacePreview    Command = "toggle-space-preview"
	CmdOpenVideo             Command = "open-video"
	CmdOpenMonitor           Command = "open-monitor"
	CmdToggleMonitorOverlay  Command = "toggle-monitor"
	CmdToggleGameHUD         Command = "toggle-game-hud"
	CmdOpenZip               Command = "open-zip"
	CmdExtractSmart          Command = "extract-smart"
	CmdExtractHere           Command = "extract-here"
	CmdExtractTo             Command = "extract-to"
	CmdExtractDialog         Command = "extract-dialog"
	CmdCompress              Command = "compress"
	CmdCompressZip           Command = "compress-zip"
	CmdCompress7z            Command = "compress-7z"
	CmdTestArchive           Command = "test-archive"
	CmdCheckUpdate           Command = "check-update"
	CmdPerfReport            Command = "perf-report"
	CmdExit                  Command = "exit"
)

// Verb is one command-line spelling.
type Verb struct {
	Arg         string
	Command     Command
	Description string
}

// Verbs is the command table, in help order.
var Verbs = []Verb{
	{"--capture", CmdCaptureRegion, "Start region capture overlay"},
	{"--capture-monitor", CmdCaptureMonitor, "Capture full monitor under cursor and copy"},
	{"--pin", CmdPinClipboard, "Pin clipboard image as floating window"},
	{"--history", CmdShowHistory, "Open clipboard history panel"},
	{"--toggle-pins", CmdTogglePins, "Show or hide all floating pins"},
	{"--launcher", CmdOpenLauncher, "Open quick launcher"},
	{"--settings", CmdShowSettings, "Open settings window"},
	{"--toggle-monitoring", CmdToggleMonitoring, "Pause or resume clipboard monitoring"},
	{"--view", CmdOpenViewer, "Open PureImage viewer (optional image or folder path)"},
	{"--video", CmdOpenVideo, "Open PureVideo player (optional video or folder path)"},
	{"--monitor", CmdOpenMonitor, "Open PureMonitor hardware and system dashboard"},
	{"--toggle-monitor", CmdToggleMonitorOverlay, "Show or hide system monitor overlays"},
	{"--toggle-game-hud", CmdToggleGameHUD, "Show or hide in-game FPS HUD"},
	{"--toggle-space-preview", CmdToggleSpacePreview, "Toggle Explorer spacebar preview"},
	{"--zip", CmdOpenZip, "Open PureZip archive manager (optional archive path)"},
	{"--extract-smart", CmdExtractSmart, "Smart extract: auto-detect root folder"},
	{"--extract-here", CmdExtractHere, "Extract to current archive directory"},
	{"--extract-to", CmdExtractTo, "Extract into a subfolder named after the archive"},
	{"--extract-dialog", CmdExtractDialog, "Choose destination and extract options"},
	{"--compress", CmdCompress, "Open compress dialog for selected files or folders"},
	{"--compress-zip", CmdCompressZip, "Compress directly to a .zip archive"},
	{"--compress-7z", CmdCompress7z, "Compress directly to a .7z archive"},
	{"--test-archive", CmdTestArchive, "Test archive integrity"},
	{"--check-update", CmdCheckUpdate, "Open settings and check for PureUtils updates"},
	{"--perf", CmdPerfReport, "Write performance trace summary to log"},
	{"--exit", CmdExit, "Quit PureUtils"},
}

// LocalizedDescription returns the localized help description for v.
func (v Verb) LocalizedDescription() string {
	switch v.Command {
	case CmdCaptureRegion:
		return i18n.S("进入区域截图遮罩", "Start region capture overlay")
	case CmdCaptureMonitor:
		return i18n.S("截取光标所在显示器整屏并复制", "Capture full monitor under cursor and copy")
	case CmdPinClipboard:
		return i18n.S("把剪贴板图片贴成悬浮贴图", "Pin clipboard image as floating window")
	case CmdShowHistory:
		return i18n.S("打开剪贴板历史面板", "Open clipboard history panel")
	case CmdTogglePins:
		return i18n.S("隐藏/显示所有贴图", "Show or hide all floating pins")
	case CmdOpenLauncher:
		return i18n.S("打开快捷启动器", "Open quick launcher")
	case CmdShowSettings:
		return i18n.S("打开设置窗口", "Open settings window")
	case CmdToggleMonitoring:
		return i18n.S("暂停/恢复剪贴板监听", "Pause or resume clipboard monitoring")
	case CmdOpenViewer:
		return i18n.S("打开 PureImage 看图器（可后接图片或目录路径）", "Open PureImage viewer (optional image or folder path)")
	case CmdOpenVideo:
		return i18n.S("打开 PureVideo 极速视频浏览（可后接视频或目录路径）", "Open PureVideo player (optional video or folder path)")
	case CmdOpenMonitor:
		return i18n.S("打开 PureMonitor 硬件信息与系统监控面板", "Open PureMonitor hardware and system dashboard")
	case CmdToggleMonitorOverlay:
		return i18n.S("显示/隐藏任务栏与悬浮系统监控", "Show or hide system monitor overlays")
	case CmdToggleGameHUD:
		return i18n.S("显示/隐藏游戏帧率 HUD", "Show or hide in-game FPS HUD")
	case CmdToggleSpacePreview:
		return i18n.S("开启/关闭资源管理器空格预览", "Toggle Explorer spacebar preview")
	case CmdOpenZip:
		return i18n.S("打开 PureZip 压缩管理器（可后接压缩文件路径）", "Open PureZip archive manager (optional archive path)")
	case CmdExtractSmart:
		return i18n.S("智能解压：单一根目录直接解压，否则解压到同名文件夹", "Smart extract: auto-detect root folder")
	case CmdExtractHere:
		return i18n.S("解压到压缩文件所在目录", "Extract to current archive directory")
	case CmdExtractTo:
		return i18n.S("解压到以压缩文件命名的文件夹", "Extract into a subfolder named after the archive")
	case CmdExtractDialog:
		return i18n.S("选择目标位置与选项后解压", "Choose destination and extract options")
	case CmdCompress:
		return i18n.S("打开新建压缩文件对话框（后接文件或文件夹）", "Open compress dialog for selected files or folders")
	case CmdCompressZip:
		return i18n.S("一键压缩为同名 .zip", "Compress directly to a .zip archive")
	case CmdCompress7z:
		return i18n.S("一键压缩为同名 .7z", "Compress directly to a .7z archive")
	case CmdTestArchive:
		return i18n.S("测试压缩文件完整性", "Test archive integrity")
	case CmdCheckUpdate:
		return i18n.S("打开设置并检查 PureUtils 新版本", "Open settings and check for PureUtils updates")
	case CmdPerfReport:
		return i18n.S("把性能打点统计写进日志", "Write performance trace summary to log")
	case CmdExit:
		return i18n.S("退出 PureUtils", "Quit PureUtils")
	default:
		return v.Description
	}
}

// LaunchRequest captures parsed CLI flags, command verbs, and optional paths.
type LaunchRequest struct {
	Command    Command
	ViewPath   string
	Paths      []string
	Background bool
	HasCommand bool
}

// StripWaitPID extracts any `--wait-pid <pid>` or `--wait-pid=<pid>` flag and returns remaining args.
func StripWaitPID(args []string) (uint32, []string) {
	var pid uint32
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if strings.EqualFold(arg, "--wait-pid") {
			if i+1 < len(args) {
				if v, err := strconv.ParseUint(strings.TrimSpace(args[i+1]), 10, 32); err == nil {
					pid = uint32(v)
				}
				i++
			}
			continue
		}
		if len(arg) > len("--wait-pid=") && strings.EqualFold(arg[:len("--wait-pid=")], "--wait-pid=") {
			if v, err := strconv.ParseUint(strings.TrimSpace(arg[len("--wait-pid="):]), 10, 32); err == nil {
				pid = uint32(v)
			}
			continue
		}
		rest = append(rest, args[i])
	}
	return pid, rest
}

// ParseArgs interprets command-line arguments into a structured LaunchRequest.
// Every positional argument is collected into Paths; ViewPath is the first one.
func ParseArgs(args []string) LaunchRequest {
	_, args = StripWaitPID(args)
	var req LaunchRequest

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if strings.EqualFold(arg, "--background") {
			req.Background = true
			continue
		}
		if len(arg) > len("--view=") && strings.EqualFold(arg[:len("--view=")], "--view=") {
			req.Command = CmdOpenViewer
			req.HasCommand = true
			req.Paths = append(req.Paths, strings.TrimSpace(arg[len("--view="):]))
			continue
		}
		if len(arg) > len("--video=") && strings.EqualFold(arg[:len("--video=")], "--video=") {
			req.Command = CmdOpenVideo
			req.HasCommand = true
			req.Paths = append(req.Paths, strings.TrimSpace(arg[len("--video="):]))
			continue
		}
		matched := false
		for _, v := range Verbs {
			if strings.EqualFold(arg, v.Arg) {
				if !req.HasCommand {
					req.Command = v.Command
					req.HasCommand = true
				}
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			req.Paths = append(req.Paths, arg)
		}
	}

	if len(req.Paths) > 0 {
		req.ViewPath = req.Paths[0]
		if !req.HasCommand {
			req.Command = CmdOpenViewer
			req.HasCommand = true
		}
	}
	return req
}

// TakesPaths reports whether cmd consumes the request's positional paths.
func (c Command) TakesPaths() bool {
	switch c {
	case CmdOpenViewer, CmdOpenVideo, CmdOpenZip, CmdExtractSmart, CmdExtractHere, CmdExtractTo,
		CmdExtractDialog, CmdCompress, CmdCompressZip, CmdCompress7z, CmdTestArchive:
		return true
	}
	return false
}

// Match returns the first known command in args.
func Match(args []string) (Command, bool) {
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		for _, v := range Verbs {
			if strings.EqualFold(arg, v.Arg) {
				return v.Command, true
			}
		}
	}
	return "", false
}

// HasHelp reports whether args ask for command-line help.
func HasHelp(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), "--help") ||
			strings.EqualFold(strings.TrimSpace(arg), "-h") {
			return true
		}
	}
	return false
}
