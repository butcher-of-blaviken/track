package tui_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/butcher-of-blaviken/track/internal/notify"
	"github.com/butcher-of-blaviken/track/internal/notify/notifytest"
	"github.com/butcher-of-blaviken/track/internal/tui"
)

// A bell event also raises a system notification, once, on its first ring. The
// bell is the signal that always works, so nothing here may change it.

func (r *rig) notifyingModel(n notify.Notifier, opts ...tui.Option) tea.Model {
	opts = append([]tui.Option{tui.WithTick(noTick), tui.WithLayout("single"), tui.WithNotifier(n)}, opts...)
	return tui.New(r.tracker, opts...)
}

const (
	sessionEndText = "Focus complete — Break started"
	breakEndText   = "Break over — ready for the next session"
)

func TestNotify_OncePerEventOnItsFirstRingAndNotOnTheRepeats(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	rec := &notifytest.Recorder{}
	m := booted(r.notifyingModel(rec))

	m, _ = tickAt(t, r, m, sessionEnd-time.Second)
	if got := rec.Events(); len(got) != 0 {
		t.Fatalf("notified %v before the session ended", got)
	}

	// The session-end bell rings 11 times over five minutes; only the first notifies.
	for _, at := range []time.Duration{0, 0, 30 * time.Second, 5 * 30 * time.Second, 10 * 30 * time.Second} {
		m, _ = tickAt(t, r, m, sessionEnd+at)
	}
	want := []notify.Event{{Title: "Track", Body: sessionEndText}}
	if got := rec.Events(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("after the session-end bell, notified %v, want %v", got, want)
	}

	// The Break's end is its own event, with its own words.
	m, _ = tickAt(t, r, m, sessionEnd+10*time.Minute)
	m, _ = tickAt(t, r, m, sessionEnd+10*time.Minute+30*time.Second)
	want = append(want, notify.Event{Title: "Track", Body: breakEndText})
	if got := rec.Events(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("after the Break-end bell, notified %v, want %v", got, want)
	}
	_ = m
}

func TestNotify_ABellAlreadyDueWhenTheAppOpensNotifiesOnceNotABurst(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	r.clock.Set(epoch.Add(sessionEnd + 2*time.Minute)) // the app was closed when the session ended
	rec := &notifytest.Recorder{}
	m := booted(r.notifyingModel(rec))
	if got := rec.Events(); len(got) != 1 || got[0].Body != sessionEndText {
		t.Fatalf("on opening, notified %v, want the session-end notification once", got)
	}
	m, _ = tickAt(t, r, m, sessionEnd+3*time.Minute)
	m, _ = tickAt(t, r, m, sessionEnd+4*time.Minute)
	if got := rec.Events(); len(got) != 1 {
		t.Errorf("later ticks of the same event notified again: %v", got)
	}
	_ = m
}

func TestNotify_SilencingTheBellDoesNotNotifyAgain(t *testing.T) {
	r := newRig(t)
	r.addTask(t, "a task")
	r.clock.Set(epoch)
	r.startSession(t)
	rec := &notifytest.Recorder{}
	m := booted(r.notifyingModel(rec))
	m, _ = tickAt(t, r, m, sessionEnd)
	m = press(m, keyJ) // silences the bell
	m, _ = tickAt(t, r, m, sessionEnd+30*time.Second)
	if got := rec.Events(); len(got) != 1 {
		t.Errorf("notified %v, want only the first ring's notification", got)
	}
	_ = m
}

func TestNotify_NoNotifierMeansNoNotificationAndTheBellStillRings(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.newModel()) // no WithNotifier
	if _, n := tickAt(t, r, m, sessionEnd); n != 1 {
		t.Errorf("rang %d bells, want 1", n)
	}
}

func TestNotify_AFailingNotifierChangesNothingForTheUser(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	rec := &notifytest.Recorder{Err: errors.New("no notification server")}
	m := booted(r.notifyingModel(rec))
	m, n := tickAt(t, r, m, sessionEnd)
	if n != 1 {
		t.Errorf("rang %d bells, want 1", n)
	}
	if got := len(rec.Events()); got != 1 {
		t.Errorf("the notifier was called %d times, want 1 (a failure is not retried)", got)
	}
	wantScreen(t, "banner", m, "Focus complete", "press any key")
	if s := screen(m); strings.Contains(s, "no notification server") {
		t.Errorf("the failure leaked onto the screen:\n%s", s)
	}
}

func TestNotify_AHungNotifierIsGivenADeadlineAndGivenUpOn(t *testing.T) {
	r := newRig(t)
	r.startSession(t)
	m := booted(r.notifyingModel(notifytest.Hang{}, tui.WithNotifyTimeout(20*time.Millisecond)))
	r.clock.Set(epoch.Add(sessionEnd))
	start := time.Now()
	_, msgs := sendAll(m, tui.TickMsg{}) // runs the notifier's command to completion
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("a hung notifier held the command for %v", took)
	}
	if bells(msgs) != 1 {
		t.Errorf("rang %d bells, want 1", bells(msgs))
	}

	rec := &notifytest.Recorder{}
	tickAt(t, r, booted(r.notifyingModel(rec)), sessionEnd)
	if !rec.AllHadDeadlines() || len(rec.Events()) != 1 {
		t.Errorf("the notifier's context had no deadline: events %v", rec.Events())
	}
}
