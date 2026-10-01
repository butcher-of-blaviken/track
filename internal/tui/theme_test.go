package tui_test

import (
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The theme uses only the 16 ANSI colours, so the app follows the terminal's
// own palette, and the faint attribute for secondary text (bright black is the
// background colour in Solarized Dark).

func TestTheme_PhasesHaveTheirOwnColours(t *testing.T) {
	r := newRig(t)
	m := booted(r.newModel())
	if got := styleOnLine(t, m, "Idle", "Idle"); !got.faint {
		t.Errorf("Idle = %+v, want faint", got)
	}

	r.startSession(t)
	m = booted(r.newModel())
	if got := styleOnLine(t, m, "Focus   30:00", "Focus"); got.fg != ansiMagenta || !got.bold {
		t.Errorf("Focus = %+v, want bold magenta", got)
	}

	m = endedModel(t, newRig(t))
	if got := styleOnLine(t, m, "Break   ", "Break"); got.fg != ansiGreen || !got.bold {
		t.Errorf("Break = %+v, want bold green", got)
	}
	if got := styleOf(t, m, "Hand-off due"); got.fg != ansiYellow {
		t.Errorf("Hand-off due = %+v, want yellow", got)
	}
}

func TestTheme_TheSelectedRowIsOneReverseBarAcrossChipsAndMatches(t *testing.T) {
	r := newRig(t)
	if _, err := r.tracker.AddTask(ctx, "write the PRD ##docs ##Q1"); err != nil {
		t.Fatal(err)
	}
	r.addTask(t, "review the code")
	m := send(booted(r.newModel()), tea.WindowSizeMsg{Width: 60, Height: 20})
	m = press(m, keyJ) // the older task, with the chips

	line := styledLineWith(t, m, "> write the PRD")
	text := string(runesOf(line))
	start := strings.Index(text, "> write the PRD")
	end := strings.Index(text, "#Q1") + len("#Q1")
	for _, c := range line[len([]rune(text[:start])):len([]rune(text[:end]))] {
		if !c.reverse || c.fg >= 0 {
			t.Fatalf("%q is %+v inside the selected row, want reverse in the terminal's own colours throughout", string(c.r), c.attrs)
		}
	}
	if len(line) != 60 || !line[59].reverse {
		t.Errorf("the bar is %d columns wide and ends with %+v, want it to run to column 60", len(line), line[len(line)-1].attrs)
	}
	if got := styleOf(t, m, "review the code"); got.reverse {
		t.Errorf("an unselected row is reverse: %+v", got)
	}
}

func runesOf(line []styledRune) []rune {
	out := make([]rune, len(line))
	for i, c := range line {
		out[i] = c.r
	}
	return out
}

func TestTheme_SearchMatchesAreBoldUnderlinedAlsoInTheSelectedRow(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "write the PRD")
	m := press(booted(r.newModel()), keySlash)
	m = typeText(m, "PRD")
	line := styledLineWith(t, m, "> write the PRD")
	text := string(runesOf(line))
	i := len([]rune(text[:strings.Index(text, "PRD")]))
	for _, c := range line[i : i+3] {
		if !c.bold || !c.underline || !c.reverse {
			t.Errorf("%q = %+v, want bold, underlined and still reverse", string(c.r), c.attrs)
		}
	}
	if c := line[i-2]; c.bold || c.underline {
		t.Errorf("an unmatched character is emphasised: %q %+v", string(c.r), c.attrs)
	}
}

func TestTheme_DoneAndArchivedTasksAreFaintWithTheirWords(t *testing.T) {
	r := newRig(t)
	done := r.addTask(t, "finished one")
	archived := r.addTask(t, "dropped one")
	r.addTask(t, "current one")
	r.setState(t, done, 1)
	r.setState(t, archived, 2)
	m := press(booted(r.newModel()), keyTab)
	wantScreen(t, "words stay", m, "finished one (done)", "dropped one (archived)")
	if got := styleOf(t, m, "finished one (done)"); !got.faint {
		t.Errorf("done row = %+v, want faint", got)
	}
	if got := styleOf(t, m, "dropped one (archived)"); !got.faint {
		t.Errorf("archived row = %+v, want faint", got)
	}
	if got := styleOf(t, m, "current one"); got.faint {
		t.Errorf("an Active row is faint: %+v", got)
	}
}

