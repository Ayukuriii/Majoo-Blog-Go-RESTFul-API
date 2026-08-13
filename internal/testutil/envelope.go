//go:build integration

package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var envelopeKeys = map[string]struct{}{
	"message": {},
	"data":    {},
	"meta":    {},
	"links":   {},
	"errors":  {},
}

// AssertEnvelope checks the unified internal/response JSON shape and that no
// nested object ever exposes a raw "id" field.
func AssertEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	ct := rec.Header().Get("Content-Type")
	require.True(t, strings.HasPrefix(ct, "application/json"), "Content-Type = %q", ct)

	raw := rec.Body.Bytes()
	var top any
	require.NoError(t, json.Unmarshal(raw, &top), "body: %s", raw)
	obj, ok := top.(map[string]any)
	require.True(t, ok, "envelope must be a JSON object, got %T: %s", top, raw)

	for k := range obj {
		if _, allowed := envelopeKeys[k]; !allowed {
			t.Errorf("unexpected envelope key %q in %s", k, raw)
		}
	}
	_, hasMessage := obj["message"]
	_, hasData := obj["data"]
	_, hasErrors := obj["errors"]
	require.True(t, hasMessage || hasData || hasErrors, "envelope missing message/data/errors: %s", raw)

	if rec.Code >= 400 {
		msg, _ := obj["message"].(string)
		require.NotEmpty(t, msg, "error envelope needs a message: %s", raw)
	}
	if rec.Code == http.StatusUnprocessableEntity {
		errs, ok := obj["errors"].(map[string]any)
		require.True(t, ok && len(errs) > 0, "422 envelope needs errors: %s", raw)
	}

	if meta, ok := obj["meta"]; ok {
		assertMeta(t, meta, raw)
	}
	if links, ok := obj["links"]; ok {
		assertLinks(t, links, raw)
	}
	if _, hasMeta := obj["meta"]; hasMeta {
		_, hasLinks := obj["links"]
		require.True(t, hasLinks, "paginated envelope needs links: %s", raw)
		_, dataOK := obj["data"].([]any)
		require.True(t, dataOK, "paginated envelope data must be an array: %s", raw)
	}

	assertNoRawID(t, top, "$")
	require.False(t, t.Failed(), "envelope shape failed: %s", raw)
	return obj
}

func assertMeta(t *testing.T, meta any, raw []byte) {
	t.Helper()
	m, ok := meta.(map[string]any)
	require.True(t, ok, "meta must be an object: %s", raw)
	for _, k := range []string{"current_page", "from", "last_page", "path", "per_page", "to", "total"} {
		_, ok := m[k]
		require.True(t, ok, "meta missing %q: %s", k, raw)
	}
}

func assertLinks(t *testing.T, links any, raw []byte) {
	t.Helper()
	m, ok := links.(map[string]any)
	require.True(t, ok, "links must be an object: %s", raw)
	for _, k := range []string{"first", "last", "prev", "next"} {
		_, ok := m[k]
		require.True(t, ok, "links missing %q: %s", k, raw)
	}
}

func assertNoRawID(t *testing.T, v any, path string) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if k == "id" {
				t.Errorf("raw %q field at %s", k, path+".id")
			}
			assertNoRawID(t, child, path+"."+k)
		}
	case []any:
		for i, child := range x {
			assertNoRawID(t, child, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

func DataObject(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	data, ok := env["data"].(map[string]any)
	require.True(t, ok, "data is not an object: %#v", env["data"])
	return data
}

func DataArray(t *testing.T, env map[string]any) []any {
	t.Helper()
	data, ok := env["data"].([]any)
	require.True(t, ok, "data is not an array: %#v", env["data"])
	return data
}

func PublicIDFrom(t *testing.T, env map[string]any) string {
	t.Helper()
	id, _ := DataObject(t, env)["public_id"].(string)
	require.NotEmpty(t, id)
	return id
}
