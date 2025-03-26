package utils

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePDF checks if the file is a valid PDF
func ValidatePDF(file multipart.File) error {
	// Read first few bytes to check PDF signature
	buf := make([]byte, 4)
	_, err := file.Read(buf)
	if err != nil {
		return err
	}

	// Reset file pointer to beginning
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	// Check for PDF signature
	if string(buf) != "%PDF" {
		return fmt.Errorf("not a valid PDF file")
	}

	return nil
}

// CreateTempFile creates a temporary file and returns its handle and path
func CreateTempFile(prefix string) (*os.File, string, error) {
	tmpFile, err := os.CreateTemp("", prefix+"_*.tmp")
	if err != nil {
		return nil, "", err
	}
	return tmpFile, tmpFile.Name(), nil
}

// SaveUploadedFile saves the uploaded file to a temporary location
func SaveUploadedFile(file multipart.File, header *multipart.FileHeader) (string, error) {
	tmpFile, tmpPath, err := CreateTempFile("upload")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, file)
	if err != nil {
		os.Remove(tmpPath) // Clean up on error
		return "", err
	}

	return tmpPath, nil
}

// CleanupTempFiles removes temporary files
func CleanupTempFiles(paths ...string) {
	for _, path := range paths {
		os.Remove(path)
	}
}

// GetSafeFilename creates a safe filename for the output
func GetSafeFilename(original string) string {
	// Remove file extension
	base := filepath.Base(original)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Replace unsafe characters
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)

	return safe + "-processed.txt"
}
