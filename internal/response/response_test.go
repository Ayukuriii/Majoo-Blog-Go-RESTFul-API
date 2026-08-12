package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithData_WithMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	WithData(rec, map[string]string{"public_id": "01HXYZ"}, "Post created")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeBody(t, rec)

	assertRawJSON(t, body["message"], `"Post created"`)
	assertRawJSON(t, body["data"], `{"public_id":"01HXYZ"}`)
	assertAbsent(t, body, "meta", "links")
}

func TestWithData_WithoutMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	WithData(rec, []string{"a", "b"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeBody(t, rec)

	assertAbsent(t, body, "message", "meta", "links")
	assertRawJSON(t, body["data"], `["a","b"]`)
}

func TestWithPaginatedData_WithMessage_NullPointersAndOmitempty(t *testing.T) {
	rec := httptest.NewRecorder()
	WithPaginatedData(rec, []any{}, Meta{
		CurrentPage: 1,
		From:        nil,
		LastPage:    1,
		Path:        "/api/posts",
		PerPage:     15,
		To:          nil,
		Total:       0,
	}, Links{
		First: "/api/posts?page=1&per_page=15",
		Last:  "/api/posts?page=1&per_page=15",
		Prev:  nil,
		Next:  nil,
	}, "Posts retrieved")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeBody(t, rec)

	assertRawJSON(t, body["message"], `"Posts retrieved"`)
	assertRawJSON(t, body["data"], `[]`)

	var meta map[string]json.RawMessage
	if err := json.Unmarshal(body["meta"], &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	assertRawJSON(t, meta["current_page"], `1`)
	assertRawJSON(t, meta["from"], `null`)
	assertRawJSON(t, meta["to"], `null`)
	assertRawJSON(t, meta["last_page"], `1`)
	assertRawJSON(t, meta["path"], `"/api/posts"`)
	assertRawJSON(t, meta["per_page"], `15`)
	assertRawJSON(t, meta["total"], `0`)
	assertAbsent(t, meta, "search", "sort", "order", "filters")

	var links map[string]json.RawMessage
	if err := json.Unmarshal(body["links"], &links); err != nil {
		t.Fatalf("unmarshal links: %v", err)
	}
	assertRawJSON(t, links["first"], `"/api/posts?page=1&per_page=15"`)
	assertRawJSON(t, links["last"], `"/api/posts?page=1&per_page=15"`)
	assertRawJSON(t, links["prev"], `null`)
	assertRawJSON(t, links["next"], `null`)
}

func TestWithPaginatedData_WithoutMessage_PopulatedMetaLinks(t *testing.T) {
	from, to := 16, 30
	prev := "/api/posts?page=1&per_page=15&search=hello&sort=created_at&order=desc&filter[status]=active"
	next := "/api/posts?page=3&per_page=15&search=hello&sort=created_at&order=desc&filter[status]=active"

	rec := httptest.NewRecorder()
	WithPaginatedData(rec, []map[string]string{{"public_id": "01HAAA", "title": "Widget"}}, Meta{
		CurrentPage: 2,
		From:        &from,
		LastPage:    10,
		Path:        "/api/posts",
		PerPage:     15,
		To:          &to,
		Total:       142,
		Search:      "hello",
		Sort:        "created_at",
		Order:       "desc",
		Filters:     map[string]any{"status": "active"},
	}, Links{
		First: "/api/posts?page=1&per_page=15&search=hello&sort=created_at&order=desc&filter[status]=active",
		Last:  "/api/posts?page=10&per_page=15&search=hello&sort=created_at&order=desc&filter[status]=active",
		Prev:  &prev,
		Next:  &next,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decodeBody(t, rec)

	assertAbsent(t, body, "message")
	assertRawJSON(t, body["data"], `[{"public_id":"01HAAA","title":"Widget"}]`)

	var meta map[string]json.RawMessage
	if err := json.Unmarshal(body["meta"], &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	assertRawJSON(t, meta["current_page"], `2`)
	assertRawJSON(t, meta["from"], `16`)
	assertRawJSON(t, meta["to"], `30`)
	assertRawJSON(t, meta["search"], `"hello"`)
	assertRawJSON(t, meta["sort"], `"created_at"`)
	assertRawJSON(t, meta["order"], `"desc"`)
	assertRawJSON(t, meta["filters"], `{"status":"active"}`)

	var links map[string]json.RawMessage
	if err := json.Unmarshal(body["links"], &links); err != nil {
		t.Fatalf("unmarshal links: %v", err)
	}
	assertRawJSON(t, links["prev"], `"`+prev+`"`)
	assertRawJSON(t, links["next"], `"`+next+`"`)
}

func TestWithMessage_DefaultAndCustomStatus(t *testing.T) {
	t.Run("default 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WithMessage(rec, "Post deleted")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		assertExactBody(t, rec, `{"message":"Post deleted"}`)
	})

	t.Run("custom status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WithMessage(rec, "Accepted", http.StatusAccepted)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
		}
		assertExactBody(t, rec, `{"message":"Accepted"}`)
	})
}

func TestError_DefaultAndCustomStatus(t *testing.T) {
	t.Run("default 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		Error(rec, "Post not found")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		assertExactBody(t, rec, `{"message":"Post not found"}`)
	})

	t.Run("custom status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		Error(rec, "Unauthorized", http.StatusUnauthorized)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		assertExactBody(t, rec, `{"message":"Unauthorized"}`)
	})
}

func TestWriteJSON_DoesNotEscapeHTMLOrUnicode(t *testing.T) {
	rec := httptest.NewRecorder()
	WithData(rec, map[string]string{
		"path":  "a/b",
		"html":  "<script>",
		"label": "日本語",
	})

	raw := rec.Body.String()
	for _, want := range []string{`a/b`, `<script>`, `日本語`} {
		if !strings.Contains(raw, want) {
			t.Errorf("body %q should contain unescaped %q", raw, want)
		}
	}
	for _, escaped := range []string{`\u003c`, `\u003e`, `\u0026`, `\/`} {
		if strings.Contains(raw, escaped) {
			t.Errorf("body %q should not contain escaped %q", raw, escaped)
		}
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v\nbody=%s", err, rec.Body.String())
	}
	return body
}

func assertRawJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var compactGot, compactWant []byte
	var err error
	if compactGot, err = json.Marshal(json.RawMessage(got)); err != nil {
		t.Fatalf("compact got: %v", err)
	}
	if compactWant, err = json.Marshal(json.RawMessage(want)); err != nil {
		t.Fatalf("compact want: %v", err)
	}
	if string(compactGot) != string(compactWant) {
		t.Fatalf("json = %s, want %s", compactGot, compactWant)
	}
}

func assertAbsent(t *testing.T, body map[string]json.RawMessage, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := body[k]; ok {
			t.Errorf("unexpected key %q present: %s", k, body[k])
		}
	}
}

func assertExactBody(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	_ = decodeBody(t, rec)
	got := strings.TrimSpace(rec.Body.String())
	var compactGot, compactWant []byte
	var err error
	if compactGot, err = json.Marshal(json.RawMessage(got)); err != nil {
		t.Fatalf("compact got: %v", err)
	}
	if compactWant, err = json.Marshal(json.RawMessage(want)); err != nil {
		t.Fatalf("compact want: %v", err)
	}
	if string(compactGot) != string(compactWant) {
		t.Fatalf("body = %s, want %s", compactGot, compactWant)
	}
}
