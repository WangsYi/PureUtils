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

	"pureutils/pkg/purekit/i18n"
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

type zipJobOp int

const (
	zipOpExtract zipJobOp = iota
	zipOpCompress
	zipOpTest
	zipOpAdd
	zipOpDelete
	zipOpRename
	zipOpUpdate
)

func parseZipJobOp(verb string) zipJobOp {
	switch strings.ToLower(strings.TrimSpace(verb)) {
	case "compress", "压缩":
		return zipOpCompress
	case "test", "测试":
		return zipOpTest
	case "add", "添加":
		return zipOpAdd
	case "delete", "删除":
		return zipOpDelete
	case "rename", "重命名":
		return zipOpRename
	case "update", "更新":
		return zipOpUpdate
	default:
		return zipOpExtract
	}
}

func zipJobActiveTitle(verb string) string {
	switch parseZipJobOp(verb) {
	case zipOpCompress:
		return i18n.S("正在压缩", "Compressing")
	case zipOpTest:
		return i18n.S("正在测试", "Testing")
	case zipOpAdd:
		return i18n.S("正在添加", "Adding")
	case zipOpDelete:
		return i18n.S("正在删除", "Deleting")
	case zipOpRename:
		return i18n.S("正在重命名", "Renaming")
	case zipOpUpdate:
		return i18n.S("正在更新", "Updating")
	default:
		return i18n.S("正在解压", "Extracting")
	}
}

func zipJobPausedTitle(verb string) string {
	switch parseZipJobOp(verb) {
	case zipOpCompress:
		return i18n.S("已暂停压缩", "Compression paused")
	case zipOpTest:
		return i18n.S("已暂停测试", "Test paused")
	case zipOpAdd:
		return i18n.S("已暂停添加", "Add paused")
	case zipOpDelete:
		return i18n.S("已暂停删除", "Delete paused")
	case zipOpRename:
		return i18n.S("已暂停重命名", "Rename paused")
	case zipOpUpdate:
		return i18n.S("已暂停更新", "Update paused")
	default:
		return i18n.S("已暂停解压", "Extraction paused")
	}
}

func zipJobDoneTitle(verb string) string {
	switch parseZipJobOp(verb) {
	case zipOpCompress:
		return i18n.S("压缩完成", "Compression complete")
	case zipOpTest:
		return i18n.S("测试完成", "Test complete")
	case zipOpAdd:
		return i18n.S("添加完成", "Files added")
	case zipOpDelete:
		return i18n.S("删除完成", "Items deleted")
	case zipOpRename:
		return i18n.S("重命名完成", "Renamed")
	case zipOpUpdate:
		return i18n.S("更新完成", "Update complete")
	default:
		return i18n.S("解压完成", "Extraction complete")
	}
}

func zipJobFailedTitle(verb string) string {
	switch parseZipJobOp(verb) {
	case zipOpCompress:
		return i18n.S("压缩失败", "Compression failed")
	case zipOpTest:
		return i18n.S("测试失败", "Test failed")
	case zipOpAdd:
		return i18n.S("添加失败", "Add failed")
	case zipOpDelete:
		return i18n.S("删除失败", "Delete failed")
	case zipOpRename:
		return i18n.S("重命名失败", "Rename failed")
	case zipOpUpdate:
		return i18n.S("更新失败", "Update failed")
	default:
		return i18n.S("解压失败", "Extraction failed")
	}
}

func zipJobVerbLabel(verb string) string {
	switch parseZipJobOp(verb) {
	case zipOpCompress:
		return i18n.S("压缩", "Compress")
	case zipOpTest:
		return i18n.S("测试", "Test")
	case zipOpAdd:
		return i18n.S("添加", "Add")
	case zipOpDelete:
		return i18n.S("删除", "Delete")
	case zipOpRename:
		return i18n.S("重命名", "Rename")
	case zipOpUpdate:
		return i18n.S("更新", "Update")
	default:
		return i18n.S("解压", "Extract")
	}
}

