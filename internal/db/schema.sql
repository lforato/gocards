CREATE TABLE IF NOT EXISTS decks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color       TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS cards (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    deck_id         INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    type            TEXT NOT NULL,
    language        TEXT NOT NULL,
    prompt          TEXT NOT NULL,
    expected_answer TEXT NOT NULL DEFAULT '',
    blanks_data     TEXT,
    choices         TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS cards_deck_id_idx ON cards(deck_id);

CREATE TABLE IF NOT EXISTS reviews (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id     INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    grade       INTEGER NOT NULL,
    reviewed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    next_due    TEXT NOT NULL,
    ease        REAL NOT NULL,
    interval    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS reviews_card_id_idx     ON reviews(card_id);
CREATE INDEX IF NOT EXISTS reviews_reviewed_at_idx ON reviews(reviewed_at);
CREATE INDEX IF NOT EXISTS reviews_next_due_idx    ON reviews(next_due);

CREATE TABLE IF NOT EXISTS study_sessions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    deck_id        INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    started_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ended_at       TEXT,
    cards_reviewed INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS study_sessions_deck_id_idx ON study_sessions(deck_id);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
