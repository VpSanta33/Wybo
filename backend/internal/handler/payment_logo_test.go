package handler

import (
	"bytes"
	"encoding/json"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wybo/backend/internal/config"
)

func TestPaymentLogoUploadValidatesContentsAndLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Upload.Dir = t.TempDir()
	cfg.Upload.MaxSize = 2 * 1024 * 1024
	h := &Handler{cfg: cfg}
	r := gin.New()
	r.POST("/logo", h.AdminPaymentLogo)
	upload := func(name string, data []byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/logo", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for _, format := range []string{"png", "jpg", "gif"} {
		var data bytes.Buffer
		var err error
		switch format {
		case "png":
			err = png.Encode(&data, img)
		case "jpg":
			err = jpeg.Encode(&data, img, nil)
		case "gif":
			err = gif.Encode(&data, img, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		// Extension is derived from the decoded image, never the submitted filename.
		w := upload("../../untrusted.svg", data.Bytes())
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var response struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(response.Data.URL, "/uploads/payment-logo-") || !strings.HasSuffix(response.Data.URL, "."+format) {
			t.Fatal("unsafe storage path", response.Data.URL)
		}
		stored, err := os.ReadFile(filepath.Join(cfg.Upload.Dir, filepath.Base(response.Data.URL)))
		if err != nil || !bytes.Equal(stored, data.Bytes()) {
			t.Fatal("stored image differs")
		}
	}
	for _, data := range [][]byte{
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		[]byte("<html>not an image</html>"),
		bytes.Repeat([]byte("x"), 2*1024*1024+1),
	} {
		if w := upload("logo.png", data); !strings.Contains(w.Body.String(), `"code":400`) {
			t.Fatal("invalid image accepted", w.Body.String())
		}
	}
	var oversized bytes.Buffer
	if err := png.Encode(&oversized, image.NewRGBA(image.Rect(0, 0, 4097, 1))); err != nil {
		t.Fatal(err)
	}
	if w := upload("large.png", oversized.Bytes()); !strings.Contains(w.Body.String(), `"code":400`) {
		t.Fatal("oversize dimensions accepted")
	}
	files, err := os.ReadDir(cfg.Upload.Dir)
	if err != nil || len(files) != 3 {
		t.Fatal("rejected image left a file")
	}
}