func zipJobProgressLabel(verb string) string {
	switch parseZipJobOp(verb) {
	case zipOpCompress:
		return i18n.S("压缩进度", "Compression progress")
	case zipOpTest:
		return i18n.S("测试进度", "Test progress")
	case zipOpAdd:
		return i18n.S("添加进度", "Add progress")
	case zipOpDelete:
		return i18n.S("删除进度", "Delete progress")
	case zipOpRename:
		return i18n.S("重命名进度", "Rename progress")
	case zipOpUpdate:
		return i18n.S("更新进度", "Update progress")
	default:
		return i18n.S("解压进度", "Extraction progress")
	}
}

// zipJob is one background archive operation (extract, compress, test, edit)
// with its own small Bandizip-style progress window.
type zipJob struct {
	plugin  *Plugin
	verb    string // canonical or ZH op key (extract / compress / test / ...)
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
	msg := i18n.F("「%s」已加密，请输入密码。", `"%s" is encrypted. Please enter the password.`, name)
	errText := ""
	if wrong {
		errText = i18n.S("密码错误，请重新输入。", "Wrong password, please try again.")
	}
	r := j.ask(&zipPrompt{kind: zipPromptPassword, title: i18n.S("需要密码", "Password Required"), message: msg, errText: errText})
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
	r := j.ask(&zipPrompt{kind: zipPromptConflict, title: i18n.S("文件已存在", "File Already Exists"), message: dest, existing: existing, incoming: incoming})
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
		Title:             zipJobVerbLabel(j.verb) + " - " + j.subject,
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
			a.notify("PureZip "+zipJobDoneTitle(j.verb), summary)
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
			j.resultHeader(c, pal, pureui.IconAlertCircle, pal.Danger, pal.DangerSoft, zipJobFailedTitle(j.verb), j.subject)
			ui.Column(c).Grow(1).Padding(10, 12).Radius(10).Background(pal.DangerSoft).Children(func() {
				msg := i18n.S("未知错误", "Unknown error")
				if err != nil {
					msg = err.Error()
				}
				ui.Text(c, msg).FontSize(12).LineHeight(1.45).TextColor(pal.Text).Selectable()
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.PrimaryButton(c, i18n.S("关闭", "Close")).OnClick(func() { j.closeWindow() })
			})
		case zipJobDone:
			j.resultHeader(c, pal, pureui.IconCheck, pal.Success, pal.SuccessSoft, zipJobDoneTitle(j.verb), j.subject)
			ui.Text(c, summary).FontSize(12).LineHeight(1.45).TextColor(pal.TextSecondary).Grow(1)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if j.target != "" {
					ui.Button(c, i18n.S("打开所在位置", "Open Location")).OnClick(func() {
						mygo.Shell.ShowItemInFolder(j.target)
						j.closeWindow()
					})
				}
				ui.PrimaryButton(c, i18n.S("关闭", "Close")).OnClick(func() { j.closeWindow() })
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
	switch parseZipJobOp(j.verb) {
	case zipOpCompress, zipOpAdd:
		icon = iconCompress
	case zipOpTest:
		icon = iconShieldCheck
	}
	title := zipJobActiveTitle(j.verb)
	if paused {
		title = zipJobPausedTitle(j.verb)
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
			cur = i18n.S("准备中…", "Preparing…")
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cur).FontSize(11.5).SingleLine().Grow(1).MinWidth(0).TextColor(pal.TextSecondary)
			if frac >= 0 {
				ui.Text(c, fmt.Sprintf("%.0f%%", frac*100)).FontSize(12).FontWeight(650).TextColor(pal.Accent).Shrink(0)
			}
		})
		ui.Progress(c, frac).Label(zipJobProgressLabel(j.verb))
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
			line1 += i18n.F(" · %d / %d 个文件", " · %d / %d files", prog.DoneFiles, prog.TotalFiles)
		}
		ui.Text(c, line1).FontSize(11.5).TextColor(pal.TextMuted)
		ui.Text(c, i18n.F("速度 %s/s · 已用 %s · 剩余 %s", "Speed %s/s · Elapsed %s · Remaining %s", zipSize(int64(speed)), zipDuration(elapsed), remain)).FontSize(11.5).TextColor(pal.TextMuted)
	})
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		label := i18n.S("暂停", "Pause")
		if paused {
			label = i18n.S("继续", "Resume")
		}
		ui.Button(c, label).OnClick(j.TogglePause)
		ui.Button(c, i18n.S("取消", "Cancel")).OnClick(j.Cancel)
	})
}

