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

	"github.com/joho/godotenv"
)

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found or error loading .env file, using system env vars")
	}

	log.Println("Starting service")
	cfg := config.Load()
	log.Printf("Configuration loaded: Port=%s, MaxConcurrent=%d, ChunkSize=%d, PdfApi=%s", cfg.Port, cfg.MaxConcurrent, cfg.ChunkSize, cfg.Pdf_api)

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
		log.Printf("From: %s | Method: %s | Content-Type: %s", r.RemoteAddr, r.Method, r.Header.Get("Content-Type"))
		defer func() {
			log.Printf("=== REQUEST COMPLETED IN %v ===\n", time.Since(startTime))
		}()

		const maxMemory = 32 << 20 // 32 MB
		if err := r.ParseMultipartForm(maxMemory); err != nil {
			log.Printf("MULTIPART FORM PARSE ERROR: %v", err)
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				http.Error(w, "Invalid request format: Expected multipart/form-data", http.StatusBadRequest)
			} else {
				http.Error(w, "Invalid form data", http.StatusBadRequest)
			}
			return
		}

		text := r.FormValue("text")
		ratioStr := r.FormValue("ratio")
		mode := r.FormValue("mode")

		log.Printf("Received Form Data: text(len)=%d, ratio='%s', mode='%s'", len(text), ratioStr, mode)

		if text == "" {
			log.Printf("VALIDATION FAILED: Empty text field")
			http.Error(w, "Text field is missing or empty", http.StatusBadRequest)
			return
		}

		ratio, err := strconv.ParseFloat(ratioStr, 64)
		if err != nil || ratio <= 0 || ratio > 1 {
			log.Printf("VALIDATION FAILED: Invalid ratio '%v'", ratioStr)
			http.Error(w, "Invalid ratio value (must be > 0 and <= 1)", http.StatusBadRequest)
			return
		}

		if mode == "" {
			log.Printf("VALIDATION FAILED: Mode field is missing, defaulting to 'document'")
			mode = "document"
		}

		if mode != "document" && mode != "transcript" {
			log.Printf("VALIDATION FAILED: Invalid mode value '%s'", mode)
			http.Error(w, "Invalid mode value (must be 'document' or 'transcript')", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute) // Consider adjusting timeout based on mode/content length?
		defer cancel()

		inputWordCount := len(strings.Fields(text))
		log.Printf("PROCESSING START | Mode: %s | Words: %d | Ratio: %.2f", mode, inputWordCount, ratio)

		var combinedResult string // Stores the final text (condensed doc or formatted transcript)

		if mode == "transcript" {
			result := workers.ProcessTranscript(ctx, text, cfg, ratio)
			if ctx.Err() != nil {
				log.Printf("Transcript processing failed due to context error: %v", ctx.Err())
				http.Error(w, "Transcript processing timed out or was cancelled", http.StatusRequestTimeout)
				return // Stop processing
			}
			// If result is empty, it might be a valid outcome (e.g., empty input) or an internal processing error.
			// Assume empty result is valid for now unless ctx.Err() was set.
			combinedResult = result
		} else { // document mode
			chunks, err := chunker.ChunkText(text, cfg.ChunkSize) // Use sentence chunking for documents
			if err != nil {
				log.Printf("Text chunking failed: %v", err)
				http.Error(w, "Text chunking failed", http.StatusInternalServerError)
				return
			}
			// Pass nil for the speaker map in document mode
			results := workers.ProcessChunks(ctx, chunks, cfg, ratio, "document", nil)
			if ctx.Err() != nil {
				log.Printf("Chunk processing failed due to context error: %v", ctx.Err())
				http.Error(w, "Document processing timed out or was cancelled", http.StatusRequestTimeout)
				return // Stop processing
			}
			combinedResult = combineResults(results) // Combine document chunks
		}

		// --- Response Handling ---
		// Only proceed if context is still valid
		if ctx.Err() == nil {
			outputWordCount := len(strings.Fields(combinedResult))
			reduction := 0.0
			if inputWordCount > 0 {
				reduction = 100.0 - (float64(outputWordCount)/float64(inputWordCount))*100.0
			}
			log.Printf("RESPONSE READY | Input: %d words | Output: %d words | Reduction: %.1f%%",
				inputWordCount, outputWordCount, reduction)

			// --- Determine if PDF should be generated ---
			pdfApiAvailable := cfg.Pdf_api != ""
			shouldGeneratePdfForDoc := mode == "document" && pdfApiAvailable && strings.Contains(combinedResult, "# ") // Document PDF only if headings exist
			shouldGeneratePdfForTranscript := mode == "transcript" && pdfApiAvailable                                  // Transcript PDF if API is set

			if shouldGeneratePdfForDoc || shouldGeneratePdfForTranscript {
				log.Printf("Attempting PDF generation via API: %s (Mode: %s)", cfg.Pdf_api, mode)
				w.Header().Set("Content-Type", "application/pdf")

				// Set appropriate PDF filename based on mode
				pdfFilename := "processed_document.pdf"
				if mode == "transcript" {
					pdfFilename = "processed_transcript.pdf"
				}
				w.Header().Set("Content-Disposition", "attachment; filename="+pdfFilename)

				// --- Call PDF Generation API ---
				var body bytes.Buffer
				mpWriter := multipart.NewWriter(&body)
				// Use markdown for the file content type, PDF API should handle it
				fileWriter, err := mpWriter.CreateFormFile("file", "content.md")
				if err != nil {
					log.Printf("PDF API FORM CREATION FAILED: %v", err)
					http.Error(w, "PDF generation setup failed", http.StatusInternalServerError)
					return
				}

				// Write the final combined text (document or transcript)
				if _, err := fileWriter.Write([]byte(combinedResult)); err != nil {
					log.Printf("PDF API WRITE FAILED: %v", err)
					http.Error(w, "PDF generation content write failed", http.StatusInternalServerError)
					return
				}
				mpWriter.Close() // Close writer before sending request

				req, err := http.NewRequestWithContext(ctx, "POST", cfg.Pdf_api, &body)
				if err != nil {
					log.Printf("PDF API REQUEST CREATION FAILED: %v", err)
					http.Error(w, "PDF generation request creation failed", http.StatusInternalServerError)
					return
				}
				// Set the correct multipart content type for the PDF API request
				req.Header.Set("Content-Type", mpWriter.FormDataContentType())

				client := &http.Client{Timeout: 2 * time.Minute} // Timeout for PDF generation
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("PDF API REQUEST FAILED: %v", err)
					http.Error(w, "PDF generation request failed", http.StatusInternalServerError)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					respBodyBytes, _ := io.ReadAll(resp.Body)
					log.Printf("PDF API RETURNED STATUS: %d. Body: %s", resp.StatusCode, string(respBodyBytes))
					http.Error(w, "PDF generation failed on external API", http.StatusInternalServerError)
					return
				}

				// Stream the PDF response back to the original client
				log.Printf("Streaming PDF response to client...")
				if _, err := io.Copy(w, resp.Body); err != nil {
					log.Printf("PDF STREAM FAILED: %v", err)
					// Don't send another http.Error if header might be partially sent
					return
				}
				log.Printf("PDF stream completed.")
				// --- End PDF API Call ---

			} else {
				// --- Send as Plain Text ---
				if pdfApiAvailable {
					if mode == "transcript" {
						log.Printf("Sending transcript as plain text (PDF API available but not triggered).")
					} else {
						log.Printf("Sending document as plain text (PDF API available but no headings found).")
					}
				} else {
					log.Printf("Sending response as plain text (PDF API not configured).")
				}

				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				// Set appropriate text filename based on mode
				txtFilename := "processed_document.txt"
				if mode == "transcript" {
					txtFilename = "processed_transcript.txt"
				}
				w.Header().Set("Content-Disposition", "attachment; filename="+txtFilename)
				io.WriteString(w, combinedResult)
				// --- End Plain Text ---
			}
		} // End check for ctx.Err() == nil
	}
}

// combineResults joins string slices, used primarily for document chunks
func combineResults(results []string) string {
	// Filter out empty strings that might result from failed chunk processing
	var validResults []string
	for _, res := range results {
		// Check if result is non-empty after trimming whitespace
		if strings.TrimSpace(res) != "" {
			validResults = append(validResults, res)
		}
	}

	// Join valid chunks with double newlines for separation
	return strings.Join(validResults, "\n\n")
}
