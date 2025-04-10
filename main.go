package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arnnvv/cutcrap/pkg/chunker"
	"github.com/arnnvv/cutcrap/pkg/config"
	"github.com/arnnvv/cutcrap/pkg/workers"

	"github.com/joho/godotenv" // Keep if using .env loading
)

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	// Make sure Content-Type is allowed if you have specific checks
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With") // Example broader set
}

func main() {
	// Load .env file if you added godotenv
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found or error loading .env file, using system env vars")
	}

	log.Println("Starting PDF processor service")
	cfg := config.Load()
	log.Printf("Configuration loaded: Port=%s, MaxConcurrent=%d, ChunkSize=%d, PdfApi=%s", cfg.Port, cfg.MaxConcurrent, cfg.ChunkSize, cfg.Pdf_api)

	http.HandleFunc("/process", func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Use the modified uploadHandler
		uploadHandler(cfg)(w, r)
	})

	log.Printf("Server starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}

func uploadHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		log.Printf("\n\n=== NEW REQUEST ===")
		log.Printf("From: %s | Method: %s | Content-Type: %s", r.RemoteAddr, r.Method, r.Header.Get("Content-Type")) // Log Content-Type
		defer func() {
			log.Printf("=== REQUEST COMPLETED IN %v ===\n", time.Since(startTime))
		}()

		// *** CHANGE HERE: Use ParseMultipartForm instead of ParseForm ***
		// Set a max memory limit (e.g., 32 MB) for parts stored in memory.
		// Larger parts will be stored on disk. Adjust as needed.
		const maxMemory = 32 << 20 // 32 MB
		if err := r.ParseMultipartForm(maxMemory); err != nil {
			log.Printf("MULTIPART FORM PARSE ERROR: %v", err)
			// Check if it's because the Content-Type wasn't multipart
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				http.Error(w, "Invalid request format: Expected multipart/form-data", http.StatusBadRequest)
			} else {
				http.Error(w, "Invalid form data", http.StatusBadRequest)
			}
			return
		}
		// ***************************************************************

		// You can still use r.FormValue after ParseMultipartForm is called successfully
		text := r.FormValue("text")
		ratioStr := r.FormValue("ratio")
		mode := r.FormValue("mode")

		log.Printf("Received Form Data: text(len)=%d, ratio='%s', mode='%s'", len(text), ratioStr, mode) // Add log to see received values

		if text == "" {
			log.Printf("VALIDATION FAILED: Empty text field received") // Updated log message
			http.Error(w, "Text field is missing or empty", http.StatusBadRequest)
			return
		}

		ratio, err := strconv.ParseFloat(ratioStr, 64)
		// Ratio check should be <= 0 (or perhaps a small positive epsilon like 0.01)
		if err != nil || ratio <= 0 || ratio > 1 {
			log.Printf("VALIDATION FAILED: Invalid ratio '%v'", ratioStr)
			http.Error(w, "Invalid ratio value (must be > 0 and <= 1)", http.StatusBadRequest)
			return
		}

		if mode == "" {
			log.Printf("VALIDATION FAILED: Mode field is missing, defaulting to 'document'")
			mode = "document" // Defaulting might be okay, but log it
		}

		if mode != "document" && mode != "transcript" {
			log.Printf("VALIDATION FAILED: Invalid mode value '%s'", mode)
			http.Error(w, "Invalid mode value (must be 'document' or 'transcript')", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		inputWordCount := len(strings.Fields(text))
		log.Printf("PROCESSING START | Mode: %s | Words: %d | Ratio: %.2f", mode, inputWordCount, ratio)

		var results []string
		var combinedResult string // Declare outside the if/else

		if mode == "transcript" {
			result := workers.ProcessTranscript(ctx, text, cfg, ratio)
			// Check for actual processing error if result is empty
			if result == "" {
				// It's possible the processing itself returned empty, not necessarily an internal server error.
				// Check context cancellation or specific errors if available.
				if ctx.Err() != nil {
					log.Printf("Transcript processing failed due to context error: %v", ctx.Err())
					http.Error(w, "Transcript processing timed out or was cancelled", http.StatusRequestTimeout)

				} else {
					log.Printf("Transcript processing returned an empty result.")
					// Decide if empty result is an error or valid outcome. Assuming valid for now.
					combinedResult = "" // Explicitly set to empty
				}

			} else {
				results = []string{result}
				combinedResult = combineResults(results)
			}
		} else { // document mode
			chunks, err := chunker.ChunkText(text, cfg.ChunkSize)
			if err != nil {
				log.Printf("Text chunking failed: %v", err)
				http.Error(w, "Text chunking failed", http.StatusInternalServerError)
				return
			}
			results = workers.ProcessChunks(ctx, chunks, cfg, ratio, "document")
			// Check context error after processing chunks
			if ctx.Err() != nil {
				log.Printf("Chunk processing failed due to context error: %v", ctx.Err())
				http.Error(w, "Document processing timed out or was cancelled", http.StatusRequestTimeout)
				return
			}
			combinedResult = combineResults(results)
		}

		// Only proceed to response if context is still valid
		if ctx.Err() == nil {
			outputWordCount := len(strings.Fields(combinedResult))
			reduction := 0.0
			if inputWordCount > 0 {
				reduction = 100.0 - (float64(outputWordCount)/float64(inputWordCount))*100.0
			}
			log.Printf("RESPONSE READY | Input: %d words | Output: %d words | Reduction: %.1f%%",
				inputWordCount, outputWordCount, reduction)

			// Check if PDF generation is requested and possible
			// The check for "# " might be too simplistic. Maybe rely on mode?
			// Or add a specific request parameter/header if PDF is desired?
			// Let's assume for now: PDF only if document mode AND headings found AND PDF_API is set.
			shouldGeneratePdf := mode == "document" && cfg.Pdf_api != "" && strings.Contains(combinedResult, "# ")

			if shouldGeneratePdf {
				log.Printf("Attempting PDF generation via API: %s", cfg.Pdf_api)
				w.Header().Set("Content-Type", "application/pdf")
				// Use a more descriptive filename
				w.Header().Set("Content-Disposition", "attachment; filename=processed_document.pdf")

				var body bytes.Buffer
				mpWriter := multipart.NewWriter(&body)
				// Use a more appropriate filename for the markdown content
				fileWriter, err := mpWriter.CreateFormFile("file", "content.md")
				if err != nil {
					log.Printf("PDF API FORM CREATION FAILED: %v", err)
					http.Error(w, "PDF generation setup failed", http.StatusInternalServerError)
					return
				}

				if _, err := fileWriter.Write([]byte(combinedResult)); err != nil {
					log.Printf("PDF API WRITE FAILED: %v", err)
					http.Error(w, "PDF generation content write failed", http.StatusInternalServerError)
					return
				}
				mpWriter.Close() // Must close before reading body or setting content type

				req, err := http.NewRequestWithContext(ctx, "POST", cfg.Pdf_api, &body)
				if err != nil {
					log.Printf("PDF API REQUEST CREATION FAILED: %v", err)
					http.Error(w, "PDF generation request creation failed", http.StatusInternalServerError)
					return
				}
				// Set the correct multipart content type for the PDF API request
				req.Header.Set("Content-Type", mpWriter.FormDataContentType())

				// Use a client with a timeout relevant to PDF generation
				client := &http.Client{Timeout: 2 * time.Minute}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("PDF API REQUEST FAILED: %v", err)
					http.Error(w, "PDF generation request failed", http.StatusInternalServerError)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					respBodyBytes, _ := io.ReadAll(resp.Body) // Try to read error body
					log.Printf("PDF API RETURNED STATUS: %d. Body: %s", resp.StatusCode, string(respBodyBytes))
					http.Error(w, "PDF generation failed on external API", http.StatusInternalServerError)
					return
				}

				// Stream the PDF response back to the original client
				if _, err := io.Copy(w, resp.Body); err != nil {
					log.Printf("PDF STREAM FAILED: %v", err)
					// Don't send another http.Error if header is already sent
					// http.Error(w, "Failed to stream PDF response", http.StatusInternalServerError)
					return
				}
			} else {
				// Default to plain text if not generating PDF
				log.Printf("Sending response as plain text.")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				// Set appropriate filename for text download
				if mode == "transcript" {
					w.Header().Set("Content-Disposition", "attachment; filename=processed_transcript.txt")
				} else {
					w.Header().Set("Content-Disposition", "attachment; filename=processed_document.txt")
				}
				io.WriteString(w, combinedResult)
			}
		} // End check for ctx.Err() == nil

	}
}

// combineResults remains the same
func combineResults(results []string) string {
	// Filter out empty strings that might result from failed chunk processing
	var validResults []string
	for _, res := range results {
		if res != "" {
			validResults = append(validResults, res)
		}
	}

	var final strings.Builder
	for i, res := range validResults {
		final.WriteString(res)
		// Add separator only between valid chunks
		if i < len(validResults)-1 {
			// Use a single newline for transcript mode for tighter formatting?
			// Or keep double newline for both for consistency? Let's keep double for now.
			final.WriteString("\n\n")
		}
	}
	return final.String()
}
