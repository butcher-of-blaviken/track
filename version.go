package main

import (
	"runtime/debug"
)

// versionString is the line `track --version` prints. `go install ...@v1.0.0`
// stamps the module version into the binary; a build from a checkout has
// "(devel)" (or a pseudo-version), so the commit says which one it was.
func versionString(info *debug.BuildInfo, ok bool) string {
	if !ok || info == nil {
		return "track (unknown version)"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return "track " + v
	}
	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	out := "track (devel)"
	if revision != "" {
		out += " " + revision[:min(len(revision), 7)]
		if modified {
			out += "-dirty"
		}
	}
	return out
}
