package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"pdf-processor/internal/chunker"
	"pdf-processor/internal/config"
	"pdf-processor/internal/workers"
	"strconv"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"
	"github.com/russross/blackfriday/v2"
)

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func main() {
	log.Println("Starting PDF processor service")
	cfg := config.Load()
	log.Printf("Configuration loaded: Port=%s, MaxConcurrent=%d, ChunkSize=%d", cfg.Port, cfg.MaxConcurrent, cfg.ChunkSize)

	http.HandleFunc("/process", func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		uploadHandler(cfg)(w, r)
	})

	log.Printf("Server starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}

func uploadHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		log.Printf("\n\n=== NEW REQUEST ===")
		log.Printf("From: %s | Method: %s", r.RemoteAddr, r.Method)
		defer log.Printf("=== REQUEST COMPLETED IN %v ===\n", time.Since(startTime))

		// 1. PARSE FORM FIRST
		if err := r.ParseForm(); err != nil {
			log.Printf("FORM PARSE ERROR: %v", err)
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		// 2. LOG RAW FORM DATA IMMEDIATELY AFTER PARSING
		log.Printf("RAW FORM VALUES: %+v", r.Form) // Add this line
		log.Printf("HEADERS: %+v", r.Header)

		// 3. GET FORM VALUES WITH FALLBACKS
		text := r.FormValue("text")
		ratioStr := r.FormValue("ratio")
		mode := r.FormValue("mode")

		log.Printf("RECEIVED PARAMS | TextLen: %d | Ratio: %s | Mode: '%s'",
			len(text), ratioStr, mode)

		// 4. VALIDATION
		if text == "" {
			log.Printf("VALIDATION FAILED: Empty text")
			http.Error(w, "Text field is missing", http.StatusBadRequest)
			return
		}

		ratio, err := strconv.ParseFloat(ratioStr, 64)
		if err != nil || ratio <= 0 || ratio > 1 {
			log.Printf("VALIDATION FAILED: Invalid ratio %v", ratioStr)
			http.Error(w, "Invalid ratio value", http.StatusBadRequest)
			return
		}

		// 5. MODE HANDLING
		if mode == "" {
			mode = "document"
			log.Printf("MODE FALLBACK: Using default '%s'", mode)
		} else {
			log.Printf("MODE SELECTED: %s", mode)
		}

		if mode != "document" && mode != "transcript" {
			log.Printf("INVALID MODE: %s", mode)
			http.Error(w, "Invalid mode value", http.StatusBadRequest)
			return
		}

		// 6. PROCESSING
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		inputWordCount := len(strings.Fields(text))
		log.Printf("PROCESSING START | Mode: %s | Words: %d", mode, inputWordCount)

		var results []string
		if mode == "transcript" {
			log.Printf("TRANSCRIPT PROCESSING | ChunkSize: %d | Overlap: %d",
				cfg.ChunkSize, cfg.ChunkOverlap)
			result := workers.ProcessTranscript(ctx, text, cfg, ratio)
			if result == "" {
				log.Printf("TRANSCRIPT FAILURE: No results")
				http.Error(w, "Transcript processing failed", http.StatusInternalServerError)
				return
			}
			results = []string{result}
		} else {
			log.Printf("DOCUMENT PROCESSING | ChunkSize: %d", cfg.ChunkSize)
			chunks, err := chunker.ChunkText(text, cfg.ChunkSize)
			if err != nil {
				log.Printf("CHUNKING FAILURE: %v", err)
				http.Error(w, "Text chunking failed", http.StatusInternalServerError)
				return
			}
			log.Printf("DOCUMENT CHUNKED | Parts: %d", len(chunks))
			results = workers.ProcessChunks(ctx, chunks, cfg, ratio, "document")
		}

		// 7. RESPONSE
		combinedResult := combineResults(results)
		outputWordCount := len(strings.Fields(combinedResult))
		log.Printf("RESPONSE READY | Input: %d words | Output: %d words | Reduction: %.1f%%",
			inputWordCount, outputWordCount,
			100.0-(float64(outputWordCount)/float64(inputWordCount))*100.0)

		// Check if the output should be PDF or text
		if strings.Contains(combinedResult, "# ") {
			// Contains markdown headings, return as PDF
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=processed.pdf")

			if err := markdownToPDF(combinedResult, w); err != nil {
				log.Printf("PDF CONVERSION FAILED: %v", err)
				http.Error(w, "PDF generation failed", http.StatusInternalServerError)
				return
			}
		} else {
			// No markdown headings, return as text
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, combinedResult)
		}
	}
}

