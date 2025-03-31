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
)

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func main() {
	log.Println("Starting PDF processor service")
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
		log.Printf("From: %s | Method: %s", r.RemoteAddr, r.Method)
		defer func() {
			log.Printf("=== REQUEST COMPLETED IN %v ===\n", time.Since(startTime))
		}()

		if err := r.ParseForm(); err != nil {
			log.Printf("FORM PARSE ERROR: %v", err)
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}

		text := r.FormValue("text")
		ratioStr := r.FormValue("ratio")
		mode := r.FormValue("mode")

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

		if mode == "" {
			mode = "document"
		}

		if mode != "document" && mode != "transcript" {
			http.Error(w, "Invalid mode value", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		inputWordCount := len(strings.Fields(text))
		log.Printf("PROCESSING START | Mode: %s | Words: %d", mode, inputWordCount)

		var results []string
		if mode == "transcript" {
			result := workers.ProcessTranscript(ctx, text, cfg, ratio)
			if result == "" {
				http.Error(w, "Transcript processing failed", http.StatusInternalServerError)
				return
			}
			results = []string{result}
		} else {
			chunks, err := chunker.ChunkText(text, cfg.ChunkSize)
			if err != nil {
				http.Error(w, "Text chunking failed", http.StatusInternalServerError)
				return
			}
			results = workers.ProcessChunks(ctx, chunks, cfg, ratio, "document")
		}

		combinedResult := combineResults(results)
		outputWordCount := len(strings.Fields(combinedResult))
		log.Printf("RESPONSE READY | Input: %d words | Output: %d words | Reduction: %.1f%%",
			inputWordCount, outputWordCount,
			100.0-(float64(outputWordCount)/float64(inputWordCount))*100.0)

		if strings.Contains(combinedResult, "# ") {
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=processed.pdf")

			var body bytes.Buffer
			mpWriter := multipart.NewWriter(&body)
			fileWriter, err := mpWriter.CreateFormFile("file", "processed.md")
			if err != nil {
				log.Printf("PDF API FORM CREATION FAILED: %v", err)
				http.Error(w, "PDF generation failed 1", http.StatusInternalServerError)
				return
			}

			if _, err := fileWriter.Write([]byte(combinedResult)); err != nil {
				log.Printf("PDF API WRITE FAILED: %v", err)
				http.Error(w, "PDF generation failed 2", http.StatusInternalServerError)
				return
			}
			mpWriter.Close()

			req, err := http.NewRequestWithContext(ctx, "POST", cfg.Pdf_api, &body)
			if err != nil {
				log.Printf("PDF API REQUEST CREATION FAILED: %v", err)
				http.Error(w, "PDF generation failed 3", http.StatusInternalServerError)
				return
			}
			req.Header.Set("Content-Type", mpWriter.FormDataContentType())

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("PDF API REQUEST FAILED: %v", err)
				http.Error(w, "PDF generation failed 4", http.StatusInternalServerError)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Printf("PDF API RETURNED STATUS: %d", resp.StatusCode)
				http.Error(w, "PDF generation failed 5", http.StatusInternalServerError)
				return
			}

			if _, err := io.Copy(w, resp.Body); err != nil {
				log.Printf("PDF STREAM FAILED: %v", err)
				http.Error(w, "PDF generation failed 6", http.StatusInternalServerError)
				return
			}
		} else {
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
