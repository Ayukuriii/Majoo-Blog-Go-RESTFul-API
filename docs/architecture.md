# Architecture Decisions

This document records the stack and design choices for the Majoo Blog RESTful API, and why they were chosen.

## Summary

| Concern | Choice | Package / approach |
|---------|--------|--------------------|
| Database | MySQL | MySQL via GORM dialector |
| ORM | GORM | `gorm.io/gorm` + `gorm.io/driver/mysql` |
| HTTP routing | Standard library | `net/http` (`ServeMux`) |
| JSON | Standard library | `encoding/json` |
| Password hashing | bcrypt | `golang.org/x/crypto/bcrypt` |
| Auth tokens | JWT | `github.com/golang-jwt/jwt/v5` |
| Request validation | Struct tags | `github.com/go-playground/validator/v10` |
| Public identifiers | `public_id` (UUID v7) | Opaque time-ordered UUID in API; numeric `id` stays internal PK |
| Deletion | Soft delete (`deleted_at`) | GORM `DeletedAt` on feature resources; audit/log tables exempt |

---

## 1. MySQL

**Decision:** Use MySQL as the primary datastore.

**Reasons:**

- Familiar relational model for blog entities (users, posts, comments) with foreign keys and transactions.
- Wide hosting and ops support; matches common deployment targets for this project.
- Stable ecosystem and first-class support in `golang-migrate` and GORM’s MySQL dialector.

**Conventions:**

- Connection settings come from environment variables (see `.env.example`); never hardcode credentials.
- Schema changes only via numbered files under `migrations/` (do not rely on GORM AutoMigrate for production schema).
- Pool / connection tuning is configured when opening the GORM DB in `internal/database`.

---

## 2. GORM as the ORM

