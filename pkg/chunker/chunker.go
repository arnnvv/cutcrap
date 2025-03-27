package chunker

import (
	"log"
	"strings"
	"unicode"
)

// ChunkText splits text into chunks based on sentence boundaries
// This is the original method used for document processing
func ChunkText(content string, chunkSize int) ([]string, error) {
	log.Printf("Starting text chunking with chunk size %d words", chunkSize)

	// Clean and normalize the text
	content = strings.ReplaceAll(content, "\r\n", " ")
	content = strings.ReplaceAll(content, "\n", " ")

	// Split the content into sentences
	sentences := splitIntoSentences(content)
	log.Printf("Split content into %d sentences", len(sentences))

	return createChunksFromSentences(sentences, chunkSize), nil
}

// ChunkTextBySpace splits text into chunks based on word count with overlap
// This is the new method optimized for podcast/dialogue processing
func ChunkTextBySpace(content string, chunkSize int, overlap int) ([]string, error) {
	log.Printf("Starting space-based text chunking with chunk size %d words and %d words overlap",
		chunkSize, overlap)

	// Clean and normalize the text
	content = strings.ReplaceAll(content, "\r\n", " ")
	content = strings.ReplaceAll(content, "\n", " ")

	// Normalize spaces
	content = strings.Join(strings.Fields(content), " ")

	// Split into words
	words := strings.Fields(content)
	log.Printf("Text contains %d words total", len(words))

	// Create chunks with overlap
	var chunks []string

	// If text is smaller than chunk size, return as single chunk
	if len(words) <= chunkSize {
		log.Printf("Text is smaller than chunk size, returning as single chunk")
		return []string{content}, nil
	}

	// Create chunks with specified overlap
	for i := 0; i < len(words); i += (chunkSize - overlap) {
		end := i + chunkSize
		if end > len(words) {
			end = len(words)
		}

		chunk := strings.Join(words[i:end], " ")
		chunks = append(chunks, chunk)

		if i > 0 && i%1000 == 0 {
			log.Printf("Created %d chunks so far", len(chunks))
		}

		if end == len(words) {
			break
		}
	}

	log.Printf("Created %d chunks using space-based chunking", len(chunks))
	return chunks, nil
}

// Split text into sentences by looking for periods followed by spaces
func splitIntoSentences(text string) []string {
	log.Printf("Splitting text into sentences, text length: %d characters", len(text))

	// Replace common abbreviations to avoid false sentence breaks
	text = replaceAbbreviations(text)

	var sentences []string
	var currentSentence strings.Builder

	for i := 0; i < len(text); i++ {
		currentSentence.WriteByte(text[i])

		// Check for end of sentence (period, exclamation, question mark followed by space or end of text)
		if (text[i] == '.' || text[i] == '!' || text[i] == '?') &&
			(i == len(text)-1 || unicode.IsSpace(rune(text[i+1]))) {

			sentence := strings.TrimSpace(currentSentence.String())
			if len(strings.Fields(sentence)) > 0 {
				sentences = append(sentences, sentence)
			}
			currentSentence.Reset()
		}
	}

	// Add any remaining text as a sentence
	if currentSentence.Len() > 0 {
		sentence := strings.TrimSpace(currentSentence.String())
		if len(strings.Fields(sentence)) > 0 {
			sentences = append(sentences, sentence)
		}
	}

	log.Printf("Found %d sentences in text", len(sentences))
	return sentences
}

// Create chunks from sentences, ensuring each chunk is close to the target size
// and ends with a complete sentence
func createChunksFromSentences(sentences []string, targetChunkSize int) []string {
	var chunks []string
	var currentChunk strings.Builder
	currentWordCount := 0

	for i, sentence := range sentences {
		sentenceWords := len(strings.Fields(sentence))

		// If adding this sentence would exceed the chunk size and we already have content,
		// finish the current chunk
		if currentWordCount > 0 && currentWordCount+sentenceWords > targetChunkSize {
			chunk := strings.TrimSpace(currentChunk.String())
			chunks = append(chunks, chunk)
			log.Printf("Created chunk with %d words", currentWordCount)

			currentChunk.Reset()
			currentWordCount = 0
		}

		// Add the sentence to the current chunk
		currentChunk.WriteString(sentence + " ")
		currentWordCount += sentenceWords

		if i > 0 && i%100 == 0 {
			log.Printf("Processed %d/%d sentences", i, len(sentences))
		}
	}

	// Add the final chunk if there's anything left
	if currentChunk.Len() > 0 {
		chunk := strings.TrimSpace(currentChunk.String())
		chunks = append(chunks, chunk)
		log.Printf("Created final chunk with %d words", currentWordCount)
	}

	log.Printf("Created %d chunks from %d sentences", len(chunks), len(sentences))
	return chunks
}

// Replace common abbreviations with placeholders to avoid false sentence breaks
func replaceAbbreviations(text string) string {
	abbreviations := []string{
		"Mr.", "Mrs.", "Ms.", "Dr.", "Prof.",
		"Inc.", "Ltd.", "Co.", "Corp.",
		"i.e.", "e.g.", "etc.",
		"vs.", "a.m.", "p.m.",
		"U.S.", "U.K.", "E.U.",
	}

	result := text
	for _, abbr := range abbreviations {
		// Replace the period in the abbreviation with a special character
		placeholder := strings.ReplaceAll(abbr, ".", "·")
		result = strings.ReplaceAll(result, abbr, placeholder)
	}

	return result
}
