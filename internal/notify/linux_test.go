package notify

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// These tests talk to a real dbus-daemon on a private socket, with a fake
// org.freedesktop.Notifications server on it, so what is checked is the call a
// real server would receive. They cannot show how GNOME, KDE, dunst or mako draw
// a banner or play a sound. Without dbus-daemon they skip, except in CI (the CI
// environment variable), where a missing daemon is a failure so they cannot
// quietly stop running.

const busConfig = `<busconfig>
  <type>session</type>
  <listen>unix:path=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`

// startBus runs a private session bus and returns its address.
func startBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("dbus-daemon is not installed, and CI must run the D-Bus tests")
		}
		t.Skip("dbus-daemon is not installed")
	}
	dir, err := os.MkdirTemp("", "track-bus") // short: a unix socket path is limited to about 100 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "bus")
	config := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(config, []byte(fmt.Sprintf(busConfig, socket)), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(daemon, "--config-file="+config, "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	address, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("dbus-daemon printed no address: %v", err)
	}
	return strings.TrimSpace(address)
}

// notification is one Notify call as a server receives it.
type notification struct {
	appName    string
	replacesID uint32
	icon       string
	summary    string
	body       string
	actions    []string
	hints      map[string]dbus.Variant
	timeout    int32
}

// fakeServer is a notification server that records what it is asked to show.
type fakeServer struct {
	mu    sync.Mutex
	calls []notification
	block chan struct{} // if set, Notify waits for it to close, like a stuck server
}

// Notify is org.freedesktop.Notifications.Notify.
func (s *fakeServer) Notify(appName string, replacesID uint32, icon, summary, body string, actions []string, hints map[string]dbus.Variant, timeout int32) (uint32, *dbus.Error) {
	if s.block != nil {
		<-s.block
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, notification{appName, replacesID, icon, summary, body, actions, hints, timeout})
	return uint32(len(s.calls)), nil
}

func (s *fakeServer) received() []notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]notification(nil), s.calls...)
}