func (j *zipJob) promptView(c *ui.Context, pal pureui.Palette, p *zipPrompt) {
	switch p.kind {
	case zipPromptPassword:
		j.resultHeader(c, pal, iconLock, pal.Warning, pal.WarningSoft, p.title, j.subject)
		ui.Text(c, p.message).FontSize(12).TextColor(pal.TextSecondary)
		submit := func() {
			if strings.TrimSpace(p.password) == "" {
				p.errText = i18n.S("请输入密码。", "Please enter a password.")
				return
			}
			j.answer(zipPromptReply{ok: true, password: p.password})
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &p.password).Grow(1).Placeholder(i18n.S("输入密码", "Enter password")).Label(i18n.S("压缩文件密码", "Archive password")).AutoFocus()
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
			if pureui.IconButton(c, icon, i18n.S("显示密码", "Show Password"), i18n.S("显示/隐藏密码", "Show / hide password"), p.showPw, false, pal).Clicked() {
				p.showPw = !p.showPw
			}
		})
		if p.errText != "" {
			ui.Text(c, p.errText).FontSize(11.5).TextColor(pal.Danger)
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, i18n.S("取消", "Cancel")).OnClick(func() { j.answer(zipPromptReply{}); j.Cancel() })
			ui.PrimaryButton(c, i18n.S("确定", "OK")).OnClick(submit)
		})
	case zipPromptConflict:
		j.resultHeader(c, pal, pureui.IconAlertCircle, pal.Warning, pal.WarningSoft, p.title, p.message)
		ui.Column(c).Gap(4).Padding(8, 12).Radius(10).Background(pal.CardBg).Children(func() {
			if p.existing != nil {
				ui.Text(c, i18n.F("现有文件：%s · %s", "Existing file: %s · %s", zipSize(p.existing.Size()), zipTime(p.existing.ModTime()))).FontSize(11.5).TextColor(pal.TextSecondary)
			}
			ui.Text(c, i18n.F("压缩包内：%s · %s", "In archive: %s · %s", zipSize(p.incoming.Size), zipTime(p.incoming.Modified))).FontSize(11.5).TextColor(pal.TextSecondary)
		})
		ui.Checkbox(c, &p.applyAll, i18n.S("对之后的所有冲突执行相同操作", "Apply to all subsequent conflicts"))
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c, i18n.S("取消", "Cancel")).OnClick(func() { j.answer(zipPromptReply{}) })
			ui.Button(c, i18n.S("跳过", "Skip")).OnClick(func() {
				j.answer(zipPromptReply{ok: true, mode: archive.OverwriteSkip, applyAll: p.applyAll})
			})
			ui.Button(c, i18n.S("自动重命名", "Auto Rename")).OnClick(func() {
				j.answer(zipPromptReply{ok: true, mode: archive.OverwriteRename, applyAll: p.applyAll})
			})
			ui.PrimaryButton(c, i18n.S("覆盖", "Overwrite")).OnClick(func() {
				j.answer(zipPromptReply{ok: true, mode: archive.OverwriteAll, applyAll: p.applyAll})
			})
		})
	}
}
