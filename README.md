# Majoo Blog RESTful API

Go RESTful API for a blog platform (MySQL + GORM, `net/http`, JWT).

## Setup

### Prerequisites

- Docker and Docker Compose
- (Optional) Go 1.26+ for running the API outside containers

### One-command local environment

1. Copy env defaults (optional — Compose already has sensible fallbacks):

   ```bash
   cp .env.example .env
   ```

2. Start the API and MySQL:

   ```bash
   make docker-up
   ```

3. Confirm the API is up:

   ```bash
   curl http://localhost:8080/health
   ```

   Expected response:

   ```json
   {"status":"ok"}
   ```

4. Stop everything:

   ```bash
   make docker-down
   ```

### Services

| Service | Host access | Notes |
|---------|-------------|--------|
| `api` | `http://localhost:8080` | Multi-stage image built from `Dockerfile` |
| `db` | `localhost:3306` | MySQL 8 with named volume `mysql_data` |

The API container is wired to the `db` service via `DB_HOST=db` and matching `DB_*` / `DATABASE_URL` env vars. Compose waits for MySQL’s healthcheck (`mysqladmin ping`) before starting `api`.

## API documentation

The OpenAPI 3.0.3 spec is [`docs/openapi.yaml`](docs/openapi.yaml). Regenerate it from handler comments:

```bash
make docs && make run
```

Then open [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html). Use **Authorize** and paste a JWT from `POST /api/auth/login` (token only; the UI sends `Authorization: Bearer <token>`).
