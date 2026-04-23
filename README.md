<img width="1983" height="793" alt="ChatGPT Image Apr 23, 2026, 03_51_31 PM" src="https://github.com/user-attachments/assets/eb2ed6fb-fa67-4a54-b802-4cd4ab23e2cf" />

**Terminal flashcards for developers.** A fast, offline-first TUI built with
Go and Bubble Tea, backed by SQLite. Optional Anthropic integration generates
and grades code / explanation cards; everything else works with no network.

---

## Why

Most flashcard apps are built for vocabulary, not for code. `gocards` is
designed around the things engineers actually study:

- **Code cards** — write a solution in `$EDITOR` (vim by default) and get it
  AI-graded against an expected answer.
- **Explain cards** — annotate or describe a snippet in your own words; the
  model grades the explanation.
- **MCQ and fill-in-the-blank** — quick, offline, no tokens.
- **Spaced repetition** — a port of the classic SM-2 scheduler picks what to
  review next.
- **Lives in your terminal** — no browser, no Electron, single ~25 MB binary.

---

## Install

### Precompiled binaries

Grab the archive for your platform from the
[Releases page](https://github.com/lforato/gocards/releases), extract it, and
drop `gocards` somewhere on your `$PATH`:

```bash
# macOS (Apple Silicon) — adapt the asset name for your OS/arch
curl -L -o gocards.tar.gz \
  https://github.com/lforato/gocards/releases/latest/download/gocards-darwin-arm64.tar.gz
tar xzf gocards.tar.gz
mv gocards /usr/local/bin/
gocards
```

Binaries are built for **darwin/amd64**, **darwin/arm64**, **linux/amd64**,
**linux/arm64**, and **windows/amd64**.

### From source

You need **Go 1.25+** and `make`. The SQLite driver is pure-Go (no CGO), so
cross-compilation Just Works.

```bash
git clone https://github.com/lforato/gocards.git
cd gocards
make build         # produces ./gocards
./gocards
```

`make install` puts the binary in `$GOBIN` (or `$GOPATH/bin`) so it's on your
`$PATH` everywhere.

### With `go install`

```bash
go install github.com/lforato/gocards/cmd/gocards@latest
```

### Makefile targets

| target | what it does |
|---|---|
| `make build` | Build `./gocards` for the current platform |
| `make install` | `go install` into `$GOBIN` / `$GOPATH/bin` |
| `make run` | Build then launch |
| `make test` | Run the full test suite |
| `make fmt` / `make vet` | Format and vet the module |
| `make tidy` | `go mod tidy` |
| `make release` | Cross-compile every supported OS/arch into `./dist` |
| `make clean` | Remove `./gocards` and `./dist` |
| `make help` | Print this list |

---

## First run

On first launch `gocards` creates `~/.gocards/` and seeds two **tutorial
decks** — one in English, one in Portuguese — that walk through the
controls and every card type. When you're comfortable, highlight a tutorial
deck on the dashboard and press `D` to delete it. They won't come back.

---

## Usage

Start the app with `gocards`. The dashboard lists your decks and your
review stats; everything else is a keypress away.

### Dashboard

| key | action |
|---|---|
| `↑ ↓` / `j k` | move cursor |
| `enter` | open deck |
| `S` | study the selected deck |
| `n` | new deck (manual or AI-assisted) |
| `g` | AI-generate cards into an existing deck |
| `s` | settings |
| `D` | delete deck (asks `y` / `N`) |
| `r` | reload |
| `q` / `ctrl+c` | quit |

### Deck view

| key | action |
|---|---|
| `↑ ↓` / `j k` | move cursor |
| `enter` | edit selected card |
| `space` / `a` | toggle selection for bulk ops |
| `s` | study this deck |
| `c` | open/generate a cheatsheet |
| `n` | add card |
| `d` | delete selected cards |
| `esc` | back to dashboard |

### Study session

| stage | keys |
|---|---|
| MCQ | `↑ ↓` pick · `enter` submit |
| Fill | `tab` next blank · `enter` submit |
| Code / Explain | `ctrl+e` open vim · save to submit · Claude grades |
| After answer | `1-4` rate recall · `ctrl+n` next · `ctrl+p` previous |
| Any time | `ctrl+d` delete card · `esc` end session |

### Settings

| key | action |
|---|---|
| `tab` | cycle fields |
| `ctrl+s` | save |
| `esc` | back |

---

## Configuration

Settings are stored in SQLite and edited from the in-app settings screen:

- **Daily review limit** — caps the number of cards per study session
  (default `50`).
- **Preferred languages** — comma-separated list fed to the AI when
  generating cards (default `javascript,typescript,python`).
- **Anthropic API key** — needed for AI generation and code grading.
  Falls back to the `ANTHROPIC_API_KEY` environment variable if unset.
- **Language** — `en` or `pt-BR`. Drives all UI strings and the language
  instruction passed to the AI.

Without an API key everything still works except AI generation and AI
grading; you can grade code/explain cards manually with `1-4`.

---

## Data directory

Everything persistent lives under `~/.gocards/`:

| file | purpose |
|---|---|
| `gocards.db` | SQLite database (decks, cards, reviews, sessions, settings) |
| `theme.vimrc` | vim colorscheme auto-loaded when opening code answers |

The schema is applied and migrated on every startup; deleting `gocards.db`
resets the app to a clean install (tutorial decks will be re-seeded).

---

## Card types

| type | how you answer | graded by |
|---|---|---|
| `mcq` | pick one of the choices | exact match |
| `fill` | fill each blank inline | exact match |
| `code` | write a solution from scratch in vim | Claude (or manual `1-4`) |
| `exp` | annotate a pre-filled snippet with comments | Claude (or manual `1-4`) |

---

## Architecture

```
gocards/
├── cmd/gocards/        # entry point
├── internal/
│   ├── db/             # sqlite open, embedded schema, migrations, seed
│   ├── models/         # domain types + card-kind registry
│   ├── store/          # queries (decks, cards, reviews, sessions, stats)
│   ├── srs/            # SM-2 spaced-repetition scheduler
│   ├── ai/             # streaming wrapper around anthropic-sdk-go
│   ├── i18n/           # per-locale string tables (en, pt-BR)
│   └── tui/            # Bubble Tea screens and widgets
└── Makefile
```

---

## Development

```bash
make test    # go test ./...
make fmt     # gofmt -s -w .
make vet     # go vet ./...
make tidy    # go mod tidy
make release # cross-compile into ./dist
```

The store package has the bulk of the unit tests; new queries or
migrations should land with matching coverage in
`internal/store/store_test.go`.
