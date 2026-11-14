package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"shortUrl/internal/model"
	"shortUrl/internal/service"
)

// Handler is the HTTP layer: bind, validate the request shape, call the
// service, translate its errors to statuses. No business logic lives here.
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// GET /short-urls?limit=&offset=
// Lists live links only (expired ones are excluded; the hourly sweep
// removes them). limit defaults to 50 and is capped at 100; offset
// defaults to 0.
func (h *Handler) ListShortURLs(c *gin.Context) {
	limit, offset, ok := parsePagination(c)
	if !ok {
		writeError(c, http.StatusBadRequest, "Invalid pagination parameters, expected ?limit=<1-100>&offset=<0+>")
		return
	}
	links, err := h.svc.List(c.Request.Context(), limit, offset)
	if err != nil {
		// Don't leak the database error to the client; log it here.
		log.Println("ListShortURLs failed:", err)
		writeError(c, http.StatusInternalServerError, "Internal server error")
		return
	}
	c.JSON(http.StatusOK, links)
}

// parsePagination reads ?limit= and ?offset=, applying the defaults and
// the 100-row cap. A non-numeric or out-of-range value is rejected.
func parsePagination(c *gin.Context) (limit, offset int, ok bool) {
	limit = 50
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return 0, 0, false
		}
		limit = n
		if limit > 100 {
			limit = 100
		}
	}
	offset = 0
	if raw := c.Query("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}

// GET /short-urls/:hashKey
func (h *Handler) GetShortURLByHashKey(c *gin.Context) {
	link, err := h.svc.GetByHash(c.Request.Context(), c.Param("hashKey"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, link)
}

// GET /redirect/:hashKey
func (h *Handler) GetRedirect(c *gin.Context) {
	target, err := h.svc.Resolve(c.Request.Context(), c.Param("hashKey"))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.Redirect(http.StatusFound, target)
}

// POST /short-urls
func (h *Handler) CreateShortURL(c *gin.Context) {
	// Cap the request body: a create is a URL plus an optional expiry,
	// so anything past 1MB is abuse, not input. Without the cap, gin
	// reads an unbounded body into memory.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)

	var input model.CreateShortURLInput
	// ShouldBindJSON, not BindJSON: gin 1.11's MustBindWith (which
	// BindJSON calls) aborts with 413 on a MaxBytesError before
	// returning, and this handler maps every bind failure — including
	// an oversized body — to 400. ShouldBindWith returns the error
	// without writing a response, so the 400 below is the only status
	// the client sees.
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	link, err := h.svc.Create(c.Request.Context(), input.OriginalURL, input.ExpiryDate)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, link)
}

// writeError answers with the single error shape {"error": msg}. Every
// error response in this package goes through here so the shape can't
// drift between handlers.
func writeError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// writeServiceError maps a service error to the status and message the
// old handlers returned for the same condition.
func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(c, http.StatusNotFound, "Short URL not found")
	case errors.Is(err, service.ErrExpired):
		writeError(c, http.StatusGone, "Short URL expired")
	case errors.Is(err, service.ErrInvalidURL):
		writeError(c, http.StatusBadRequest, "Invalid URL provided")
	case errors.Is(err, service.ErrInvalidExpiry):
		writeError(c, http.StatusBadRequest, "Invalid expiry date, expected format YYYY-MM-DD HH:MM")
	case errors.Is(err, service.ErrExpiryPast):
		writeError(c, http.StatusBadRequest, "Expiry date must be a future timestamp")
	case errors.Is(err, service.ErrHashExhausted):
		writeError(c, http.StatusInternalServerError, "Failed to generate a unique short url, please retry")
	default:
		// Anything unrecognised here is a server-side failure (usually
		// the database). Log the real error, but answer with a generic
		// message — the raw error can expose SQLite internals.
		log.Println("Unhandled service error:", err)
		writeError(c, http.StatusInternalServerError, "Internal server error")
	}
}
