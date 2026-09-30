package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func build(version string, settings ...string) *debug.BuildInfo {
	info := &debug.BuildInfo{Main: debug.Module{Version: version}}
	for i := 0; i+1 < len(settings); i += 2 {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return info
}

func TestVersionString(t *testing.T) {
	const rev = "0123456789abcdef0123456789abcdef01234567"
	for name, tc := range map[string]struct {
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		"installed at a tag":        {build("v1.0.0"), true, "track v1.0.0"},
		"go install @latest":        {build("v1.2.3"), true, "track v1.2.3"},
		"a pseudo-version is kept":  {build("v0.0.0-20260930120000-0123456789ab+dirty"), true, "track v0.0.0-20260930120000-0123456789ab+dirty"},
		"devel with a commit":       {build("(devel)", "vcs.revision", rev, "vcs.modified", "false"), true, "track (devel) 0123456"},
		"devel with local changes":  {build("(devel)", "vcs.revision", rev, "vcs.modified", "true"), true, "track (devel) 0123456-dirty"},
		"devel without vcs info":    {build("(devel)"), true, "track (devel)"},
		"empty version, vcs info":   {build("", "vcs.revision", rev), true, "track (devel) 0123456"},
		"a short revision is whole": {build("(devel)", "vcs.revision", "abc"), true, "track (devel) abc"},
		"no build info":             {nil, false, "track (unknown version)"},
	} {
		if got := versionString(tc.info, tc.ok); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

func TestRealMain_VersionFlagAndSubcommandPrintTheVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"version"}, {"--data-dir", t.TempDir(), "version"}} {
		var out, errOut strings.Builder
		code := realMain(args, env(nil), &out, &errOut)
		if code != 0 || !strings.HasPrefix(out.String(), "track ") || strings.Count(out.String(), "\n") != 1 || errOut.Len() != 0 {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, out.String(), errOut.String())
		}
	}
}

func TestRealMain_VersionTakesNoArguments(t *testing.T) {
	var out, errOut strings.Builder
	if code := realMain([]string{"version", "now"}, env(nil), &out, &errOut); code != exitUsage || out.Len() != 0 || errOut.Len() == 0 {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
}

func TestUsage_MentionsTheVersion(t *testing.T) {
	var b strings.Builder
	usage(&b)
	if !strings.Contains(b.String(), "--version") {
		t.Errorf("usage lacks --version:\n%s", b.String())
	}
}
