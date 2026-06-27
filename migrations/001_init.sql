CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret TEXT,
    mfa_enabled BOOLEAN NOT NULL DEFAULT 0,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until TEXT,
    registered_at TEXT NOT NULL,
    last_login_at TEXT
);

CREATE INDEX idx_users_username ON users(username);