// serve puts a fake notification server on the bus.
func serve(t *testing.T, address string, s *fakeServer) {
	t.Helper()
	conn, err := dbus.Dial(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Auth(nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Hello(); err != nil {
		t.Fatal(err)
	}
	if err := conn.Export(s, notificationsPath, notificationsDest); err != nil {
		t.Fatal(err)
	}
	if reply, err := conn.RequestName(notificationsDest, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("could not own %s: %v, %v", notificationsDest, reply, err)
	}
}

func TestLinux_CallsNotifyWithTheEventAndASoundHint(t *testing.T) {
	address := startBus(t)
	server := &fakeServer{}
	serve(t, address, server)

	n := &Linux{Sound: DefaultLinuxSound, address: address}
	if err := n.Notify(context.Background(), Event{Title: "Track", Body: "Focus complete — Break started"}); err != nil {
		t.Fatal(err)
	}
	got := server.received()
	if len(got) != 1 {
		t.Fatalf("server got %d notifications, want 1", len(got))
	}
	c := got[0]
	if c.appName != "Track" || c.replacesID != 0 || c.icon != "" || c.summary != "Track" || c.body != "Focus complete — Break started" {
		t.Errorf("notification = %+v, want app Track, new (replaces 0), no icon, the event's title and body", c)
	}
	if len(c.actions) != 0 || c.timeout != -1 {
		t.Errorf("actions %v, timeout %d; want none, and -1 (the server's default)", c.actions, c.timeout)
	}
	if hint, ok := c.hints["sound-name"]; !ok || hint.Value() != DefaultLinuxSound || len(c.hints) != 1 {
		t.Errorf("hints = %v, want only sound-name %q", c.hints, DefaultLinuxSound)
	}
}

func TestLinux_TextIsDataAndNoSoundSendsNoHint(t *testing.T) {
	address := startBus(t)
	server := &fakeServer{}
	serve(t, address, server)

	n := &Linux{address: address}
	text := `<b>x</b> "quoted" ; & 'single' \ ` + "\nnew line"
	if err := n.Notify(context.Background(), Event{Title: text, Body: text}); err != nil {
		t.Fatal(err)
	}
	c := server.received()[0]
	if c.summary != text || c.body != text {
		t.Errorf("text arrived as %q / %q, want it unchanged", c.summary, c.body)
	}
	if len(c.hints) != 0 {
		t.Errorf("hints = %v, want none when there is no sound", c.hints)
	}
}

func TestLinux_ABusWithNoNotificationServerIsErrNoServer(t *testing.T) {
	address := startBus(t) // nobody owns org.freedesktop.Notifications
	n := &Linux{Sound: DefaultLinuxSound, address: address}
	if err := n.Notify(context.Background(), Event{Title: "T"}); !errors.Is(err, ErrNoServer) {
		t.Errorf("err = %v, want ErrNoServer", err)
	}
}

func TestLinux_NoBusAtTheAddressIsErrNoBusAndPromptly(t *testing.T) {
	n := &Linux{address: "unix:path=" + filepath.Join(t.TempDir(), "no-such-bus")}
	start := time.Now()
	if err := n.Notify(context.Background(), Event{Title: "T"}); !errors.Is(err, ErrNoBus) {
		t.Errorf("err = %v, want ErrNoBus", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("took %v to find there is no bus", took)
	}
}

// With no session bus to find, godbus would run dbus-launch to start one, which
// on a headless machine spawns a daemon as a side effect of a notification. The
// notifier must not, and must not depend on what the machine running the test has
// (a CI runner has a real session bus at /run/user/<uid>/bus), so it is given an
// environment with none, and a stand-in dbus-launch on PATH records whether it ran.
func TestLinux_NoSessionBusIsErrNoBusAndNeverLaunchesOne(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "launched")
	script := "#!/bin/sh\necho launched > " + marker + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "dbus-launch"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	for name, env := range map[string]map[string]string{
		"nothing set":     {},
		"autolaunch":      {"DBUS_SESSION_BUS_ADDRESS": "autolaunch:"},
		"empty runtime":   {"XDG_RUNTIME_DIR": t.TempDir()},
		"missing runtime": {"XDG_RUNTIME_DIR": filepath.Join(t.TempDir(), "gone")},
	} {
		n := NewLinux()
		n.getenv = func(k string) string { return env[k] }
		if err := n.Notify(context.Background(), Event{Title: "T"}); !errors.Is(err, ErrNoBus) {
			t.Errorf("%s: err = %v, want ErrNoBus", name, err)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the notifier ran dbus-launch, which starts a new bus daemon")
	}
}

func TestSessionBusAddress_IsTheEnvironmentsThenSystemdsSocketAndNothingElse(t *testing.T) {
	runtime, err := os.MkdirTemp("", "track-rt") // short: a unix socket path is limited to about 100 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtime) })
	socket, err := net.Listen("unix", filepath.Join(runtime, "bus")) // what systemd leaves there
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = socket.Close() })
	for name, tc := range map[string]struct {
		env  map[string]string
		want string
		fail bool
	}{
		"the specification's variable": {map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/x/bus"}, "unix:path=/x/bus", false},
		"it wins over the runtime dir": {map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/x/bus", "XDG_RUNTIME_DIR": runtime}, "unix:path=/x/bus", false},
		"systemd's socket":             {map[string]string{"XDG_RUNTIME_DIR": runtime}, "unix:path=" + filepath.Join(runtime, "bus"), false},
		"autolaunch is none":           {map[string]string{"DBUS_SESSION_BUS_ADDRESS": "autolaunch:"}, "", true},
		"nothing":                      {map[string]string{}, "", true},
	} {
		got, err := sessionBusAddress(func(k string) string { return tc.env[k] })
		if (err != nil) != tc.fail || got != tc.want {
			t.Errorf("%s: = %q, %v; want %q, failure %v", name, got, err, tc.want, tc.fail)
		}
	}
}

func TestLinux_GivesUpWhenItsContextIsDone(t *testing.T) {
	address := startBus(t)
	server := &fakeServer{block: make(chan struct{})}
	serve(t, address, server)
	t.Cleanup(func() { close(server.block) })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := (&Linux{address: address}).Notify(ctx, Event{Title: "T"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context's deadline", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("a stuck server held Notify for %v", took)
	}
}

func TestLinux_NameAndDefaults(t *testing.T) {
	n := NewLinux()
	if n.Name() == "" || n.Sound != DefaultLinuxSound {
		t.Errorf("NewLinux = %+v (%q), want a name and the default sound", n, n.Name())
	}
	if !reflect.DeepEqual(NameOf(n), n.Name()) {
		t.Errorf("NameOf = %q, want %q", NameOf(n), n.Name())
	}
}
