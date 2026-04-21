# gocards

Terminal flashcards for developers — a Go / Bubble Tea port of the devcards
web app. SQLite-backed, fully offline capable, with optional Anthropic-powered
card generation and grading. Code answers are edited in `$EDITOR` (defaults to
`vim`) with a bundled theme.

## Install

```bash
cd gocards
go build -o gocards .
./gocards
```

Go 1.24+ required.

## Data

Persistence lives in `~/.gocards/`:

| file | purpose |
|---|---|
| `gocards.db` | SQLite database (decks, cards, reviews, sessions, settings) |
| `theme.vimrc` | vim colorscheme loaded via `-S` when opening code answers |

The schema is migrated automatically on startup, and a small sample deck is
seeded on first run.

## Configuration

Three settings are stored in the DB and editable from the in-app Settings
screen:

- **Daily review limit** (default 50)
- **Preferred languages** (comma-separated, used by AI generation)
- **Anthropic API key** (falls back to `ANTHROPIC_API_KEY` env var)

## Keys

### Dashboard
| key | action |
|---|---|
| `↑ ↓ / j k` | move cursor |
| `enter` | open deck / action |
| `S` | study selected deck |
| `n` | new cards (AI or manual) |
| `s` | settings |
| `r` | reload |
| `q` / `ctrl+c` | quit |

### Deck view
| key | action |
|---|---|
| `enter` / `e` | edit selected card |
| `s` | study this deck |
| `n` | add card |
| `d` / `x` | delete card (asks for confirmation) |
| `esc` / `backspace` | back to dashboard |

### Study
| key | action |
|---|---|
| MCQ | `↑ ↓` pick · `enter` submit |
| Fill | `tab` switch blank · `enter` submit |
| Code / Exp | `e` open vim · on save, answer is graded by Claude |
| `1-5` | override grader / manual grade |
| `enter` | next card |
| `esc` | end session |

### Settings
| key | action |
|---|---|
| `tab` | cycle fields |
| `ctrl+s` | save |
| `esc` | back |

## Architecture

```
gocards/
├── main.go                      # entry
├── internal/
│   ├── db/                      # sqlite open + embedded schema + seed
│   ├── models/                  # domain types mirroring shared/types.ts
│   ├── store/                   # queries (decks, cards, reviews, sessions, stats)
│   ├── srs/                     # port of the JS SRS algorithm
│   ├── ai/                      # streaming wrapper around anthropic-sdk-go
│   ├── editor/                  # tea.ExecProcess-based vim launcher with theme
│   └── tui/
│       ├── app.go               # root screen stack
│       ├── styles.go            # lipgloss palette
│       ├── heatmap.go           # 13-week activity grid
│       ├── dashboard.go         # stats + deck list
│       ├── deck.go              # card list inside a deck
│       ├── study.go             # review loop (mcq / fill / code / exp)
│       ├── create.go            # AI generation + bulk insert
│       ├── edit.go              # single-card editor
│       └── settings.go          # key-value settings
```

## Card types

| type | UI |
|---|---|
| `mcq` | choose one of the offered choices |
| `fill` | fill each blank inline |
| `code` | write a solution from scratch in vim |
| `exp` | annotate the pre-filled code block with comments in vim |

`code` and `exp` cards are graded by Claude when an API key is set; without a
key you grade yourself with `1-5`.

## Vim theme

On first run `~/.gocards/theme.vimrc` is written. It's auto-loaded via `-S`
whenever the external editor is `vim` / `nvim` / `vi`. Delete the file to
regenerate after edits are made in the binary.

## Tests

```bash
go test ./...
```
