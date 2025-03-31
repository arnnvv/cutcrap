package transcript

import (
	"log"
	"regexp"
	"strings"
)

func FormatTranscript(text string) string {
	log.Println("Formatting transcript text")

	text = regexp.MustCompile(`\r\n`).ReplaceAllString(text, "\n")

	text = regexp.MustCompile(`\n{2,}`).ReplaceAllString(text, "\n")

	text = regexp.MustCompile(`\s{2,}`).ReplaceAllString(text, " ")

	lines := strings.Split(text, "\n")
	var result []string
	var currentSpeaker string
	var currentText string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		speakerMatch := regexp.MustCompile(`^([^:]+):\s*(.*)$`).FindStringSubmatch(line)
		if len(speakerMatch) < 3 {
			if currentSpeaker != "" {
				currentText += " " + line
			} else {
				result = append(result, line)
			}
			continue
		}

		speaker := strings.TrimSpace(speakerMatch[1])
		content := strings.TrimSpace(speakerMatch[2])

		if speaker == currentSpeaker {
			currentText += " " + content
		} else {
			if currentSpeaker != "" {
				result = append(result, currentSpeaker+": "+currentText)
			}
			currentSpeaker = speaker
			currentText = content
		}
	}

	if currentSpeaker != "" {
		result = append(result, currentSpeaker+": "+currentText)
	}

	text = strings.Join(result, "\n")

	text = strings.TrimPrefix(text, "\n")

	log.Println("Transcript formatting completed")
	return text
}

func CombineTranscriptChunks(chunks []string) string {
	log.Printf("Combining %d transcript chunks", len(chunks))

	var processedChunks []string
	for i, chunk := range chunks {
		formatted := FormatTranscript(chunk)
		processedChunks = append(processedChunks, formatted)
		log.Printf("Processed chunk %d/%d", i+1, len(chunks))
	}

	combined := strings.Join(processedChunks, "\n\n")

	combined = regexp.MustCompile(`\n{3,}`).ReplaceAllString(combined, "\n\n")

	log.Printf("Successfully combined transcript chunks into %d words",
		len(strings.Fields(combined)))

	return combined
}
