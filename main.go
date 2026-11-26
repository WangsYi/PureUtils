package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"pureutils/internal/host"
	"pureutils/pkg/purekit/config"
	"pureutils/pkg/purekit/core"
)

func main() {
	args := os.Args[1:]
	if core.HasHelp(args) {
		fmt.Println("PureUtils — PureShot 截图贴图 / PureClip 剪贴板历史 / PureImage 看图 / PureVideo 视频 / PureZip 压缩管理")
		fmt.Println("用法: PureUtils [命令 | 图片或视频路径 | 压缩文件路径 | 目录路径]")
		fmt.Println("命令:")
		for _, v := range core.Verbs {
			fmt.Printf("  %-22s %s\n", v.Arg, v.Description)
		}
		fmt.Printf("  %-22s %s\n", "--background", "仅在后台托盘驻留，不主动打开窗口")
		return
	}

	paths := core.ResolvePaths()
	if err := paths.Ensure(); err != nil {
		fmt.Fprintln(os.Stderr, "无法创建数据目录:", err)
		os.Exit(1)
	}
	_, crashErr := core.SetupCrashOutput(paths.Logs)
	cfgMgr := config.Load(paths.Settings)
	hc := cfgMgr.Host()
	logger, err := core.OpenLogger(paths.Logs, hc.Advanced.VerboseLogging)
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法打开日志:", err)
		os.Exit(1)
	}
	if crashErr != nil {
		logger.Warnf("初始化崩溃日志失败: %v", crashErr)
	}

	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	modules := core.LoadModuleConfig(exeDir, host.ViewerCompiled())

	mygo.App.SetName("PureUtils")
	mygo.App.SetVersion(core.AppVersion)
	mygo.Theme.SetSource(mygo.ThemeSource(hc.ThemeSource()))

	if !mygo.App.RequestSingleInstanceLock() {
		logger.Infof("已有实例在运行，命令已转交")
		logger.Close()
		return
	}

	a, err := host.NewAppWithModules(paths, cfgMgr, modules, logger)
	if err != nil {
		logger.Errorf("启动失败: %v", err)
		logger.Close()
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}

	mygo.App.OnSecondInstance(func(secondArgs []string, _ string) {
		a.HandleArgs(secondArgs)
	})
	mygo.App.OnWindowAllClosed(func() {
		if !a.ShouldStayInTray() {
			mygo.App.Quit()
		}
	})
	mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) {
		a.Shutdown()
	})
	mygo.App.WhenReady(func() {
		a.Start(args)
	})

	if err := mygo.App.Run(); err != nil {
		logger.Errorf("事件循环退出: %v", err)
		log.Fatal(err)
	}
}
