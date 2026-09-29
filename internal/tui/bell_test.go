package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/tui"
)

// sendAll delivers msg and everything its commands produce, and returns all the
// messages those commands produced so a test can see what was sent to the terminal.
func sendAll(m tea.Model, msg tea.Msg) (tea.Model, []tea.Msg) {
	m, cmd := m.Update(msg)
	var produced []tea.Msg
	for _, p := range collect(cmd) {
		produced = append(produced, p)
		var more []tea.Msg
		m, more = sendAll(m, p)
		produced = append(produced, more...)
	}
	return m, produced
}

// bootAll is booted, also returning what Init's commands produced.
func bootAll(m tea.Model) (tea.Model, []tea.Msg) {
	var all []tea.Msg
	for _, msg := range collect(m.Init()) {
		all = append(all, msg)
		var more []tea.Msg
		m, more = sendAll(m, msg)
		all = append(all, more...)
	}
	return m, all
}

// bells counts the terminal bells (raw BEL characters) among msgs.
func bells(msgs []tea.Msg) int {
	n := 0
	for _, msg := range msgs {
		if raw, ok := msg.(tea.RawMsg); ok && raw.Msg == "\a" {
			n++
		}
	}
	return n
}

// tickAt moves the clock to offset after the session's planned end (30m after
// the epoch the rig starts at, since the session starts at once) and ticks.
func tickAt(t *testing.T, r *rig, m tea.Model, sinceStart time.Duration) (tea.Model, int) {
	t.Helper()
	r.clock.Set(epoch.Add(sinceStart))
	m, msgs := sendAll(m, tui.TickMsg{})
	return m, bells(msgs)
}

const sessionEnd = 30 * time.Minute // when the rig's 30m session completes

func TestBell_NothingRingsOrShowsWhileFocusing(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m, n := tickAt(t, r, m, sessionEnd-time.Second)
	if n != 0 || strings.Contains(screen(m), "complete") {
		t.Errorf("rang %d bells / banner shown one second before the end:\n%s", n, screen(m))
	}
}

func TestBell_SessionEndRingsAndShowsTheBanner(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())

	m, n := tickAt(t, r, m, sessionEnd)
	if n != 1 {
		t.Errorf("rang %d bells at the session's end, want 1", n)
	}
	wantScreen(t, "banner", m, "Focus complete", "press any key")
}

func TestBell_RepeatsOnTheScheduleAndNeverTwiceForTheSameRing(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())

	steps := []struct {
		at   time.Duration // after the session's end
		want int           // bells rung on this tick
	}{
		{0, 1},                       // ring 1
		{0, 0},                       // same moment: nothing new is due
		{29 * time.Second, 0},        // still ring 1's slot
		{30 * time.Second, 1},        // ring 2
		{31 * time.Second, 0},        //
		{5 * 30 * time.Second, 1},    // jumped to ring 6: one bell, not a burst
		{5*30*time.Second + 1, 0},    // and not again
		{10 * 30 * time.Second, 1},   // the last ring (11)
		{11 * 30 * time.Second, 0},   // the window is over
		{time.Hour - time.Minute, 0}, // long silent
	}
	for _, s := range steps {
		var n int
		m, n = tickAt(t, r, m, sessionEnd+s.at)
		if n != s.want {
			t.Errorf("at end+%v: rang %d bells, want %d", s.at, n, s.want)
		}
	}
}

func TestBell_AKeyPressSilencesItAndDismissesTheBannerButStillWorks(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "older")
	r.addTask(t, "newer")
	r.clock.Set(epoch) // addTask moved the clock on; the session below should start at the epoch
	r.startSession(t)
	m := booted(r.newModel())
	m, n := tickAt(t, r, m, sessionEnd)
	if n != 1 {
		t.Fatalf("rang %d, want 1", n)
	}
	// Resolve the hand-off elsewhere, so the prompt closes and the next key goes
	// to the list, while the bell is still ringing.
	if err := r.tracker.SkipHandoff(ctx); err != nil {
		t.Fatal(err)
	}
	m, _ = tickAt(t, r, m, sessionEnd+time.Second)
	wantScreen(t, "still ringing", m, "Focus complete")
	wantSelected(t, "before the key", m, "newer")

	m = press(m, keyJ) // acknowledges, and also moves the cursor as usual
	if strings.Contains(screen(m), "Focus complete") {
		t.Errorf("the banner is still shown after a key press:\n%s", screen(m))
	}
	if got := selected(t, m); strings.Contains(got, "newer") {
		t.Errorf("the cursor did not move, so the key that silenced the bell was swallowed: selected %q", got)
	}
	wantScreen(t, "the lasting state remains", m, "Break")

	for _, after := range []time.Duration{30 * time.Second, 2 * time.Minute, 5 * time.Minute} {
		if m, n = tickAt(t, r, m, sessionEnd+after); n != 0 {
			t.Errorf("rang %d bells after being silenced (end+%v), want 0", n, after)
		}
	}
}

func TestBell_BreakEndIsANewEventEvenIfTheFirstWasNeverSilenced(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m, _ = tickAt(t, r, m, sessionEnd) // session-end bell, ignored by the user

	m, n := tickAt(t, r, m, sessionEnd+10*time.Minute) // the 10m Break ends
	if n != 1 {
		t.Errorf("rang %d bells when the Break ended, want 1", n)
	}
	wantScreen(t, "break-over banner", m, "Break over")
	if strings.Contains(screen(m), "Focus complete") {
		t.Errorf("the session-end banner should be replaced:\n%s", screen(m))
	}

	m = press(m, keyJ)
	if strings.Contains(screen(m), "Break over") {
		t.Errorf("the banner survived a key press:\n%s", screen(m))
	}
	if _, n = tickAt(t, r, m, sessionEnd+10*time.Minute+30*time.Second); n != 0 {
		t.Errorf("rang %d bells after silencing the Break-over bell, want 0", n)
	}
}

func TestBell_AnEventFromLongAgoIsSilent(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m, n := tickAt(t, r, m, sessionEnd+5*time.Hour)
	if n != 0 || strings.Contains(screen(m), "complete") || strings.Contains(screen(m), "Break over") {
		t.Errorf("a stale event rang %d bells or showed a banner:\n%s", n, screen(m))
	}
	wantScreen(t, "the state is still shown", m, "Idle", "Hand-off due")
}

func TestBell_ReopeningJustAfterTheEndRingsOnceAndShowsTheBanner(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.clock.Set(epoch.Add(sessionEnd + 40*time.Second)) // the app was closed when it ended

	m, msgs := bootAll(r.newModel())
	if n := bells(msgs); n != 1 {
		t.Errorf("rang %d bells on reopening, want 1", n)
	}
	wantScreen(t, "banner", m, "Focus complete")
}

func TestBell_RingsAndShowsWhileTheAddPromptIsOpen(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel())
	m = press(m, keyA)
	m = typeText(m, "half typed")

	m, n := tickAt(t, r, m, sessionEnd)
	if n != 1 {
		t.Errorf("rang %d bells with the prompt open, want 1", n)
	}
	wantScreen(t, "banner and the typed text", m, "Focus complete", "half typed")
}
