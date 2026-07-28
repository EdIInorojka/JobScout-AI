# JobScout AI

JobScout AI is a private job-search assistant for one owner. The current MVP includes the core search pipeline plus application preparation and Telegram approval:

- Go modular monolith
- PostgreSQL schema and SQL migration
- official HeadHunter collector
- vacancy normalization, deduplication, hard filters, deterministic scoring
- REST API for profile, search, vacancies, resumes, and applications
- minimal Telegram bot with `/start`, `/search`, `/recommended`, and application approval flow
- Docker Compose, OpenAPI, and tests

## Run locally

1. Copy `.env.example` to `.env` and fill the values.
2. Apply migrations:
   ```bash
   make migrate
   ```
3. Start the API:
   ```bash
   make serve
   ```

With Docker Compose:

```bash
docker compose run --rm migrate
docker compose up --build app
```

## Architecture

- `internal/core` - entities, normalization, filters, scoring, status rules
- `internal/store` - repository interfaces and implementations
- `internal/integrations/hh` - official HeadHunter API client
- `internal/integrations/telegram` - Telegram Bot API client
- `internal/app` - application use cases, HTTP handlers, Telegram flow
- `cmd/jobscout` - entrypoint and process lifecycle

## Implemented endpoints

- `GET /healthz`
- `GET /v1/profile`
- `POST /v1/profile`
- `POST /v1/search`
- `GET /v1/vacancies`
- `GET /v1/vacancies/recommended`
- `GET /v1/vacancies/{id}`
- `PATCH /v1/vacancies/{id}/status`
- `POST /v1/vacancies/import-url`
- `POST /v1/vacancies/{id}/applications/prepare`
- `GET /v1/resumes`
- `POST /v1/resumes`
- `GET /v1/resumes/{id}`
- `PATCH /v1/resumes/{id}`
- `GET /v1/applications`
- `GET /v1/applications/{id}`
- `POST /v1/applications/{id}/approve`
- `POST /v1/applications/{id}/cancel`
- `POST /v1/applications/{id}/mark-submitted`
- `PATCH /v1/applications/{id}/outcome`

## Notes

- Only official HeadHunter API usage is implemented.
- Applications are prepared deterministically and require explicit approval before submission.
- Telegram bot is optional and disabled when the token is missing.

## Testing

Unit suite:

```bash
go test ./...
go vet ./...
go build ./...
```

PostgreSQL integration suite:

```bash
docker compose up -d db
export DATABASE_URL=postgres://jobscout:jobscout@localhost:5432/jobscout?sslmode=disable
export TEST_DATABASE_URL=$DATABASE_URL
make migrate
make test-integration
docker compose down -v
```

HeadHunter fake-server suite:

```bash
make test-hh
```

Full local verification:

```bash
gofmt -w .
go test ./...
go vet ./...
go build ./...
git diff --check
```
