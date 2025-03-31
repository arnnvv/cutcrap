package utils

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

func ValidatePDF(file multipart.File) error {
	buf := make([]byte, 4)
	_, err := file.Read(buf)
	if err != nil {
		return err
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	if string(buf) != "%PDF" {
		return fmt.Errorf("not a valid PDF file")
	}

	return nil
}

func CreateTempFile(prefix string) (*os.File, string, error) {
	tmpFile, err := os.CreateTemp("", prefix+"_*.tmp")
	if err != nil {
		return nil, "", err
	}
	return tmpFile, tmpFile.Name(), nil
}

func SaveUploadedFile(file multipart.File, header *multipart.FileHeader) (string, error) {
	tmpFile, tmpPath, err := CreateTempFile("upload")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, file)
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	return tmpPath, nil
}

func CleanupTempFiles(paths ...string) {
	for _, path := range paths {
		os.Remove(path)
	}
}

func GetSafeFilename(original string) string {
	base := filepath.Base(original)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)

	return safe + "-processed.txt"
}
