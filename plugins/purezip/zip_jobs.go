package purezip

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	pureui "pureutils/pkg/purekit/ui"
	"pureutils/plugins/purezip/archive"
)

// zipPromptKind selects which question a running job asks the user.
type zipPromptKind int

const (
	zipPromptPassword zipPromptKind = iota + 1
	zipPromptConflict
)

type zipPrompt struct {
	kind     zipPromptKind
	title    string
	message  string
	password string
	showPw   bool
	errText  string
	applyAll bool
	existing os.FileInfo
	incoming archive.Entry
	reply    chan zipPromptReply
}

type zipPromptReply struct {
	ok       bool
	password string
	mode     archive.OverwriteMode
	applyAll bool
}

// zipJobState is the life cycle of a background archive job.
type zipJobState int

const (
	zipJobRunning zipJobState = iota
	zipJobDone
	zipJobFailed
)

// zipJob is one background archive operation (extract, compress, test, edit)
// with its own small Bandizip-style progress window.
type zipJob struct {
	plugin  *Plugin
	verb    string // 解压 / 压缩 / 测试 / 更新
	subject string // file name shown under the verb
	target  string // folder or file revealed after success

	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	cond        *sync.Cond
	prog        archive.Progress
	started     time.Time
	paused      bool
	pausedAt    time.Time
	pausedTotal time.Duration
	state       zipJobState
	err         error
	summary     string
	prompt      *zipPrompt
	conflictAll archive.OverwriteMode
	hasAll      bool

	keepOpen  bool // stay on the success page instead of auto-closing
	lastPoke  atomic.Int64
	win       *mygo.Window
	shown     bool
	onDone    func(err error)
	revealDir bool
}

func newZipJob(a *Plugin, verb, subject string) *zipJob {
	ctx, cancel := context.WithCancel(context.Background())
	j := &zipJob{plugin: a, verb: verb, subject: subject, ctx: ctx, cancel: cancel, started: time.Now()}
	j.cond = sync.NewCond(&j.mu)
	return j
}

// Cancel stops the job and wakes a paused worker.
func (j *zipJob) Cancel() {
	j.mu.Lock()
	j.cancel()
	j.paused = false
	j.cond.Broadcast()
	if j.prompt != nil && j.prompt.reply != nil {
		select {
		case j.prompt.reply <- zipPromptReply{}:
		default:
		}
	}
	j.mu.Unlock()
	j.poke(true)
}

// TogglePause pauses or resumes the worker at its next progress report.
func (j *zipJob) TogglePause() {
	j.mu.Lock()
	if j.paused {
		j.paused = false
		j.pausedTotal += time.Since(j.pausedAt)
		j.cond.Broadcast()
	} else {
		j.paused = true
		j.pausedAt = time.Now()
	}
	j.mu.Unlock()
	j.poke(true)
}

// report is the engine ProgressFunc; it blocks while the job is paused.
func (j *zipJob) report(p archive.Progress) {
	j.mu.Lock()
	j.prog = p
	for j.paused && j.ctx.Err() == nil {
		j.cond.Wait()
	}
	j.mu.Unlock()
	j.poke(false)
}

func (j *zipJob) snapshot() (archive.Progress, bool, time.Duration, zipJobState, error, string, *zipPrompt) {
	j.mu.Lock()
	defer j.mu.Unlock()
	elapsed := time.Since(j.started) - j.pausedTotal
	if j.paused {
		elapsed -= time.Since(j.pausedAt)
	}
	return j.prog, j.paused, elapsed, j.state, j.err, j.summary, j.prompt
}

// poke repaints the progress window, throttled to ~16 fps unless forced.
func (j *zipJob) poke(force bool) {
	if j.plugin.headless() {
		return
	}
	now := time.Now().UnixMilli()
	if !force && now-j.lastPoke.Load() < 60 {
		return
	}
	j.lastPoke.Store(now)
	mygo.RunOnMain(func() {
		if j.win == nil {
			return
		}
		prog, _, _, state, _, _, _ := j.snapshot()
		if state == zipJobRunning && prog.TotalBytes > 0 {
			j.win.SetProgressBar(mygo.ProgressBar{Value: float64(prog.DoneBytes) / float64(prog.TotalBytes)})
		}
		j.win.Invalidate()
	})
}

// ask shows a prompt inside the progress window and blocks the worker until
// the user answers or the job is canceled.
func (j *zipJob) ask(p *zipPrompt) zipPromptReply {
	if j.plugin.headless() {
		if j.plugin.promptHook != nil {
			return j.plugin.promptHook(p)
		}
		return zipPromptReply{}
	}
	p.reply = make(chan zipPromptReply, 1)
	mygo.RunOnMain(func() {
		j.mu.Lock()
		j.prompt = p
		j.mu.Unlock()
		j.show()
		if j.win != nil {
			j.win.Focus()
			j.win.Invalidate()
		}
	})
	select {
	case r := <-p.reply:
		return r
	case <-j.ctx.Done():
		return zipPromptReply{}
	}
}

