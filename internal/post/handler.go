package post

import (
	"blog-api/internal/middleware"
	"blog-api/internal/response"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
)

func RegisterRoutes(mux *http.ServeMux, s Service, auth func(http.Handler) http.Handler) {
	mux.Handle("POST /api/posts", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		create(w, r, s)
	})))
	mux.HandleFunc("GET /api/posts", func(w http.ResponseWriter, r *http.Request) {
		list(w, r, s)
	})
	mux.HandleFunc("GET /api/posts/{publicId}", func(w http.ResponseWriter, r *http.Request) {
		get(w, r, s)
	})
	mux.Handle("PATCH /api/posts/{publicId}", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		update(w, r, s)
	})))
	mux.Handle("DELETE /api/posts/{publicId}", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remove(w, r, s)
	})))
	mux.Handle("POST /api/posts/{publicId}/publish", auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		publish(w, r, s)
	})))
}

// create creates a draft post for the authenticated user.
//
//	@Summary		Create a post
//	@Tags			posts
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		CreatePostRequest	true	"Post payload"
//	@Success		201		{object}	response.DataEnvelope{data=PostResponse}
//	@Failure		400		{object}	response.MessageEnvelope
//	@Failure		401		{object}	response.MessageEnvelope
//	@Failure		422		{object}	response.ValidationEnvelope
//	@Failure		500		{object}	response.MessageEnvelope
//	@Router			/api/posts [post]
func create(w http.ResponseWriter, r *http.Request, s Service) {
	authorPublicID, ok := callerPublicID(w, r)
	if !ok {
		return
	}
	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	out, err := s.Create(r.Context(), authorPublicID, req)
	if err != nil {
		writeError(w, err, "failed to create post")
		return
	}
	response.WithStatusData(w, http.StatusCreated, out, "Post created")
}

// list returns a paginated list of posts.
//
//	@Summary		List posts
//	@Tags			posts
//	@Produce		json
//	@Param			page			query		int		false	"1-based page index"	default(1)
//	@Param			per_page		query		int		false	"Page size (max 100)"	default(15)
//	@Param			search			query		string	false	"Free-text search"
//	@Param			sort			query		string	false	"Sort column (created_at, title)"
//	@Param			order			query		string	false	"Sort direction"	Enums(asc, desc)
//	@Param			filter[status]	query		string	false	"Exact status filter"	Enums(draft, published)
//	@Success		200				{object}	response.PaginatedEnvelope{data=[]PostResponse}
//	@Failure		500				{object}	response.MessageEnvelope
//	@Router			/api/posts [get]
func list(w http.ResponseWriter, r *http.Request, s Service) {
	out, meta, links, err := s.List(r.Context(), parseListQuery(r))
	if err != nil {
		writeError(w, err, "failed to list posts")
		return
	}
	response.WithPaginatedData(w, out, meta, links, "Posts retrieved")
}

// get returns a single post by public_id.
//
//	@Summary		Get a post
//	@Tags			posts
//	@Produce		json
//	@Param			publicId	path		string	true	"Post public_id (UUID v7)"
//	@Success		200			{object}	response.DataEnvelope{data=PostResponse}
//	@Failure		404			{object}	response.MessageEnvelope
//	@Failure		500			{object}	response.MessageEnvelope
//	@Router			/api/posts/{publicId} [get]
func get(w http.ResponseWriter, r *http.Request, s Service) {
	out, err := s.Get(r.Context(), r.PathValue("publicId"))
	if err != nil {
		writeError(w, err, "failed to get post")
		return
	}
	response.WithData(w, out, "Post retrieved")
}

// update partially updates a post owned by the caller.
//
//	@Summary		Update a post
//	@Tags			posts
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			publicId	path		string				true	"Post public_id (UUID v7)"
//	@Param			body		body		UpdatePostRequest	true	"Fields to update"
//	@Success		200			{object}	response.DataEnvelope{data=PostResponse}
//	@Failure		400			{object}	response.MessageEnvelope
//	@Failure		401			{object}	response.MessageEnvelope
//	@Failure		403			{object}	response.MessageEnvelope
//	@Failure		404			{object}	response.MessageEnvelope
//	@Failure		422			{object}	response.ValidationEnvelope
//	@Failure		500			{object}	response.MessageEnvelope
//	@Router			/api/posts/{publicId} [patch]
func update(w http.ResponseWriter, r *http.Request, s Service) {
	authorPublicID, ok := callerPublicID(w, r)
	if !ok {
		return
	}
	var req UpdatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	out, err := s.Update(r.Context(), authorPublicID, r.PathValue("publicId"), req)
	if err != nil {
		writeError(w, err, "failed to update post")
		return
	}
	response.WithData(w, out, "Post updated")
}

// remove soft-deletes a post owned by the caller.
//
//	@Summary		Delete a post
//	@Tags			posts
//	@Produce		json
//	@Security		BearerAuth
//	@Param			publicId	path		string	true	"Post public_id (UUID v7)"
//	@Success		200			{object}	response.MessageEnvelope
//	@Failure		401			{object}	response.MessageEnvelope
//	@Failure		403			{object}	response.MessageEnvelope
//	@Failure		404			{object}	response.MessageEnvelope
//	@Failure		500			{object}	response.MessageEnvelope
//	@Router			/api/posts/{publicId} [delete]
func remove(w http.ResponseWriter, r *http.Request, s Service) {
	authorPublicID, ok := callerPublicID(w, r)
	if !ok {
		return
	}
	if err := s.Delete(r.Context(), authorPublicID, r.PathValue("publicId")); err != nil {
		writeError(w, err, "failed to delete post")
		return
	}
	response.WithMessage(w, "Post deleted")
}

// publish publishes a post owned by the caller.
//
//	@Summary		Publish a post
//	@Tags			posts
//	@Produce		json
//	@Security		BearerAuth
//	@Param			publicId	path		string	true	"Post public_id (UUID v7)"
//	@Success		200			{object}	response.DataEnvelope{data=PostResponse}
//	@Failure		401			{object}	response.MessageEnvelope
//	@Failure		403			{object}	response.MessageEnvelope
//	@Failure		404			{object}	response.MessageEnvelope
//	@Failure		500			{object}	response.MessageEnvelope
//	@Router			/api/posts/{publicId}/publish [post]
func publish(w http.ResponseWriter, r *http.Request, s Service) {
	authorPublicID, ok := callerPublicID(w, r)
	if !ok {
		return
	}
	out, err := s.PublishWithSnapshot(r.Context(), authorPublicID, r.PathValue("publicId"))
	if err != nil {
		writeError(w, err, "failed to publish post")
		return
	}
	response.WithData(w, out, "Post published")
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
		response.ValidationError(w, validatorErr) // 422
	case errors.Is(err, ErrForbidden):
		response.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrNotFound):
		response.Error(w, "post not found", http.StatusNotFound)
	default:
		response.Error(w, fallback, http.StatusInternalServerError)
	}
}

func parseListQuery(r *http.Request) ListQuery {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	filters := map[string]string{}
	if status := q.Get("filter[status]"); status != "" {
		filters["status"] = status
	}
	return ListQuery{
		Page:    page,
		PerPage: perPage,
		Search:  q.Get("search"),
		Sort:    q.Get("sort"),
		Order:   q.Get("order"),
		Filters: filters,
		Path:    r.URL.Path, // "/api/posts" — used in meta.path and links
	}
}
