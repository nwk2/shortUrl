package handler

import (
	"errors"
	"net/http"

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

// GET /short-urls
func (h *Handler) ListShortUrls(c *gin.Context) {
	links, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, links)
}

// GET /short-urls/:hashKey
func (h *Handler) GetShortUrlByHashKey(c *gin.Context) {
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
func (h *Handler) CreateShortUrl(c *gin.Context) {
	var input model.CreateShortUrlInput
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	link, err := h.svc.Create(c.Request.Context(), input.OriginalUrl, input.ExpiryDate)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, link)
}

// writeServiceError maps a service error to the status and message the
// old handlers returned for the same condition.
func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "ShortUrl not found"})
	case errors.Is(err, service.ErrExpired):
		c.JSON(http.StatusGone, gin.H{"error": "ShortUrl expired"})
	case errors.Is(err, service.ErrInvalidURL):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid url provided..."})
	case errors.Is(err, service.ErrInvalidExpiry):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry date, expected format YYYY-MM-DD HH:MM"})
	case errors.Is(err, service.ErrExpiryPast):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Expiry date must be a future timestamp"})
	case errors.Is(err, service.ErrHashExhausted):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate a unique short url, please retry"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
