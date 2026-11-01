package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/model"
	"github.com/wybo/backend/pkg/response"
)

// AdminListAnnouncements 公告列表
func (h *Handler) AdminListAnnouncements(c *gin.Context) {
	list, err := h.svc.ListAllAnnouncements()
	if err != nil {
		response.Error(c, 500, "查询失败")
		return
	}
	response.OK(c, list)
}

// AdminCreateAnnouncement 新增公告
func (h *Handler) AdminCreateAnnouncement(c *gin.Context) {
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Sort    int    `json:"sort"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
		response.Error(c, 400, "标题必填")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	a := &model.Announcement{Title: req.Title, Content: req.Content, Sort: req.Sort, Enabled: enabled}
	if err := h.svc.CreateAnnouncement(a); err != nil {
		response.Error(c, 500, "创建失败")
		return
	}
	response.OK(c, a)
}

// AdminUpdateAnnouncement 更新公告
func (h *Handler) AdminUpdateAnnouncement(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Sort    int    `json:"sort"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	a := &model.Announcement{ID: uint(id), Title: req.Title, Content: req.Content, Sort: req.Sort, Enabled: req.Enabled}
	if err := h.svc.UpdateAnnouncement(a); err != nil {
		response.Error(c, 500, "更新失败")
		return
	}
	response.OK(c, nil)
}

// AdminDeleteAnnouncement 删除公告
func (h *Handler) AdminDeleteAnnouncement(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DeleteAnnouncement(uint(id)); err != nil {
		response.Error(c, 500, "删除失败")
		return
	}
	response.OK(c, nil)
}
