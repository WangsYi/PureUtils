package core

import "strings"

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
	CmdOpenZip               Command = "open-zip"
	CmdExtractSmart          Command = "extract-smart"
	CmdExtractHere           Command = "extract-here"
	CmdExtractTo             Command = "extract-to"
	CmdExtractDialog         Command = "extract-dialog"
	CmdCompress              Command = "compress"
	CmdCompressZip           Command = "compress-zip"
	CmdCompress7z            Command = "compress-7z"
	CmdTestArchive           Command = "test-archive"
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
	{"--capture", CmdCaptureRegion, "进入区域截图遮罩"},
	{"--capture-monitor", CmdCaptureMonitor, "截取光标所在显示器整屏并复制"},
	{"--pin", CmdPinClipboard, "把剪贴板图片贴成悬浮贴图"},
	{"--history", CmdShowHistory, "打开剪贴板历史面板"},
	{"--toggle-pins", CmdTogglePins, "隐藏/显示所有贴图"},
	{"--launcher", CmdOpenLauncher, "打开快捷启动器"},
	{"--settings", CmdShowSettings, "打开设置窗口"},
	{"--toggle-monitoring", CmdToggleMonitoring, "暂停/恢复剪贴板监听"},
	{"--view", CmdOpenViewer, "打开 PureImage 看图器（可后接图片或目录路径）"},
	{"--toggle-space-preview", CmdToggleSpacePreview, "开启/关闭资源管理器空格预览"},
	{"--zip", CmdOpenZip, "打开 PureZip 压缩管理器（可后接压缩文件路径）"},
	{"--extract-smart", CmdExtractSmart, "智能解压：单一根目录直接解压，否则解压到同名文件夹"},
	{"--extract-here", CmdExtractHere, "解压到压缩文件所在目录"},
	{"--extract-to", CmdExtractTo, "解压到以压缩文件命名的文件夹"},
	{"--extract-dialog", CmdExtractDialog, "选择目标位置与选项后解压"},
	{"--compress", CmdCompress, "打开新建压缩文件对话框（后接文件或文件夹）"},
	{"--compress-zip", CmdCompressZip, "一键压缩为同名 .zip"},
	{"--compress-7z", CmdCompress7z, "一键压缩为同名 .7z"},
	{"--test-archive", CmdTestArchive, "测试压缩文件完整性"},
	{"--perf", CmdPerfReport, "把性能打点统计写进日志"},
	{"--exit", CmdExit, "退出 PureUtils"},
}

// LaunchRequest captures parsed CLI flags, command verbs, and optional paths.
type LaunchRequest struct {
	Command    Command
	ViewPath   string
	Paths      []string
	Background bool
	HasCommand bool
}

// ParseArgs interprets command-line arguments into a structured LaunchRequest.
// Every positional argument is collected into Paths; ViewPath is the first one.
func ParseArgs(args []string) LaunchRequest {
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
	case CmdOpenViewer, CmdOpenZip, CmdExtractSmart, CmdExtractHere, CmdExtractTo,
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
