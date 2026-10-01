# Desktop notifications for the bell: research

> This is the first note in `docs/research/`, the new home for research notes
> (findings gathered from primary sources, kept apart from decisions in `docs/adr/`
> and hand-off notes in `docs/handoffs/`).
>
> Written 2026-10-01 on macOS 26.5.2 (Darwin 25.5.0), tmux 3.7b, Terminal.app.
> Nothing here was changed in the code. No notification was fired while researching,
> and I cannot see the screen, so "a banner appears" is never confirmed here.
>
> Evidence labels: **[verified]** = I read the source or ran it on this machine;
> **[documented]** = the owner's docs/spec say so; **[unverified]** = secondary
> source, memory, or not testable here.

## TL;DR (recommendation)

1. **Do not build a daemon, and do not depend on a native app bundle.** A pure-Go `go install` binary cannot use `UNUserNotificationCenter` (needs a signed app bundle). Everything below is a shell-out or an escape sequence.
2. **macOS primary: `osascript` `display notification` with a `sound name`**, called through an injected `Notifier` interface, passing text as `argv` (never interpolated into the script). It works with zero installs, but it is attributed to **Script Editor**, and needs Notification permission granted to Script Editor once, or it fails silently. First-run setup must be documented. Optionally prefer `terminal-notifier` if on `PATH` (3.x is alive again, but see caveats) .
3. **Terminal escape sequence as the second mechanism, mainly for SSH and for users whose terminal already turns it into a banner:** OSC 9 (iTerm2, Ghostty, WezTerm, kitty, foot, Windows Terminal) or OSC 777 (`notify;title;body`, Ghostty, WezTerm, foot, Windows Terminal canary). Support is **not detectable** in general; the user must opt in. Inside tmux it needs `allow-passthrough on`, or `all` if the pane may be in a hidden window.
4. **Linux: `notify-send` with `-h string:sound-name:...`**, falling back to nothing (bell only). Whether a sound plays depends on the notification server (GNOME Shell and dunst are known to differ; unverified here).
5. **Keep the bell exactly as is** and add the notification on the same event: fire it on the **first ring of each bell event only**, not every repeat. The notification is an additional signal, never a replacement.
6. **Config (additive):** one new key, `notifications`, default `"off"` for the first release (matches the PRD "behind a config flag"). Values: `"off"`, `"auto"`, `"desktop"`, `"terminal"`.
7. Failure is invisible by nature on macOS (the command exits 0 when the banner is suppressed). Provide a `track` self-test command or a one-line notice, not a retry loop.

## Where the bell lives today (so a notifier fits)

- `internal/core/bell.go`: `BellKind` (`BellSessionEnd`, `BellBreakEnd`) and `Bell{Session, Kind, Scheduled}` [verified]. The core keeps no ack state.
- `internal/tui/model.go`: `(*Model).ring()` returns `tea.Raw("\a")` once per refresh when `snap.Bell.Scheduled > m.bell.rung` and the user has not pressed a key (`acknowledgeBell`). `ringState` holds `id`, `rung`, `acked` [verified].
- Options pattern: `tui.WithTick`, `WithFocusDuration`, `WithTheme`, `WithDocs` are `func(*Model)` options [verified].
- PRD [verified]: "Desktop (OS-level) notifications" and "Background daemon, or a bell when the app is closed" are v1 non-goals; v2 candidates: "Desktop notifications (behind a config flag) and possibly a background daemon". ADR 0002: core has no goroutines/timers. ADR 0003: config keys and commands are additive only.

A notification can therefore be added in `ring()` next to the bell without touching `internal/core`.

## Comparison table

