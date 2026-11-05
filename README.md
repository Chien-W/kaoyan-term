# kaoyan-term

Terminal-based continuous vocabulary dictation tool disguised as an authentic Go backend microservice log stream.

## Features

- **Zero Chinese, Zero Vocabulary Spoilers**: Screen is 100% pure dense technical backend logs (Raft consensus, WAL flush, connection pools, gRPC stream handshakes).
- **Audio-Driven Dictation**: The word is pronounced purely into your earphones.
- **In-Place Input**: You type the word into the log's `hash_token=...` field.
- **Realistic Ambient Logs**: Interspersed with authentic background heartbeat and compaction logs.
- **Instant Clean Boss Key**: `Esc` or `Ctrl+C` completely wipes the terminal and scrollback buffer.

## Controls

- `[Letters]`: Type token from earphone audio
- `[Space]`: Replay pronunciation in earphones
- `[Tab]` or `[?]`: Hint next letter
- `[Backspace]`: Delete character
- `[Esc]` or `[Ctrl+C]`: Instant clean exit
