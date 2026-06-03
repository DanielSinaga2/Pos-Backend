package utils

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveUploadedImage(t *testing.T) {
	header := multipartFileHeader(t, "menu.png", []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	})

	urlPath, savedPath, err := SaveUploadedImage(header, filepath.Join(t.TempDir(), "menus"), "menu_1")
	if err != nil {
		t.Fatalf("save uploaded image: %v", err)
	}
	if !strings.HasPrefix(urlPath, "/uploads/menus/menu_1_") {
		t.Fatalf("unexpected URL path: %s", urlPath)
	}
	if filepath.Ext(savedPath) != ".png" {
		t.Fatalf("unexpected saved extension: %s", savedPath)
	}
}

func TestSaveUploadedImageRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  []byte
	}{
		{name: "invalid extension", filename: "menu.gif", content: []byte("GIF89a")},
		{name: "mismatched content", filename: "menu.jpg", content: []byte("not an image")},
		{name: "too large", filename: "menu.png", content: make([]byte, MaxUploadFileSize+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := multipartFileHeader(t, test.filename, test.content)
			_, _, err := SaveUploadedImage(header, filepath.Join(t.TempDir(), "menus"), "menu_1")
			if err != ErrInvalidUploadFile {
				t.Fatalf("expected invalid file error, got %v", err)
			}
		})
	}
}

func multipartFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request, err := http.NewRequest(http.MethodPost, "/", &body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(MaxUploadFileSize + 1024*1024); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	return request.MultipartForm.File["image"][0]
}