| Mechanism | macOS | Linux | Over SSH | In tmux | Sound | Permissions / setup | Dependencies | Maintenance (2026-10) |
|---|---|---|---|---|---|---|---|---|
| `osascript display notification` | Yes, built in. Attributed to **Script Editor** | n/a | **No** (needs the user's GUI session; runs on the remote host) | Works (it is a child process, no passthrough) | `sound name "Glass"` etc. (names = files in `/System/Library/Sounds`) | Script Editor must have Notifications allowed; otherwise silent, exit 0 | None (system binary) | Apple, stable, but notification plumbing is tightening each release |
| `terminal-notifier` 3.x | Yes (UserNotifications) | n/a | No (3.0 notes: SSH/launchd exits with a diagnostic) | Works | `-sound NAME` / `default` | Own permission prompt on first run; own app identity | Homebrew install (or `make install`) | 3.0.0 on 2026-08-23 (first release in 9 yrs), 3.1.0 on 2026-08-30. Young after a long gap |
| `alerter` | Yes | n/a | No | Works | `--sound NAME` | Defaults to the Terminal identity (`com.apple.Terminal`) on Sequoia+ (26.4) | `brew install vjeantet/tap/alerter` | v26.5 on 2026-02-19, MIT, active |
| `afplay` | Sound only, no banner | n/a | No | Works | Plays any file; blocks until done | None | System binary | n/a |
| OSC 9 | iTerm2, Ghostty, WezTerm, kitty; **not** Terminal.app (unverified) | foot, Ghostty, WezTerm, kitty | **Yes** (travels to the local terminal) | Needs `allow-passthrough` and DCS wrap | Terminal decides (Ghostty macOS: default sound) | Terminal's own notification permission | None | iTerm2 doc'd; widely copied |
| OSC 777 `notify` | Ghostty, WezTerm | foot, Ghostty, WezTerm; GNOME VTE: no real notification (see Q3) | Yes | Same as OSC 9 | Terminal decides | Same | None | No formal spec |
| OSC 99 | kitty; foot; Ghostty has a parser, handler not found | kitty, foot | Yes | Same, plus multi-chunk sequences | **Spec-defined (`s=`)**, kitty only in practice | Same | None | Spec is versioned, with query support (`p=?`) |
| `notify-send` (libnotify) | n/a | Yes if a notification server runs | No (no session bus over plain SSH) | Works | `-h string:sound-name:...` (server dependent) | None | `libnotify-bin`/`libnotify` package | Stable |
| godbus to `org.freedesktop.Notifications` | n/a | Yes, pure Go | No | Works | `sound-name` / `sound-file` hints (server dependent) | None | `godbus/dbus` v5.2.2 (2025-12-29, BSD-2) | Active |
| beeep v0.11.2 | Shells to terminal-notifier then osascript | D-Bus (esiqveland/notify), then notify-send, then kdialog | No | Works | Beep only via bell/`beep`; `Alert` sets sound only on macOS | Same as underlying | See Q4 | Last release 2025-12-11 |
| `UNUserNotificationCenter` | Needs signed app bundle | n/a | n/a | n/a | Yes | Authorization prompt | Cgo/objc, bundle | Out of scope (Q6) |

## Q1. macOS `osascript display notification`

**Syntax [documented].** Apple's *Mac Automation Scripting Guide, Displaying Notifications*
(developer.apple.com/library/archive/documentation/LanguagesUtilities/Conceptual/MacAutomationScriptingGuide/DisplayNotifications.html):

```applescript
display notification "All graphics have been converted." with title "My Graphic Processing Script" subtitle "Processing is complete." sound name "Frog"
```

Parameters: message text (required), `with title`, `subtitle`, `sound name`. The guide does not say how to discover valid sound names; its example uses `"Frog"`.

**Valid sound names [verified on this machine].** `ls /System/Library/Sounds` on macOS 26.5.2 lists 14 files: Basso, Blow, Bottle, Frog, Funk, Glass, Hero, Morse, Ping, Pop, Purr, Sosumi, Submarine, Tink (`.aiff`). The name is the file name without extension. terminal-notifier's README says the same for its `-sound` ("Names come from the files in `/System/Library/Sounds`. Use `default` for the standard notification sound"). That `~/Library/Sounds` and `/Library/Sounds` are also searched is **[unverified]** (not tested; testing would play a sound). An unknown name: behaviour **[unverified]**, not tested because it would fire a notification.

**Attribution [documented, with a secondary confirmation].** Apple's guide: after a script shows a notification, "the script or Script Editor (if the script is run from within Script Editor) is added to the list of notifying apps" in Notifications settings, where the user picks alerts vs banners. From the command line the sender shows up as **Script Editor**, not the terminal: MacScripter thread "Trying to use Terminal for Display Notification" (macscripter.net/t/trying-to-use-terminal-for-display-notification/76593, Sequoia) **[unverified, secondary]**. The same thread reports the fix: run a `display notification` in Script Editor itself once to trigger the permission prompt, then `osascript` from a terminal works. I could not test attribution on Tahoe (macOS 26) without firing a banner, so the Sonoma/Sequoia/Tahoe behaviour is **[unverified]** for 26; the only first-party hint for Sequoia is in Q2 (alerter 26.4 release notes).

**This machine [verified, read-only].** `Script Editor.app` and `Terminal.app` are installed, but `defaults read com.apple.ncprefs` lists no entry for Script Editor, Terminal, iTerm, Ghostty or WezTerm. So on this Mac the first `osascript` notification would most likely hit the permission/first-use path. Not confirmed by running it.

**Permission and silent failure.** The secondary source above says that without permission `osascript` completes with no banner. alerter's 26.4 release notes (github.com/vjeantet/alerter/releases, 2026-02-17) state: "macOS 15 silently drops notifications from unrecognized app identities" **[documented by that project]**. So expect: exit code 0, nothing shown. There is no API to ask whether it was shown.

**Focus / Do Not Disturb.** Notifications from any sender go through Notification Center, and a Focus mode silences them. Apple's "Get notifications on Mac" page: "Use Focus to silence notifications when you need to concentrate" [documented, general]. terminal-notifier's `-diagnose` docs list "A Focus mode is on. Focus suppresses notifications silently" and "Scheduled Summary is collecting them into a digest" [documented]. Bypassing needs an Apple entitlement (terminal-notifier 3.0 removed `-ignoreDnD` for that reason). A sound is subject to the same suppression plus the per-app "Play sound for notifications" switch **[unverified; standard Settings behaviour]**.

**Terminal frontmost.** Not tested; **[unverified]**. Apple's notification settings page I fetched did not cover foreground behaviour.

**Quoting / injection [verified on this machine].** Building `display notification "<user text>"` by string formatting is an injection and breakage risk, because task titles are user text and AppleScript string syntax differs from Go's. Note that beeep builds the script with `fmt.Sprintf("... %q ...")` (Go quoting, e.g. `\u`/`\x` escapes that AppleScript does not understand; source in `notify_darwin.go`, beeep v0.11.2). Safe form, `osascript` documents that arguments after the script go to the `run` handler (`man osascript`):

```sh
osascript -e 'on run argv' \
  -e 'display notification (item 1 of argv) with title (item 2 of argv) sound name (item 3 of argv)' \
  -e 'end run' -- "$body" "$title" Glass
```

I ran the equivalent with a harmless `return` (no notification) and an argument of `x" & (do shell script "echo PWNED") & "`: it came back as inert text, `--` was consumed, and a second argument starting with `-` was fine. In Go use `exec.Command("osascript", "-e", ..., "-e", ..., "--", body, title, sound)` (no shell). Still validate `sound` against a fixed set (or the directory listing), since it is a script identifier, not just data.

## Q2. terminal-notifier, alerter, afplay

- **terminal-notifier** (github.com/julienXX/terminal-notifier): 3.0.0 on 2026-08-23 ("First release in nine years", rebuilt on UserNotifications because `NSUserNotification` "was deprecated in macOS 11 and had been breaking progressively since"), 3.1.0 on 2026-08-30 (scheduling). Requires macOS 10.14+. `-sender`, `-appIcon` and `-ignoreDnD` are now no-ops with a warning. Notable: **beeep still passes `-appIcon`** (no effect on 3.x), and `-sender` removal reflects that spoofing an identity no longer works. Install: `brew install terminal-notifier`, or `make install`; downloaded app bundles are quarantined (not notarized; README says `xattr -dr com.apple.quarantine`). It has its own permission prompt (asks once) and `-diagnose`. Over SSH or launchd it exits with a diagnostic instead of hanging. License shown as NOASSERTION by GitHub (not checked further). Verdict: could be an *optional* preferred backend if on `PATH` (it gives proper attribution as itself, plus `-sound`), but not something to require. Before 3.0 it had many years without a release, so treat its long-term health as unproven.
- **alerter** (github.com/vjeantet/alerter, MIT): v26.5 on 2026-02-19. Release 26.4 defaults to the Terminal identity (`com.apple.Terminal`) because of the Sequoia silent-drop behaviour. Install `brew install vjeantet/tap/alerter` (or MacPorts). Same optional-backend verdict.
- **afplay** [verified: `man afplay`, `/usr/bin/afplay`]: plays an audio file to the default output; no banner; synchronous, so run it in a goroutine/`Cmd`. Does not follow Focus. Useful only if sound must be separate from the notification (e.g. a different sound per BellKind, or sound when notifications are suppressed).

## Q3. Terminal escape sequences

### Specs and Go helpers

| Sequence | Format | Spec |
|---|---|---|
| OSC 9 | `ESC ] 9 ; <text> ST` | iTerm2: "To post a notification: OSC 9 ; [Message content goes here] ST" (iterm2.com/documentation-escape-codes.html) [documented]. In Ghostty, WezTerm, iTerm2 and Windows Terminal, OSC 9 is also the ConEmu family (`9;4` progress, `9;9` cwd, etc.): a message whose first `;` field is numeric is treated as ConEmu in iTerm2 (`VT100ScreenMutableState+TerminalDelegate.m`, `terminalPostUserNotification:`) and Ghostty (`osc9.zig`, ConEmu check first, falls back to notification) [verified]. **So never let a body start with `<digits>;`**; prefix with the title or text. |
| OSC 777 | `ESC ] 777 ; notify ; <title> ; <body> ST` | No spec. From the urxvt `notify` Perl extension; foot's source comment cites it (`osc.c`, `osc_notify`) and the terminal-wg thread (gitlab.freedesktop.org/terminal-wg/specifications/-/issues/13) says it is only defined by the Fedora VTE patch [documented, secondary]. WezTerm docs give `\e]777;notify;title;body\e\\` (wezterm.org/config/lua/config/notification_handling.html) [documented]. |
| OSC 99 | `ESC ] 99 ; <metadata> ; <payload> ST` | kitty spec (sw.kovidgoyal.net/kitty/desktop-notifications/; `docs/desktop-notifications.rst`) [documented]. Supports query (`i=<id>:p=?` returns supported keys `a`, `p`, `s`, `o`, `u`...), `s=` sound (`system`, `silent`, `error`, `warn`, `info`, `question`; "Terminals *should* support at least `system` and `silent`"), `o=always|unfocused|invisible` (since kitty 0.31.0), `a=focus,report`. The kitty spec also says kitty supports OSC 9 "legacy". |

**Go helpers [verified, module cache].** `github.com/charmbracelet/x/ansi@v0.11.7` (the version in `go.mod`) has, in `notification.go`:
`ansi.Notify(s)` = `"\x1b]9;" + s + "\x07"` (OSC 9) and `ansi.DesktopNotification(payload, metadata...)` (OSC 99). **There is no OSC 777 helper** (write `"\x1b]777;notify;"+title+";"+body+"\x07"` by hand; sanitize `;` and control characters). `passthrough.go` has `ansi.TmuxPassthrough(seq)`, which wraps in `\ePtmux;` with ESCs doubled (its comment: needs `allow-passthrough`), and `ansi.ScreenPassthrough`.

**Bubble Tea v2.0.10 [verified, module cache].** No built-in notification feature (grep for "notif" finds only `signal.Notify`). `tea.Raw(any) Cmd` returns a `RawMsg` that writes the string to the terminal untouched (`raw.go`), which is how `ring()` already emits `\a`. So: `tea.Raw(ansi.Notify(body))`, or `tea.Raw(ansi.TmuxPassthrough(ansi.Notify(body)))` inside tmux. `tea.RequestTerminalVersion` sends XTVERSION and delivers `tea.TerminalVersionMsg`, but needs `WithInput` and the terminal may not reply (`xterm.go`).

### Who implements what (as of 2026-10-01)

| Terminal | OSC 9 | OSC 777 | OSC 99 | Focus behaviour | Sound | Source |
|---|---|---|---|---|---|---|
| **Terminal.app** (Apple) | Not supported | Not supported | Not supported | n/a | n/a | **[unverified]**, closed source; no first-party doc found listing support |
| **iTerm2** | Yes | Not in its escape-code doc | No (not checked in source) | Profile setting "Notification Center alerts"; Filter Alerts has "Suppress alerts from active session" (iterm2.com/documentation-preferences-profiles-terminal.html) [documented]. Source gates on `config.postUserNotifications` | Per macOS notification settings (unverified) | doc + `VT100ScreenMutableState+TerminalDelegate.m` |
| **Ghostty** | Yes | Yes (`rxvt_extension.zig`) | Parser exists (`osc.zig`, `kitty_desktop_notification`), I found **no handler** wired in `stream_handler.zig` [verified in source on main; discussion ghostty-org/ghostty#10998 asks for it] | `desktop-notifications = true` by default (`Config.zig`, "applications running in the terminal can show desktop notifications using ... OSC 9 or OSC 777"). macOS code (`SurfaceView_AppKit.swift`, `showUserNotification`) posts with `UNNotificationSound.default`; if the surface is focused it removes the notification after 3 s, and on focus gain; whether the banner is *shown* while focused is **[unverified]** | Default sound on macOS (source) | Config.zig, Ghostty.App.swift |
| **WezTerm** | Yes | Yes | Not in the doc | `notification_handling`: default `AlwaysShow`; options `NeverShow`, `SuppressFromFocusedPane`, `SuppressFromFocusedTab`, `SuppressFromFocusedWindow` [documented] | macOS toast code uses UserNotifications and says "application must be code-signed" (`wezterm-toast-notification/src/macos.rs`); sound **[unverified]**. Note: latest tagged release is 2024-02-03; main is active | wezterm.org docs |
| **kitty** | Yes ("legacy") | Not stated | **Yes (owner of the spec)** | `o=` key; kitty honours `always|unfocused|invisible` | `s=` | kitty docs |
| **Alacritty** | No | No | No | n/a | n/a | Open request alacritty/alacritty#7105 "osc notification support" [verified via GitHub issue state] |
| **VS Code terminal** | Not found | Not found | Not found | n/a | n/a | **[unverified]**: my code searches returned nothing relevant; treat as unsupported |
| **Windows Terminal** | Unclear | **Added** (microsoft/terminal#20012/#7718 closed 2026-06-04; the setting is off by default, in Canary at the time of the thread; stable release timing not confirmed) | No | Spec expectation in #7718: only when in the background | Toast | GitHub issues (Windows is unsupported by Track anyway) |
| **GNOME Terminal / VTE** | No | VTE's `urxvt_extension` (`vteseq.cc`) handles `precmd`/`preexec`/`notify;Command completed` as shell-integration properties, gated on `enable_legacy_osc777()`; it shows **no generic notification** | No | n/a | n/a | VTE source [verified]; app-level behaviour of Fedora patches **[unverified]** |
| **foot** | Yes (`osc.c` case 9) | Yes (case 777) | Yes (case 99) | `desktop-notifications.inhibit-when-focused` (CHANGELOG) | Via its `desktop-notifications.command`, user-configured | foot source/CHANGELOG [verified] |

**Detecting support.** There is no portable query for OSC 9/777. Options: (a) kitty's OSC 99 query `i=1:p=?` answers only on terminals that implement OSC 99 (kitty, foot); silence means "unknown", not "no"; (b) XTVERSION (`tea.RequestTerminalVersion`) names the terminal if it answers; (c) env vars (`TERM_PROGRAM`, `KITTY_WINDOW_ID`, `GHOSTTY_RESOURCES_DIR`, `WEZTERM_EXECUTABLE`, `ITERM_SESSION_ID`) are conventions, not verified here except `TERM_PROGRAM=Apple_Terminal` on this machine. **Inside tmux all of these describe tmux or the stale environment**, and tmux answers XTVERSION itself [unverified: `man tmux` has no mention of XTVERSION or `TERM_PROGRAM`]. Conclusion: auto-detect for the *desktop* path (macOS/Linux), and treat the escape-sequence path as an explicit opt-in.

**Terminal vs OS notification focus.** The sequences let the terminal decide; most skip or suppress when focused (iTerm2 "Suppress alerts from active session", foot inhibit-when-focused, WezTerm configurable, kitty `o=`). That is a feature for Track: the TUI is already visible when focused, and the bell still rings.

### tmux

- `man tmux` on this machine (3.7b) [verified]: "`allow-passthrough [on | off | all]` Allow programs in the pane to bypass tmux using a terminal escape sequence (`\ePtmux;...\e\\`). If set to on, passthrough sequences will be allowed only if the pane is visible. If set to all, they will be allowed even if the pane is invisible."
- tmux FAQ (github.com/tmux/tmux/wiki/FAQ) [documented]: "Any `\033` characters in the wrapped sequence must be doubled", prefixed by `tmux;`; "As of tmux 3.3, the `allow-passthrough` option must be set to `on` or `all`". tmux `CHANGES`: the option was added "(default off)", and later "a third state `all`" [verified in CHANGES].
- Consequences: the user must set `set -g allow-passthrough on` (or `all`, if Track may run in a window they are not looking at, which is the main use case for a timer). Without it tmux silently discards the sequence. With tmux < 3.3 passthrough was on by default (my reading of the FAQ sentence; exact older default not verified).
- tmux does not forward a bare OSC 9/777 without the DCS wrap (it parses and drops unknown OSC; **[unverified]**, not tested because it could fire a notification in the outer terminal). The bell is different: tmux has `monitor-bell`, `bell-action`, `visual-bell` options and flags the window (`man tmux`), which is what the PRD's e2e check relies on.
- Detect tmux with `$TMUX`. Wrap with `ansi.TmuxPassthrough(...)`.
- Terminal.app users in tmux: nothing helps; the escape path does not exist there.

## Q4. Go libraries

| | beeep | esiqveland/notify | godbus/dbus | go-toast | gosx-notifier |
|---|---|---|---|---|---|
| Version / date | v0.11.2, 2025-12-11 (github.com/gen2brain/beeep) | v0.14.0, 2026-06-23 | v5.2.2, 2025-12-29 | v1.1.2 (dependency of beeep) | not evaluated, **unverified** |
| License | BSD-2-Clause | BSD-3-Clause | BSD-2-Clause | not checked | not checked |
| Pure Go? | Yes on macOS/Linux (shell-outs and D-Bus); `go.mod` pulls `go-toast`, `godbus`, `esiqveland/notify`, `jackmordaunt/icns`, `systray` (Windows), `golang.org/x/sys` | Yes (godbus) | Yes | Windows only | n/a |
| macOS | `terminal-notifier` if on PATH (`-title -message -group -appIcon [-sound default]`), else `osascript -e display notification %q with title %q [sound name "default"]` | n/a | n/a | n/a | n/a |
| Linux | D-Bus (esiqveland/notify), then `notify-send`, then `kdialog` (`nodbus` build tag disables D-Bus) | Direct D-Bus | D-Bus transport | n/a | n/a |
| Sound | `Alert` = urgent notify + `Beep`; on macOS the sound is `-sound default`/`sound name "default"`; on Linux `Beep` writes to the PC speaker device or falls back to BEL. No per-notification sound name option | Hints API (see Q5) | n/a | n/a | n/a |
| Known issues | Open issues #75/#76 propose switching macOS to alerter; #67 "not working for macOS Sequoia 15.2" (closed); #70 "Hanging on Notify". `-appIcon` no longer works with terminal-notifier 3.x. `%q` escaping (Q1). Pulls Windows deps into a macOS/Linux-only app's module graph | n/a | n/a | n/a | n/a |

(beeep docs via Context7 `/gen2brain/beeep`; source read from the repo at `notify_darwin.go`, `notify_unix.go`, `beep_darwin.go`, `beep_unix.go`.)

**Comparison with our own thin wrapper.** beeep gives no sound-name control, no argv safety on macOS, an extra dependency tree, and tries terminal-notifier first (which has just changed incompatibly). A ~60-line internal package that `exec`s `osascript` (argv form) and `notify-send`, behind an interface, gives exact control, keeps `go.mod` small, and is trivial to fake. **My judgement: write it ourselves; use `godbus` only if a Linux user needs sound hints that `notify-send` cannot send.**

## Q5. Linux

- **Spec [verified, fetched from specifications.freedesktop.org/notification/latest/, v1.3 of 2024-08-18]:** hints `"sound-file"` (path), `"sound-name"` ("A themeable named sound from the freedesktop.org sound naming specification ... to play when the notification pops up", e.g. `message-new-instant`), `"suppress-sound"` ("Causes the server to suppress playing any sounds, if it has that ability. This is usually set when the client itself is going to play its own sound"), `"urgency"` (byte). Server capability `"sound"`: "The server supports sounds on notifications. If returned, the server must support the `sound-file` and `sound-name` hints". "Neither clients nor notification servers are required to support any hints."
- **Sound names [verified, sound-naming spec]:** `message-new-instant`, `complete-download`, `alarm-clock-elapsed`, `dialog-information`, `bell-terminal`, `bell-window-system`, `dialog-error`. `alarm-clock-elapsed` or `message-new-instant` suit a timer.
- **Server differences [unverified here; from general knowledge, check before claiming in docs]:** KDE Plasma honours the sound hints; dunst has no built-in sound (use `-h`/scripts); GNOME Shell does not play the hint sounds itself. This is why a Linux sound may require a separate player. I could not verify these in this session.
- **No daemon / headless / SSH:** a D-Bus client fails when there is no session bus (no `DBUS_SESSION_BUS_ADDRESS`); `notify-send` exits non-zero. Over SSH there is normally no session bus for the SSH login. Handle by falling back to the bell only (the existing behaviour), never an error dialog. beeep's own fallback chain is D-Bus then notify-send then kdialog.
- **Sound players [unverified, not installed here]:** `canberra-gtk-play -i <event-id>` (libcanberra, event-sound-theme names), `paplay <file>` (PulseAudio/PipeWire). Neither is guaranteed present.
- `notify-send` is not installed on this macOS machine (`which` says so), so its flags (`-h string:sound-name:...`, `-a`, `-u`, `-t`) are **[unverified here]**; beeep passes `-a`, `-i`, `-t`, `-u` (source).

## Q6. Native macOS (UNUserNotificationCenter / NSUserNotification)

Out of scope for a `go install` binary. `NSUserNotification` is deprecated since macOS 11 and "had been breaking progressively" (terminal-notifier 3.0.0 release notes). `UNUserNotificationCenter` reads the sender's **real signed identity**: terminal-notifier 3.0 notes ("UserNotifications reads the real signed identity so there is nothing to override"), WezTerm's source comment ("the application must be code-signed for UNUserNotificationCenter to work"), and Ghostty and WezTerm both use it from signed `.app` bundles. A plain Mach-O from `go install` has no bundle ID, no signing, and would need cgo/objc. Apple's own page (developer.apple.com/documentation/usernotifications) is JavaScript-rendered and did not load via fetch, so the bundle requirement is **[documented by projects, not read in Apple's text]**.

## Q7. SSH / remote and when Track is not frontmost

- **Local desktop mechanisms** (`osascript`, `terminal-notifier`, `notify-send`, D-Bus) run on the machine where Track runs. Over SSH that is the remote host, which has no GUI session for your desktop: nothing appears (terminal-notifier 3.0 reports exactly this case: "Running over SSH or from launchd exits with a diagnostic").
- **Only terminal escape sequences cross SSH**, because the bytes travel the same pty to your local terminal. This also works in tmux, with passthrough (Q3), and tmux on the remote host additionally needs the `allow-passthrough` option.
- **Track not frontmost:** that is the case notifications are for. The bell alone rings in the terminal tab; macOS may bounce the Dock icon (Ghostty `bell-features=attention` is on by default, per `Config.zig`; iTerm2 can post a Notification Center alert on bell [documented in its profile docs]). A desktop notification fires regardless of focus (osascript/terminal-notifier), whereas escape-sequence notifications are often *suppressed when focused* by the terminal.
- **Track not running** (app closed): nothing; that is the "background daemon" non-goal.

## Design implications (my judgement)

**Mechanisms and fallback order**
1. `notifications = "auto"` on macOS: `terminal-notifier` if on `PATH` (3.x only: check `-version`) then `osascript`. On Linux: `notify-send` if on `PATH` and `DBUS_SESSION_BUS_ADDRESS` set. Over SSH (`SSH_CONNECTION` set, no local GUI): skip desktop path; use the terminal path only if explicitly enabled.
2. `"terminal"`: emit OSC 9 (and optionally OSC 777) through `tea.Raw`, wrapped with `ansi.TmuxPassthrough` when `$TMUX` is set. Opt-in because support cannot be detected.
3. Always keep the BEL. If every notification mechanism fails, behaviour equals today.

**Per-ring vs once.** Notify on the **first ring of each (Session, Kind)** only, i.e. when `m.bell.rung` goes 0 to >= 1. The bell repeats 11 times over 5 minutes; 11 banners would stack or be muted by macOS grouping, and a notification persists in Notification Center anyway. A second notification on the Break-end event is natural (new `bellID`). A `notify_repeats`-style knob can come later.

**Testability with the pull-based design.** Add in `internal/tui`:

```go
type Notification struct { Title, Body string; Kind core.BellKind }
type Notifier interface { Notify(context.Context, Notification) error }
func WithNotifier(n Notifier) Option { return func(m *Model) { m.notifier = n } }
```

`ring()` returns `tea.Batch(tea.Raw("\a"), notifyCmd)` where `notifyCmd` is a `tea.Cmd` that calls the notifier with a timeout (Cmds already run off the Update goroutine, so no new goroutines in the model or core, which keeps ADR 0002). Tests inject a fake that records calls, exactly as `WithTick` does, and assert "once per event, not on repeats, not after a keypress ack". The real notifiers live in a new package (e.g. `internal/notify`) with `exec.Command` behind a function field so command lines (including the argv form) can be unit tested without running anything. Terminal-sequence notifier returns `tea.Raw(...)` rather than an error.

**Config key shape.** One additive key: `notifications` (string), values `"off" | "auto" | "desktop" | "terminal"`; optional `notification_sound` (macOS name from the fixed list; default `"Glass"` or empty = silent). Default `"off"` first, per PRD "behind a config flag"; consider flipping the default to `"auto"` in a later minor version only with an ADR 0003 note (changing a default is not removal, but is a behaviour change). Validation style should match existing keys (`theme`, `bell_interval`), with the error listing allowed values. Env/CLI override flag (`--notify`) is optional.

**Silent failure.** Cannot be detected on macOS (exit 0). Mitigate: (1) a `track notify-test` subcommand (additive) that sends one notification and prints the permission steps ("System Settings > Notifications > Script Editor"); (2) log failures from exec errors to the existing `notice` line once per session; (3) document that Focus modes mute it and the bell still plays. Never retry in a loop.

**What NOT to do:** a background daemon or LaunchAgent; interpolating task text into AppleScript; using `UNUserNotificationCenter` or an app bundle; depending on `terminal-notifier` or `alerter` as a hard requirement; auto-sending OSC 9/777 to unknown terminals by default (unsupported terminals may print junk; Terminal.app and Alacritty do not support them); firing a notification on every repeat; putting goroutines or timers in `internal/core`; spoofing `com.apple.Terminal` as a sender.

## Open decisions for the user

1. **Default:** `notifications` off, or `auto`? (PRD says behind a flag; you asked to "take it up a notch".)
2. **macOS backend:** osascript only (zero install, attributed to Script Editor, needs one-time permission), or also prefer `terminal-notifier`/`alerter` when installed (own identity, but a Homebrew dependency and a young 3.x)? Do you want to document a Homebrew-tap `depends_on` or `caveats` line?
3. **Terminal-sequence path:** include it now (SSH and tmux users) or defer? If included, OSC 9 only, or OSC 9 + 777? Do we document the tmux `allow-passthrough` setting?
4. **First ring only, or every ring?** And a different message/sound for Break-end vs Session-end?
5. **Sound:** configurable macOS sound name (default?), and is a separate `afplay` sound path wanted when Focus mutes banners?
6. **Linux:** `notify-send` only, or a godbus implementation for proper sound hints? Is Linux notification support in scope for the first release?
7. **Notification content:** include the Task title in the body (user text and possible sensitive data in the notification centre and lock screen) or keep it generic ("Focus session ended")?
8. **Self-test command:** add `track notify-test` (new subcommand, additive)?
9. **ADR:** record this as ADR 0005 (UI-owned notifier, no daemon), updating the PRD non-goal wording, before implementing?
10. **Verification gap:** someone must manually confirm on this Mac (Tahoe 26.5.2) that `osascript` notifications from Terminal show as "Script Editor", what the permission prompt looks like, and whether it fires with the terminal frontmost. I could not do that without firing a notification.
