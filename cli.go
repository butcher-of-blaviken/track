package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/butcher-of-blaviken/track/internal/config"
	"github.com/butcher-of-blaviken/track/internal/core"
	"github.com/butcher-of-blaviken/track/internal/export"
)

// command is a non-interactive subcommand that takes some free text.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, t *core.Tracker, text string, stdout io.Writer) error
}

var commands = []command{
	{name: "add", summary: "create a Task from the text; ##tag words become Tags", run: runAdd},
	{name: "note", summary: "save the text as a note in the inbox, to file onto a Task later", run: runNote},
}

func findCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

func commandUsage(w io.Writer, c command) {
	_, _ = fmt.Fprintf(w, "Usage: track %[1]s <text>\n\n%[2]s.\nThe text may be several words; quote it (in fish an unquoted ##tag starts a comment),\nand use -- before text that starts with a dash. Flags go before the subcommand:\ntrack --data-dir DIR %[1]s <text>\n", c.name, c.summary)
}

// runCommand runs a subcommand with the words after its name. Subcommands need
// only the database, so they never read the config file.
func runCommand(c command, words []string, opts options, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("track "+c.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	if err := fs.Parse(words); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			commandUsage(stderr, c)
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "track %s: %v\nRun 'track %s -h' for usage.\n", c.name, err, c.name)
		return exitUsage
	}
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintf(stderr, "track %s: needs some text\n", c.name)
		commandUsage(stderr, c)
		return exitUsage
	}

	a, err := openApp(opts.dataDir, getenv, config.Defaults())
	if err != nil {
		return report(err, stderr)
	}
	defer func() { _ = a.close() }()
	return report(c.run(context.Background(), a.tracker, strings.Join(fs.Args(), " "), stdout), stderr)
}

func runAdd(ctx context.Context, t *core.Tracker, text string, stdout io.Writer) error {
	task, err := t.AddTask(ctx, text)
	if errors.Is(err, core.ErrEmptyTitle) {
		return errors.New("a task needs a title (tags alone don't count)")
	}
	if err != nil {
		return err
	}
	line := task.Title
	if len(task.Tags) > 0 {
		chips := make([]string, len(task.Tags))
		for i, tag := range task.Tags {
			chips[i] = "#" + tag
		}
		line += "  " + strings.Join(chips, " ")
	}
	_, err = fmt.Fprintf(stdout, "Added task %d: %s\n", task.ID, line)
	return err
}

func runNote(ctx context.Context, t *core.Tracker, text string, stdout io.Writer) error {
	note, err := t.AddUnfiledNote(ctx, text)
	if errors.Is(err, core.ErrEmptyNote) {
		return errors.New("write a note")
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Saved note %d to the inbox\n", note.ID)
	return err
}

func exportUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: track export [--format json|markdown] [--output FILE]

Write every Task, Focus session and note, in every state, as JSON (the default)
or Markdown, to stdout or to FILE. The file is written in one piece, so a failed
export never leaves half of one. Flags go before the subcommand:
track --data-dir DIR export
`)
}

// runExport is `track export`. Like add and note it needs only the database,
// and it only reads it, so it is safe while the app is open.
func runExport(words []string, opts options, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("track export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	format := fs.String("format", "json", "")
	output := fs.String("output", "", "")
	fs.StringVar(output, "o", "", "")
	fs.StringVar(format, "f", "json", "")
	if err := fs.Parse(words); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			exportUsage(stderr)
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "track export: %v\nRun 'track export -h' for usage.\n", err)
		return exitUsage
	}
	var render func(io.Writer, core.Export) error
	switch strings.ToLower(*format) {
	case "json":
		render = export.JSON
	case "markdown", "md":
		render = export.Markdown
	default:
		_, _ = fmt.Fprintf(stderr, "track export: unknown format %q (json or markdown)\nRun 'track export -h' for usage.\n", *format)
		return exitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "track export: unexpected argument %q\nRun 'track export -h' for usage.\n", fs.Arg(0))
		return exitUsage
	}

	a, err := openApp(opts.dataDir, getenv, config.Defaults())
	if err != nil {
		return report(err, stderr)
	}
	defer func() { _ = a.close() }()
	data, err := a.tracker.Export(context.Background())
	if err != nil {
		return report(err, stderr)
	}
	if *output == "" {
		return report(render(stdout, data), stderr)
	}
	return report(writeFileAtomically(*output, func(w io.Writer) error { return render(w, data) }), stderr)
}

// writeFileAtomically writes to a temporary file beside path and renames it
// into place, so path is either untouched or complete.
func writeFileAtomically(path string, write func(io.Writer) error) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = write(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func reportUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: track report [--day today|yesterday|YYYY-MM-DD] [--standup] [--format markdown|json] [--output FILE]

What you worked on in a day: the tasks finished, the tasks worked on with their
focused time and the notes written on them, and the notes that belong to no task.
Markdown (the default) is ready to paste into a standup update or a message.

  --day D         the day to report: today (the default), yesterday or a date
                  written YYYY-MM-DD. Days are local time.
  --standup       yesterday and today so far, the usual scope of a standup
                  update; it cannot be combined with --day
  --format, -f    markdown (or md) or json
  --output, -o    write FILE, in one piece, instead of stdout

It only reads the database, so it is safe while the app is open. Flags go
before the subcommand: track --data-dir DIR report
`)
}

