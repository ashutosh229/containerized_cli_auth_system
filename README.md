<p align="center">
  <h1 align="center">🔐 Containerized CLI Auth System</h1>
  <p align="center">A secure, interactive Go CLI for user registration, login, optional TOTP-based 2FA, account lockout, and session management — backed by SQLite and designed for Docker.</p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/SQLite-modernc-003B57?style=flat-square&logo=sqlite&logoColor=white" />
  <img src="https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker&logoColor=white" />
  <img src="https://img.shields.io/badge/2FA-TOTP%20%2F%20Google%20Authenticator-34A853?style=flat-square" />
  <img src="https://img.shields.io/badge/License-MIT-yellow?style=flat-square" />
</p>

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
  - [Docker (Recommended)](#docker-recommended)
  - [Run Locally](#run-locally)
- [Configuration](#configuration)
- [Commands](#commands)
  - [Guest Commands](#guest-commands-before-login)
  - [Authenticated Commands](#authenticated-commands-after-login)
- [MFA Setup](#mfa-setup)
- [Security Details](#security-details)
- [Database Schema](#database-schema)
- [Testing](#testing)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

**Containerized CLI Auth System** is a production-oriented, self-contained authentication shell written in Go. It demonstrates a complete auth lifecycle — registration, login, TOTP-based multi-factor authentication, account lockout, and session expiry — all from an interactive terminal prompt.

Data is persisted to a SQLite database stored in a Docker volume, so user accounts and state survive container restarts.

---

## Features

| Feature                     | Details                                                              |
| --------------------------- | -------------------------------------------------------------------- |
| 🔑 **Registration & Login** | Username/password auth with strict input validation                  |
| 🔒 **Password Hashing**     | `bcrypt` with configurable cost factor                               |
| 📱 **TOTP-Based 2FA**       | Google Authenticator / Authy compatible (`otpauth://` URL + secret)  |
| 🔐 **Account Lockout**      | Configurable lockout after repeated failed attempts                  |
| ⏱️ **Session Management**   | In-memory sessions with configurable expiry                          |
| 🖥️ **Interactive Shell**    | Command history, tab completion, and ANSI-colored output             |
| 🗄️ **SQLite Persistence**   | Schema migrations included; data survives restarts via Docker volume |
| 🐳 **Docker-First**         | Dockerfile + Compose setup; no host dependencies required            |
| 🧪 **Testable Design**      | Repository interfaces allow pure in-memory test doubles              |

---

## Architecture

```mermaid
flowchart TD

    A[CLI Shell (interactive)\ncmd/authcli → internal/cli]

    B[Auth Service Layer\ninternal/auth]

    B1[Register / Login / Logout]
    B2[TOTP Enable / Disable / Verify]
    B3[Account Lockout Logic]
    B4[In-Memory Session Store]

    C[Persistence Layer (SQLite)\ninternal/store]

    C1[UserStore (CRUD operations)]
    C2[Migration runner (migrations/*.sql)]

    A --> B
    B --> C

    B --> B1
    B --> B2
    B --> B3
    B --> B4

    C --> C1
    C --> C2
```

---

## Project Structure

```text
.
├── cmd/
│   └── authcli/
│       └── main.go            # CLI entrypoint
├── internal/
│   ├── auth/
│   │   ├── model.go           # User and Session types
│   │   ├── repository.go      # UserRepository interface
│   │   ├── service.go         # Business logic: auth, sessions, TOTP
│   │   └── service_test.go    # Unit tests with in-memory repo
│   ├── cli/
│   │   ├── shell.go           # Interactive shell & command dispatch
│   │   ├── colors.go          # ANSI printer and table renderer
│   │   ├── history.go         # Readline history file
│   │   └── input.go           # Prompt helpers (line & password)
│   ├── config/
│   │   └── config.go          # Environment variable configuration
│   └── store/
│       └── sqlite.go          # SQLite DB, migrations, UserStore
├── migrations/
│   └── 001_init.sql           # Initial database schema
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

---

## Prerequisites

**For Docker (recommended):**

- [Docker](https://docs.docker.com/get-docker/) ≥ 24
- [Docker Compose](https://docs.docker.com/compose/) v2

**For local development:**

- [Go](https://go.dev/dl/) ≥ 1.23
- CGO not required (uses `modernc.org/sqlite`, a pure-Go SQLite port)

---

## Quick Start

### Docker (Recommended)

Build the image and launch an interactive session:

```bash
docker compose run --rm authcli
```

SQLite data is stored in `./data/auth.db` on your host (bind-mounted via `docker-compose.yml`), so user accounts persist between runs.

To build the image separately:

```bash
docker compose build
```

### Run Locally

```bash
# Download dependencies
go mod download

# Run the CLI (creates data/auth.db in the current directory)
go run ./cmd/authcli
```

By default, the local run stores data at `data/auth.db`. Override with the `DB_DSN` environment variable.

---

## Configuration

All configuration is provided via environment variables with safe defaults.

| Variable              | Default                  | Description                                          |
| --------------------- | ------------------------ | ---------------------------------------------------- |
| `DB_DSN`              | `data/auth.db`           | Path to the SQLite database file                     |
| `SESSION_TIMEOUT`     | `30m`                    | Duration of a logged-in session (Go duration format) |
| `MAX_FAILED_ATTEMPTS` | `5`                      | Failed login attempts before account lockout         |
| `LOCKOUT_DURATION`    | `15m`                    | How long a locked account remains locked             |
| `TOTP_ISSUER`         | `Containerized CLI Auth` | Issuer name shown in authenticator apps              |
| `BCRYPT_COST`         | bcrypt default (`10`)    | bcrypt hashing cost (higher = slower = more secure)  |
| `TZ`                  | `Asia/Kolkata` (Docker)  | Timezone for display formatting                      |

Example — override for a more restrictive setup:

```bash
MAX_FAILED_ATTEMPTS=3 LOCKOUT_DURATION=30m SESSION_TIMEOUT=15m go run ./cmd/authcli
```

---

## Commands

### Guest Commands (before login)

| Command    | Description                                                 |
| ---------- | ----------------------------------------------------------- |
| `register` | Create a new user account (prompts for username & password) |
| `login`    | Authenticate; prompts for TOTP code if 2FA is enabled       |
| `help`     | Display available commands                                  |
| `exit`     | Quit the program                                            |

### Authenticated Commands (after login)

| Command       | Description                                                                       |
| ------------- | --------------------------------------------------------------------------------- |
| `whoami`      | Show username, registration date, 2FA status, session expiry, and last login      |
| `enable-2fa`  | Generate a TOTP secret, display the `otpauth://` URL, and confirm with first code |
| `disable-2fa` | Disable 2FA after re-verifying password and current TOTP code                     |
| `logout`      | End the current session                                                           |
| `help`        | Display available commands                                                        |
| `clear`       | Clear the terminal screen                                                         |

> **Note:** `exit` is intentionally blocked while logged in. Use `logout` first to protect against accidental session abandonment.

---

## MFA Setup

1. Log in to your account.
2. Run `enable-2fa`.
3. The CLI prints a TOTP secret and an `otpauth://` URL.
4. Add either to your authenticator app:
   - **Google Authenticator** — scan the URL as a QR code (use a QR generator), or enter the secret manually.
   - **1Password / Authy / Bitwarden** — paste the `otpauth://` URL directly.
5. Enter the current 6-digit code shown by your app to confirm setup.

From the next login onwards, you will be prompted for your authenticator code after your password.

To remove 2FA, run `disable-2fa` and confirm with your password and current authenticator code.

---

## Security Details

| Concern                | Approach                                                                    |
| ---------------------- | --------------------------------------------------------------------------- |
| **Password storage**   | `bcrypt` with configurable cost; never stored in plaintext                  |
| **TOTP**               | RFC 6238-compliant via `github.com/pquerna/otp`; 30-second window           |
| **Session tokens**     | 32-byte cryptographically random tokens (`crypto/rand`), hex-encoded        |
| **Account lockout**    | Locks after N failed attempts (password or TOTP); persisted to DB           |
| **Session expiry**     | Checked on every command dispatch; expired sessions are evicted immediately |
| **Container security** | Runs as non-root user (`authcli`, UID 10001) inside Debian slim image       |
| **DB isolation**       | `MaxOpenConns(1)` + WAL mode for safe concurrent access                     |
| **Foreign keys**       | Enforced via `PRAGMA foreign_keys = ON`                                     |

---

## Database Schema

```sql
CREATE TABLE users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT    NOT NULL UNIQUE,
    password_hash   TEXT    NOT NULL,
    totp_secret     TEXT,
    mfa_enabled     BOOLEAN NOT NULL DEFAULT 0,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until    TEXT,
    registered_at   TEXT    NOT NULL,
    last_login_at   TEXT
);

CREATE INDEX idx_users_username ON users(username);
```

Timestamps are stored as RFC3339Nano strings in UTC.

Schema migrations are applied automatically on startup from the `migrations/` directory. Versions are tracked in the `schema_migrations` table — each `.sql` file is applied exactly once, in lexicographic order.

---

## Testing

Unit tests use an in-memory repository (no SQLite dependency) and a fake clock for deterministic time-based scenarios.

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run only auth package tests
go test -v ./internal/auth/...
```

**Test coverage includes:**

- Successful registration and login
- Duplicate username rejection
- Account lockout after N failed attempts
- Session expiry via time-travel with a mutable clock

---

## Contributing

1. Fork the repository.
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Commit your changes: `git commit -m 'feat: add my feature'`
4. Push to the branch: `git push origin feature/my-feature`
5. Open a Pull Request.

Please ensure `go test ./...` passes and `go vet ./...` reports no issues before submitting.

---

## License

This project is licensed under the [MIT License](LICENSE).

Copyright © 2026 [Ashutosh Kumar Jha](https://github.com/ashutosh229)