**Decision:** Access MySQL through [GORM](https://gorm.io) (`gorm.io/gorm` with `gorm.io/driver/mysql`). Do not use raw `database/sql` repositories as the primary data layer.

**Reasons:**

- **Model-centric mapping** — structs with tags map cleanly to tables (`id`, `public_id`, FKs, timestamps), which fits a feature-based layout (`model.go` + `repository.go`).
- **Less boilerplate** — common CRUD, associations, and pagination need less hand-written SQL than `database/sql` scanners.
- **Associations** — blog domains (user → posts → comments) are expressed with GORM associations / preloads while still storing FKs on internal `id`.
- **Transactions** — `db.WithContext(ctx).Transaction(...)` keeps service-owned transaction boundaries without managing `*sql.Tx` by hand.
- **Context & safety** — `WithContext` propagates cancellation; use bound arguments / struct conditions (never string-concatenate user input into queries).
- **MySQL dialector** — official driver integration matches the MySQL choice above.

**Conventions:**

- Open and configure `*gorm.DB` in `internal/database` (DSN from env, pool settings via underlying `sql.DB`).
- Repositories accept `*gorm.DB` (or a transactional session) and return domain models / errors — never HTTP types.
- Services own transaction boundaries: begin via GORM transaction helpers, pass the tx session into repository calls.
- Prefer GORM APIs for CRUD; use `Raw` / `Exec` only when a query is awkward or performance-critical, still with bound parameters.
- Whitelist sort/filter columns in the **service** before applying `Order` / `Where` — never pass raw client field names into GORM.
- Keep schema authority in `migrations/`; treat AutoMigrate as optional local convenience only, not the production migration path.
- Lookup by `public_id` with GORM (`Where("public_id = ?", ...)`); assign FKs using internal `id` after the resolve → validate flow (§8).
- Feature models use `gorm.DeletedAt` so soft-deleted rows are excluded by default (§9).

**Do not:**

- Call GORM from handlers.
- Put business rules (active checks, ownership) only in repository hooks — keep them in the service layer.
- Expose GORM models directly as API JSON if they contain `id` or password hashes — map to response DTOs.

---

## 3. `net/http` as the router

**Decision:** Route HTTP with the standard library `net/http` (`http.ServeMux` and related APIs). Do not introduce a third-party router (chi, gin, echo, httprouter, etc.) unless requirements change.

**Reasons:**

- Zero extra dependency for routing; smaller surface area and fewer upgrade concerns.
- Go 1.22+ method-aware patterns (`GET /api/posts/{publicId}`) cover the needs of this API.
- Middleware is plain `func(http.Handler) http.Handler`, which composes cleanly with stdlib handlers.
- Keeps the learning and review surface on application code, not framework magic.

**Conventions:**

- Register routes under `/api/...`.
- Wrap the mux with shared middleware (recovery, auth, CORS) in `cmd/api/main.go`.
- Feature handlers stay thin: decode → call service → write `internal/response` helpers.

---

## 4. `encoding/json`

**Decision:** Encode and decode JSON with the standard library `encoding/json` only.

**Reasons:**

- Sufficient for request/response DTOs and the unified response envelope.
- No third-party JSON library to audit or pin.
- Aligns with response helpers that already target a stable JSON shape.

**Conventions:**

- All success/error bodies go through `internal/response` (`WithData`, `WithPaginatedData`, `WithMessage`, `Error`).
- Prefer struct tags (`json:"..."`) on DTOs; avoid ad-hoc `map[string]any` in handlers except for trivial health checks.
- Match project JSON flags where the response package defines them (unescaped Unicode/slashes, preserve zero fractions).

---

## 5. bcrypt for passwords

**Decision:** Hash and verify passwords with `golang.org/x/crypto/bcrypt`.

**Reasons:**

- Industry-standard adaptive hash suitable for password storage (not reversible encryption).
- Built-in cost factor so work factor can be raised over time.
- Avoids inventing custom crypto or relying on outdated schemes (plain MD5/SHA alone).

**Conventions:**

- Never store or log plaintext passwords.
- Hash in the **service** layer on register / password change; compare with `bcrypt.CompareHashAndPassword` on login.
- Do not put bcrypt logic in handlers or repositories beyond persisting the resulting hash string.

---

## 6. `golang-jwt/jwt/v5` for authentication

**Decision:** Issue and validate access tokens with `github.com/golang-jwt/jwt/v5`.

**Reasons:**

- Stateless auth for a RESTful blog API (no server-side session store required for access tokens).
- Well-maintained v5 API with clear signing/validation flows.
- Claims can carry stable subject info (e.g. user `public_id`) without exposing internal numeric IDs in clients if preferred.

**Conventions:**

- JWT secret/keys and TTL come from config/env.
- Validation lives in `internal/middleware` (or a small auth helper used by middleware).
- Features must not reimplement token parsing; they read identity from context set by middleware.
- Prefer putting **public** identifiers in claims for any value that might reach logs or clients; keep internal `id` out of tokens when possible.

---

## 7. go-playground/validator for request validation

**Decision:** Validate inbound request DTOs with `github.com/go-playground/validator/v10` in the **service** layer.

**Reasons:**

- Declarative rules via struct tags (`validate:"required,email,max=200"`) keep validation next to the DTO.
- Consistent error surface across features.
- Separates “is the payload well-formed?” from HTTP transport concerns.

**Conventions:**

- Handlers only decode/bind; services call `validate.Struct(...)`.
- Do not trust client field names for SQL sort/filter — whitelist in the service after validation.
- Map validation failures to a clear `response.Error` message (typically 400).

---

## 8. `public_id` as the public-facing identifier (UUID v7)

**Decision:** Every externally addressable resource has a `public_id` generated as a **[UUID version 7](https://www.rfc-editor.org/rfc/rfc9562.html#name-uuid-version-7)** (RFC 9562). The numeric `id` remains the primary key and is used only inside the database and application internals. **Never expose raw database `id` in API paths, request bodies, or response JSON.**

### Why

- Sequential integer IDs leak growth metrics and enable easy enumeration (`/posts/1`, `/posts/2`, …).
- Opaque public IDs reduce IDOR-style probing and hide internal row ordering.
- Keeping a numeric PK preserves efficient joins, indexes, and foreign keys (including GORM associations).
- **UUID v7** is preferred over UUID v4 or ULID because it is:
  - a standard UUID (RFC 9562) with familiar 8-4-4-4-12 string form
  - time-ordered (Unix epoch ms in the high bits), which sorts better in indexes and logs than random UUIDs
  - still not guessable like auto-increment integers (random bits after the timestamp)

### Schema pattern

Typical columns on a resource table:

| Column | Role |
|--------|------|
| `id` | Internal PK (`BIGINT` / auto-increment). Used in FKs (`post_id`, `user_id`, …). |
| `public_id` | Unique, indexed public identifier — **UUID v7** (`CHAR(36)`). Appears in URLs and JSON as `public_id` (or resource-specific name like `post_public_id` only if needed). |
| `deleted_at` | Soft-delete timestamp on feature resources (see §9). |
| Domain columns | `title`, `body`, `status`, timestamps, etc. |

Foreign keys between tables always reference **`id`**, never `public_id`.

Generate `public_id` in the **service** (or a small shared helper) when creating a resource — do not accept client-supplied `public_id` on create unless there is an explicit idempotency design.

### API surface

- Paths: `/api/posts/{publicId}`, `/api/comments/{publicId}`, …
- JSON: `"public_id": "0190f0e2-8c3a-7b2d-9e4f-1a2b3c4d5e6f"`, never `"id": 1`.
- Clients send parent references by **parent `public_id`** (e.g. create comment with `post_public_id`), not parent numeric id.

### Resolve flow (child → parent / related record)

When a request references another record by public identifier (path param or body field), the service follows this order:

1. **Lookup by `public_id`**  
   Load the referenced row (e.g. post) via GORM using the UUID v7 from the client.

2. **Validate the record**  
   Apply business rules before using it as a relation target, for example:
   - exists (not found → 404)
   - `is_active` / status flags
   - ownership / authorization when required
   - soft-delete / visibility rules

3. **Use internal `id` for relations**  
   If validation passes, take the parent’s internal `id` and use it for FK writes or associations  
   (e.g. insert comment with `post_id = 1` where `1` is the post’s PK — never returned to the client).

```
Client: POST /api/comments  { "post_public_id": "0190f0e2-8c3a-7b2d-9e4f-1a2b3c4d5e6f", "body": "..." }
                │
                ▼
Service: gorm Find post WHERE public_id = ?
                │
                ▼
         validate (exists, is_active, …)
                │
                ▼
         comment.post_id = post.id   // internal only
                │
                ▼
Response: { "data": { "public_id": "0191a2b3-...", "body": "...", ... } }  // UUID v7; no numeric ids
```

### Reading nested resources

Same rule when listing or loading children:

1. Resolve parent by `public_id`.
2. Validate parent.
3. Query children with `Where("post_id = ?", parent.ID)` (or equivalent association scope).
4. Map each child to a response DTO that exposes `public_id` only.

### Do not

- Put `id` on response DTOs or in JWT claims meant for clients.
- Accept numeric database ids from the client for lookups or FK assignment.
- Treat client-supplied `public_id` as a foreign key — always resolve → validate → use internal `id`.
- Use UUID v4, ULID, or other formats for new `public_id` values — stick to UUID v7.

---

## 9. Soft delete (`deleted_at`)

**Decision:** User-facing feature resources soft-delete via a nullable `deleted_at` column. GORM models use `gorm.DeletedAt` so default queries exclude deleted rows.

**Reasons:**

- Recoverability and audit-friendly history without immediate hard deletes.
- Matches GORM’s built-in soft-delete support without custom delete flags.
- Keeps FK cascades for rare hard-purge paths while normal API deletes only set `deleted_at`.

**Conventions:**

- Feature tables (`users`, `posts`, `comments`, …) include `deleted_at DATETIME(3) NULL` and an index on `deleted_at`.
- Repositories use GORM’s default scope; hard delete (`Unscoped`) only for explicit purge/admin flows.
- Audit/event log tables (e.g. `post_publish_log`) omit soft delete — they record facts and stay until hard-purged with the parent.
- Unique constraints (e.g. `users.email`) still apply to soft-deleted rows until hard-purged.
- Resolve-flow validation (§8) must treat soft-deleted parents as not found / unavailable.

---

## Request flow (all choices together)

```
HTTP Request (net/http)
  → middleware (JWT via golang-jwt/jwt/v5, recovery, …)
  → handler (encoding/json decode; path public_id)
  → service (validator; resolve public_id → validate → internal id; bcrypt when needed; GORM tx)
  → repository (GORM + MySQL; WithContext)
  → service maps entities → DTOs (public_id only)
  → handler writes unified JSON envelope (encoding/json)
```

Layering and package layout follow `.cursorrules`. This document explains **why** the stack and `public_id` rules exist; `.cursorrules` is the day-to-day implementation blueprint.
