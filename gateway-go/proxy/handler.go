package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gateway-go/converter"
	"gateway-go/credential"
	"gateway-go/token"
)

const (
	maxRetries       = 3
	doneMarker       = "[done]"
	maxContinuations = 3
)

var (
	cooldownRegex = regexp.MustCompile(`(?i)(?:try again in|retry after|wait)\s+(\d+)\s*(?:seconds?|s)`)
)

type Proxy struct {
	upstreamURL string
	credManager *credential.Manager
	tokenStats  *token.Stats
	httpClient  *http.Client
}

func NewProxy(upstreamURL string, credManager *credential.Manager, tokenStats *token.Stats) *Proxy {
	return &Proxy{
		upstreamURL: upstreamURL,
		credManager: credManager,
		tokenStats:  tokenStats,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// HandleNonStreaming handles a non-streaming request with retry logic.
func (p *Proxy) HandleNonStreaming(w http.ResponseWriter, oaiReq *converter.OpenAIRequest, reqID string) {
	gemReq, err := converter.OpenAIToGemini(oaiReq)
	if err != nil {
		writeJSONError(w, 400, "format conversion error: "+err.Error())
		return
	}

	// Inject anti-truncation instruction
	injectAntiTruncation(gemReq)

	// Estimate input tokens
	inputText := extractAllText(oaiReq)
	inputTokens := token.EstimateInputTokens(inputText, 0)

	model := oaiReq.Model
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		cred, err := p.credManager.GetCredential(model)
		if err != nil {
			lastErr = err
			continue
		}

		gemResp, statusCode, err := p.doRequest(gemReq, model, cred, false)
		if err != nil {
			lastErr = err
			if isRetryable(statusCode) {
				cooldown := parseCooldown(err.Error())
				p.credManager.RecordError(cred, statusCode, model, cooldown)
				backoff(attempt)
				continue
			}
			if statusCode == 400 || statusCode == 403 {
				p.credManager.RecordError(cred, statusCode, model, 0)
			}
			writeJSONError(w, statusCode, err.Error())
			return
		}

		// Remove [done] marker from response
		cleanDoneMarker(gemResp)

		oaiResp := converter.GeminiToOpenAI(gemResp, model, reqID)
		oaiResp.Created = time.Now().Unix()

		// Record token stats
		outputTokens := 0
		if gemResp.UsageMetadata != nil {
			outputTokens = gemResp.UsageMetadata.CandidatesTokenCount
		}
		p.tokenStats.Record(cred.ID, model, inputTokens, outputTokens)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oaiResp)
		return
	}

	writeJSONError(w, 502, "all retries exhausted: "+lastErr.Error())
}

// HandleStreaming handles a streaming request with retry and anti-truncation.
func (p *Proxy) HandleStreaming(w http.ResponseWriter, oaiReq *converter.OpenAIRequest, reqID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, 500, "streaming not supported")
		return
	}

	gemReq, err := converter.OpenAIToGemini(oaiReq)
	if err != nil {
		writeJSONError(w, 400, "format conversion error: "+err.Error())
		return
	}

	injectAntiTruncation(gemReq)

	inputText := extractAllText(oaiReq)
	inputTokens := token.EstimateInputTokens(inputText, 0)

	model := oaiReq.Model

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	var collectedText strings.Builder
	foundDone := false
	totalOutputTokens := 0

	var currentCred *credential.Credential

	for continuation := 0; continuation <= maxContinuations; continuation++ {
		var cred *credential.Credential
		var err error

		if currentCred != nil && continuation > 0 {
			cred = currentCred
		} else {
			for attempt := 0; attempt <= maxRetries; attempt++ {
				cred, err = p.credManager.GetCredential(model)
				if err != nil {
					if attempt == maxRetries {
						fmt.Fprintf(w, "data: {\"error\": \"no credentials available\"}\n\n")
						flusher.Flush()
						return
					}
					backoff(attempt)
					continue
				}
				break
			}
		}

		currentCred = cred

		resp, err := p.doStreamRequest(gemReq, model, cred)
		if err != nil {
			// Try retry with different credential
			retried := false
			for attempt := 0; attempt < maxRetries; attempt++ {
				newCred, credErr := p.credManager.PreWarmCredential(model, cred.ID)
				if credErr != nil {
					continue
				}
				cred = newCred
				currentCred = cred
				resp, err = p.doStreamRequest(gemReq, model, cred)
				if err == nil {
					retried = true
					break
				}
				backoff(attempt)
			}
			if !retried {
				fmt.Fprintf(w, "data: {\"error\": \"upstream request failed: %s\"}\n\n", err.Error())
				flusher.Flush()
				return
			}
		}

		// Process stream
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := line[6:]
			var gemResp converter.GeminiResponse
			if err := json.Unmarshal([]byte(data), &gemResp); err != nil {
				continue
			}

			// Extract text and check for [done]
			chunkText := extractChunkText(&gemResp)
			if strings.Contains(chunkText, doneMarker) {
				foundDone = true
				chunkText = strings.ReplaceAll(chunkText, doneMarker, "")
				// Update the gemini response with cleaned text
				cleanDoneMarker(&gemResp)
			}
			collectedText.WriteString(chunkText)

			// Track output tokens from last chunk
			if gemResp.UsageMetadata != nil {
				totalOutputTokens = gemResp.UsageMetadata.CandidatesTokenCount
			}

			// Convert and forward
			oaiChunk := converter.GeminiChunkToOpenAIChunk(&gemResp, model, reqID)
			oaiChunk.Created = time.Now().Unix()

			chunkJSON, _ := json.Marshal(oaiChunk)
			fmt.Fprintf(w, "data: %s\n\n", chunkJSON)
			flusher.Flush()
		}
		resp.Body.Close()

		if foundDone {
			break
		}

		// No [done] found - build continuation request
		if continuation < maxContinuations {
			gemReq = buildContinuation(gemReq, collectedText.String())
		}
	}

	// Send [DONE] marker
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	// Record token stats
	p.tokenStats.Record(currentCred.ID, model, inputTokens, totalOutputTokens)
}

