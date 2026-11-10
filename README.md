# Nova Vida SMS

School management system (enrolment and finance) for Nova Vida, Xai-Xai. Go API + PostgreSQL + React.

- Design: [`docs/technical-design.md`](docs/technical-design.md)
- Progress: [`docs/progress.md`](docs/progress.md)
- UI reference: [`docs/ui-brief.md`](docs/ui-brief.md), [`docs/prototype/index.html`](docs/prototype/index.html)

## Run the API locally

Requirements: Go 1.24+, PostgreSQL 16.

```sh
createdb novavida
export DATABASE_URL="postgres://postgres@localhost:5432/novavida?sslmode=disable"
cd api
go mod tidy                     # first time only: writes go.sum
go run ./cmd/cli seed           # migrations + 2027 reference data
go run ./cmd/cli create-admin -login admin@novavida.co.mz -name "Administrador" -password 'change-me-now'
COOKIE_SECURE=false go run ./cmd/server
curl localhost:8080/api/healthz
```

## Tests

```sh
createdb novavida_test
export TEST_DATABASE_URL="postgres://postgres@localhost:5432/novavida_test?sslmode=disable"
cd api && go test -p 1 ./...
```

Integration tests drop and recreate the `public` schema of `TEST_DATABASE_URL`. Never point it at real data.

## Run with Docker Compose

```sh
cd deploy
cp .env.example .env            # set POSTGRES_PASSWORD and SITE_ADDRESS
docker compose up -d --build
docker compose run --rm --entrypoint /app/cli api seed
docker compose run --rm --entrypoint /app/cli api create-admin -login admin@novavida.co.mz -password 'change-me-now'
```

Caddy serves the web build from `web/dist` and proxies `/api` to the API. For a local HTTP test set `SITE_ADDRESS=:80` and `COOKIE_SECURE=false`.
