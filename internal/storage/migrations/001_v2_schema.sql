-- 001_v2_schema.sql: Prompt Studio Specification v2 (Unified Sessions + Library + Accent-Insensitive FTS5)

CREATE TABLE IF NOT EXISTS collections (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  is_system   INTEGER NOT NULL DEFAULT 0,
  sort_order  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sessions (
  id                 TEXT PRIMARY KEY,
  title              TEXT NOT NULL,
  description        TEXT NOT NULL DEFAULT '',
  origin             TEXT NOT NULL,
  request            TEXT,
  target             TEXT NOT NULL DEFAULT 'chat',
  collection_id      TEXT REFERENCES collections(id),
  saved_at           INTEGER,
  pinned_version_id  TEXT,
  content_hash       TEXT,
  created_at         INTEGER NOT NULL,
  opened_at          INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS prompt_versions (
  id               TEXT PRIMARY KEY,
  session_id       TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  parent_id        TEXT REFERENCES prompt_versions(id),
  seq              INTEGER NOT NULL,
  source           TEXT NOT NULL,
  instruction      TEXT,
  content          TEXT NOT NULL,
  assumptions      TEXT,
  incomplete       INTEGER NOT NULL DEFAULT 0,
  target           TEXT,             -- chat, api or agent. NULL means inherit sessions.target (rows before v2.1)
  template_version TEXT,
  model            TEXT,
  input_tokens     INTEGER NOT NULL DEFAULT 0,
  output_tokens    INTEGER NOT NULL DEFAULT 0,
  latency_ms       INTEGER NOT NULL DEFAULT 0,
  created_at       INTEGER NOT NULL,
  UNIQUE (session_id, seq)
);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS sessions_fts USING fts5(
  session_id UNINDEXED,
  title,
  request,
  content,
  tokenize = 'unicode61 remove_diacritics 2'
);

CREATE INDEX IF NOT EXISTS idx_sessions_opened ON sessions(opened_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_saved ON sessions(saved_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_collection ON sessions(collection_id, saved_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_hash ON sessions(content_hash);
CREATE INDEX IF NOT EXISTS idx_versions_session ON prompt_versions(session_id, seq);
