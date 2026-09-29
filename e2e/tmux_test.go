//go:build e2e

// Package e2e drives the real binary inside tmux and asserts on what a user
// would see. Assertions match stable labels and shapes, never layout or style,
// so the UI can be redesigned without rewriting them.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var binary string

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Println("tmux not installed; skipping e2e tests")
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "track-e2e-bin")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "track")
	build := exec.Command("go", "build", "-o", binary, "..")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Println("building the binary failed:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// term is one tmux session running the app in an 80x24 pseudo-terminal, on a
// private tmux server so it cannot touch the user's own sessions.
type term struct {
	t      *testing.T
	socket string
}

// launch starts the app with the given environment and arguments.
func launch(t *testing.T, env []string, args ...string) *term {
	t.Helper()
	tm := &term{t: t, socket: fmt.Sprintf("track-e2e-%d-%d", os.Getpid(), time.Now().UnixNano())}
	t.Cleanup(func() { _ = tm.tmux("kill-server").Run() })

	cmd := []string{"new-session", "-d", "-s", "app", "-x", "80", "-y", "24"}
	for _, e := range env {
		cmd = append(cmd, "-e", e)
	}
	cmd = append(cmd, "--", binary)
	cmd = append(cmd, args...)
	if out, err := tm.tmux(cmd...).CombinedOutput(); err != nil {
		t.Fatalf("starting tmux: %v\n%s", err, out)
	}
	// Keep the pane around after the app exits so its exit status can be read.
	_ = tm.tmux("set-option", "-t", "app", "remain-on-exit", "on").Run()
	return tm
}

func (tm *term) tmux(args ...string) *exec.Cmd {
	return exec.Command("tmux", append([]string{"-L", tm.socket, "-f", "/dev/null"}, args...)...)
}

func (tm *term) screen() string {
	out, err := tm.tmux("capture-pane", "-p", "-t", "app").Output()
	if err != nil {
		return "(no screen: " + err.Error() + ")"
	}
	return string(out)
}

// waitUntil polls the screen until ok returns true, and otherwise fails with
// the last screen so a red test shows what the user would have seen.
func (tm *term) waitUntil(what string, timeout time.Duration, ok func(screen string) bool) string {
	tm.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		screen := tm.screen()
		if ok(screen) {
			return screen
		}
		if time.Now().After(deadline) {
			tm.t.Fatalf("timed out after %v waiting for %s; last screen:\n%s", timeout, what, screen)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (tm *term) waitFor(pattern string, timeout time.Duration) string {
	tm.t.Helper()
	re := regexp.MustCompile(pattern)
	return tm.waitUntil("/"+pattern+"/", timeout, re.MatchString)
}

func (tm *term) press(keys ...string) {
	tm.t.Helper()
	if out, err := tm.tmux(append([]string{"send-keys", "-t", "app"}, keys...)...).CombinedOutput(); err != nil {
		tm.t.Fatalf("send-keys: %v\n%s", err, out)
	}
}

// typeText types text literally, as if the user typed it.
func (tm *term) typeText(text string) {
	tm.t.Helper()
	if out, err := tm.tmux("send-keys", "-l", "-t", "app", text).CombinedOutput(); err != nil {
		tm.t.Fatalf("send-keys: %v\n%s", err, out)
	}
}

// exitStatus waits for the app to exit and returns its status.
func (tm *term) exitStatus(timeout time.Duration) int {
	tm.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		out, err := tm.tmux("display-message", "-p", "-t", "app", "#{pane_dead} #{pane_dead_status}").Output()
		if f := strings.Fields(string(out)); err == nil && len(f) >= 1 && f[0] == "1" {
			status := 0
			if len(f) == 2 {
				status, _ = strconv.Atoi(f[1])
			}
			return status
		}
		if time.Now().After(deadline) {
			tm.t.Fatalf("the app did not exit within %v; last screen:\n%s", timeout, tm.screen())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
