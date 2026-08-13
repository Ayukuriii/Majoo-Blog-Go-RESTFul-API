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

`cmd/api` and `internal/database` stay at 0% because this step targets services and repositories, not process bootstrap or MySQL dialector wiring. Handler files in feature packages are also unexercised here; they are covered by `test/http/*.http` instead.

To refresh:

```bash
go test ./... -cover
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -n 1
```
