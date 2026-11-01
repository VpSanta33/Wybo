package handler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/pkg/response"
)

// AdminUpload 上传图片（返回可访问 URL）
func (h *Handler) AdminUpload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, 400, "请选择文件")
		return
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		response.Error(c, 400, "仅支持图片文件")
		return
	}
	if header.Size > h.cfg.Upload.MaxSize {
		response.Error(c, 400, "文件过大")
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".png"
	}
	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)

	if err := os.MkdirAll(h.cfg.Upload.Dir, 0o755); err != nil {
		response.Error(c, 500, "创建目录失败")
		return
	}
	dst, err := os.Create(filepath.Join(h.cfg.Upload.Dir, name))
	if err != nil {
		response.Error(c, 500, "保存失败")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		response.Error(c, 500, "写入失败")
		return
	}

	response.OK(c, gin.H{"url": "/uploads/" + name})
}
