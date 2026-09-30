package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var keyR = tea.KeyPressMsg{Code: 'r', Text: "r"}

// work runs a session on the Task from start, for the given time, then stops
// it (or lets it complete at 30m), leaving the clock where it ended.
func (r *rig) work(t *testing.T, task core.TaskID, start time.Time, spent time.Duration, opts core.StartOptions) {
	t.Helper()
	r.clock.Set(start)
	if _, err := r.tracker.StartSession(ctx, task, 30*time.Minute, opts); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(spent)
	if spent < 30*time.Minute {
		if _, err := r.tracker.StopSession(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestReport_ROpensItAndEscOrRGoesBack(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "write the PRD ##docs")
	r.work(t, task.ID, epoch.Add(time.Hour), 25*time.Minute, core.StartOptions{})
	r.clock.Set(epoch.Add(5 * time.Hour))
	m := booted(r.newModel())
	m = press(m, keyEsc) // decline the resume prompt

	m = press(m, keyR)
	wantScreen(t, "report", m,
		"Report: Today", "tab: This week",
		"Focused: 25m over 1 session", "Break overrides: 0",
		"By task", "25m", "write the PRD",
		"By tag", "#docs",
		"esc back")
	for _, back := range []tea.KeyPressMsg{keyEsc, keyR} {
		m = press(m, back)
		wantNoText(t, "back", m, "Report:")
		m = press(m, keyR)
		wantScreen(t, "reopened", m, "Report: Today")
	}
}

func TestReport_TabSwitchesBetweenTodayAndThisWeek(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "weekly")
	// The rig's "today" is Tuesday 29 Sep 2026 (UTC); Monday is in the same week.
	r.work(t, task.ID, epoch.Add(-24*time.Hour), 30*time.Minute, core.StartOptions{}) // Monday
	r.work(t, task.ID, epoch.Add(time.Hour), 10*time.Minute, core.StartOptions{})     // today
	r.clock.Set(epoch.Add(5 * time.Hour))
	m := press(booted(r.newModel()), keyEsc, keyR)

	wantScreen(t, "today", m, "Report: Today", "Focused: 10m over 1 session", "tab: This week")
	m = press(m, keyTab)
	wantScreen(t, "week", m, "Report: This week", "Focused: 40m over 2 sessions", "tab: Today")
	m = press(m, keyTab)
	wantScreen(t, "today again", m, "Report: Today")
}

func TestReport_SaysSoWhenThereIsNoFocusedTime(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "idle")
	m := press(booted(r.newModel()), keyR)
	wantScreen(t, "today", m, "Report: Today", "No focused time today.")
	m = press(m, keyTab)
	wantScreen(t, "week", m, "No focused time this week.")
}

func TestReport_CountsBreakOverridesAndMarksFinishedTasks(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "rushed")
	r.work(t, task.ID, epoch.Add(time.Hour), 30*time.Minute, core.StartOptions{})
	// Start again 4 minutes into the Break, overriding it.
	r.clock.Advance(4 * time.Minute)
	if _, err := r.tracker.StartSession(ctx, task.ID, 30*time.Minute, core.StartOptions{OverrideBreak: true}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(10 * time.Minute)
	if _, err := r.tracker.StopSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tracker.MarkDone(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	m := press(booted(r.newModel()), keyEsc, keyR)
	wantScreen(t, "overrides", m, "Break overrides: 1", "Focused: 40m over 2 sessions", "rushed (done)")
}

func TestReport_ShowsAnUntaggedRowOnlyWhenNeeded(t *testing.T) {
	r := newRig(t)
	tagged := r.addTask(t, "tagged ##docs")
	bare := r.addTask(t, "bare")
	r.work(t, tagged.ID, epoch.Add(time.Hour), 30*time.Minute, core.StartOptions{})
	r.clock.Advance(15 * time.Minute)
	r.work(t, bare.ID, r.clock.Now(), 10*time.Minute, core.StartOptions{})
	m := press(booted(r.newModel()), keyEsc, keyR)
	wantScreen(t, "both", m, "#docs", "(untagged)")

	r2 := newRig(t)
	only := r2.addTask(t, "only ##docs")
	r2.work(t, only.ID, epoch.Add(time.Hour), 10*time.Minute, core.StartOptions{})
	m2 := press(booted(r2.newModel()), keyEsc, keyR)
	wantNoText(t, "all tagged", m2, "(untagged)")
}

func TestReport_CountsUpWhileASessionRuns(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := press(booted(r.newModel()), keyR)
	wantScreen(t, "start", m, "No focused time today.")

	r.clock.Advance(7 * time.Minute)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after 7m", m, "Focused: 7m over 1 session")
	r.clock.Advance(5 * time.Minute)
	m = send(m, tui.TickMsg{})
	wantScreen(t, "after 12m", m, "Focused: 12m over 1 session")
}

func TestReport_ALongReportScrollsAndNeverOutgrowsTheTerminal(t *testing.T) {
	r := newRig(t)
	for i := range 15 {
		task := r.addTask(t, "task "+string(rune('a'+i)))
		r.work(t, task.ID, epoch.Add(time.Duration(i)*time.Hour), time.Duration(i+1)*time.Minute, core.StartOptions{})
	}
	r.clock.Set(epoch.Add(14*time.Hour + 30*time.Minute)) // 23:30, still the same day
	const height = 14
	m := booted(r.newModel())
	m = send(m, tea.WindowSizeMsg{Width: 70, Height: height})
	m = press(m, keyEsc, keyR)
	wantScreen(t, "top", m, "Focused:", "By task", "task o") // the longest, 15m
	wantNoText(t, "top", m, "By tag")

	for range 60 {
		m = press(m, keyJ)
		if n := len(strings.Split(screen(m), "\n")); n > height {
			t.Fatalf("%d lines on a %d-line terminal:\n%s", n, height, screen(m))
		}
	}
	wantScreen(t, "end", m, "Focused:", "By tag", "(untagged)")
	for range 60 {
		m = press(m, keyK)
	}
	wantScreen(t, "top again", m, "task o")
}

func TestReport_FootersFitTheTerminal(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "a task with a rather long title to push past narrow terminals ##tag")
	r.work(t, task.ID, epoch.Add(time.Hour), 10*time.Minute, core.StartOptions{})
	for width := 12; width <= 60; width += 4 {
		m := booted(r.newModel())
		m = send(m, tea.WindowSizeMsg{Width: width, Height: 14})
		view := press(m, keyEsc, keyR)
		for i, line := range strings.Split(view.View().Content, "\n") {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("width %d: line %d is %d cells: %q", width, i, w, line)
			}
		}
	}
}
