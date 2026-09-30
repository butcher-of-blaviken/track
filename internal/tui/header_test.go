package tui_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

// phaseRow is the plain text of the header's phase line, found by its phase word.
func phaseRow(t *testing.T, m tea.Model, word string) string {
	t.Helper()
	for _, line := range strings.Split(screen(m), "\n") {
		if regexp.MustCompile(`^\s+` + word + `\b`).MatchString(line) {
			return line
		}
	}
	t.Fatalf("no phase line starts with %q:\n%s", word, screen(m))
	return ""
}

// barOf counts the filled and empty cells of the bar on a line.
func barOf(line string) (filled, empty int) {
	return strings.Count(line, "█"), strings.Count(line, "░")
}

func sized(m tea.Model, width, height int) tea.Model {
	return send(m, tea.WindowSizeMsg{Width: width, Height: height})
}

func TestHeader_FocusShowsABadgeTheClockTheTitleAndAProgressBar(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := sized(booted(r.newModel()), 80, 24)

	row := phaseRow(t, m, "Focus")
	if !regexp.MustCompile(`Focus\s+30:00\s+task\s+░+ 0%$`).MatchString(row) {
		t.Errorf("a session that just began: %q", row)
	}
	for _, tc := range []struct {
		elapsed time.Duration
		clock   string
		percent int
	}{
		{7*time.Minute + 30*time.Second, "22:30", 25},
		{15 * time.Minute, "15:00", 50},
		{20 * time.Minute, "10:00", 67}, // 66.7 rounds up
		{29*time.Minute + 30*time.Second, "00:30", 98},
	} {
		r.clock.Set(epoch.Add(tc.elapsed))
		m = send(m, tui.TickMsg{})
		row = phaseRow(t, m, "Focus")
		filled, empty := barOf(row)
		want := int(float64(filled+empty)*float64(tc.elapsed)/float64(30*time.Minute) + 0.5)
		if filled != want {
			t.Errorf("at %v the bar has %d of %d filled, want %d", tc.elapsed, filled, filled+empty, want)
		}
		if !strings.Contains(row, tc.clock) || !strings.HasSuffix(row, fmt.Sprintf("%d%%", tc.percent)) {
			t.Errorf("at %v: %q, want %s and %d%%", tc.elapsed, row, tc.clock, tc.percent)
		}
	}
}

func TestHeader_TheBarIsTwentyEightCellsWhenThereIsRoomAndTheBadgeKeepsItsSpaces(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := sized(booted(r.newModel()), 100, 24)
	filled, empty := barOf(phaseRow(t, m, "Focus"))
	if filled+empty != 28 {
		t.Errorf("bar is %d cells, want 28", filled+empty)
	}
	if !strings.Contains(phaseRow(t, m, "Focus"), " Focus ") {
		t.Errorf("the badge lost its padding: %q", phaseRow(t, m, "Focus"))
	}
}

func TestHeader_BadgesAndBarsTakeThePhaseColour(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := sized(booted(r.newModel()), 80, 24)
	r.clock.Set(epoch.Add(15 * time.Minute))
	m = send(m, tui.TickMsg{})

	if got := styleOnLine(t, m, "Focus", " Focus "); !got.reverse || got.fg != ansiMagenta || !got.bold {
		t.Errorf("Focus badge = %+v, want bold reverse magenta", got)
	}
	if got := styleOnLine(t, m, "Focus", "█"); got.fg != ansiMagenta || got.reverse {
		t.Errorf("filled bar = %+v, want magenta", got)
	}
	if got := styleOnLine(t, m, "Focus", "░"); !got.faint {
		t.Errorf("empty bar = %+v, want faint", got)
	}

	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	r.clock.Set(epoch.Add(sessionEnd + 5*time.Minute))
	m = send(m, tui.TickMsg{})
	if got := styleOnLine(t, m, "Break   05:00", " Break "); !got.reverse || got.fg != ansiGreen {
		t.Errorf("Break badge = %+v, want reverse green", got)
	}
	if got := styleOnLine(t, m, "Break   05:00", "█"); got.fg != ansiGreen {
		t.Errorf("Break bar = %+v, want green", got)
	}
}

