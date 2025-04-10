// pkg/workers/pool.go

package workers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/arnnvv/cutcrap/pkg/api"
	"github.com/arnnvv/cutcrap/pkg/chunker"
	"github.com/arnnvv/cutcrap/pkg/config"
	"github.com/arnnvv/cutcrap/pkg/transcript" // Needs the NEW parseSpeakerAnalysis and CombineTranscriptChunks
)

// ProcessChunks processes text chunks in parallel.
// For transcript mode, it now passes the Role->Name map to the API call.
func ProcessChunks(ctx context.Context, chunks []string, cfg *config.Config, ratio float64, mode string, speakerRoleNameMap map[string]string) []string { // Takes map now
	startTime := time.Now()
	totalInputWords := 0
	for _, chunk := range chunks {
		totalInputWords += len(strings.Fields(chunk))
	}

	log.Printf("Starting to process %d chunks (mode: %s, total input: %d words)", len(chunks), mode, totalInputWords)
	if mode == "transcript" && len(speakerRoleNameMap) > 0 {
		log.Printf("Using Speaker Role->Name map during chunk processing: %v", speakerRoleNameMap)
	} else if mode == "transcript" {
		log.Println("Processing transcript chunks WITHOUT speaker map context.")
	}

	var (
		wg         sync.WaitGroup
		results    = make([]string, len(chunks))
		semaphore  = make(chan struct{}, cfg.MaxConcurrent)
		resultChan = make(chan struct {
			index   int
			content string
			err     error
		})
	)

	// Worker dispatcher goroutine
	go func() {
		defer close(resultChan)
		log.Printf("Worker dispatcher: Starting %d workers.", len(chunks))
		for i, chunk := range chunks {
			if ctx.Err() != nil {
				log.Printf("Ctx cancelled before dispatch chunk %d.", i)
				break
			}
			wg.Add(1)
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				log.Printf("Ctx cancelled waiting for semaphore chunk %d.", i)
				wg.Done()
				return
			}

			// Pass the map to the worker
			go func(index int, text string, roleNameMap map[string]string) {
				chunkStartTime := time.Now()
				var processedContent string
				var processErr error
				logPrefix := fmt.Sprintf("Worker chunk %d", index)
				defer func() {
					log.Printf("%s completed in %v", logPrefix, time.Since(chunkStartTime))
					resultChan <- struct {
						index   int
						content string
						err     error
					}{index, processedContent, processErr}
					<-semaphore
					wg.Done()
				}()

				if ctx.Err() != nil {
					log.Printf("%s: Ctx cancelled before processing.", logPrefix)
					processErr = ctx.Err()
					return
				}

				targetWordCount := int(float64(cfg.ChunkSize) * ratio)
				if targetWordCount <= 0 {
					targetWordCount = 1
				}

				// Call API function, passing the roleNameMap
				processedContent, processErr = api.ProcessTextWithMode(ctx, text, cfg.OpenRouterKey, targetWordCount, mode, roleNameMap) // Pass map

				if processErr != nil {
					log.Printf("%s: Error during API processing: %v", logPrefix, processErr)
					processedContent = ""
				} else if ctx.Err() != nil {
					log.Printf("%s: Ctx cancelled after processing. Discarding.", logPrefix)
					processErr = ctx.Err()
					processedContent = ""
				} else {
					log.Printf("%s: Successfully processed, result: %d words", logPrefix, len(strings.Fields(processedContent)))
				}
			}(i, chunk, speakerRoleNameMap) // Pass map here
		}
		log.Println("Worker dispatcher: All workers dispatched, waiting...")
		wg.Wait()
		log.Println("Worker dispatcher: All workers completed.")
	}()

	// Collect results
	log.Println("Main thread: Collecting results...")
	processedCounter, errorCount := 0, 0
	for res := range resultChan {
		processedCounter++
		if res.err != nil {
			errorCount++
			log.Printf("Main thread: Error chunk %d: %v", res.index, res.err)
		} else if res.index >= 0 && res.index < len(results) {
			results[res.index] = res.content
		} else {
			errorCount++
			log.Printf("Error: Invalid index %d", res.index)
		}
	}
	log.Printf("Main thread: Collection complete. Success: %d, Errors: %d", processedCounter-errorCount, errorCount)

	// Filter results
	validResultsCount, totalOutputWords := 0, 0
	var finalResults []string
	for _, r := range results {
		trimmedResult := strings.TrimSpace(r)
		if trimmedResult != "" {
			validResultsCount++
			totalOutputWords += len(strings.Fields(trimmedResult))
			finalResults = append(finalResults, trimmedResult)
		}
	}

	// Log stats
	if mode == "document" { /* ... log document stats ... */
	} else { /* ... log transcript stats ... */
	}
	log.Printf("%s chunk processing completed in %v. Input: %d words, Output: %d words. Valid chunks: %d/%d",
		mode, time.Since(startTime), totalInputWords, totalOutputWords, validResultsCount, len(chunks))

	return finalResults
}

// ProcessTranscript orchestrates: Analyze -> Chunk -> Process (with map) -> Combine (simple)
func ProcessTranscript(ctx context.Context, text string, cfg *config.Config, ratio float64) string {
	log.Printf("Processing transcript (simple map approach) %d words, ratio %.2f", len(strings.Fields(text)), ratio)
	overallStartTime := time.Now()

	// --- Step 1: Analyze Speakers -> Get Role->Name Map ---
	// Use the *new* parseSpeakerAnalysis which returns map[string]string
	speakerAnalysisRaw, err := api.AnalyzeSpeakers(ctx, text, cfg.OpenRouterKey) // Still get raw text
	if err != nil {
		log.Printf("WARNING: Speaker analysis failed: %v.", err)
		speakerAnalysisRaw = ""
	}
	if ctx.Err() != nil {
		log.Printf("Ctx cancelled during analysis.")
		return ""
	}

	// Parse the raw analysis into the simple map
	speakerRoleNameMap := transcript.ParseSpeakerAnalysis(speakerAnalysisRaw)
	// -----------------------------

	// --- Step 2: Chunk the Text ---
	chunks, err := chunker.ChunkTextBySpace(text, cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		log.Printf("Error chunking: %v", err)
		return ""
	}
	if len(chunks) == 0 {
		log.Printf("Zero chunks created.")
		return ""
	}
	log.Printf("Chunked transcript into %d parts.", len(chunks))
	// -----------------------------

	// --- Step 3: Process Chunks (Pass map to workers) ---
	processedChunks := ProcessChunks(ctx, chunks, cfg, ratio, "transcript", speakerRoleNameMap) // Pass the map
	// -----------------------------------------------------

	if ctx.Err() != nil {
		log.Printf("Ctx cancelled during chunk processing.")
		return ""
	}
	if len(processedChunks) == 0 {
		log.Printf("No valid results from chunk processing.")
		return ""
	}
	log.Printf("Successfully processed %d chunks via API.", len(processedChunks))

	// --- Step 4: Combine and Final Format (Simple Bolding) ---
	// Use the *new* CombineTranscriptChunks which doesn't need the map anymore
	finalResult := transcript.CombineTranscriptChunks(processedChunks)
	// -------------------------------------------------------------

	log.Printf("Transcript processing completed in %v. Final words: %d", time.Since(overallStartTime), len(strings.Fields(finalResult)))
	return finalResult
}
