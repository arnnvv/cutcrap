package transcript

import (
	"log"
	"regexp"
	"strings"
)

// FormatTranscript cleans and formats the transcript, preserving consecutive lines from the same speaker
func FormatTranscript(text string) string {
	log.Println("Formatting transcript text")

	// Step 1: Normalize line breaks
	text = regexp.MustCompile(`\r\n`).ReplaceAllString(text, "\n")

	// Step 2: Replace multiple consecutive line breaks with a single one
	text = regexp.MustCompile(`\n{2,}`).ReplaceAllString(text, "\n")

	// Step 3: Clean up extra spaces
	text = regexp.MustCompile(`\s{2,}`).ReplaceAllString(text, " ")

	// Step 4: Combine consecutive lines from the same speaker
	lines := strings.Split(text, "\n")
	var result []string
	var currentSpeaker string
	var currentText string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Extract speaker from line
		speakerMatch := regexp.MustCompile(`^([^:]+):\s*(.*)$`).FindStringSubmatch(line)
		if len(speakerMatch) < 3 {
			// If no speaker pattern, append to current text if we have a speaker
			if currentSpeaker != "" {
				currentText += " " + line
			} else {
				// Otherwise just add as-is
				result = append(result, line)
			}
			continue
		}

		speaker := strings.TrimSpace(speakerMatch[1])
		content := strings.TrimSpace(speakerMatch[2])

		if speaker == currentSpeaker {
			// Same speaker, append content
			currentText += " " + content
		} else {
			// New speaker, save previous speaker's content if any
			if currentSpeaker != "" {
				result = append(result, currentSpeaker+": "+currentText)
			}
			// Start tracking new speaker
			currentSpeaker = speaker
			currentText = content
		}
	}

	// Add the last speaker's content
	if currentSpeaker != "" {
		result = append(result, currentSpeaker+": "+currentText)
	}

	// Join result with line breaks
	text = strings.Join(result, "\n")

	// Ensure the transcript starts without a newline
	text = strings.TrimPrefix(text, "\n")

	log.Println("Transcript formatting completed")
	return text
}

// CombineTranscriptChunks merges processed transcript chunks with proper formatting
func CombineTranscriptChunks(chunks []string) string {
	log.Printf("Combining %d transcript chunks", len(chunks))

	// Simply join the chunks with minimal processing
	// Avoid standardizing speakers to preserve Guest/Host labels
	var processedChunks []string
	for i, chunk := range chunks {
		// Format the chunk but don't standardize speakers
		formatted := FormatTranscript(chunk)
		processedChunks = append(processedChunks, formatted)
		log.Printf("Processed chunk %d/%d", i+1, len(chunks))
	}

	// Combine chunks with proper spacing
	combined := strings.Join(processedChunks, "\n\n")

	// Final cleanup
	combined = regexp.MustCompile(`\n{3,}`).ReplaceAllString(combined, "\n\n")

	log.Printf("Successfully combined transcript chunks into %d words",
		len(strings.Fields(combined)))

	return combined
}