func TestTheme_ATagKeepsItsColourWhateverItsCaseAndWhereverItAppears(t *testing.T) {
	r := newRig(t)
	if _, err := r.tracker.AddTask(ctx, "first ##Docs"); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	if _, err := r.tracker.AddTask(ctx, "second ##docs ##work ##a ##b ##c ##d"); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	r.addTask(t, "parked") // the newest task takes the cursor, so the tagged rows are plain
	m := send(booted(r.newModel()), tea.WindowSizeMsg{Width: 80, Height: 20})

	seen := map[int]bool{}
	var docs int
	for _, tag := range []string{"#Docs", "#work", "#a", "#b", "#c", "#d"} {
		got := styleOf(t, m, tag)
		if got.reverse || got.fg < 0 {
			t.Errorf("%s = %+v, want a plain foreground colour", tag, got)
		}
		switch got.fg {
		case ansiCyan, ansiYellow, ansiMagenta, ansiGreen:
		default:
			t.Errorf("%s = colour %d, want cyan, yellow, magenta or green", tag, got.fg)
		}
		seen[got.fg] = true
		if tag == "#Docs" {
			docs = got.fg
		}
	}
	if len(seen) < 2 {
		t.Errorf("all tags share one colour: %v", seen)
	}

	// Tags are first-use cased, so "second" shows #Docs too: the same colour as on "first".
	line := styledLineWith(t, m, "second")
	text := string(runesOf(line))
	i := len([]rune(text[:strings.Index(text, "#Docs")]))
	if line[i].fg != docs {
		t.Errorf("#Docs on the second row is colour %d, on the first %d", line[i].fg, docs)
	}

	// The detail shows the same colour.
	m = press(m, keyJ, keyL)
	if got := styleOf(t, m, "#Docs"); got.fg != docs {
		t.Errorf("#Docs in the detail is colour %d, want %d", got.fg, docs)
	}
}

func TestTheme_TheFooterShowsKeysInTheAccentAndDescriptionsMuted(t *testing.T) {
	m := booted(newRig(t).newModel())
	if got := styleOnLine(t, m, "enter start", "enter"); got.fg != ansiBlue || !got.bold {
		t.Errorf("key = %+v, want bold blue", got)
	}
	if got := styleOnLine(t, m, "enter start", "start"); !got.faint || got.fg >= 0 {
		t.Errorf("description = %+v, want faint in the terminal's own colour", got)
	}
}

func TestTheme_UnfiledNotesNeedYouSoTheyAreYellow(t *testing.T) {
	r := newRig(t)
	r.unfile(t, "a loose note")
	m := booted(r.newModel())
	if got := styleOf(t, m, "Unfiled notes: 1 (i to file)"); got.fg != ansiYellow {
		t.Errorf("unfiled notes line = %+v, want yellow", got)
	}
}

func TestTheme_ErrorsAreRedAndOnlyErrors(t *testing.T) {
	m := booted(newRigOn(t, failingStore{newRig(t).store}).newModel())
	if got := styleOf(t, m, "error: disk on fire"); got.fg != ansiRed {
		t.Errorf("error = %+v, want red", got)
	}
}

func TestTheme_TheBellBannerIsColouredByKind(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m, _ = tickAt(t, r, m, sessionEnd)
	if got := styleOf(t, m, "Focus complete"); !got.reverse || got.fg != ansiGreen {
		t.Errorf("session-end banner = %+v, want reverse green (a Break has started)", got)
	}
	m, _ = tickAt(t, r, m, sessionEnd+10*time.Minute)
	if got := styleOf(t, m, "Break over"); !got.reverse || got.fg != ansiYellow {
		t.Errorf("break-end banner = %+v, want reverse yellow (it needs you)", got)
	}
}

func TestTheme_TheInboxCursorRowIsReverseVideo(t *testing.T) {
	r := newRig(t)
	r.unfile(t, "first note")
	r.unfile(t, "second note")
	m := press(send(booted(r.newModel()), tea.WindowSizeMsg{Width: 60, Height: 20}), keyI)
	if got := styleOf(t, m, "first note"); !got.reverse || got.fg >= 0 {
		t.Errorf("selected note = %+v, want reverse in the terminal's own colours", got)
	}
	if got := styleOf(t, m, "second note"); got.reverse {
		t.Errorf("unselected note is reverse: %+v", got)
	}
}

