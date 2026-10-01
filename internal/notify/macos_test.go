package notify

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type call struct {
	name string
	args []string
}

// fakeRunner records the commands it is asked to run.
func fakeRunner(calls *[]call, out string, err error) runner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, call{name, args})
		return []byte(out), err
	}
}

func TestMacOS_RunsOsascriptWithAFixedScriptAndTheTextAsArguments(t *testing.T) {
	var calls []call
	n := &MacOS{Sound: "Glass", run: fakeRunner(&calls, "", nil)}
	if err := n.Notify(context.Background(), Event{Title: "Track", Body: "Focus complete — Break started"}); err != nil {
		t.Fatal(err)
	}
	want := []call{{"osascript", []string{
		"-e", "on run argv",
		"-e", "display notification (item 2 of argv) with title (item 1 of argv) sound name (item 3 of argv)",
		"-e", "end run",
		"--", "Track", "Focus complete — Break started", "Glass",
	}}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("ran %q, want %q", calls, want)
	}
}

func TestMacOS_NoSoundLeavesOutTheSoundClause(t *testing.T) {
	var calls []call
	n := &MacOS{run: fakeRunner(&calls, "", nil)}
	_ = n.Notify(context.Background(), Event{Title: "T", Body: "B"})
	got := calls[0].args
	if joined := strings.Join(got, " "); strings.Contains(joined, "sound") {
		t.Errorf("a notification with no sound mentions one: %q", got)
	}
	if tail := got[len(got)-2:]; !reflect.DeepEqual(tail, []string{"T", "B"}) {
		t.Errorf("arguments = %q, want the title and body last", got)
	}
}

// Text is data, never script: whatever the title or body holds, the three script
// lines are the same, and the text appears only as its own argument.
func TestMacOS_TextWithQuotesAndScriptNeverReachesTheScript(t *testing.T) {
	evil := []string{
		`x" & (do shell script "touch /tmp/pwned") & "y`,
		`it's a \ test`,
		"line one\nline two",
		`"; end run; on run`,
		"",
	}
	var base []string
	for _, text := range evil {
		var calls []call
		n := &MacOS{Sound: "Glass", run: fakeRunner(&calls, "", nil)}
		_ = n.Notify(context.Background(), Event{Title: text, Body: text})
		args := calls[0].args
		script := args[:7] // -e line -e line -e line --
		if base == nil {
			base = append([]string(nil), script...)
		}
		if !reflect.DeepEqual(script, base) {
			t.Errorf("%q changed the script: %q", text, script)
		}
		if got := args[7:]; !reflect.DeepEqual(got, []string{text, text, "Glass"}) {
			t.Errorf("%q arrived as %q, want it unchanged", text, got)
		}
	}
}

func TestMacOS_AFailureIsAnErrorThatSaysWhyAndOsascriptMissingIsOne(t *testing.T) {
	var calls []call
	n := &MacOS{run: fakeRunner(&calls, "execution error: Not authorized (-1743)\n", errors.New("exit status 1"))}
	err := n.Notify(context.Background(), Event{})
	if err == nil || !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "Not authorized") {
		t.Errorf("err = %v, want one with the exit status and osascript's own message", err)
	}

	n = &MacOS{run: fakeRunner(&calls, "", exec.ErrNotFound)}
	if err := n.Notify(context.Background(), Event{}); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("err = %v, want one wrapping exec.ErrNotFound", err)
	}
}

func TestMacOS_GivesUpWhenItsContextIsDone(t *testing.T) {
	n := &MacOS{run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done() // the way exec.CommandContext kills a stuck osascript
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := n.Notify(ctx, Event{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context's deadline", err)
	}
}

// The unit tests above prove what we pass; this proves osascript treats it as
// data. It runs a script that echoes its argument, not display notification, so
// no banner is shown. It only runs on a Mac.
func TestMacOS_OsascriptReallyPassesArgumentsAsData(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("osascript is a macOS tool")
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skip("no osascript")
	}
	evil := `x" & (do shell script "exit 1") & "y`
	out, err := exec.Command("osascript", "-e", "on run argv", "-e", "return item 1 of argv", "-e", "end run", "--", evil).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != evil {
		t.Errorf("osascript echoed %q, %v; want the argument back unchanged", out, err)
	}
}
