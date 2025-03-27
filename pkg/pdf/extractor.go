package pdf

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

func ExtractContent(r io.ReaderAt) (string, error) {
	startTime := time.Now()
	log.Println("Starting PDF content extraction")

	// Create a temporary file to handle the ReaderAt interface
	tmpFile, err := os.CreateTemp("", "pdf-extract-*.pdf")
	if err != nil {
		log.Printf("Failed to create temporary file: %v", err)
		return "", err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	log.Printf("Created temporary file: %s", tmpFile.Name())

	// Copy content from ReaderAt to the temp file
	if readSeeker, ok := r.(io.ReadSeeker); ok {
		// If it's also a ReadSeeker, use that for efficiency
		log.Println("Using ReadSeeker interface for efficient copying")
		bytesWritten, err := io.Copy(tmpFile, readSeeker)
		if err != nil {
			log.Printf("Failed to copy content to temporary file: %v", err)
			return "", err
		}
		log.Printf("Copied %d bytes to temporary file", bytesWritten)
	} else {
		// Otherwise read from the ReaderAt
		log.Println("Using ReadAt interface for copying")
		data := make([]byte, 1024)
		var offset int64
		var totalBytes int64
		for {
			n, err := r.ReadAt(data, offset)
			if n > 0 {
				if _, err := tmpFile.Write(data[:n]); err != nil {
					log.Printf("Failed to write to temporary file: %v", err)
					return "", err
				}
				totalBytes += int64(n)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("Error reading from source: %v", err)
				return "", err
			}
			offset += int64(n)
		}
		log.Printf("Copied %d bytes to temporary file", totalBytes)
	}

	// Rewind the file
	if _, err := tmpFile.Seek(0, 0); err != nil {
		log.Printf("Failed to rewind temporary file: %v", err)
		return "", err
	}
	log.Println("Rewound temporary file to beginning")

	// Open the PDF file
	log.Println("Opening PDF file with ledongthuc/pdf library")
	f, reader, err := pdf.Open(tmpFile.Name())
	if err != nil {
		log.Printf("Failed to open PDF file: %v", err)
		return "", err
	}
	defer f.Close()
	log.Printf("PDF opened successfully, pages: %d", reader.NumPage())

	// Extract text
	log.Println("Extracting plain text from PDF")
	var buf bytes.Buffer
	b, err := reader.GetPlainText()
	if err != nil {
		log.Printf("Failed to extract plain text: %v", err)
		return "", err
	}
	bytesRead, err := buf.ReadFrom(b)
	if err != nil {
		log.Printf("Failed to read plain text into buffer: %v", err)
		return "", err
	}
	log.Printf("Read %d bytes of plain text from PDF", bytesRead)

	result := normalizeText(buf.String())
	wordCount := len(strings.Fields(result))
	log.Printf("Text extraction completed in %v, extracted %d words", time.Since(startTime), wordCount)
	return result, nil
}

func normalizeText(s string) string {
	log.Println("Normalizing extracted text")
	normalized := strings.ReplaceAll(s, "\r\n", "\n")
	wordCount := len(strings.Fields(normalized))
	log.Printf("Text normalized, final length: %d words", wordCount)
	return normalized
}