func combineResults(results []string) string {
	var final strings.Builder
	for i, res := range results {
		final.WriteString(res)
		if i < len(results)-1 {
			final.WriteString("\n\n")
		}
	}
	return final.String()
}

// markdownToPDF converts markdown text to PDF and writes to the provided writer
func markdownToPDF(markdown string, w io.Writer) error {
	// Parse markdown to HTML
	html := blackfriday.Run([]byte(markdown))

	// Create a new PDF document
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20) // Set margins for better readability
	pdf.AddPage()

	// Add a title
	pdf.SetFont("Arial", "B", 18)
	pdf.Cell(0, 10, "Processed Document")
	pdf.Ln(15)

	// Process the HTML content
	lines := strings.Split(string(html), "\n")
	inParagraph := false
	paragraphText := ""

	for _, line := range lines {
		// Handle headings (# Heading)
		if strings.Contains(line, "<h1>") {
			// If we were in a paragraph, finish it first
			if inParagraph {
				pdf.SetFont("Arial", "", 12)
				pdf.MultiCell(0, 6, paragraphText, "", "", false)
				pdf.Ln(5)
				inParagraph = false
				paragraphText = ""
			}

			heading := stripTags(line)
			pdf.SetFont("Arial", "B", 16)
			pdf.Cell(0, 10, heading)
			pdf.Ln(10)
		} else if strings.Contains(line, "<h2>") {
			// If we were in a paragraph, finish it first
			if inParagraph {
				pdf.SetFont("Arial", "", 12)
				pdf.MultiCell(0, 6, paragraphText, "", "", false)
				pdf.Ln(5)
				inParagraph = false
				paragraphText = ""
			}

			heading := stripTags(line)
			pdf.SetFont("Arial", "B", 14)
			pdf.Cell(0, 10, heading)
			pdf.Ln(8)
		} else if strings.Contains(line, "<h3>") {
			// If we were in a paragraph, finish it first
			if inParagraph {
				pdf.SetFont("Arial", "", 12)
				pdf.MultiCell(0, 6, paragraphText, "", "", false)
				pdf.Ln(5)
				inParagraph = false
				paragraphText = ""
			}

			heading := stripTags(line)
			pdf.SetFont("Arial", "BI", 12)
			pdf.Cell(0, 10, heading)
			pdf.Ln(8)
		} else if strings.Contains(line, "<p>") {
			// Regular paragraph text
			text := stripTags(line)
			if text != "" {
				if !inParagraph {
					inParagraph = true
					paragraphText = text
				} else {
					paragraphText += " " + text
				}
			}
		} else if line == "" && inParagraph {
			// End of paragraph
			pdf.SetFont("Arial", "", 12)
			pdf.MultiCell(0, 6, paragraphText, "", "", false)
			pdf.Ln(5)
			inParagraph = false
			paragraphText = ""
		}
	}

	// If we're still in a paragraph at the end, finish it
	if inParagraph {
		pdf.SetFont("Arial", "", 12)
		pdf.MultiCell(0, 6, paragraphText, "", "", false)
	}

	// Add page numbers
	pageCount := pdf.PageCount()
	for i := 1; i <= pageCount; i++ {
		pdf.SetPage(i)
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.CellFormat(0, 10, fmt.Sprintf("Page %d of %d", i, pageCount), "", 0, "C", false, 0, "")
	}

	// Write the PDF to the output writer
	return pdf.Output(w)
}

// stripTags removes HTML tags from a string
func stripTags(html string) string {
	var buf bytes.Buffer
	var inTag bool

	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			buf.WriteRune(r)
		}
	}

	return strings.TrimSpace(buf.String())
}
