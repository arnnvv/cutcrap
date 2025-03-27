package workers

import (
	"context"
	"log"
	"pdf-processor/pkg/api"
	"pdf-processor/pkg/chunker"
	"pdf-processor/pkg/config"
	"pdf-processor/pkg/transcript"
	"strings"
	"sync"
	"time"
)

func ProcessChunks(ctx context.Context, chunks []string, cfg *config.Config, ratio float64, mode string) []string {
	startTime := time.Now()

	totalInputWords := 0
	for _, chunk := range chunks {
		totalInputWords += len(strings.Fields(chunk))
	}

	log.Printf("Starting to process %d chunks with max concurrency %d (total input: %d words) in %s mode",
		len(chunks), cfg.MaxConcurrent, totalInputWords, mode)

	var (
		wg         sync.WaitGroup
		results    = make([]string, len(chunks))
		semaphore  = make(chan struct{}, cfg.MaxConcurrent)
		resultChan = make(chan struct {
			index   int
			content string
		})
	)

	go func() {
		log.Printf("Worker goroutine started, will process %d chunks", len(chunks))
		for i, chunk := range chunks {
			wg.Add(1)
			semaphore <- struct{}{}
			chunkWords := len(strings.Fields(chunk))
			log.Printf("Dispatching worker for chunk %d/%d (size: %d words)", i+1, len(chunks), chunkWords)

			go func(index int, text string) {
				chunkStartTime := time.Now()
				defer func() {
					<-semaphore
					wg.Done()
					log.Printf("Worker for chunk %d completed in %v", index, time.Since(chunkStartTime))
				}()

				inputWords := len(strings.Fields(text))
				log.Printf("Processing chunk %d (%d words) in %s mode", index, inputWords, mode)

				targetWordCount := int(float64(cfg.ChunkSize) * ratio)
				if targetWordCount <= 0 {
					targetWordCount = 1
				}

				var content string
				var err error

				if mode == "transcript" {
					content, err = api.ProcessTranscript(ctx, text, cfg.OpenRouterKey, targetWordCount)
				} else {
					content, err = api.ProcessText(ctx, text, cfg.OpenRouterKey, targetWordCount)
				}

				if err != nil {
					log.Printf("Error processing chunk %d: %v", index, err)
				} else {
					outputWords := len(strings.Fields(content))
					log.Printf("Successfully processed chunk %d, result: %d words", index, outputWords)
					resultChan <- struct {
						index   int
						content string
					}{index, content}
				}
			}(i, chunk)
		}
		log.Println("All workers dispatched, waiting for completion")
		wg.Wait()
		log.Println("All workers completed, closing result channel")
		close(resultChan)
	}()

	log.Println("Collecting results from workers")
	resultCount := 0
	for res := range resultChan {
		resultCount++
		resultWords := len(strings.Fields(res.content))
		log.Printf("Received result %d/%d for chunk %d (%d words)", resultCount, len(chunks), res.index, resultWords)
		results[res.index] = res.content
	}

	validResults := 0
	totalOutputWords := 0
	for _, r := range results {
		if r != "" {
			validResults++
			totalOutputWords += len(strings.Fields(r))
		}
	}

	reductionPercent := 100.0
	if totalInputWords > 0 {
		reductionPercent = 100.0 - (float64(totalOutputWords)/float64(totalInputWords))*100.0
	}

	log.Printf("Processing completed in %v, received %d valid results out of %d chunks",
		time.Since(startTime), validResults, len(chunks))
	log.Printf("Total input: %d words, total output: %d words (%.1f%% reduction)",
		totalInputWords, totalOutputWords, reductionPercent)

	if mode == "transcript" && validResults > 0 {
		log.Printf("Applying transcript formatting to %d chunks", validResults)
		var validChunks []string
		for _, r := range results {
			if r != "" {
				validChunks = append(validChunks, r)
			}
		}

		combinedResult := transcript.CombineTranscriptChunks(validChunks)
		return []string{combinedResult}
	}

	return results
}

func ProcessTranscript(ctx context.Context, text string, cfg *config.Config, ratio float64) string {
	log.Printf("Processing transcript of %d words with ratio %.2f", len(strings.Fields(text)), ratio)

	chunks, err := chunker.ChunkTextBySpace(text, cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		log.Printf("Error chunking transcript text: %v", err)
		return ""
	}

	log.Printf("Chunked transcript into %d parts", len(chunks))

	results := ProcessChunks(ctx, chunks, cfg, ratio, "transcript")

	if len(results) == 0 {
		log.Printf("No results returned from transcript processing")
		return ""
	}

	return results[0]
}
