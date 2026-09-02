CREATE TABLE IF NOT EXISTS products (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    base_url     TEXT NOT NULL DEFAULT '',
    auth_type    TEXT NOT NULL DEFAULT '',
    auth_notes   TEXT NOT NULL DEFAULT '',
    auth_schema  TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS product_aliases (
    alias      TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id)
);

CREATE TABLE IF NOT EXISTS endpoints (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id    TEXT NOT NULL REFERENCES products(id),
    release       TEXT NOT NULL DEFAULT '',
    method        TEXT NOT NULL,
    path          TEXT NOT NULL,
    summary       TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    tags          TEXT NOT NULL DEFAULT '[]',
    parameters    TEXT NOT NULL DEFAULT '[]',
    request_body  TEXT NOT NULL DEFAULT '{}',
    responses     TEXT NOT NULL DEFAULT '{}',
    source_format TEXT NOT NULL DEFAULT '',
    UNIQUE(product_id, release, method, path)
);

CREATE VIRTUAL TABLE IF NOT EXISTS endpoints_fts USING fts5(
    summary,
    description,
    path,
    tags,
    content='endpoints',
    content_rowid='id',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS endpoints_fts_insert
    AFTER INSERT ON endpoints BEGIN
    INSERT INTO endpoints_fts(rowid, summary, description, path, tags)
    VALUES (new.id, new.summary, new.description, new.path, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS endpoints_fts_delete
    BEFORE DELETE ON endpoints BEGIN
    INSERT INTO endpoints_fts(endpoints_fts, rowid, summary, description, path, tags)
    VALUES ('delete', old.id, old.summary, old.description, old.path, old.tags);
END;

CREATE TRIGGER IF NOT EXISTS endpoints_fts_update
    AFTER UPDATE ON endpoints BEGIN
    INSERT INTO endpoints_fts(endpoints_fts, rowid, summary, description, path, tags)
    VALUES ('delete', old.id, old.summary, old.description, old.path, old.tags);
    INSERT INTO endpoints_fts(rowid, summary, description, path, tags)
    VALUES (new.id, new.summary, new.description, new.path, new.tags);
END;

CREATE TABLE IF NOT EXISTS nac_paths (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id    TEXT NOT NULL REFERENCES products(id),
    release       TEXT NOT NULL DEFAULT '',
    path          TEXT NOT NULL,
    object_name   TEXT NOT NULL DEFAULT '',
    gui_location  TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    schema        TEXT NOT NULL DEFAULT '{}',
    examples      TEXT NOT NULL DEFAULT '[]',
    source_format TEXT NOT NULL DEFAULT '',
    UNIQUE(product_id, release, path)
);

CREATE VIRTUAL TABLE IF NOT EXISTS nac_paths_fts USING fts5(
    object_name,
    description,
    path,
    examples,
    content='nac_paths',
    content_rowid='id',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS nac_paths_fts_insert
    AFTER INSERT ON nac_paths BEGIN
    INSERT INTO nac_paths_fts(rowid, object_name, description, path, examples)
    VALUES (new.id, new.object_name, new.description, new.path, new.examples);
END;

CREATE TRIGGER IF NOT EXISTS nac_paths_fts_delete
    BEFORE DELETE ON nac_paths BEGIN
    INSERT INTO nac_paths_fts(nac_paths_fts, rowid, object_name, description, path, examples)
    VALUES ('delete', old.id, old.object_name, old.description, old.path, old.examples);
END;

CREATE TRIGGER IF NOT EXISTS nac_paths_fts_update
    AFTER UPDATE ON nac_paths BEGIN
    INSERT INTO nac_paths_fts(nac_paths_fts, rowid, object_name, description, path, examples)
    VALUES ('delete', old.id, old.object_name, old.description, old.path, old.examples);
    INSERT INTO nac_paths_fts(rowid, object_name, description, path, examples)
    VALUES (new.id, new.object_name, new.description, new.path, new.examples);
END;

CREATE TABLE IF NOT EXISTS synonyms (
    term       TEXT NOT NULL,
    expansion  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS synonyms_term ON synonyms(term);
