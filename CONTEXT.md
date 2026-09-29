# Track

A terminal app for tracking the things you're working on, timing focused work on them, and leaving yourself notes about where you left off.

## Language

**Task**:
A unit of work you're tracking. Long-lived; may span many days and many Focus sessions.
Is always in one of three states: **Active** (shown by default), **Done** (hidden by default, searchable, reopenable), or **Archived** (abandoned; hidden unless explicitly requested). History and time totals are kept in every state.
_Avoid_: Item, ticket, todo

**Tag**:
A flat label attached to a Task, many-to-many, created on first use by writing `##name` in the Task's text; the marker is stripped from the Task's title. Case-insensitive, displayed with the casing of first use. No hierarchy and no separate Project concept.
_Avoid_: Label, category, project

**Focus session**:
One timed block of work spent on exactly one Task. Always belongs to a Task. Ends either **completed** (ran its full length) or **stopped early** (ended by you, keeping the elapsed time). It cannot be paused; resuming work means starting a new Focus session.
_Avoid_: Pomodoro, timer, sprint

**Break**:
A timed rest that starts automatically when a Focus session completes; a session stopped early does not trigger one. While it runs, starting a new Focus session is blocked unless you confirm an override, which is recorded.
_Avoid_: Rest, pause, cooldown

**Hand-off note**:
A one-line note saying where you left off, prompted when a Focus session ends (including early stops) and skippable. Appended to the Task's log as a timestamped entry, never overwriting earlier ones. Can also be added to a Task ad hoc, with no Focus session.
_Avoid_: Comment, memo, status

**Unfiled note**:
A note captured with no Task, later filed onto a Task. Once filed it is an ordinary log entry, timestamped from when it was written.
_Avoid_: Inbox item, scratch note, draft
