# kaoyan-term

Terminal-based continuous vocabulary dictation tool disguised as an authentic Go backend microservice log stream.

## Features

- **Spaced Repetition (Ebbinghaus)**: Automatically schedules reviews across 1d, 2d, 4d, 7d, 15d, and 30d intervals.
- **Smart Queue (New vs Review)**: Automatically prioritizes today's due review words before introducing daily new words.
- **Progress Tracking & Dashboard**:
  - Run `kaoyan -status` to view visual study progress, completion rates, and daily review quotas.
  - While running, logs display disguised `offset="17/6357"` and `type="fresh"|"retry"`.
  - Press `Ctrl+P` in-session for an instant telemetry progress sync log.
- **No Overlapping Audio**: Process lifecycle managed via Context cancellation.
- **Bilingual Earphone Audio**: Plays native US pronunciation followed by concise Chinese definition directly into earphones.
- **Instant Boss Key**: `Esc` or `Ctrl+C` wipes terminal and scrollback buffer completely.

## CLI Usage

```bash
# 1. Start continuous practice (smart queue: reviews first, then new words)
kaoyan

# 2. View comprehensive 2027 NETEM review dashboard
kaoyan -status

# 3. Practice today's due reviews only
kaoyan -review

# 4. Practice error book words only
kaoyan -wrong

# 5. Start from specific word offset
kaoyan -start 100
```

## In-Session Controls

- `[Letters]`: Type token from earphone audio
- `[Space]`: Replay bilingual audio (English + Chinese) in earphones
- `[Tab]` or `[?]`: Hint next letter
- `[Ctrl+P]`: Print disguised telemetry progress stats in log
- `[Ctrl+V]`: Toggle showing/hiding definition expr in logs
- `[Backspace]`: Delete character
- `[Esc]` or `[Ctrl+C]`: Instant clean exit
