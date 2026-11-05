# kaoyan-term

Terminal-based continuous vocabulary dictation tool disguised as an authentic Go backend microservice log stream.

## Features

- **No Overlapping Audio**: Audio process is dynamically managed via Context cancellation; tapping Tab or advancing words never overlaps sounds.
- **Bilingual Earphone Audio**: Plays native US English pronunciation followed by concise Chinese definition directly into your earphones (`you` -> `你，你们`).
- **Authentic Log Disguise**: Screen displays only real-looking Zap structured logs with timestamps, callers, and latency metrics.
- **On-Screen Hint Toggle (`Ctrl+V`)**: Toggle whether the business rule definition `expr="..."` is displayed in the debug log.
- **Instant Boss Key**: `Esc` or `Ctrl+C` wipes terminal and scrollback buffer completely.

## Controls

- `[Letters]`: Type token from earphone audio
- `[Space]`: Replay bilingual pronunciation (English + Chinese) in earphones
- `[Tab]` or `[?]`: Hint next letter (without audio collision)
- `[Ctrl+V]`: Toggle showing/hiding definition expr in logs
- `[Backspace]`: Delete character
- `[Esc]` or `[Ctrl+C]`: Instant clean exit
