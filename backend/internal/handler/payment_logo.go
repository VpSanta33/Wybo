package handler

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/pkg/response"
)

// AdminPaymentLogo stores raster logos; merchant configuration is saved separately.
func (h *Handler) AdminPaymentLogo(c *gin.Context) {
	h.uploadBrandImage(c)
}

func (h *Handler) AdminSiteImage(c *gin.Context) {
	h.uploadBrandImage(c)
}

func (h *Handler) uploadBrandImage(c *gin.Context) {
	limit := int64(2 * 1024 * 1024)
	if h.cfg.Upload.MaxSize > 0 && h.cfg.Upload.MaxSize < limit {
		limit = h.cfg.Upload.MaxSize
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+128*1024)
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, 400, "请选择图片，图片不能超过 2 MB")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		response.Error(c, 400, "图片文件过大或读取失败")
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "gif") {
		response.Error(c, 400, "请上传有效的 PNG、JPG 或 GIF 图片")
		return
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 16*1024*1024 {
		response.Error(c, 400, "图片尺寸不能超过 4096 × 4096")
		return
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		response.Error(c, 400, "图片已损坏，请重新上传")
		return
	}
	if format == "jpeg" {
		format = "jpg"
	}
	if err := os.MkdirAll(h.cfg.Upload.Dir, 0o755); err != nil {
		response.Error(c, 500, "创建上传目录失败")
		return
	}
	dst, err := os.CreateTemp(h.cfg.Upload.Dir, "payment-logo-*."+format)
	if err != nil {
		response.Error(c, 500, "保存图片失败")
		return
	}
	_, writeErr := dst.Write(data)
	closeErr := dst.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(dst.Name())
		response.Error(c, 500, "写入图片失败")
		return
	}
	path := "/uploads/" + filepath.Base(dst.Name())
	if err := h.publicService(c).RegisterStationUpload(path); err != nil {
		_ = os.Remove(dst.Name())
		response.Error(c, 500, "保存图片归属失败")
		return
	}
	response.OK(c, gin.H{"url": path})
}
