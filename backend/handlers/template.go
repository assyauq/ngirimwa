package handlers

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"

	"kirimwa/backend/database"
	"kirimwa/backend/models"

	"github.com/gin-gonic/gin"
)

const maxTemplateMediaBytes = 64 << 20

type templateInput struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	SortOrder int    `json:"sort_order"`
}

func bindTemplateInput(c *gin.Context) (templateInput, error) {
	var req templateInput
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		req.Title = c.PostForm("title")
		req.Body = c.PostForm("body")
		req.SortOrder, _ = strconv.Atoi(c.PostForm("sort_order"))
		return req, nil
	}
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	return req, err
}

func templateMediaType(mimetype string) string {
	switch {
	case strings.HasPrefix(mimetype, "image/"):
		return "image"
	case strings.HasPrefix(mimetype, "video/"):
		return "video"
	case strings.HasPrefix(mimetype, "audio/"):
		return "audio"
	default:
		return "document"
	}
}

func readTemplateMedia(c *gin.Context, agentID uint) (mediaType, mediaPath, fileName, mimetype string, found bool, err error) {
	fh, formErr := c.FormFile("file")
	if formErr != nil {
		return "", "", "", "", false, nil
	}
	f, openErr := fh.Open()
	if openErr != nil {
		return "", "", "", "", true, openErr
	}
	defer f.Close()
	data, readErr := io.ReadAll(io.LimitReader(f, maxTemplateMediaBytes+1))
	if readErr != nil {
		return "", "", "", "", true, readErr
	}
	if len(data) > maxTemplateMediaBytes {
		return "", "", "", "", true, errTemplateMediaTooLarge
	}
	mimetype = strings.TrimSpace(fh.Header.Get("Content-Type"))
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	mediaPath = storeMedia(agentID, data, mimetype, fh.Filename)
	if mediaPath == "" {
		return "", "", "", "", true, errTemplateMediaStore
	}
	return templateMediaType(mimetype), mediaPath, fh.Filename, mimetype, true, nil
}

var (
	errTemplateMediaTooLarge = &templateMediaError{"Ukuran lampiran maksimal 64 MB"}
	errTemplateMediaStore    = &templateMediaError{"Gagal menyimpan lampiran"}
)

type templateMediaError struct{ message string }

func (e *templateMediaError) Error() string { return e.message }

func ListTemplates(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var tpls []models.Template
	database.DB.Where("agent_id = ?", id).Order("sort_order asc, id asc").Find(&tpls)
	c.JSON(200, gin.H{"data": tpls})
}

func CreateTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	req, err := bindTemplateInput(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		c.JSON(400, gin.H{"error": "Judul & isi template wajib diisi"})
		return
	}
	t := models.Template{AgentID: id, Title: req.Title, Body: req.Body, SortOrder: req.SortOrder}
	mediaType, mediaPath, fileName, mimetype, found, mediaErr := readTemplateMedia(c, id)
	if mediaErr != nil {
		status := 400
		if mediaErr == errTemplateMediaTooLarge || strings.Contains(mediaErr.Error(), "64 MB") {
			status = 413
		}
		c.JSON(status, gin.H{"error": mediaErr.Error()})
		return
	}
	if found {
		t.MediaType, t.MediaPath, t.FileName, t.Mimetype = mediaType, mediaPath, fileName, mimetype
	}
	if err := database.DB.Create(&t).Error; err != nil {
		if t.MediaPath != "" {
			_ = os.Remove(t.MediaPath)
		}
		c.JSON(500, gin.H{"error": "Gagal menyimpan template"})
		return
	}
	c.JSON(201, gin.H{"data": t})
}

func UpdateTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var t models.Template
	if database.DB.Where("agent_id = ?", id).First(&t, c.Param("tid")).Error != nil {
		c.JSON(404, gin.H{"error": "Template tidak ditemukan"})
		return
	}
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		req, err := bindTemplateInput(c)
		if err != nil {
			c.JSON(400, gin.H{"error": "Format data tidak valid"})
			return
		}
		t.Title, t.Body, t.SortOrder = req.Title, req.Body, req.SortOrder
	} else {
		var req struct {
			Title     *string `json:"title"`
			Body      *string `json:"body"`
			SortOrder *int    `json:"sort_order"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			c.JSON(400, gin.H{"error": "Format data tidak valid"})
			return
		}
		if req.Title != nil {
			t.Title = *req.Title
		}
		if req.Body != nil {
			t.Body = *req.Body
		}
		if req.SortOrder != nil {
			t.SortOrder = *req.SortOrder
		}
	}
	if strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Body) == "" {
		c.JSON(400, gin.H{"error": "Judul & isi template wajib diisi"})
		return
	}
	oldMediaPath := t.MediaPath
	mediaType, mediaPath, fileName, mimetype, found, mediaErr := readTemplateMedia(c, id)
	if mediaErr != nil {
		status := 400
		if mediaErr == errTemplateMediaTooLarge || strings.Contains(mediaErr.Error(), "64 MB") {
			status = 413
		}
		c.JSON(status, gin.H{"error": mediaErr.Error()})
		return
	}
	removeMedia := c.PostForm("remove_media") == "true"
	if found {
		t.MediaType, t.MediaPath, t.FileName, t.Mimetype = mediaType, mediaPath, fileName, mimetype
	} else if removeMedia {
		t.MediaType, t.MediaPath, t.FileName, t.Mimetype = "", "", "", ""
	}
	if err := database.DB.Save(&t).Error; err != nil {
		if found && mediaPath != "" {
			_ = os.Remove(mediaPath)
		}
		c.JSON(500, gin.H{"error": "Gagal menyimpan template"})
		return
	}
	if oldMediaPath != "" && oldMediaPath != t.MediaPath {
		_ = os.Remove(oldMediaPath)
	}
	c.JSON(200, gin.H{"data": t})
}

func DownloadTemplateMedia(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var t models.Template
	if database.DB.Where("agent_id = ?", id).First(&t, c.Param("tid")).Error != nil || t.MediaPath == "" {
		c.JSON(404, gin.H{"error": "Lampiran template tidak ditemukan"})
		return
	}
	c.Header("Content-Type", t.Mimetype)
	c.FileAttachment(t.MediaPath, t.FileName)
}

func DeleteTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var t models.Template
	if database.DB.Where("agent_id = ?", id).First(&t, c.Param("tid")).Error != nil {
		c.JSON(404, gin.H{"error": "Template tidak ditemukan"})
		return
	}
	if err := database.DB.Delete(&t).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menghapus template"})
		return
	}
	if t.MediaPath != "" {
		_ = os.Remove(t.MediaPath)
	}
	c.JSON(200, gin.H{"message": "Deleted"})
}