func (j *zipJob) answer(r zipPromptReply) {
	j.mu.Lock()
	p := j.prompt
	j.prompt = nil
	j.mu.Unlock()
	if p != nil && p.reply != nil {
		p.reply <- r
	}
	j.poke(true)
}

// askPassword asks for an archive password; ok=false means canceled.
func (j *zipJob) askPassword(name string, wrong bool) (string, bool) {
	msg := "「" + name + "」已加密，请输入密码。"
	errText := ""
	if wrong {
		errText = "密码错误，请重新输入。"
	}
	r := j.ask(&zipPrompt{kind: zipPromptPassword, title: "需要密码", message: msg, errText: errText})
	return r.password, r.ok && r.password != ""
}

// askConflict implements archive.ConflictFunc with an optional "apply to all".
func (j *zipJob) askConflict(dest string, existing os.FileInfo, incoming archive.Entry) archive.OverwriteMode {
	j.mu.Lock()
	if j.hasAll {
		m := j.conflictAll
		j.mu.Unlock()
		return m
	}
	j.mu.Unlock()
	r := j.ask(&zipPrompt{kind: zipPromptConflict, title: "文件已存在", message: dest, existing: existing, incoming: incoming})
	if !r.ok {
		j.Cancel()
		return archive.OverwriteSkip
	}
	if r.applyAll {
		j.mu.Lock()
		j.hasAll = true
		j.conflictAll = r.mode
		j.mu.Unlock()
	}
	return r.mode
}

// zipWithPassword runs fn, prompting for a password while fn reports
// ErrPasswordRequired / ErrWrongPassword.
func (j *zipJob) withPassword(name string, pw string, fn func(pw string) error) (string, error) {
	for {
		err := fn(pw)
		if err == nil {
			return pw, nil
		}
		wrong := errors.Is(err, archive.ErrWrongPassword)
		if !wrong && !errors.Is(err, archive.ErrPasswordRequired) {
			return pw, err
		}
		next, ok := j.askPassword(name, wrong || pw != "")
		if !ok {
			return pw, archive.ErrCanceled
		}
		pw = next
	}
}

// show creates (if needed) and shows the progress window.
func (j *zipJob) show() {
	if j.plugin.headless() || j.shown {
		return
	}
	j.shown = true
	if j.win == nil {
		j.createWindow()
	}
	j.win.Show()
}

func (j *zipJob) createWindow() {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             j.verb + " - " + j.subject,
		Width:             480,
		Height:            268,
		Hidden:            true,
		DisableResize:     true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Content:           ui.View(j.view),
	})
	j.plugin.applyZipWindowIdentity(win)
	win.OnClose(func(_ *mygo.CloseEvent) {
		_, _, _, state, _, _, _ := j.snapshot()
		if state == zipJobRunning {
			j.Cancel()
		}
		j.plugin.removeZipJob(j)
		j.win = nil
		j.plugin.notifyWindowClosed()
	})
	j.win = win
}

// start runs work on a goroutine (synchronously when headless). The window
// appears after a short delay so instant jobs never flash.
func (a *Plugin) startZipJob(j *zipJob, work func(j *zipJob) (string, error)) {
	a.jobsMu.Lock()
	a.jobs = append(a.jobs, j)
	a.jobsMu.Unlock()
	run := func() {
		summary, err := work(j)
		if err == nil && j.ctx.Err() != nil {
			err = archive.ErrCanceled
		}
		finish := func() { j.finish(summary, err) }
		if a.headless() {
			finish()
		} else {
			mygo.RunOnMain(finish)
		}
	}
	if a.headless() {
		run()
		return
	}
	j.createWindow()
	time.AfterFunc(280*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			_, _, _, state, _, _, _ := j.snapshot()
			if state == zipJobRunning {
				j.show()
			}
		})
	})
	go run()
}

