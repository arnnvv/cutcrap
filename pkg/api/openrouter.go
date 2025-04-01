package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string  `json:"finishReason"`
		AvgLogprobs  float64 `json:"avgLogprobs"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
		PromptTokensDetails  []struct {
			Modality   string `json:"modality"`
			TokenCount int    `json:"tokenCount"`
		} `json:"promptTokensDetails"`
		CandidatesTokensDetails []struct {
			Modality   string `json:"modality"`
			TokenCount int    `json:"tokenCount"`
		} `json:"candidatesTokensDetails"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

func ProcessText(ctx context.Context, text, apiKey string, targetWordCount int) (string, error) {
	return processTextWithMode(ctx, text, apiKey, targetWordCount, "document")
}

func ProcessTranscript(ctx context.Context, text, apiKey string, targetWordCount int) (string, error) {
	return processTextWithMode(ctx, text, apiKey, targetWordCount, "transcript")
}

func processTextWithMode(ctx context.Context, text, apiKey string, targetWordCount int, mode string) (string, error) {
	startTime := time.Now()
	inputWordCount := len(strings.Fields(text))
	log.Printf("Processing text chunk of %d words (target: %d words) in %s mode",
		inputWordCount, targetWordCount, mode)

	var prompt string
	if mode == "transcript" {
		prompt = `You are processing a chunk of subtitles from a YouTube podcast. Format this text as a clean, readable transcript with the following rules:

    Clearly identify speakers as either "Guest:" or "Host:" based on context clues and speech patterns.

    Use extremely simple English suitable for a 10-year-old child, with basic vocabulary only.
    keep the words almost same as in origional subtitles , improve grammer , spellings and sentence structure.
    Do not use advanced vocabulary, idioms, or complicated expressions.

Format the output exactly like this:

Guest: [exact simplified speech]
Host: [exact simplified speech]

Important: Return ONLY the formatted transcript without any introductions, explanations, summaries, or additional comments.`
	} else {
		prompt = fmt.Sprintf(`Condense this text to exactly exactly %d words while:
- Preserving all key plot points and essential information and data.
- Removing redundant descriptions and unnecessary elaborations
- Using extremely simple English with basic vocabulary (like for a 10-year-old)
- must maintain the origional narration style as it is.
- If you identify any headings in the text, format them as "# Heading" on their own line in markdown style.

Important: Return ONLY the condensed text without any introductions, explanations, or summaries. Do not include phrases like "Here's the condensed version" or "In summary". Just provide the rewritten text directly.`, targetWordCount)
	}

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{
						"text": fmt.Sprintf("%s\n\n%s", text, prompt),
					},
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	log.Printf("Preparing API request to Gemini API with model gemini-2.0-flash")
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro-exp-03-25:generateContent?key="+apiKey, bytes.NewReader(body))

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	log.Printf("Sending request to Gemini API")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("API request failed: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("API request returned non-OK status: %s", resp.Status)
		return "", fmt.Errorf("API request failed: %s", resp.Status)
	}
	log.Printf("Received response from Gemini API with status %s", resp.Status)

	var response GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Failed to decode API response: %v", err)
		return "", err
	}

	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		log.Printf("API response contained no content")
		return "", fmt.Errorf("no content in response")
	}

	result := response.Candidates[0].Content.Parts[0].Text
	outputWordCount := len(strings.Fields(result))
	reductionPercent := 100.0
	if inputWordCount > 0 {
		reductionPercent = 100.0 - (float64(outputWordCount)/float64(inputWordCount))*100.0
	}

	log.Printf("Successfully processed text in %v, result length: %d words (reduced from %d words, %.1f%% reduction)",
		time.Since(startTime), outputWordCount, inputWordCount, reductionPercent)
	return result, nil
}
