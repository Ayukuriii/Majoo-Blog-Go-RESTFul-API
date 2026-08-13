//go:build integration

package post_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"blog-api/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unknownPublicID = "0190f0e2-8c3a-7b2d-9e4f-ffffffffffff"

func emailFor(t *testing.T, suffix string) string {
	t.Helper()
	base := strings.ToLower(strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()))
	return fmt.Sprintf("%s-%s@example.com", base, suffix)
}

func TestIntegration_CreatePublishDelete(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "Owner")

	created := s.DoJSON(t, http.MethodPost, "/api/posts", owner.Token, map[string]any{
		"title": "Hello world",
		"body":  "First post body",
	}, http.StatusCreated)
	assert.Equal(t, "Post created", created["message"])
	post := testutil.DataObject(t, created)
	postID, _ := post["public_id"].(string)
	require.NotEmpty(t, postID)
	assert.Equal(t, "draft", post["status"])
	author, _ := post["author"].(map[string]any)
	assert.Equal(t, owner.PublicID, author["public_id"])

	published := s.DoJSON(t, http.MethodPost, "/api/posts/"+postID+"/publish", owner.Token, nil, http.StatusOK)
	assert.Equal(t, "Post published", published["message"])
	assert.Equal(t, "published", testutil.DataObject(t, published)["status"])

	var logCount int64
	err := s.DB.Raw(`
		SELECT COUNT(*) FROM post_publish_log pl
		INNER JOIN posts p ON p.id = pl.post_id
		WHERE p.public_id = ?`, postID).Scan(&logCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), logCount)

	listed := s.DoJSON(t, http.MethodGet, "/api/posts?page=1&per_page=15", "", nil, http.StatusOK)
	assert.Equal(t, "Posts retrieved", listed["message"])
	assert.GreaterOrEqual(t, len(testutil.DataArray(t, listed)), 1)

	got := s.DoJSON(t, http.MethodGet, "/api/posts/"+postID, "", nil, http.StatusOK)
	assert.Equal(t, "Post retrieved", got["message"])

	deleted := s.DoJSON(t, http.MethodDelete, "/api/posts/"+postID, owner.Token, nil, http.StatusOK)
	assert.Equal(t, "Post deleted", deleted["message"])
	_, hasData := deleted["data"]
	assert.False(t, hasData)

	s.DoJSON(t, http.MethodGet, "/api/posts/"+postID, "", nil, http.StatusNotFound)
}

func TestIntegration_UnauthorizedMissingToken(t *testing.T) {
	s := testutil.NewServer(t)
	env := s.DoJSON(t, http.MethodPost, "/api/posts", "", map[string]any{
		"title": "Nope",
		"body":  "Missing auth",
	}, http.StatusUnauthorized)
	assert.Equal(t, "missing or invalid authorization header", env["message"])
}

func TestIntegration_ForbiddenCrossUserEditDelete(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "")
	other := s.RegisterAndLogin(t, emailFor(t, "other"), "secret123", "")

	created := s.DoJSON(t, http.MethodPost, "/api/posts", owner.Token, map[string]any{
		"title": "Owner post",
		"body":  "Only owner may change this",
	}, http.StatusCreated)
	postID := testutil.PublicIDFrom(t, created)

	patch := s.DoJSON(t, http.MethodPatch, "/api/posts/"+postID, other.Token, map[string]any{
		"title": "Hijacked",
	}, http.StatusForbidden)
	assert.Equal(t, "forbidden", patch["message"])

	del := s.DoJSON(t, http.MethodDelete, "/api/posts/"+postID, other.Token, nil, http.StatusForbidden)
	assert.Equal(t, "forbidden", del["message"])
}

func TestIntegration_NotFoundUnknownPublicID(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "")
	env := s.DoJSON(t, http.MethodGet, "/api/posts/"+unknownPublicID, "", nil, http.StatusNotFound)
	assert.Equal(t, "post not found", env["message"])

	pub := s.DoJSON(t, http.MethodPost, "/api/posts/"+unknownPublicID+"/publish", owner.Token, nil, http.StatusNotFound)
	assert.Equal(t, "post not found", pub["message"])
}

func TestIntegration_ValidationBadPayload(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "")
	env := s.DoJSON(t, http.MethodPost, "/api/posts", owner.Token, map[string]any{}, http.StatusUnprocessableEntity)
	assert.Equal(t, "validation failed", env["message"])
	errs := env["errors"].(map[string]any)
	assert.NotEmpty(t, errs["title"])
	assert.NotEmpty(t, errs["body"])
}
