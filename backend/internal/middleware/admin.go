package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"sync"
	"time"
)

type limitEntry struct {
	Count int
	Until time.Time
}

func RateLimit(max int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	entries := map[string]limitEntry{}
	return func(c *gin.Context) {
		key := c.ClientIP()
		now := time.Now()
		mu.Lock()
		if len(entries) > 10000 {
			for k, v := range entries {
				if now.After(v.Until) {
					delete(entries, k)
				}
			}
		}
		v := entries[key]
		if now.After(v.Until) {
			v = limitEntry{Until: now.Add(window)}
		}
		v.Count++
		entries[key] = v
		mu.Unlock()
		if v.Count > max {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "请求过于频繁，请稍后再试"})
			return
		}
		c.Next()
	}
}