// parseReportDay is a --day value: today, yesterday or a YYYY-MM-DD date, which
// must not be after today.
func parseReportDay(text string, now time.Time) (time.Time, error) {
	var day time.Time
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "today":
		return now, nil
	case "yesterday":
		return now.AddDate(0, 0, -1), nil
	default:
		var err error
		if day, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(text), now.Location()); err != nil {
			return time.Time{}, fmt.Errorf("unknown day %q (use today, yesterday or a date like 2026-09-30)", text)
		}
	}
	if y, m, d := now.Date(); day.After(time.Date(y, m, d, 0, 0, 0, 0, now.Location())) {
		return time.Time{}, fmt.Errorf("%s has not happened yet", text)
	}
	return day, nil
}

// runDayReport is `track report`. Like export it needs only the database and
// only reads it.
func runDayReport(words []string, opts options, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("track report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	dayText := fs.String("day", "today", "")
	standup := fs.Bool("standup", false, "")
	format := fs.String("format", "markdown", "")
	fs.StringVar(format, "f", "markdown", "")
	output := fs.String("output", "", "")
	fs.StringVar(output, "o", "", "")
	usageErr := func(format string, args ...any) int {
		_, _ = fmt.Fprintf(stderr, "track report: "+format+"\nRun 'track report -h' for usage.\n", args...)
		return exitUsage
	}
	if err := fs.Parse(words); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			reportUsage(stderr)
			return 0
		}
		return usageErr("%v", err)
	}
	var render func(io.Writer, []core.Report) error
	switch strings.ToLower(*format) {
	case "markdown", "md":
		render = export.ReportMarkdown
	case "json":
		render = export.ReportJSON
	default:
		return usageErr("unknown format %q (markdown or json)", *format)
	}
	if fs.NArg() > 0 {
		return usageErr("unexpected argument %q", fs.Arg(0))
	}
	dayGiven := false
	fs.Visit(func(f *flag.Flag) { dayGiven = dayGiven || f.Name == "day" })
	if *standup && dayGiven {
		return usageErr("--standup already means yesterday and today; drop --day")
	}

	a, err := openApp(opts.dataDir, getenv, config.Defaults())
	if err != nil {
		return report(err, stderr)
	}
	defer func() { _ = a.close() }()
	now := a.tracker.Now()
	var days []time.Time
	if *standup {
		days = []time.Time{now.AddDate(0, 0, -1), now}
	} else {
		day, err := parseReportDay(*dayText, now)
		if err != nil {
			return usageErr("%v", err)
		}
		days = []time.Time{day}
	}
	reports := make([]core.Report, 0, len(days))
	for _, day := range days {
		r, err := a.tracker.Day(context.Background(), day)
		if err != nil {
			return report(err, stderr)
		}
		reports = append(reports, r)
	}
	if *output == "" {
		return report(render(stdout, reports), stderr)
	}
	return report(writeFileAtomically(*output, func(w io.Writer) error { return render(w, reports) }), stderr)
}
