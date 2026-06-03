package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const MaxUploadFileSize int64 = 5 * 1024 * 1024

var ErrInvalidUploadFile = errors.New("file tidak valid")

var allowedImageTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

func SaveUploadedImage(header *multipart.FileHeader, directory, prefix string) (string, string, error) {
	if header == nil || header.Size <= 0 || header.Size > MaxUploadFileSize {
		return "", "", ErrInvalidUploadFile
	}

	extension := strings.ToLower(filepath.Ext(header.Filename))
	expectedContentType, ok := allowedImageTypes[extension]
	if !ok {
		return "", "", ErrInvalidUploadFile
	}

	source, err := header.Open()
	if err != nil {
		return "", "", ErrInvalidUploadFile
	}
	defer source.Close()

	buffer := make([]byte, 512)
	readCount, err := source.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", ErrInvalidUploadFile
	}
	if http.DetectContentType(buffer[:readCount]) != expectedContentType {
		return "", "", ErrInvalidUploadFile
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", "", fmt.Errorf("reset uploaded file: %w", err)
	}

	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", "", fmt.Errorf("create upload directory: %w", err)
	}
	randomSuffix, err := randomFileSuffix()
	if err != nil {
		return "", "", fmt.Errorf("generate uploaded filename: %w", err)
	}
	filename := fmt.Sprintf(
		"%s_%s_%s%s",
		sanitizeFilePrefix(prefix),
		time.Now().Format("20060102_150405"),
		randomSuffix,
		extension,
	)
	destinationPath := filepath.Join(directory, filename)
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", "", fmt.Errorf("create uploaded file: %w", err)
	}

	_, copyErr := io.Copy(destination, io.LimitReader(source, MaxUploadFileSize+1))
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(destinationPath)
		if copyErr != nil {
			return "", "", fmt.Errorf("save uploaded file: %w", copyErr)
		}
		return "", "", fmt.Errorf("close uploaded file: %w", closeErr)
	}

	urlPath := "/uploads/" + filepath.ToSlash(filepath.Join(filepath.Base(directory), filename))
	return urlPath, destinationPath, nil
}

func RemoveUploadedFile(path string) {
	cleanPath := filepath.Clean(path)
	uploadRoot, err := filepath.Abs("uploads")
	if err != nil {
		return
	}
	absolutePath, err := filepath.Abs(cleanPath)
	if err != nil {
		return
	}
	relativePath, err := filepath.Rel(uploadRoot, absolutePath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return
	}
	_ = os.Remove(absolutePath)
}

func RemoveUploadedFileByURL(urlPath string) {
	if !strings.HasPrefix(urlPath, "/uploads/") {
		return
	}
	relativePath := strings.TrimPrefix(urlPath, "/uploads/")
	RemoveUploadedFile(filepath.Join("uploads", filepath.FromSlash(relativePath)))
}

func randomFileSuffix() (string, error) {
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func sanitizeFilePrefix(prefix string) string {
	var builder strings.Builder
	for _, character := range prefix {
		switch {
		case character >= 'a' && character <= 'z':
			builder.WriteRune(character)
		case character >= 'A' && character <= 'Z':
			builder.WriteRune(character)
		case character >= '0' && character <= '9':
			builder.WriteRune(character)
		case character == '-' || character == '_':
			builder.WriteRune(character)
		default:
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}
