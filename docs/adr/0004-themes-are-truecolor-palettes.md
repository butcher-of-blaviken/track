# Themes are truecolor palettes beside an ANSI default

Track's colours come in two kinds. The **default** theme uses only the terminal's 16 ANSI colours, plus faint and reverse video, so it takes on whatever theme the terminal has and costs nothing to configure. The other themes are **palettes** of fixed RGB colours (One Dark, Gruvbox Dark, Solarized Dark and Light, One Light), chosen with one config key, `theme`, so Track looks the same in any terminal.

Both are needed. The default alone was not enough: ANSI blue is a very dark blue on many black terminals, and the cursor row, which was blue in reverse video, was hard to read there (#109). Fixes within the 16 colours (plain reverse video for the cursor row, bright blue for the accent on a dark background) make it readable everywhere, but cannot make it look the same everywhere, or give the accent-coloured cursor bar the design wants.

Rules that keep the palettes safe:

- **A palette must read.** Every colour on the palette's background, and the cursor bar's text on its fill, must be at least 3:1 (the WCAG minimum for bold and interface text), checked by a test. Where a scheme's own colours fail (Solarized Light's green, yellow and cyan, and the faint text of both Solarized schemes), the palette departs from it slightly rather than the bar being lowered.
- **A palette needs 256 colours.** Bubble Tea downsamples RGB to the nearest of 256, and the palettes are tested to stay above 2.5:1 that way. With 16 colours, no colour or `NO_COLOR` the default theme is used: the nearest 16 colours are arbitrary, and stripping a palette's colours would leave the cursor row unmarked.
- **A palette does not paint the terminal's background.** It assumes the matching terminal.
- **One additive config key.** It is optional, defaults to `default`, and follows ADR 0003.

## Considered Options

- **ANSI only, no theme key:** the smallest thing, and the user's terminal theme stays in charge. But the look depends entirely on the terminal, and bugs like #109 can only be avoided by a lowest common denominator.
- **Paint the whole screen's background:** a palette would then always match, but it fights the terminal's own background, transparency and image backgrounds, and leaves edges unpainted wherever the screen does not cover the terminal.
- **User-defined palettes in the config file:** the config would grow a table of colours that must be validated and tested for contrast. Left for later; the built-in names can stay as they are.
- **A key to cycle themes in the app, a `--theme` flag or an environment variable:** the other look setting, `layout`, is config-only, and none of these were needed.

## Consequences

A new palette is a few lines in `internal/tui/palettes.go`, its name in `config.Themes` (a test fails if the two lists differ) and a README mention. Its contrast is tested for free. A user who sets a palette that does not match their terminal gets colours that read badly, and the README says to pick the matching one.
