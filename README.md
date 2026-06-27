# Containerized CLI Login System with Optional 2FA

A secure interactive Go CLI for user registration, login, optional TOTP-based MFA, account lockout, and session management. SQLite is used for persistence and is stored in a Docker volume so data survives container restarts.

## Features

- Register and login with username/password.
- Passwords are hashed with bcrypt.
- Optional Google Authenticator compatible TOTP MFA.
- Account lockout after repeated failed login attempts.
- In-memory sessions with configurable expiration.
- Interactive prompt with command history and tab completion.
- SQLite schema migration included in `migrations/`.
- Dockerfile and Compose setup with persistent volume.

## Quick Start

Build and run the CLI in Docker:

```bash
docker compose run --rm authcli
```

The application stores SQLite data in the `auth-data` volume at `/data/auth.db`.

Run locally:

```bash
go mod download
go run ./cmd/authcli
```

By default local runs store data at `data/auth.db`.

## Configuration

Set environment variables as needed:

| Variable | Default | Description |
| --- | --- | --- |
| `DB_DSN` | `data/auth.db` | SQLite database path |
| `SESSION_TIMEOUT` | `30m` | Session duration, Go duration format |
| `MAX_FAILED_ATTEMPTS` | `5` | Failed attempts before lockout |
| `LOCKOUT_DURATION` | `15m` | Lockout duration |
| `TOTP_ISSUER` | `Containerized CLI Auth` | Issuer shown in authenticator apps |
| `BCRYPT_COST` | bcrypt default | Password hashing cost |

## Commands

Before login:

- `register` creates a user.
- `login` authenticates with password and TOTP when enabled.
- `help` lists available commands.
- `exit` quits the program.

After login:

- `whoami` shows username, registration date, MFA status, session expiry, and last login.
- `enable-2fa` creates a TOTP secret and verifies the first code.
- `disable-2fa` disables MFA after password and TOTP verification.
- `logout` ends the session.
- `help` lists available commands.

## MFA Setup

Run `enable-2fa` after login. The CLI prints both a TOTP secret and an `otpauth://` URL. Add either one to Google Authenticator, 1Password, Authy, or another TOTP-compatible app, then enter the current code to confirm.

## Tests

```bash
go test ./...
```

## Project Layout

```text
cmd/authcli/        CLI entrypoint
internal/auth/      Authentication, MFA, lockout, and session logic
internal/cli/       Interactive shell
internal/config/    Environment configuration
internal/store/     SQLite persistence and migrations
migrations/         Database schema
```
