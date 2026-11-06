# kaoyan-term

Terminal-based continuous vocabulary dictation tool disguised as an authentic Go backend microservice log stream.

## Features

- **No Overlapping Audio**: Audio process lifecycle managed via Context cancellation.
- **Bilingual Earphone Audio**: Plays native US pronunciation followed by concise Chinese definition directly into earphones.
- **Compact Log Formatting**: Compact log output (< 65 cols) prevents line-wrapping in split IDE terminals.
- **Strict Submission Flow**:
  - `[Letters]`: Freely type tokens in-place.
  - `[Tab]` or `[?]`: Hints/completes the next letter without skipping words.
  - `[Enter]`: Submits and validates token (logs status=200 on success, status=401 retry on mismatch).
  - `[Ctrl+K]` or `[Ctrl+E]`: Permanently bypass/master simple words.
  - `[Space]`: Replays audio in headphones.
  - `[Ctrl+V]`: Toggles definition expr on screen.
  - `[Ctrl+P]`: Prints progress telemetry in log.
  - `[Esc]` or `[Ctrl+C]`: Instant clean exit.

## CLI Usage

```bash
# 1. Start continuous practice (reviews first, then new words)
kaoyan

# 2. View 2027 NETEM review dashboard
kaoyan -status

# 3. Practice today's due reviews only
kaoyan -review

# 4. Practice error book words only
kaoyan -wrong

# 5. Start from specific word offset
kaoyan -start 100
```
