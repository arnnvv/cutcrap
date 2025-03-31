package transcript

import (
	"log"
	"regexp"
	"strings"
)

type SpeakerInfo struct {
	OriginalLabel string
	StandardLabel string
	Occurrences   int
}

func DetectSpeakers(text string) map[string]string {
	log.Println("Detecting speakers in transcript text")

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z\s\.]{0,20}):\s`),
		regexp.MustCompile(`\[([A-Za-z][A-Za-z\s\.]{0,20})\]:`),
		regexp.MustCompile(`(?i)(speaker|person)\s*([a-z0-9])(\s|:)`),
		regexp.MustCompile(`(?i)(host|guest|interviewer|interviewee)(\s|:)`),
	}

	speakerMap := make(map[string]SpeakerInfo)

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				speaker := strings.TrimSpace(match[1])
				if speaker != "" {
					info, exists := speakerMap[speaker]
					if exists {
						info.Occurrences++
						speakerMap[speaker] = info
					} else {
						speakerMap[speaker] = SpeakerInfo{
							OriginalLabel: speaker,
							StandardLabel: "",
							Occurrences:   1,
						}
					}
				}
			}
		}
	}

	result := make(map[string]string)

	for name := range speakerMap {
		if strings.EqualFold(name, "Guest") || strings.EqualFold(name, "Host") {
			result[name+":"] = name + ":"
		}
	}

	log.Printf("Preserved %d speaker labels in transcript", len(result))
	return result
}

func StandardizeSpeakers(text string, speakerMap map[string]string) string {
	if len(speakerMap) == 0 {
		return text
	}

	log.Println("Standardizing speaker labels in transcript")

	result := text
	for original, standard := range speakerMap {
		pattern := regexp.MustCompile(`(?m)(^|\n)` + regexp.QuoteMeta(original) + `\s*`)
		result = pattern.ReplaceAllString(result, "${1}"+standard+" ")
	}

	return result
}