func TestHeader_BreakCountsDownWithItsOwnBar(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := sized(booted(r.newModel()), 80, 24)
	r.clock.Set(epoch.Add(sessionEnd + 5*time.Minute)) // halfway through a 10m Break
	m = send(m, tui.TickMsg{})
	row := phaseRow(t, m, "Break")
	if !regexp.MustCompile(`Break\s+05:00\s+.*50%$`).MatchString(row) {
		t.Errorf("row = %q, want 05:00 left and 50%%", row)
	}
	if filled, empty := barOf(row); filled != empty {
		t.Errorf("halfway through, the bar has %d filled and %d empty", filled, empty)
	}
}

func TestHeader_IdleSaysWhatToDoNext(t *testing.T) {
	r := newRig(t)
	m := sized(booted(r.newModel()), 100, 24)
	wantScreen(t, "no tasks", m, "Idle")
	wantNoText(t, "no hint without tasks", m, "Pick a task")

	r.addTask(t, "something")
	m = send(m, tui.TickMsg{})
	wantScreen(t, "with a task", m, "Idle", "Pick a task and press enter to start a 30 minute session")
	if got := styleOf(t, m, "Pick a task and press enter to start a 30 minute session"); !got.faint {
		t.Errorf("hint = %+v, want faint", got)
	}
}

func TestHeader_IdleHintFollowsTheConfiguredLength(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "something")
	m := tui.New(r.tracker, tui.WithTick(noTick), tui.WithFocusDuration(45*time.Minute))
	m2 := sized(booted(m), 100, 24)
	wantScreen(t, "configured", m2, "start a 45 minute session")
}

func TestHeader_HandoffDueIsItsOwnYellowBadgeWhileTheBreakKeepsShowing(t *testing.T) {
	r := newRig(t)
	m := sized(endedModel(t, r), 100, 24)
	wantScreen(t, "both", m, "Hand-off due", "Break")
	if got := styleOf(t, m, " Hand-off due "); !got.reverse || got.fg != ansiYellow {
		t.Errorf("hand-off badge = %+v, want reverse yellow", got)
	}
}

func TestHeader_TodaysTotalSitsOnTheTitleLineFromSixtyColumns(t *testing.T) {
	r := newRig(t)
	id := r.addTask(t, "work").ID
	for range 2 {
		if _, err := r.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(41 * time.Minute) // session and Break
	}
	m := sized(booted(r.newModel()), 80, 24)
	first := strings.Split(screen(m), "\n")[0]
	if !regexp.MustCompile(`^Track\s+today 1h · 2 sessions$`).MatchString(first) {
		t.Errorf("title line = %q", first)
	}
	if w := ansi.StringWidth(strings.TrimRight(first, " ")); w != 78 {
		t.Errorf("the summary ends at column %d, want it right-aligned to 78", w)
	}
	if got := styleOf(t, m, "today 1h · 2 sessions"); !got.faint {
		t.Errorf("summary = %+v, want faint", got)
	}

	narrow := sized(m, 40, 24)
	wantNoText(t, "too narrow", narrow, "today")
	unknown := booted(r.newModel())
	wantNoText(t, "unknown width", unknown, "today")
}

func TestHeader_NoTotalUntilSomeTimeHasBeenFocused(t *testing.T) {
	m := sized(booted(newRig(t).newModel()), 80, 24)
	wantNoText(t, "nothing focused today", m, "today")
}

func TestHeader_OneSessionIsSingular(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.clock.Advance(31 * time.Minute)
	m := sized(booted(r.newModel()), 80, 24)
	wantScreen(t, "singular", m, "today 30m · 1 session")
	wantNoText(t, "not plural", m, "1 sessions")
}

func TestHeader_TheBarNarrowsThenGoesAndTheClockSurvivesToTheEnd(t *testing.T) {
	for _, title := range []string{"task", "a rather long task title that will not fit"} {
		t.Run(title, func(t *testing.T) { barNarrowsThenGoes(t, title) })
	}
}

