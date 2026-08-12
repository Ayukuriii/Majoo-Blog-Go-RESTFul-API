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

func list(w http.ResponseWriter, r *http.Request, s Service) {
	out, meta, links, err := s.List(r.Context(), parseListQuery(r))
	if err != nil {
		writeError(w, err, "failed to list posts")
		return
	}
	response.WithPaginatedData(w, out, meta, links, "Posts retrieved")
}

func get(w http.ResponseWriter, r *http.Request, s Service) {
	out, err := s.Get(r.Context(), r.PathValue("publicId"))
	if err != nil {
		writeError(w, err, "failed to get post")
		return
	}
	response.WithData(w, out, "Post retrieved")
}

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