func (j *zipJob) finish(summary string, err error) {
	j.mu.Lock()
	j.summary = summary
	j.err = err
	if err != nil && !errors.Is(err, archive.ErrCanceled) {
		j.state = zipJobFailed
	} else {
		j.state = zipJobDone
	}
	state := j.state
	j.mu.Unlock()

	a := j.plugin
	if a.log() != nil {
		if err != nil {
			a.log().Warnf("PureZip %s %s: %v", j.verb, j.subject, err)
		} else {
			a.log().Infof("PureZip %s %s: %s", j.verb, j.subject, summary)
		}
	}
	if j.onDone != nil {
		j.onDone(err)
	}
	if a.headless() {
		a.removeZipJob(j)
		return
	}
	if j.win != nil {
		j.win.SetProgressBar(mygo.ProgressBar{})
	}
	switch {
	case errors.Is(err, archive.ErrCanceled):
		j.closeWindow()
	case state == zipJobFailed:
		j.show()
		if j.win != nil {
			j.win.SetProgressBar(mygo.ProgressBar{State: mygo.ProgressError, Value: 1})
			j.win.Focus()
			j.win.Invalidate()
		}
	case j.keepOpen:
		j.show()
		if j.win != nil {
			j.win.Invalidate()
		}
	default:
		j.closeWindow()
		if summary != "" && !j.shown {
			// Quick jobs finish before the window shows; confirm with a toast-like notification.
			a.notify("PureZip "+j.verb+"完成", summary)
		}
	}
}

func (j *zipJob) closeWindow() {
	if j.win != nil {
		w := j.win
		j.win = nil
		w.Close()
	}
	j.plugin.removeZipJob(j)
}

func (a *Plugin) removeZipJob(j *zipJob) {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	for i, x := range a.jobs {
		if x == j {
			a.jobs = append(a.jobs[:i], a.jobs[i+1:]...)
			return
		}
	}
}

// view renders the progress / prompt / result page of a job window.
func (j *zipJob) view(c *ui.Context) {
	pal := pureui.ApplyTheme(c)
	prog, paused, elapsed, state, err, summary, prompt := j.snapshot()

	ui.Column(c).Fill().Background(pal.Bg).Padding(18, 20, 16, 20).Gap(12).Children(func() {
		if prompt != nil {
			j.promptView(c, pal, prompt)
			return
		}
		switch state {
		case zipJobFailed:
			j.resultHeader(c, pal, pureui.IconAlertCircle, pal.Danger, pal.DangerSoft, j.verb+"失败", j.subject)
			ui.Column(c).Grow(1).Padding(10, 12).Radius(10).Background(pal.DangerSoft).Children(func() {
				msg := "未知错误"
				if err != nil {
					msg = err.Error()
				}
				ui.Text(c, msg).FontSize(12).LineHeight(1.45).TextColor(pal.Text).Selectable()
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.PrimaryButton(c, "关闭").OnClick(func() { j.closeWindow() })
			})
		case zipJobDone:
			j.resultHeader(c, pal, pureui.IconCheck, pal.Success, pal.SuccessSoft, j.verb+"完成", j.subject)
			ui.Text(c, summary).FontSize(12).LineHeight(1.45).TextColor(pal.TextSecondary).Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if j.target != "" {
					ui.Button(c, "打开所在位置").OnClick(func() {
						mygo.Shell.ShowItemInFolder(j.target)
						j.closeWindow()
					})
				}
				ui.PrimaryButton(c, "关闭").OnClick(func() { j.closeWindow() })
			})
		default:
			j.progressView(c, pal, prog, paused, elapsed)
		}
	})
	if c.Shortcut(0, ui.KeyEscape) && prompt == nil {
		if state == zipJobRunning {
			j.Cancel()
		} else {
			j.closeWindow()
		}
	}
}

func (j *zipJob) resultHeader(c *ui.Context, pal pureui.Palette, icon *ui.SVG, fg, bg ui.Color, title, sub string) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(40, 40).Radius(11).Background(bg).Center().Shrink(0).Children(func() {
			ui.Icon(c, icon).Size(20, 20).TextColor(fg)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(650).TextColor(pal.Text)
			ui.Text(c, sub).FontSize(11.5).SingleLine().TextColor(pal.TextMuted)
		})
	})
}

