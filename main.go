package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"pdf-processor/internal/chunker"
	"pdf-processor/internal/config"
	"pdf-processor/internal/workers"
	"strconv"
	"strings"
	"time"
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
		w.Header().Set("Content-Type", "text/plain")
		combinedResult := combineResults(results)
		outputWordCount := len(strings.Fields(combinedResult))
		log.Printf("RESPONSE READY | Input: %d words | Output: %d words | Reduction: %.1f%%",
			inputWordCount, outputWordCount,
			100.0-(float64(outputWordCount)/float64(inputWordCount))*100.0)
		io.WriteString(w, combinedResult)
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
