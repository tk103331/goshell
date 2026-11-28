package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// resumeDialog draws the programs that can be brought back: the commands
// they asked to be resumed with, and a button for each. The commands are
// shown as they are, so that the user sees what the app would run.
func (w *Window) resumeDialog(c *ui.Context) {
	if !w.resumeOpen {
		return
	}
	lk := lookFor(c)
	records := w.resumeRecords
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(520).Padding(20).Gap(12).Children(func() {
			dialogHeading(c, locales.ResumeTitle())
			ui.Text(c, locales.ResumeHint()).FontSize(12).TextColor(lk.textMuted).LineHeight(1.35)
			if len(records) == 0 {
				ui.Text(c, locales.ResumeNone()).FontSize(12.5).TextColor(lk.textMuted)
			} else {
				ui.Scroll(c).MaxHeight(320).Children(func() {
					ui.Column(c).FillWidth().Gap(6).Children(func() {
						for _, rec := range records {
							w.resumeRow(c, lk, rec)
						}
					})
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.BtnCancel()).Clicked() {
					open = false
				}
				if len(records) > 0 {
					if ui.PrimaryButton(c, locales.ResumeAll()).Clicked() {
						w.resumeAll(records)
						open = false
					}
				}
			})
		})
	})
	if !open {
		w.resumeOpen = false
	}
}

// resumeRow draws one program that can be brought back: the command it
// asked to be resumed with, and the directory to run it in.
func (w *Window) resumeRow(c *ui.Context, lk *look, rec *resumeRecord) {
	ui.Row(c).FillWidth().Padding(9, 10).Gap(10).Radius(8).Background(lk.chip).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
			ui.Text(c, resumeCommandText(rec)).FontSize(12.5).TextColor(lk.text).SingleLine().Ellipsis("…")
			if rec.Cwd != "" {
				ui.Text(c, rec.Cwd).FontSize(11).TextColor(lk.textMuted).SingleLine().Ellipsis("…")
			}
		})
		if ui.Button(c, locales.ResumeBtn()).Clicked() {
			w.resumeProgram(rec)
		}
	})
}

// resumeCommandText is the command of a record, as the user reads it: the
// program and its arguments as they are, with the ones that hold spaces
// quoted. The characters that may mislead are taken out, as the protocol
// asks of a terminal that shows a command.
func resumeCommandText(rec *resumeRecord) string {
	if rec == nil {
		return ""
	}
	parts := make([]string, 0, len(rec.Argv))
	for i, arg := range rec.Argv {
		text := sanitizeText(arg)
		if i > 0 && strings.ContainsAny(text, " \t") {
			text = "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
}

// openResume shows the programs that can be brought back.
func (w *Window) openResume() {
	if w.resume == nil {
		return
	}
	w.resumeRecords = w.resume.resumable()
	w.resumeOpen = true
}

// checkResume asks whether programs that can be brought back are waiting
// for the user.
func (w *Window) checkResume() {
	if w.resume == nil {
		return
	}
	if len(w.resume.resumable()) == 0 {
		return
	}
	w.onMain(func() { w.openResume() })
}

// resumeAll brings every program back that can be.
func (w *Window) resumeAll(records []*resumeRecord) {
	// the list changes as each program comes back
	for _, rec := range append([]*resumeRecord(nil), records...) {
		w.resumeProgram(rec)
	}
}

// resumeProgram brings a program back: it opens a tab with the command the
// program asked to be resumed with, in the directory it asked for.
func (w *Window) resumeProgram(rec *resumeRecord) {
	if rec == nil || len(rec.Argv) == 0 {
		return
	}
	// The directory the program asked for: if it is gone, the program is
	// not brought back, as the protocol asks.
	dir := rec.Cwd
	if dir != "" {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			w.showToast(locales.ResumeNoDir(dir), "error")
			return
		}
	}
	// The program is looked up on PATH, in the environment the command is
	// started in.
	if !resumeProgramFound(rec.Argv[0]) {
		w.showToast(locales.ResumeNotFound(rec.Argv[0]), "error")
		return
	}
	tab, _, err := w.newTab(rec.Argv[0], nil, nil, terminal.Options{Command: rec.Argv, Dir: dir})
	if err != nil {
		w.showError(err)
		return
	}
	// The record belongs to the program the terminal starts now: it
	// reports again when it wants to be brought back once more.
	w.resume.remove(rec.PID)
	w.forgetResumeRecord(rec.PID)
	w.onMain(func() {
		w.addTermTab(tab)
	})
}

// forgetResumeRecord drops a record from the list the dialog shows. The list
// is built anew, as the dialog may be reading the one it was given.
func (w *Window) forgetResumeRecord(pid int) {
	kept := make([]*resumeRecord, 0, len(w.resumeRecords))
	for _, rec := range w.resumeRecords {
		if rec.PID != pid {
			kept = append(kept, rec)
		}
	}
	w.resumeRecords = kept
}

// resumeProgramFound says whether the program of a command is there. The
// app's own PATH may be short, as when the app was started from the
// Finder, so the user's login shell is asked for its PATH: the protocol
// says a bare name is looked up in the PATH of the environment the command
// is started in.
func resumeProgramFound(name string) bool {
	if filepath.IsAbs(name) {
		st, err := os.Stat(name)
		return err == nil && !st.IsDir()
	}
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	path := userShellPath()
	if path == "" {
		return false
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		st, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !st.IsDir() {
			// the app's own PATH is short: the program is looked up in
			// the PATH the command is started in
			os.Setenv("PATH", path)
			return true
		}
	}
	return false
}

// userShellPath is the PATH the user's login shell sets, which the app's
// own PATH may not have when the app was started from the Finder.
func userShellPath() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return ""
	}
	out, err := exec.Command(shell, "-l", "-c", "env").Output()
	if err != nil {
		return ""
	}
	// the startup files may have printed something: the PATH is the last
	// line that names one
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if value, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), "PATH="); ok {
			return value
		}
	}
	return ""
}
