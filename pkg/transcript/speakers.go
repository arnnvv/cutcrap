package transcript

import (
	"log"
	"regexp"
	"strings"
)

// SpeakerInfo stores information about detected speakers
type SpeakerInfo struct {
	OriginalLabel string
	StandardLabel string
	Occurrences   int
}

// DetectSpeakers analyzes text to find speaker patterns and standardizes them
func DetectSpeakers(text string) map[string]string {
	log.Println("Detecting speakers in transcript text")

	// Common patterns for speaker identification
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z\s\.]{0,20}):\s`),           // Name:
		regexp.MustCompile(`\[([A-Za-z][A-Za-z\s\.]{0,20})\]:`),              // [Name]:
		regexp.MustCompile(`(?i)(speaker|person)\s*([a-z0-9])(\s|:)`),        // Speaker A, Person 1
		regexp.MustCompile(`(?i)(host|guest|interviewer|interviewee)(\s|:)`), // Host, Guest, etc.
	}

	// Map to store unique speakers
	speakerMap := make(map[string]SpeakerInfo)

	// Find all potential speaker indicators
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				speaker := strings.TrimSpace(match[1])
				if speaker != "" {
					// Store or update speaker info
					info, exists := speakerMap[speaker]
					if exists {
						info.Occurrences++
						speakerMap[speaker] = info
					} else {
						speakerMap[speaker] = SpeakerInfo{
							OriginalLabel: speaker,
							StandardLabel: "", // Will be assigned later
							Occurrences:   1,
						}
					}
				}
			}
		}
	}

	// Assign standard labels - MODIFIED to prefer Host/Guest
	result := make(map[string]string)

	// Keep exact mapping for Guest/Host labels (no transformation)
	for name := range speakerMap {
		if strings.EqualFold(name, "Guest") || strings.EqualFold(name, "Host") {
			result[name+":"] = name + ":"
		}
	}

	// No label transformation - return either empty map or just the Guest/Host mapping
	log.Printf("Preserved %d speaker labels in transcript", len(result))
	return result
}

// StandardizeSpeakers - MODIFIED to preserve original labels when possible
func StandardizeSpeakers(text string, speakerMap map[string]string) string {
	// If no speaker map is provided or it's empty, return the original text
	if len(speakerMap) == 0 {
		return text
	}

	log.Println("Standardizing speaker labels in transcript")

	result := text
	for original, standard := range speakerMap {
		// Replace at the beginning of lines or after newlines
		pattern := regexp.MustCompile(`(?m)(^|\n)` + regexp.QuoteMeta(original) + `\s*`)
		result = pattern.ReplaceAllString(result, "${1}"+standard+" ")
	}

	return result
}
