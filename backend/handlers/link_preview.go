package handlers

import (
	"net/http"
	"strings"

	"kirimwa/backend/services"

	"github.com/gin-gonic/gin"
)

// LinkPreview — GET /agents/:id/link-preview?url=...
// Mengambil OpenGraph/title dari satu URL publik untuk preview otomatis di
// Inbox. Aman SSRF (reuse assertPublicHTTPURL); 400/404 bila URL tidak valid
// atau tidak ada data preview — frontend cukup menyembunyikan kartu.
func LinkPreview(c *gin.Context) {
	_, ok := resolveAgent(c)
	if !ok {
		return
	}
	raw := strings.TrimSpace(c.Query("url"))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url wajib diisi"})
		return
	}
	if len(raw) > 2048 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url terlalu panjang"})
		return
	}
	preview, err := services.FetchLinkPreview(raw)
	if err != nil {
		// URL tidak aman (SSRF), gagal fetch, atau tanpa data og — bukan error
		// fatal untuk UX; frontend hanya tidak menampilkan kartu.
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": preview})
}
