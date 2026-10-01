package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butcher-of-blaviken/track/internal/notify"
	"github.com/butcher-of-blaviken/track/internal/notify/notifytest"
)

// named is a notifier that says what it shows notifications with, as the real
// ones do.
type named struct {
	*notifytest.Recorder
	name string
}

func (n named) Name() string { return n.name }

func testNotifier(err error) named {
	return named{Recorder: &notifytest.Recorder{Err: err}, name: "osascript"}
}

func runTest(words []string, n notify.Notifier, goos string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = runNotifyTest(words, n, goos, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestNotifyTest_SendsOneNotificationAndOnMacOSSaysWhatToCheck(t *testing.T) {
	n := testNotifier(nil)
	code, stdout, stderr := runTest(nil, n, "darwin")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q; want success", code, stderr)
	}
	events := n.Events()
	if len(events) != 1 || events[0].Title != "Track" || !strings.Contains(events[0].Body, "test notification") {
		t.Errorf("sent %v, want one test notification titled Track", events)
	}
	for _, want := range []string{"Sent a test notification with osascript", "Script Editor", "Allow", "Do Not Disturb", `notifications = "desktop"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	if !n.AllHadDeadlines() {
		t.Error("the notifier had no deadline, so a hung osascript would hang the command")
	}
}

func TestNotifyTest_AFailureIsReportedWithItsCauseAndAFailingExit(t *testing.T) {
	code, stdout, stderr := runTest(nil, testNotifier(errors.New("osascript: exit status 1: Not authorized")), "darwin")
	if code != exitFailure {
		t.Errorf("exit %d, want %d", code, exitFailure)
	}
	for _, want := range []string{"osascript", "Not authorized"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q: %s", want, stderr)
		}
	}
	if strings.Contains(stdout, "Sent a test notification") {
		t.Errorf("claims success after a failure:\n%s", stdout)
	}
}

func TestNotifyTest_NoNotifierForThePlatformSaysSoAndFails(t *testing.T) {
	code, stdout, stderr := runTest(nil, notify.Nop{}, "plan9")
	if code != exitFailure || stdout != "" || !strings.Contains(stderr, "plan9") || !strings.Contains(stderr, "bell") {
		t.Errorf("exit %d, stdout %q, stderr %q; want a failure that names the platform and the bell", code, stdout, stderr)
	}
}

func TestNotifyTest_TakesNoArgumentsAndSendsNothingWhenGivenOne(t *testing.T) {
	n := testNotifier(nil)
	code, _, stderr := runTest([]string{"extra"}, n, "darwin")
	if code != exitUsage || !strings.Contains(stderr, `"extra"`) || len(n.Events()) != 0 {
		t.Errorf("exit %d, stderr %q, sent %v; want a usage error and no notification", code, stderr, n.Events())
	}
}

func TestNotifyTest_AHungNotifierIsGivenUpOn(t *testing.T) {
	old := notifyTestTimeout
	notifyTestTimeout = 20 * time.Millisecond
	t.Cleanup(func() { notifyTestTimeout = old })
	hung := named{Recorder: &notifytest.Recorder{}, name: "osascript"}
	start := time.Now()
	code, _, stderr := runTest(nil, hangingNamed{hung}, "darwin")
	if code != exitFailure || !strings.Contains(stderr, "deadline") || time.Since(start) > 5*time.Second {
		t.Errorf("exit %d after %v, stderr %q; want a prompt failure naming the deadline", code, time.Since(start), stderr)
	}
}

type hangingNamed struct{ named }

func (hangingNamed) Notify(ctx context.Context, _ notify.Event) error {
	<-ctx.Done()
	return ctx.Err()
}

// Through the real command line, an extra argument is refused before anything is
// sent, so this never shows a notification, even on a Mac.
func TestNotifyTest_IsACommandOfTheCommandLine(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := realMain([]string{"notify-test", "extra"}, env(nil), &out, &errOut); code != exitUsage || !strings.Contains(errOut.String(), "unexpected argument") {
		t.Errorf("exit %d, stderr %q; want a usage error from notify-test", code, errOut.String())
	}
	var usageText bytes.Buffer
	usage(&usageText)
	if !strings.Contains(usageText.String(), "track notify-test") {
		t.Error("--help does not list notify-test")
	}
	errOut.Reset()
	realMain([]string{"nope"}, env(nil), &out, &errOut)
	if !strings.Contains(errOut.String(), "notify-test") {
		t.Errorf("the unknown-command message does not list notify-test: %s", errOut.String())
	}
}