func (j *zipJob) progressView(c *ui.Context, pal pureui.Palette, prog archive.Progress, paused bool, elapsed time.Duration) {
	icon := iconExtract
	if j.verb == "压缩" || j.verb == "添加" {
		icon = iconCompress
	} else if j.verb == "测试" {
		icon = iconShieldCheck
	}
	title := "正在" + j.verb
	if paused {
		title = "已暂停" + j.verb
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(40, 40).Radius(11).Gradient(pal.Accent, pal.AccentEnd, 135).Center().Shrink(0).
			InsetShadow(0, 1, 0, 0, ui.RGBA(255, 255, 255, 0.25)).Children(func() {
			ui.Icon(c, icon).Size(20, 20).TextColor(pal.AccentText)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title+"…").FontSize(15).FontWeight(650).TextColor(pal.Text)
			ui.Text(c, j.subject).FontSize(11.5).SingleLine().TextColor(pal.TextMuted)
		})
	})

	frac := -1.0
	if prog.TotalBytes > 0 {
		frac = float64(prog.DoneBytes) / float64(prog.TotalBytes)
		if frac > 1 {
			frac = 1
		}
	} else if prog.TotalFiles > 0 {
		frac = float64(prog.DoneFiles) / float64(prog.TotalFiles)
	}
	ui.Column(c).Gap(6).Children(func() {
		cur := prog.Current
		if cur == "" {
			cur = "准备中…"
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cur).FontSize(11.5).SingleLine().Grow(1).MinWidth(0).TextColor(pal.TextSecondary)
			if frac >= 0 {
				ui.Text(c, fmt.Sprintf("%.0f%%", frac*100)).FontSize(12).FontWeight(650).TextColor(pal.Accent).Shrink(0)
			}
		})
		ui.Progress(c, frac).Label(j.verb + "进度")
	})

	speed := 0.0
	if secs := elapsed.Seconds(); secs > 0.2 {
		speed = float64(prog.DoneBytes) / secs
	}
	remain := "—"
	if speed > 0 && prog.TotalBytes > prog.DoneBytes {
		remain = zipDuration(time.Duration(float64(prog.TotalBytes-prog.DoneBytes) / speed * float64(time.Second)))
	}
	ui.Column(c).Gap(3).Grow(1).Children(func() {
		line1 := zipSize(prog.DoneBytes)
		if prog.TotalBytes > 0 {
			line1 += " / " + zipSize(prog.TotalBytes)
		}
		if prog.TotalFiles > 0 {
			line1 += fmt.Sprintf(" · %d / %d 个文件", prog.DoneFiles, prog.TotalFiles)
		}
		ui.Text(c, line1).FontSize(11.5).TextColor(pal.TextMuted)
		ui.Text(c, fmt.Sprintf("速度 %s/s · 已用 %s · 剩余 %s", zipSize(int64(speed)), zipDuration(elapsed), remain)).FontSize(11.5).TextColor(pal.TextMuted)
	})
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		label := "暂停"
		if paused {
			label = "继续"
		}
		ui.Button(c, label).OnClick(j.TogglePause)
		ui.Button(c, "取消").OnClick(j.Cancel)
	})
}

func (j *zipJob) promptView(c *ui.Context, pal pureui.Palette, p *zipPrompt) {
	switch p.kind {
	case zipPromptPassword:
		j.resultHeader(c, pal, iconLock, pal.Warning, pal.WarningSoft, p.title, j.subject)
		ui.Text(c, p.message).FontSize(12).TextColor(pal.TextSecondary)
		submit := func() {
			if strings.TrimSpace(p.password) == "" {
				p.errText = "请输入密码。"
				return
			}
			j.answer(zipPromptReply{ok: true, password: p.password})
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &p.password).Grow(1).Placeholder("输入密码").Label("压缩文件密码").AutoFocus()
			if !p.showPw {
				in.Password()
			}
			if in.Submitted() {
				submit()
			}
			icon := pureui.IconEye
			if p.showPw {
				icon = iconEyeOff
			}
			if pureui.IconButton(c, icon, "显示密码", "显示/隐藏密码", p.showPw, false, pal).Clicked() {
				p.showPw = !p.showPw
			}
		})
		if p.errText != "" {
			ui.Text(c, p.errText).FontSize(11.5).TextColor(pal.Danger)
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, "取消").OnClick(func() { j.answer(zipPromptReply{}) ; j.Cancel() })
			ui.PrimaryButton(c, "确定").OnClick(submit)
		})
	case zipPromptConflict:
		j.resultHeader(c, pal, pureui.IconAlertCircle, pal.Warning, pal.WarningSoft, p.title, p.message)
		ui.Column(c).Gap(4).Padding(8, 12).Radius(10).Background(pal.CardBg).Children(func() {
			if p.existing != nil {
				ui.Text(c, fmt.Sprintf("现有文件：%s · %s", zipSize(p.existing.Size()), zipTime(p.existing.ModTime()))).FontSize(11.5).TextColor(pal.TextSecondary)
			}
			ui.Text(c, fmt.Sprintf("压缩包内：%s · %s", zipSize(p.incoming.Size), zipTime(p.incoming.Modified))).FontSize(11.5).TextColor(pal.TextSecondary)
		})
		ui.Checkbox(c, &p.applyAll, "对之后的所有冲突执行相同操作")
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, "取消").OnClick(func() { j.answer(zipPromptReply{}) })
			ui.Button(c, "跳过").OnClick(func() {
				j.answer(zipPromptReply{ok: true, mode: archive.OverwriteSkip, applyAll: p.applyAll})
			})
			ui.Button(c, "自动重命名").OnClick(func() {
				j.answer(zipPromptReply{ok: true, mode: archive.OverwriteRename, applyAll: p.applyAll})
			})
			ui.PrimaryButton(c, "覆盖").OnClick(func() {
				j.answer(zipPromptReply{ok: true, mode: archive.OverwriteAll, applyAll: p.applyAll})
			})
		})
	}
}