func TestTheme_DetailAndReportHeadingsUseTheAccentAndTimestampsAreMuted(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "write the PRD")
	if _, err := r.tracker.AddNote(ctx, task.ID, "remember this"); err != nil {
		t.Fatal(err)
	}
	m := press(booted(r.newModel()), keyL)
	if got := styleOf(t, m, "Notes"); got.fg != ansiBlue || !got.bold {
		t.Errorf("Notes heading = %+v, want bold blue", got)
	}
	if got := styleOf(t, m, epoch.Add(time.Minute).Local().Format("2006-01-02 15:04")); !got.faint {
		t.Errorf("note time = %+v, want faint", got)
	}

	r.startSession(t)
	m = press(booted(r.newModel()), keyR)
	if got := styleOf(t, m, "Report: Today"); got.fg != ansiBlue || !got.bold {
		t.Errorf("report title = %+v, want bold blue", got)
	}
}

func TestTheme_PromptHintsAreMuted(t *testing.T) {
	m := press(booted(newRig(t).newModel()), keyA)
	if got := styleOf(t, m, "what are you working on?"); !got.faint {
		t.Errorf("prompt hint = %+v, want faint", got)
	}
}

func TestTheme_NoStyledLineIsWiderThanTheWindow(t *testing.T) {
	r := newRig(t)
	if _, err := r.tracker.AddTask(ctx, "a rather long title that has to be cut somewhere ##with ##several ##tags"); err != nil {
		t.Fatal(err)
	}
	r.startSession(t)
	for _, width := range []int{20, 40, 80} {
		m := send(booted(r.newModel()), tea.WindowSizeMsg{Width: width, Height: 14})
		wantNoWiderThan(t, "list", m, width)
		wantNoWiderThan(t, "detail", press(m, keyL), width)
		wantNoWiderThan(t, "report", press(m, keyR), width)
		wantNoWiderThan(t, "help", press(m, keyHelp), width)
	}
}

// ANSI blue is a very dark blue on many terminals, unreadable as text on a dark
// background. The terminal is asked for its background, and the accent becomes
// bright blue on a dark one.

const ansiBrightBlue = 12

func TestTheme_TheAccentIsBrightBlueOnADarkTerminalBackground(t *testing.T) {
	r := newRig(t)
	task := r.addTask(t, "write the PRD")
	if _, err := r.tracker.AddNote(ctx, task.ID, "remember this"); err != nil {
		t.Fatal(err)
	}
	m := send(booted(r.newModel()), tea.BackgroundColorMsg{Color: color.Black})
	if got := styleOnLine(t, m, "enter start", "enter"); got.fg != ansiBrightBlue || !got.bold {
		t.Errorf("footer key = %+v, want bold bright blue", got)
	}
	if got := styleOf(t, press(m, keyL), "Notes"); got.fg != ansiBrightBlue || !got.bold {
		t.Errorf("Notes heading = %+v, want bold bright blue", got)
	}
	m = send(booted(r.newSplitModel()), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = send(m, tea.BackgroundColorMsg{Color: color.Black})
	if line := styledLineWith(t, m, "Tasks"); line[0].fg != ansiBrightBlue {
		t.Errorf("the focused panel's border = %+v, want bright blue", line[0].attrs)
	}
}

func TestTheme_TheAccentStaysBlueOnALightTerminalBackground(t *testing.T) {
	m := send(booted(newRig(t).newModel()), tea.BackgroundColorMsg{Color: color.White})
	if got := styleOnLine(t, m, "enter start", "enter"); got.fg != ansiBlue || !got.bold {
		t.Errorf("footer key = %+v, want bold blue", got)
	}
}

func TestTheme_APaletteWithItsOwnColoursIgnoresTheTerminalBackground(t *testing.T) {
	r := newRig(t)
	m := send(booted(themed(r)), tea.BackgroundColorMsg{Color: color.Black})
	if got := raw(m); !strings.Contains(got, fg(0x0a, 0x0b, 0x0c)) || strings.Contains(got, ";94m") || strings.Contains(got, "[94m") {
		t.Errorf("the accent is not the palette's:\n%q", got)
	}
	// It does not need to ask, either: the default palette's Init has one more command.
	if got, def := len(collect(themed(r).Init())), len(collect(r.newModel().Init())); got >= def {
		t.Errorf("a themed model's Init has %d messages, the default's %d: it asked for the background colour anyway", got, def)
	}
}
