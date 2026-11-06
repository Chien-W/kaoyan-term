# kaoyan-term

A high-performance terminal vocabulary learning CLI designed for the 2027 Chinese Master's Entrance Exam (NETEM), featuring Baicizhan-style rich learning views and authentic Go backend disguises.

## Features

- **Baicizhan-Style Rich Vocabulary View**:
  - Word display with accurate US/UK international phonetics.
  - Multi-POS classification (`v.`, `n.`, `adj.`, `pron.`) with structured Chinese meanings.
  - Authentic考研真题 & high-frequency context sentences with Chinese translations.
  - Key exam phrases and collocations.
- **Dual Visual Modes (Seamless Hotkey Switching)**:
  - **Card View (Default)**: Clean, focused developer-styled ASCII card.
  - **Go Test Disguise (`Ctrl+T`)**: Formats all vocabulary fixtures into realistic `=== RUN TestMetricCollector` outputs.
  - **Stealth Microservice Log (`Ctrl+V`)**: Instantly purges screen into pure English Zap/Logrus logs.
- **No Overlapping Audio**: Audio process lifecycle managed via Context cancellation.
- **Bilingual Earphone Audio**: Plays native US pronunciation followed by concise Chinese definition directly into earphones.
- **Interactive Workflow**:
  - `[Letters]`: Freely type tokens with real-time cyan/red feedback.
  - `[Tab]` or `[?]`: Hints the next letter without advancing words.
  - `[Enter]`: Submits and validates token (`status=200` pass vs `status=401 retry`).
  - `[Ctrl+K]`: Permanently bypasses/kills simple words (斩词秒杀).
  - `[Space]`: Replays bilingual pronunciation in headphones.
  - `[Ctrl+T]`: Toggles between Baicizhan Card view and Go Test disguise.
  - `[Ctrl+V]`: Toggles Stealth microservice log mode (with scrollback wipe).
  - `[Ctrl+P]`: Prints real-time learning progress telemetry log.
  - `[Esc]` or `[Ctrl+C]`: Boss key for instant screen purge and exit.

## CLI Usage

```bash
# 1. Start continuous practice (default: Baicizhan Card View)
kaoyan

# 2. Start in Go Test disguise mode
kaoyan -mode test

# 3. Start in stealth microservice log mode
kaoyan -stealth

# 4. View 2027 NETEM review dashboard
kaoyan -status

# 5. Practice today's due reviews only
kaoyan -review

# 6. Practice error book words only
kaoyan -wrong

# 7. Start from specific word offset
kaoyan -start 100
```
