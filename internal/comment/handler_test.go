//go:build integration

package comment_test

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

func TestIntegration_HappyPathRegisterLoginPostPublishCommentDelete(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "Owner")

	created := s.DoJSON(t, http.MethodPost, "/api/posts", owner.Token, map[string]any{
		"title": "Published story",
		"body":  "Ready for comments",
	}, http.StatusCreated)
	postID := testutil.PublicIDFrom(t, created)

	published := s.DoJSON(t, http.MethodPost, "/api/posts/"+postID+"/publish", owner.Token, nil, http.StatusOK)
	assert.Equal(t, "published", testutil.DataObject(t, published)["status"])

	var logCount int64
	err := s.DB.Raw(`
		SELECT COUNT(*) FROM post_publish_log pl
		INNER JOIN posts p ON p.id = pl.post_id
		WHERE p.public_id = ?`, postID).Scan(&logCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), logCount)

	commenter := s.RegisterAndLogin(t, emailFor(t, "commenter"), "secret123", "Commenter")
	createdC := s.DoJSON(t, http.MethodPost, "/api/posts/"+postID+"/comments", commenter.Token, map[string]any{
		"body": "Nice post",
	}, http.StatusCreated)
	assert.Equal(t, "Comment created", createdC["message"])
	c := testutil.DataObject(t, createdC)
	commentID, _ := c["public_id"].(string)
	require.NotEmpty(t, commentID)
	assert.Equal(t, "Nice post", c["body"])
	postRef, _ := c["post"].(map[string]any)
	authorRef, _ := c["author"].(map[string]any)
	assert.Equal(t, postID, postRef["public_id"])
	assert.Equal(t, commenter.PublicID, authorRef["public_id"])

	listed := s.DoJSON(t, http.MethodGet, "/api/posts/"+postID+"/comments", "", nil, http.StatusOK)
	assert.Equal(t, "Comments retrieved", listed["message"])
	assert.Len(t, testutil.DataArray(t, listed), 1)

	deletedC := s.DoJSON(t, http.MethodDelete, "/api/comments/"+commentID, commenter.Token, nil, http.StatusOK)
	assert.Equal(t, "Comment deleted", deletedC["message"])

	deletedP := s.DoJSON(t, http.MethodDelete, "/api/posts/"+postID, owner.Token, nil, http.StatusOK)
	assert.Equal(t, "Post deleted", deletedP["message"])
}

func TestIntegration_UnauthorizedMissingToken(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "")
	created := s.DoJSON(t, http.MethodPost, "/api/posts", owner.Token, map[string]any{
		"title": "T",
		"body":  "B",
	}, http.StatusCreated)
	postID := testutil.PublicIDFrom(t, created)

	env := s.DoJSON(t, http.MethodPost, "/api/posts/"+postID+"/comments", "", map[string]any{
		"body": "no token",
	}, http.StatusUnauthorized)
	assert.Equal(t, "missing or invalid authorization header", env["message"])
}

func TestIntegration_ForbiddenCrossUserDelete(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "")
	commenter := s.RegisterAndLogin(t, emailFor(t, "commenter"), "secret123", "")
	stranger := s.RegisterAndLogin(t, emailFor(t, "stranger"), "secret123", "")

	created := s.DoJSON(t, http.MethodPost, "/api/posts", owner.Token, map[string]any{
		"title": "T",
		"body":  "B",
	}, http.StatusCreated)
	postID := testutil.PublicIDFrom(t, created)
	createdC := s.DoJSON(t, http.MethodPost, "/api/posts/"+postID+"/comments", commenter.Token, map[string]any{
		"body": "mine",
	}, http.StatusCreated)
	commentID := testutil.PublicIDFrom(t, createdC)

	env := s.DoJSON(t, http.MethodDelete, "/api/comments/"+commentID, stranger.Token, nil, http.StatusForbidden)
	assert.Equal(t, "forbidden", env["message"])
}

func TestIntegration_NotFoundUnknownPublicID(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "")

	env := s.DoJSON(t, http.MethodGet, "/api/posts/"+unknownPublicID+"/comments", "", nil, http.StatusNotFound)
	assert.Equal(t, "post not found", env["message"])

	del := s.DoJSON(t, http.MethodDelete, "/api/comments/"+unknownPublicID, owner.Token, nil, http.StatusNotFound)
	assert.Equal(t, "comment not found", del["message"])
}

func TestIntegration_ValidationBadPayload(t *testing.T) {
	s := testutil.NewServer(t)
	owner := s.RegisterAndLogin(t, emailFor(t, "owner"), "secret123", "")
	created := s.DoJSON(t, http.MethodPost, "/api/posts", owner.Token, map[string]any{
		"title": "T",
		"body":  "B",
	}, http.StatusCreated)
	postID := testutil.PublicIDFrom(t, created)

	env := s.DoJSON(t, http.MethodPost, "/api/posts/"+postID+"/comments", owner.Token, map[string]any{}, http.StatusUnprocessableEntity)
	assert.Equal(t, "validation failed", env["message"])
	errs := env["errors"].(map[string]any)
	assert.NotEmpty(t, errs["body"])
}
