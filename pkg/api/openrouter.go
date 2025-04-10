// pkg/api/openrouter.go

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// GeminiResponse struct remains the same
type GeminiResponse struct { // ... (keep struct definition) ...
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
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

// AnalyzeSpeakers remains the same (returns raw analysis string)
func AnalyzeSpeakers(ctx context.Context, fullText, apiKey string) (string, error) {
	// ... (Keep implementation the same) ...
	startTime := time.Now()
	log.Printf("Starting speaker analysis for text of %d words", len(strings.Fields(fullText)))

	analysisPrompt := `Analyze the following podcast transcript to identify the speakers. Provide the following information in a clear, concise list format:
1. Total number of distinct speakers detected.
2. Identify the HOST (usually the one asking questions, leading the conversation, or doing intros/outros). Provide their name if clearly mentioned.
3. Identify the GUEST(s). Provide their names if clearly mentioned. If multiple guests, list them as Guest 1, Guest 2, etc.
4. For each speaker (Host and Guests), provide a brief 1-sentence description of their apparent role or topic focus if discernible from the text.

Focus ONLY on information present in the transcript. Do not guess information not present.

Transcript:
--- TRANSCRIPT START ---
%s
--- TRANSCRIPT END ---

Return ONLY the analysis result using this exact format:
- Total Speakers: [Number]
- Host: [Name or "Host"], [Brief Description]
- Guest 1: [Name or "Guest 1"], [Brief Description]
- Guest 2: [Name or "Guest 2"], [Brief Description]
... (continue for all detected guests)`

	payload := map[string]any{
		"contents": []map[string]any{{"parts": []map[string]string{{"text": fmt.Sprintf(analysisPrompt, fullText)}}}},
		// Optional generationConfig
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed marshal analysis payload: %w", err)
	}

	apiURL := "https://generativelanguage.googleapis.com/v1beta/models/models/gemini-2.0-flash:generateContent?key=" + apiKey // Or your preferred model
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed create analysis request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("analysis API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("analysis API non-OK status: %s. Body: %s", resp.Status, string(respBodyBytes))
	}

	var response GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed decode analysis response: %w", err)
	}
	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in analysis response")
	}

	analysisResult := response.Candidates[0].Content.Parts[0].Text
	log.Printf("Successfully completed speaker analysis in %v.", time.Since(startTime))
	return analysisResult, nil
}

// --- ProcessTextWithMode --- NOW ACCEPTS speakerRoleNameMap map[string]string ---
func ProcessTextWithMode(ctx context.Context, text, apiKey string, targetWordCount int, mode string, speakerRoleNameMap map[string]string) (string, error) { // Changed last param
	startTime := time.Now()
	inputWordCount := len(strings.Fields(text))
	log.Printf("Processing text chunk (mode: %s, %d words, target: %d)", mode, inputWordCount, targetWordCount)

	var prompt string
	if mode == "transcript" {
		// --- NEW DYNAMIC TRANSCRIPT PROMPT USING THE MAP ---
		var speakerMappingInstructions string
		if len(speakerRoleNameMap) > 0 {
			var instructions []string
			instructions = append(instructions, "Use this mapping to identify speakers:")
			for role, name := range speakerRoleNameMap {
				instructions = append(instructions, fmt.Sprintf("- If you identify '%s', use the name '%s'.", role, name))
			}
			// Add instruction for unknown speakers? Or tell it to guess? Let's try being strict.
			instructions = append(instructions, "- If a speaker doesn't match a role above, try to use their name if explicitly mentioned in the text.")
			instructions = append(instructions, "- If no name is clear for a turn, label it 'Unknown Speaker'.") // Or omit? Let's try omit first.
			speakerMappingInstructions = strings.Join(instructions, "\n")
		} else {
			log.Printf("Warning: Processing transcript chunk without speaker map.")
			speakerMappingInstructions = "Speaker identification information is unavailable. Use speaker names if clearly mentioned in the text, otherwise label speakers generically (e.g., 'Speaker 1', 'Speaker 2')."
		}

		prompt = fmt.Sprintf(`You are processing a chunk of subtitles from a podcast. Your task is to format this chunk as a clean, readable transcript segment using extremely simple English (like for a 10-year-old).

**SPEAKER IDENTIFICATION RULES:**
%s

**FORMATTING RULES:**
1. Use very simple English, basic vocabulary only.
2. Slightly improve grammar, spelling, and sentence structure for readability, but keep the meaning identical to the original subtitles.
3. Format the output strictly line-by-line, starting each line ONLY with the speaker's correct NAME followed by a colon.
   Example:
   Shandon: [Simplified speech]
   Nikil Vora: [Simplified speech]
   Chirag: [Simplified speech]

**IMPORTANT CONSTRAINTS:**
- Return ONLY the formatted transcript lines for THIS CHUNK. Each line MUST start with a speaker's NAME followed by a colon.
- Do NOT include roles (like "Host", "Guest 1"). Use ONLY the names provided in the mapping or identified directly.
- Do not shortern the length a lot
- Do NOT add introductions, summaries, explanations, or comments.
- Do NOT repeat the speaker identification rules in your response.

--- CURRENT CHUNK START ---
%s
--- CURRENT CHUNK END ---

Formatted Output:`, speakerMappingInstructions, text) // Use the map instructions
		// --- END NEW PROMPT ---

	} else { // document mode (prompt remains the same)
		prompt = fmt.Sprintf(`Condense this text to approximately %d words while:
- Preserving all key plot points and essential information and data.
- Using extremely simple English with basic vocabulary (like for a 10-year-old).
- Maintaining the original narration style as much as possible.
- If you identify any headings in the text, format them as "# Heading" on their own line in markdown style.

Important: Return ONLY the condensed text without any introductions, explanations, or summaries.

--- TEXT TO CONDENSE START ---
%s
--- TEXT TO CONDENSE END ---

Condensed Text:`, targetWordCount, text)
	}

	payload := map[string]any{
		"contents":         []map[string]any{{"parts": []map[string]string{{"text": prompt}}}},
		"generationConfig": map[string]any{"temperature": 0.4}, // Adjust as needed
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed marshal API payload: %w", err)
	}

	apiURL := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=" + apiKey // Use appropriate model
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed create API request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed (%s mode): %w", mode, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("API non-OK status (%s mode): %s. Body: %s", mode, resp.Status, string(respBodyBytes))
		return "", fmt.Errorf("API request failed (%s mode): %s", mode, resp.Status)
	}

	var response GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed decode API response (%s mode): %w", mode, err)
	}
	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in API response (%s mode)", mode)
	}

	result := response.Candidates[0].Content.Parts[0].Text
	outputWordCount := len(strings.Fields(result))
	log.Printf("API call successful (%s mode). Result: %d words. Time: %v", mode, outputWordCount, time.Since(startTime))
	return result, nil
}