func barNarrowsThenGoes(t *testing.T, title string) {
	r := newRig(t)
	if _, err := r.tracker.AddTask(ctx, title); err != nil {
		t.Fatal(err)
	}
	tasks, _ := r.tracker.Tasks(ctx)
	if _, err := r.tracker.StartSession(ctx, tasks[0].ID, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(10 * time.Minute)

	prev := 1 << 30
	var widths []int
	for w := 140; w >= 16; w-- {
		widths = append(widths, w)
	}
	for _, width := range widths {
		m := sized(booted(r.newModel()), width, 24)
		wantNoWiderThan(t, fmt.Sprintf("width %d", width), m, width)
		row := phaseRow(t, m, "Focus")
		if !strings.Contains(row, "20:00") {
			t.Errorf("width %d: the clock is gone: %q", width, row)
		}
		filled, empty := barOf(row)
		cells := filled + empty
		switch {
		case cells > prev:
			t.Errorf("width %d: the bar grew from %d to %d cells as the window shrank", width, prev, cells)
		case cells != 0 && cells < 8:
			t.Errorf("width %d: a %d cell bar is too small to read; it should go", width, cells)
		case cells > 28:
			t.Errorf("width %d: a %d cell bar, want at most 28", width, cells)
		}
		// Room for the badge, clock, the whole title, a gap, the percentage and
		// the smallest bar means the bar is there; less means it is not.
		needs := 2 + 14 + 2 + len([]rune(title)) + 2 + len("33%") + 1 + 8
		if (width >= needs) != (cells > 0) {
			t.Errorf("width %d (the bar needs %d): %d bar cells in %q", width, needs, cells, row)
		}
		if cells == 0 && strings.Contains(row, "%") {
			t.Errorf("width %d: a percentage with no bar: %q", width, row)
		}
		prev = cells
	}
	if title == "task" {
		return
	}
	m := sized(booted(r.newModel()), 56, 24)
	if row := phaseRow(t, m, "Focus"); !strings.Contains(row, "…") {
		t.Errorf("a long title should be cut with an ellipsis once the bar has gone: %q", row)
	}
	m = sized(booted(r.newModel()), 80, 24)
	if row := phaseRow(t, m, "Focus"); strings.Contains(row, "…") {
		t.Errorf("the bar should shrink before the title is cut: %q", row)
	}
	m = sized(booted(r.newModel()), 140, 24)
	if row := phaseRow(t, m, "Focus"); strings.Contains(row, "…") {
		t.Errorf("room enough, yet the title was cut: %q", row)
	}
}

// sessionsCounting counts how often the sessions are read.
type sessionsCounting struct {
	core.Store
	n *atomic.Int32
}

func (s sessionsCounting) Sessions(ctx context.Context) ([]core.FocusSession, error) {
	s.n.Add(1)
	return s.Store.Sessions(ctx)
}

func TestHeader_TheTotalIsReadOnlyWhenItCouldHaveChanged(t *testing.T) {
	var reads atomic.Int32
	r := newRig(t)
	r2 := newRigOn(t, sessionsCounting{r.store, &reads})
	r2.clock.Set(epoch)
	id := r2.addTask(t, "new").ID // moves the clock on a minute, so do it before measuring
	m := sized(booted(r2.newModel()), 80, 24)
	base := reads.Load()
	if base == 0 {
		t.Fatal("the total was never read")
	}

	for range 20 {
		r2.clock.Advance(time.Second)
		m = send(m, tui.TickMsg{})
	}
	if got := reads.Load(); got != base {
		t.Errorf("read %d times in 20 quiet seconds, want no more than the %d at the start", got, base)
	}

	r2.clock.Advance(31 * time.Second)
	m = send(m, tui.TickMsg{})
	if got := reads.Load(); got != base+1 {
		t.Errorf("read %d times after 30s, want %d", got, base+1)
	}

	before := reads.Load()
	if _, err := r2.tracker.StartSession(ctx, id, 30*time.Minute, core.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	_ = send(m, tui.TickMsg{})
	if reads.Load() <= before {
		t.Error("a new session did not refresh the total")
	}
}
