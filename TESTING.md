# Testing

## Unit tests

Feature logic below the HTTP layer lives in `internal/<feature>/service.go` and `internal/<feature>/repository.go`. Unit tests use [testify](https://github.com/stretchr/testify) (`assert` / `require`).

Services are tested with in-memory stubs (or SQLite when a real GORM transaction is required). Repositories use an in-memory SQLite database (`github.com/glebarez/sqlite`) so lookups, pagination, and soft-delete behave like production GORM without MySQL.

```bash
make test
# or
make test-coverage
# or
go test ./...
```

Covered in this pass (`internal/user`, `internal/post`, `internal/comment`):

- Password hashing: bcrypt storage, min-length / empty rejection, 72-byte limit, corrupt hash → `ErrInvalidCredentials`
- JWT: `sub` is `public_id`, expiry claim, tampered signature, expired token, wrong secret
- Pagination whitelist: unknown `sort` / `filter` keys are ignored; `page` / `per_page` defaults
- Ownership: non-owners get `ErrForbidden`; comment delete allowed for comment author or post author
- Resolve-by-`public_id`: both an unknown well-formed UUID and a malformed string return `ErrNotFound` (lookup, not format validation)

## Coverage

Recorded from `go test ./... -cover` (2026-08-13):

| Package                        | Coverage |
| ------------------------------ | -------- |
| `blog-api/cmd/api`             | 0.0%     |
| `blog-api/internal/comment`    | 62.7%    |
| `blog-api/internal/config`     | 80.0%    |
| `blog-api/internal/database`   | 0.0%     |
| `blog-api/internal/middleware` | 69.9%    |
| `blog-api/internal/post`       | 63.2%    |
| `blog-api/internal/response`   | 87.7%    |
| `blog-api/internal/user`       | 52.0%    |

Overall statement coverage (`go test ./... -coverprofile=coverage.out` then `go tool cover -func=coverage.out`): **61.4%**.

`cmd/api` and `internal/database` stay at 0% because unit tests target services and repositories, not process bootstrap or MySQL dialector wiring. HTTP handlers are covered by `make test-integration` and `test/http/*.http`.

To refresh:

```bash
go test ./... -cover
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -n 1
```

## Integration tests

Handler tests live in `internal/<feature>/handler_test.go` (`//go:build integration`). They use `net/http/httptest` against a **real MySQL** schema (same server as docker-compose / local MySQL, separate database). They are not run by `make test`.

```bash
# MySQL must be reachable. Schema is created and migrated automatically.
make test-integration
```

`make test-integration` includes `.env` when present, then runs:

```bash
go test -tags=integration -p 1 -count=1 ./internal/user ./internal/post ./internal/comment
```

Packages run sequentially (`-p 1`) because they share one test schema and truncate between cases.

### Required environment

| Variable           | Fallback      | Default     | Purpose                                        |
| ------------------ | ------------- | ----------- | ---------------------------------------------- |
| `TEST_DB_HOST`     | `DB_HOST`     | `127.0.0.1` | MySQL host                                     |
| `TEST_DB_PORT`     | `DB_PORT`     | `3306`      | MySQL port                                     |
| `TEST_DB_USER`     | `DB_USER`     | `root`      | User that can `CREATE DATABASE` and DDL        |
| `TEST_DB_PASSWORD` | `DB_PASSWORD` | _(empty)_   | Password                                       |
| `TEST_DB_NAME`     | —             | `blog_test` | **Dedicated** schema (never the app `DB_NAME`) |

The test user must be able to create `TEST_DB_NAME` (or you create it first) and run migrations. Example against docker-compose MySQL:

```bash
export TEST_DB_HOST=127.0.0.1
export TEST_DB_PORT=3306
export TEST_DB_USER=root
export TEST_DB_PASSWORD=rootsecret
export TEST_DB_NAME=blog_test
make test-integration
```

With this repo’s `.env` (`DB_HOST` / `DB_PORT` / `DB_USER`), you can omit `TEST_DB_*` except keep `TEST_DB_NAME` off the application database (`majoo_blog` / `blog`). The default `blog_test` is intentional so tests do not truncate production data.

Covered: register → login → create post → publish (`post_publish_log` row) → comment → delete; plus 401 (missing token), 403 (cross-user edit/delete), 404 (unknown `public_id`), 422 (bad payload). Every JSON body is asserted against the `internal/response` envelope (allowed keys `message` / `data` / `meta` / `links` / `errors`; no raw `"id"` field). Publish-transaction rollback is **not** re-tested here.
