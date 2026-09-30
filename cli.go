package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/butcher-of-blaviken/track/internal/config"
	"github.com/butcher-of-blaviken/track/internal/core"
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
