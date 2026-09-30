package main

import (
	_ "embed"
	"fmt"
	"io"
)

// readmeText is the README the binary carries, so the docs match the version
// that is installed. The app shows it with H, and `track docs` prints it.
//
//go:embed README.md
var readmeText string

// printDocs is `track docs`.
func printDocs(extra []string, stdout, stderr io.Writer) int {
	if len(extra) > 0 {
		_, _ = fmt.Fprintf(stderr, "track docs: unexpected argument %q\nRun 'track --help' for usage.\n", extra[0])
		return exitUsage
	}
	_, err := io.WriteString(stdout, readmeText)
	return report(err, stderr)
}
