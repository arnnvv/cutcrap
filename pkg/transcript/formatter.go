// pkg/transcript/formatter.go

package transcript

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

// parseSpeakerAnalysis remains the same (returns simple Role -> Name map)
func ParseSpeakerAnalysis(analysis string) map[string]string {
	// ... (keep implementation from previous version) ...
	mapping := make(map[string]string) // Simple Role -> Name
	if analysis == "" {
		// log.Println("No speaker analysis provided for parsing.") // Less verbose
		return mapping
	}

	lines := strings.Split(analysis, "\n")
	pattern := regexp.MustCompile(`^\s*-\s*\**([^:]+?)\**\s*:\s*\**([^,\n*]+?)\**\s*(?:[,\n].*)?$`)
	foundListStart := false

	// log.Println("Parsing speaker analysis for Role -> Name map...") // Less verbose
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if !foundListStart {
			if strings.HasPrefix(trimmedLine, "-") {
				foundListStart = true
			} else {
				continue
			}
		}

		if foundListStart {
			matches := pattern.FindStringSubmatch(trimmedLine)
			if len(matches) >= 3 {
				role := strings.Trim(strings.TrimSpace(matches[1]), "* ")
				name := strings.Trim(strings.TrimSpace(matches[2]), "* ")

				if role != "" && name != "" && role != "Total Speakers" {
					if _, exists := mapping[role]; !exists {
						mapping[role] = name
						// log.Printf("Parsed mapping: Role '%s' -> Name '%s'", role, name) // Less verbose
					} else {
						// log.Printf("Warning: Duplicate role '%s' found during parsing. Keeping first.", role) // Less verbose
					}
				}
			} else if trimmedLine != "" && strings.HasPrefix(trimmedLine, "-") {
				// log.Printf("Could not parse speaker line format: '%s'", trimmedLine) // Less verbose
			}
		}
	}
	// if len(mapping) == 0 { log.Println("Warning: Parsing speaker analysis resulted in an empty map.") } // Less verbose
	return mapping
}

// FormatTranscript - Minimal cleanup, assuming AI gives "Name: Speech"
func FormatTranscript(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimSpace(text)
}

// --- CombineTranscriptChunks --- UPDATED TO MERGE CONSECUTIVE SPEAKERS ---
func CombineTranscriptChunks(chunks []string) string {
	log.Printf("Combining %d processed chunks, merging speakers, and applying final bolding", len(chunks))

	// --- Step 1: Combine chunks ---
	var builder strings.Builder
	for _, chunk := range chunks {
		// Apply minimal formatting per chunk if needed
		formattedChunk := FormatTranscript(chunk)
		if formattedChunk != "" {
			// Append the chunk content
			builder.WriteString(formattedChunk)
			// Ensure a single newline exists between joined chunk contents if needed
			// This might create duplicate newlines initially, which we clean later
			builder.WriteString("\n")
		}
	}
	combined := builder.String()
	// Normalize newlines and remove excessive blank lines created by joining
	combined = regexp.MustCompile(`\r\n`).ReplaceAllString(combined, "\n")
	combined = regexp.MustCompile(`\n{2,}`).ReplaceAllString(combined, "\n") // Reduce multiple newlines to one
	combined = strings.TrimSpace(combined)

	// --- Step 2: Merge Consecutive Speaker Lines and Apply Bolding ---
	var finalLines []string // Stores the final formatted blocks
	var currentSpeaker string = ""
	var currentSpeech strings.Builder

	speakerLineRegex := regexp.MustCompile(`^([^:]+):\s*(.*)$`) // Extracts name and speech
	lines := strings.Split(combined, "\n")

	// Function to flush the current speaker's buffered speech
	flushSpeakerBlock := func() {
		if currentSpeaker != "" && currentSpeech.Len() > 0 {
			// Format the complete block for the previous speaker
			formattedBlock := fmt.Sprintf("**%s**: %s", currentSpeaker, strings.TrimSpace(currentSpeech.String()))
			finalLines = append(finalLines, formattedBlock)
			currentSpeech.Reset() // Reset buffer for the next speaker block
		}
	}

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue // Skip empty lines between actual content lines
		}

		matches := speakerLineRegex.FindStringSubmatch(trimmedLine)
		if len(matches) == 3 {
			// It's a speaker line (e.g., "Shandon: Speech")
			speaker := strings.TrimSpace(matches[1])
			speechPart := strings.TrimSpace(matches[2])

			if speechPart == "" {
				continue
			} // Skip lines with speaker but no speech

			if speaker == currentSpeaker {
				// Same speaker continues, append speech
				if currentSpeech.Len() > 0 {
					currentSpeech.WriteString(" ") // Add space between merged lines
				}
				currentSpeech.WriteString(speechPart)
			} else {
				// New speaker starts
				flushSpeakerBlock() // Write out the previous speaker's complete block

				// Start the new speaker's block
				currentSpeaker = speaker
				currentSpeech.WriteString(speechPart) // Add the first line of speech for the new speaker
			}
		} else {
			// Line doesn't match "Speaker: Speech" format.
			// Could be orphaned speech or AI error.
			// Option 1: Append to previous speaker if one exists? (Potentially risky)
			// Option 2: Log and discard (Cleaner)
			log.Printf("Warning: Skipping line without speaker tag during final merge: '%s'", trimmedLine)

			// Option 1 Implementation (if chosen):
			// if currentSpeaker != "" {
			//     if currentSpeech.Len() > 0 { currentSpeech.WriteString(" ") }
			//     currentSpeech.WriteString(trimmedLine)
			//     log.Printf("Warning: Appended line without speaker tag to previous speaker '%s'", currentSpeaker)
			// } else {
			//     log.Printf("Warning: Skipping line without speaker tag before first speaker: '%s'", trimmedLine)
			// }
		}
	} // End of loop through lines

	// Flush the very last speaker block after the loop finishes
	flushSpeakerBlock()

	// Join the final formatted blocks with double newlines
	finalOutput := strings.Join(finalLines, "\n\n")

	log.Printf("Successfully combined and formatted transcript. Final word count: %d", len(strings.Fields(finalOutput)))
	return finalOutput
}