func (p *Proxy) doRequest(gemReq *converter.GeminiRequest, model string, cred *credential.Credential, stream bool) (*converter.GeminiResponse, int, error) {
	body, _ := json.Marshal(gemReq)

	endpoint := "generateContent"
	if stream {
		endpoint = "streamGenerateContent"
	}
	url := fmt.Sprintf("%s/v1/models/%s:%s", p.upstreamURL, model, endpoint)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, fmt.Errorf("upstream error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var gemResp converter.GeminiResponse
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse upstream response: %w", err)
	}

	return &gemResp, 200, nil
}

func (p *Proxy) doStreamRequest(gemReq *converter.GeminiRequest, model string, cred *credential.Credential) (*http.Response, error) {
	body, _ := json.Marshal(gemReq)
	url := fmt.Sprintf("%s/v1/models/%s:streamGenerateContent", p.upstreamURL, model)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		statusCode := resp.StatusCode
		if isRetryable(statusCode) {
			cooldown := parseCooldown(string(respBody))
			p.credManager.RecordError(cred, statusCode, model, cooldown)
		} else if statusCode == 400 || statusCode == 403 {
			p.credManager.RecordError(cred, statusCode, model, 0)
		}

		return nil, fmt.Errorf("upstream error (status %d): %s", statusCode, string(respBody))
	}

	return resp, nil
}

func injectAntiTruncation(req *converter.GeminiRequest) {
	instruction := fmt.Sprintf(`When you have completed your full response, you must output %s on a separate line at the very end. Only output %s when your answer is complete.`, doneMarker, doneMarker)

	if req.SystemInstruction != nil {
		if len(req.SystemInstruction.Parts) > 0 {
			req.SystemInstruction.Parts[0].Text += "\n\n" + instruction
		}
	} else {
		req.SystemInstruction = &converter.GeminiContent{
			Parts: []converter.GeminiPart{{Text: instruction}},
			Role:  "user",
		}
	}
}

func buildContinuation(original *converter.GeminiRequest, collectedText string) *converter.GeminiRequest {
	newReq := *original

	suffix := collectedText
	if len(suffix) > 100 {
		suffix = suffix[len(suffix)-100:]
	}

	continuation := fmt.Sprintf(
		"Continue from where you left off. You have already output approximately %d characters ending with:\n\"...%s\"\n\nContinue outputting the remaining content:",
		len(collectedText), suffix,
	)

	newContents := make([]converter.GeminiContent, len(original.Contents))
	copy(newContents, original.Contents)

	newContents = append(newContents, converter.GeminiContent{
		Parts: []converter.GeminiPart{{Text: collectedText}},
		Role:  "model",
	})
	newContents = append(newContents, converter.GeminiContent{
		Parts: []converter.GeminiPart{{Text: continuation}},
		Role:  "user",
	})

	newReq.Contents = newContents
	return &newReq
}

func extractChunkText(resp *converter.GeminiResponse) string {
	var texts []string
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
	}
	return strings.Join(texts, "")
}

func cleanDoneMarker(resp *converter.GeminiResponse) {
	for i := range resp.Candidates {
		for j := range resp.Candidates[i].Content.Parts {
			if resp.Candidates[i].Content.Parts[j].Text != "" {
				resp.Candidates[i].Content.Parts[j].Text = strings.ReplaceAll(
					resp.Candidates[i].Content.Parts[j].Text, doneMarker, "")
			}
		}
	}
}

func extractAllText(req *converter.OpenAIRequest) string {
	var sb strings.Builder
	for _, msg := range req.Messages {
		text := converter.ExtractTextContent(msg.Content)
		sb.WriteString(text)
	}
	return sb.String()
}

func isRetryable(statusCode int) bool {
	return statusCode == 429 || statusCode == 503
}

func parseCooldown(errorMsg string) int {
	matches := cooldownRegex.FindStringSubmatch(errorMsg)
	if len(matches) >= 2 {
		if seconds, err := strconv.Atoi(matches[1]); err == nil {
			return seconds
		}
	}
	return 0
}

func backoff(attempt int) {
	delay := 100 * (1 << attempt) // 100ms, 200ms, 400ms
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "gateway_error",
			"code":    code,
		},
	})
}
