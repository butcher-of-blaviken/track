package tui_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/export"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

var (
	keyPrevDay = tea.KeyPressMsg{Code: '[', Text: "["}
	keyNextDay = tea.KeyPressMsg{Code: ']', Text: "]"}
)

// monday is the day before the rig's Tuesday: a 25 minute session on "write the
// PRD", a note on it, a note on another task, an unfiled note, and a task
// finished that afternoon. The clock is left at 5am on Tuesday.
func (r *rig) monday(t *testing.T) (prd, bank core.Task) {
	t.Helper()
	prd, bank = r.addTask(t, "write the PRD ##docs"), r.addTask(t, "call the bank")
	done := r.addTask(t, "release it")
	r.work(t, prd.ID, epoch.Add(-23*time.Hour), 25*time.Minute, core.StartOptions{}) // Monday 10:00
	note := func(at time.Duration, task core.TaskID, text string) {
		t.Helper()
		r.clock.Set(epoch.Add(at))
		var err error
		if task == 0 {
			_, err = r.tracker.AddUnfiledNote(ctx, text)
		} else {
			_, err = r.tracker.AddNote(ctx, task, text)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	note(-22*time.Hour, prd.ID, "left off at section 2")
	note(-21*time.Hour, bank.ID, "they close at 5")
	note(-20*time.Hour, 0, "a stray idea")
	r.clock.Set(epoch.Add(-17 * time.Hour)) // Monday 16:00
	if _, err := r.tracker.MarkDone(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	r.clock.Set(epoch.Add(5 * time.Hour))
	return prd, bank
}

func reportOpen(r *rig) tea.Model { return press(booted(r.newModel()), keyEsc, keyR) }

func TestReportDays_BracketsStepBackAndForthFromToday(t *testing.T) {
	r := newRig(t)
	r.monday(t)
	r.work(t, 1, epoch.Add(time.Hour), 10*time.Minute, core.StartOptions{}) // today
	r.clock.Set(epoch.Add(5 * time.Hour))
	m := reportOpen(r)
	wantScreen(t, "today", m, "Report: Today", "Focused: 10m over 1 session")

	m = press(m, keyPrevDay)
	wantScreen(t, "yesterday", m, "Report: Mon 28 Sep", "Focused: 25m over 1 session", "tab: This week")
	wantNoText(t, "today's report", m, "Report: Today")

	m = press(m, keyNextDay)
	wantScreen(t, "today again", m, "Report: Today", "Focused: 10m over 1 session")
	m = press(m, keyNextDay)
	wantScreen(t, "no further than today", m, "Report: Today", "That is today.")
	m = press(m, keyPrevDay, keyPrevDay)
	wantScreen(t, "two days back", m, "Report: Sun 27 Sep", "No focused time on Sun 27 Sep.")
}

func TestReportDays_AStepBackSurvivesTheNextRefreshAndAnOldOneIsIgnored(t *testing.T) {
	r := newRig(t)
	r.monday(t)
	m := reportOpen(r)

	m = press(m, keyPrevDay)
	m = send(m, tui.TickMsg(epoch.Add(5*time.Hour)))
	wantScreen(t, "still yesterday after a tick", m, "Report: Mon 28 Sep", "Focused: 25m over 1 session")

	// A read started for yesterday that comes back after returning to today
	// must not put yesterday on screen.
	next, slow := m.Update(keyNextDay)
	m = next
	stale := collect(slow) // this read is for today
	prev, old := m.Update(keyPrevDay)
	m = prev
	late := collect(old) // the read for yesterday, held back
	next, now := m.Update(keyNextDay)
	m = next
	for _, msg := range collect(now) {
		m = send(m, msg)
	}
	for _, msg := range append(stale, late...) {
		m = send(m, msg)
	}
	wantScreen(t, "today", m, "Report: Today", "No focused time today.")
	wantNoText(t, "yesterday's work", m, "write the PRD")
}

func TestReportDays_AnOldReadOfTheOtherPeriodIsIgnored(t *testing.T) {
	r := newRig(t)
	r.monday(t)
	m := reportOpen(r)

	next, held := m.Update(keyTab) // the week's read, held back
	week := collect(held)
	m = next
	m = press(m, keyTab) // back to today, whose read is delivered
	for _, msg := range week {
		m = send(m, msg)
	}
	wantScreen(t, "today", m, "Report: Today", "No focused time today.")
	wantNoText(t, "the week", m, "This week:")
}

func TestReportDays_AnotherDayShowsItsNotesAndWhatWasFinished(t *testing.T) {
	r := newRig(t)
	r.monday(t)
	m := press(reportOpen(r), keyPrevDay)
	wantScreen(t, "sections", m,
		"Finished", "release it",
		"By task", "25m", "write the PRD", "left off at section 2",
		"call the bank", "they close at 5",
		"Notes", "a stray idea",
		"By tag", "#docs")
	wantNoText(t, "tomorrow's items", m, "No focused time")
}

func TestReportDays_ADayWithOnlyNotesIsNotEmpty(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "thinking")
	if _, err := r.tracker.AddNote(ctx, task.ID, "just an idea"); err != nil {
		t.Fatal(err)
	}
	m := reportOpen(r)
	wantScreen(t, "today", m, "Report: Today", "just an idea", "thinking")
	wantNoText(t, "empty message", m, "No focused time today.")
	wantNoText(t, "an empty tag table", m, "By tag")
}

func TestReportDays_TheTodayViewOfAQuietDayIsAsBefore(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "idle")
	wantScreen(t, "quiet", reportOpen(r), "Report: Today", "No focused time today.")
}

func TestReportDays_YCopiesTheMarkdownOfTheDayShown(t *testing.T) {
	r := newRig(t)
	r.monday(t)
	m := press(reportOpen(r), keyPrevDay)

	day, err := r.tracker.Day(ctx, epoch.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := export.ReportMarkdown(&want, []core.Report{day}); err != nil {
		t.Fatal(err)
	}

	next, cmd := m.Update(keyY)
	var copied []string
	for _, msg := range collect(cmd) {
		// tea.SetClipboard's message is unexported; it prints as its text.
		copied = append(copied, fmt.Sprint(msg))
	}
	if len(copied) != 1 || copied[0] != want.String() {
		t.Fatalf("clipboard = %q, want %q", copied, want.String())
	}
	for _, part := range []string{"## Mon 28 Sep: 25m focused", "### Finished\n- release it", "  - left off at section 2", "### Notes\n- a stray idea"} {
		if !strings.Contains(copied[0], part) {
			t.Errorf("the copied text lacks %q:\n%s", part, copied[0])
		}
	}
	wantScreen(t, "notice", next, "Sent to the clipboard")
}

func TestReportDays_YAndTheBracketsSayWhyWhenTheyCannotApply(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "idle")
	m := reportOpen(r)

	next, cmd := m.Update(keyY)
	if msgs := collect(cmd); len(msgs) != 0 {
		t.Errorf("an empty day copied %v", msgs)
	}
	wantScreen(t, "empty day", next, "Nothing to copy")

	// The week has data to copy, but it is the day that is copied.
	busy := newRig(t)
	busy.monday(t)
	w := press(reportOpen(busy), keyTab)
	wantScreen(t, "the week", w, "Report: This week")
	next, cmd = w.Update(keyY)
	if msgs := collect(cmd); len(msgs) != 0 {
		t.Errorf("the week copied %v", msgs)
	}
	wantScreen(t, "week copy", next, "Press tab for a single day first.")
	wantScreen(t, "week step back", press(w, keyPrevDay), "Report: This week", "Press tab for a single day first.")
	wantScreen(t, "week step forward", press(w, keyNextDay), "Report: This week", "Press tab for a single day first.")
	wantNoText(t, "the week's extras", w, "a stray idea")
	wantNoText(t, "the week's finished", w, "release it")

	// And nothing is copied before the report has been read.
	early, _ := busy.newModel().Update(keyEsc)
	early, _ = early.Update(keyR)
	next, cmd = early.Update(keyY)
	if msgs := collect(cmd); len(msgs) != 0 {
		t.Errorf("an unread report copied %v", msgs)
	}
	wantScreen(t, "unread", next, "still loading")
}

func TestReportDays_TheNoticeGoesOnTheNextKey(t *testing.T) {
	r := newRig(t)
	r.monday(t)
	m := press(reportOpen(r), keyPrevDay)
	next, _ := m.Update(keyY)
	wantScreen(t, "sent", next, "Sent to the clipboard.")
	wantNoText(t, "gone", press(next, keyJ), "Sent to the clipboard.")
}

func TestReportDays_SteppingStartsTheNewDayAtItsTop(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "busy")
	for _, day := range []time.Duration{-24 * time.Hour, 0} {
		r.work(t, task.ID, epoch.Add(day), 25*time.Minute, core.StartOptions{})
		for i := 0; i < 12; i++ {
			r.clock.Set(epoch.Add(day + time.Duration(i+1)*time.Minute))
			if _, err := r.tracker.AddNote(ctx, task.ID, fmt.Sprintf("note %02d", i)); err != nil {
				t.Fatal(err)
			}
		}
	}
	r.clock.Set(epoch.Add(5 * time.Hour))
	m := sized(reportOpen(r), 80, 12)
	m = press(m, keyJ, keyJ, keyJ, keyJ, keyJ, keyJ)
	wantNoText(t, "scrolled", m, "By task")
	wantScreen(t, "after stepping", press(m, keyPrevDay), "Report: Mon 28 Sep", "By task")
}

func TestReportDays_OpeningTheReportAgainStartsOnToday(t *testing.T) {
	r := newRig(t)
	r.monday(t)
	m := press(reportOpen(r), keyPrevDay, keyEsc)
	wantScreen(t, "reopened", press(m, keyR), "Report: Today")
}

func TestReportDays_TheKeysAreInTheFooterAndTheFooterFitsEightyColumns(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "idle")
	m := sized(reportOpen(r), 80, 24)
	wantScreen(t, "footer", m, "[/] day", "y copy")
	wantNoWiderThan(t, "footer", m, 80)
}
