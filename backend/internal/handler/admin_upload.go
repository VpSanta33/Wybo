package handler

import "github.com/gin-gonic/gin"

// AdminUpload validates product images using the same raster decoder as brand images.
func (h *Handler) AdminUpload(c *gin.Context) {
	h.uploadBrandImage(c)
}
