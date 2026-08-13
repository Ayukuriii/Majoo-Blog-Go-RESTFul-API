package comment

import (
	"blog-api/internal/middleware"
	"blog-api/internal/post"
	"blog-api/internal/response"
	"blog-api/internal/user"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
)

func RegisterRoutes(mux *http.ServeMux, s Service, auth func(http.Handler) http.Handler) {
	mux.Handle("POST /api/posts/{publicId}/comments", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		create(w, r, s)
	})))
	mux.HandleFunc("GET /api/posts/{publicId}/comments", func(w http.ResponseWriter, r *http.Request) {
		list(w, r, s)
	})
	mux.Handle("DELETE /api/comments/{publicId}", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remove(w, r, s)
	})))
}

// create adds a comment on a post.
//
//	@Summary		Create a comment
//	@Tags			comments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			publicId	path		string					true	"Post public_id (UUID v7)"
//	@Param			body		body		CreateCommentRequest	true	"Comment payload"
//	@Success		201			{object}	response.DataEnvelope{data=CommentResponse}
//	@Failure		400			{object}	response.MessageEnvelope
//	@Failure		401			{object}	response.MessageEnvelope
//	@Failure		404			{object}	response.MessageEnvelope
//	@Failure		422			{object}	response.ValidationEnvelope
//	@Failure		500			{object}	response.MessageEnvelope
//	@Router			/api/posts/{publicId}/comments [post]
func create(w http.ResponseWriter, r *http.Request, s Service) {
	callerPublicID, ok := callerPublicID(w, r)
	if !ok {
		return
	}
	var req CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	pathPostPublicID := r.PathValue("publicId")
	if req.PostPublicID == "" {
		req.PostPublicID = pathPostPublicID
	} else if req.PostPublicID != pathPostPublicID {
		response.Error(w, "post_public_id does not match path", http.StatusBadRequest)
		return
	}
	out, err := s.Create(r.Context(), callerPublicID, req)
	if err != nil {
		writeError(w, err, "failed to create comment")
		return
	}
	response.WithStatusData(w, http.StatusCreated, out, "Comment created")
}

// list returns a paginated list of comments on a post.
//
//	@Summary		List comments
//	@Tags			comments
//	@Produce		json
//	@Param			publicId	path		string	true	"Post public_id (UUID v7)"
//	@Param			page		query		int		false	"1-based page index"	default(1)
//	@Param			per_page	query		int		false	"Page size (max 100)"	default(15)
//	@Param			search		query		string	false	"Free-text search"
//	@Param			sort		query		string	false	"Sort column (created_at)"
//	@Param			order		query		string	false	"Sort direction"	Enums(asc, desc)
//	@Success		200			{object}	response.PaginatedEnvelope{data=[]CommentResponse}
//	@Failure		404			{object}	response.MessageEnvelope
//	@Failure		500			{object}	response.MessageEnvelope
//	@Router			/api/posts/{publicId}/comments [get]
func list(w http.ResponseWriter, r *http.Request, s Service) {
	out, meta, links, err := s.List(r.Context(), r.PathValue("publicId"), parseListQuery(r))
	if err != nil {
		writeError(w, err, "failed to list comments")
		return
	}
	response.WithPaginatedData(w, out, meta, links, "Comments retrieved")
}

// remove soft-deletes a comment (author or post owner).
//
//	@Summary		Delete a comment
//	@Tags			comments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			publicId	path		string	true	"Comment public_id (UUID v7)"
//	@Success		200			{object}	response.MessageEnvelope
//	@Failure		401			{object}	response.MessageEnvelope
//	@Failure		403			{object}	response.MessageEnvelope
//	@Failure		404			{object}	response.MessageEnvelope
//	@Failure		500			{object}	response.MessageEnvelope
//	@Router			/api/comments/{publicId} [delete]
func remove(w http.ResponseWriter, r *http.Request, s Service) {
	callerPublicID, ok := callerPublicID(w, r)
	if !ok {
		return
	}
	if err := s.Delete(r.Context(), callerPublicID, r.PathValue("publicId")); err != nil {
		writeError(w, err, "failed to delete comment")
		return
	}
	response.WithMessage(w, "Comment deleted")
}

func callerPublicID(w http.ResponseWriter, r *http.Request) (string, bool) {
	publicID, ok := middleware.UserPublicIDFromContext(r.Context())
	if !ok {
		response.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return publicID, true
}

func writeError(w http.ResponseWriter, err error, fallback string) {
	var validatorErr validator.ValidationErrors
	switch {
	case errors.As(err, &validatorErr):
		response.ValidationError(w, validatorErr)
	case errors.Is(err, ErrForbidden):
		response.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrNotFound):
		response.Error(w, "comment not found", http.StatusNotFound)
	case errors.Is(err, post.ErrNotFound):
		response.Error(w, "post not found", http.StatusNotFound)
	case errors.Is(err, user.ErrNotFound):
		response.Error(w, "user not found", http.StatusNotFound)
	default:
		response.Error(w, fallback, http.StatusInternalServerError)
	}
}

func parseListQuery(r *http.Request) ListQuery {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	return ListQuery{
		Page:    page,
		PerPage: perPage,
		Search:  q.Get("search"),
		Sort:    q.Get("sort"),
		Order:   q.Get("order"),
		Path:    r.URL.Path, // /api/posts/{publicId}/comments
	}
}
