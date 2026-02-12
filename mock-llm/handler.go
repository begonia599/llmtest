package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Global config protected by mutex
var (
	configMu       sync.RWMutex
	globalLatency  = "medium"
	globalErrorRate float64 = 0.0
)

// Gemini format structures
type GeminiRequest struct {
	Contents         []GeminiContent        `json:"contents"`
	GenerationConfig map[string]any         `json:"generationConfig,omitempty"`
	Tools            []any                  `json:"tools,omitempty"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type GeminiPart struct {
	Text         string       `json:"text,omitempty"`
	FunctionCall *FunctionCall `json:"functionCall,omitempty"`
}

type FunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type GeminiResponse struct {
	Candidates    []GeminiCandidate `json:"candidates"`
	UsageMetadata *UsageMetadata    `json:"usageMetadata,omitempty"`
}

type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index"`
}

type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type ErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

type ConfigRequest struct {
	Latency   string  `json:"latency,omitempty"`
	ErrorRate float64 `json:"errorRate,omitempty"`
}

func pickPreset(r *http.Request) Preset {
	// Allow specifying preset via header
	if idStr := r.Header.Get("X-Mock-Preset"); idStr != "" {
		if id, err := strconv.Atoi(idStr); err == nil && id >= 1 && id <= len(Presets) {
			return Presets[id-1]
		}
	}
	return Presets[rand.Intn(len(Presets))]
}

func getLatency(r *http.Request) string {
	if l := r.Header.Get("X-Mock-Latency"); l != "" {
		return l
	}
	configMu.RLock()
	defer configMu.RUnlock()
	return globalLatency
}

func getErrorRate(r *http.Request) float64 {
	if rateStr := r.Header.Get("X-Mock-Error-Rate"); rateStr != "" {
		if rate, err := strconv.ParseFloat(rateStr, 64); err == nil {
			return rate
		}
	}
	configMu.RLock()
	defer configMu.RUnlock()
	return globalErrorRate
}

func applyLatency(latency string) {
	switch latency {
	case "fast":
		time.Sleep(50 * time.Millisecond)
	case "medium":
		time.Sleep(200 * time.Millisecond)
	case "slow":
		time.Sleep(1000 * time.Millisecond)
	case "realistic":
		delay := 500 + rand.Intn(2500)
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

func shouldError(errorRate float64) (bool, int) {
	if errorRate <= 0 {
		return false, 0
	}
	if rand.Float64() < errorRate {
		// 70% chance of 429, 15% 503, 10% 400, 5% 403
		roll := rand.Float64()
		switch {
		case roll < 0.70:
			return true, 429
		case roll < 0.85:
			return true, 503
		case roll < 0.95:
			return true, 400
		default:
			return true, 403
		}
	}
	return false, 0
}

func writeError(w http.ResponseWriter, code int) {
	var msg, status string
	switch code {
	case 429:
		msg = fmt.Sprintf("Quota exceeded. Try again in %d seconds.", 5+rand.Intn(25))
		status = "RESOURCE_EXHAUSTED"
		w.Header().Set("Retry-After", strconv.Itoa(5+rand.Intn(25)))
	case 503:
		msg = "Service temporarily unavailable. Please retry."
		status = "UNAVAILABLE"
	case 400:
		msg = "Invalid request: malformed input content."
		status = "INVALID_ARGUMENT"
	case 403:
		msg = "Permission denied: API key not authorized for this resource."
		status = "PERMISSION_DENIED"
	}
	resp := ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = msg
	resp.Error.Status = status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp)
}

func buildGeminiResponse(preset Preset) GeminiResponse {
	parts := []GeminiPart{}
	if preset.ToolCall != nil {
		parts = append(parts, GeminiPart{
			Text: preset.ResponseText,
		})
		parts = append(parts, GeminiPart{
			FunctionCall: &FunctionCall{
				Name: preset.ToolCall.FunctionName,
				Args: preset.ToolCall.Args,
			},
		})
	} else {
		parts = append(parts, GeminiPart{
			Text: preset.ResponseText,
		})
	}

	return GeminiResponse{
		Candidates: []GeminiCandidate{
			{
				Content: GeminiContent{
					Parts: parts,
					Role:  "model",
				},
				FinishReason: "STOP",
				Index:        0,
			},
		},
		UsageMetadata: &UsageMetadata{
			PromptTokenCount:     preset.InputTokens,
			CandidatesTokenCount: preset.OutputTokens,
			TotalTokenCount:      preset.InputTokens + preset.OutputTokens,
		},
	}
}

func handleGenerateContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	errorRate := getErrorRate(r)
	if shouldErr, code := shouldError(errorRate); shouldErr {
		writeError(w, code)
		return
	}

	preset := pickPreset(r)
	latency := getLatency(r)
	applyLatency(latency)

	resp := buildGeminiResponse(preset)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleStreamGenerateContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	errorRate := getErrorRate(r)
	if shouldErr, code := shouldError(errorRate); shouldErr {
		writeError(w, code)
		return
	}

	preset := pickPreset(r)
	latency := getLatency(r)

	// Apply first-token latency
	applyLatency(latency)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	text := preset.ResponseText
	chunks := splitIntoChunks(text)

	for i, chunk := range chunks {
		isLast := i == len(chunks)-1

		candidate := GeminiCandidate{
			Content: GeminiContent{
				Parts: []GeminiPart{{Text: chunk}},
				Role:  "model",
			},
			Index: 0,
		}

		resp := GeminiResponse{
			Candidates: []GeminiCandidate{candidate},
		}

		if isLast {
			resp.Candidates[0].FinishReason = "STOP"
			resp.UsageMetadata = &UsageMetadata{
				PromptTokenCount:     preset.InputTokens,
				CandidatesTokenCount: preset.OutputTokens,
				TotalTokenCount:      preset.InputTokens + preset.OutputTokens,
			}

			// Add tool call as separate part in last chunk if applicable
			if preset.ToolCall != nil {
				resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, GeminiPart{
					FunctionCall: &FunctionCall{
						Name: preset.ToolCall.FunctionName,
						Args: preset.ToolCall.Args,
					},
				})
			}
		}

		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if !isLast {
			// Random delay between chunks: 20-100ms
			delay := 20 + rand.Intn(80)
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
	}
}

func splitIntoChunks(text string) []string {
	var chunks []string
	for len(text) > 0 {
		// Random chunk size: 20-80 chars
		size := 20 + rand.Intn(60)
		if size > len(text) {
			size = len(text)
		}
		// Try not to split in the middle of a UTF-8 character
		for size < len(text) && !isUTF8Start(text[size]) {
			size++
		}
		chunks = append(chunks, text[:size])
		text = text[size:]
	}
	return chunks
}

func isUTF8Start(b byte) bool {
	return b&0xC0 != 0x80
}

func handleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Simulate OAuth token refresh with fixed 50ms delay
	time.Sleep(50 * time.Millisecond)

	errorRate := getErrorRate(r)
	// Use a lower error rate for token refresh (1/5 of normal)
	if shouldErr, code := shouldError(errorRate / 5); shouldErr {
		writeError(w, code)
		return
	}

	resp := map[string]any{
		"access_token": fmt.Sprintf("mock_token_%d", time.Now().UnixNano()),
		"expires_in":   3600,
		"token_type":   "Bearer",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		configMu.RLock()
		resp := map[string]any{
			"latency":    globalLatency,
			"error_rate": globalErrorRate,
		}
		configMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

	case http.MethodPost:
		var req ConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		configMu.Lock()
		if req.Latency != "" {
			valid := map[string]bool{"fast": true, "medium": true, "slow": true, "realistic": true}
			if valid[req.Latency] {
				globalLatency = req.Latency
			}
		}
		if req.ErrorRate >= 0 && req.ErrorRate <= 1 {
			globalErrorRate = req.ErrorRate
		}
		configMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handlePresets(w http.ResponseWriter, r *http.Request) {
	type presetInfo struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
		OutputLen    int    `json:"output_length"`
		HasToolCall  bool   `json:"has_tool_call"`
		IsMultiTurn  bool   `json:"is_multi_turn"`
	}

	infos := make([]presetInfo, len(Presets))
	for i, p := range Presets {
		infos[i] = presetInfo{
			ID:           p.ID,
			Name:         p.Name,
			InputTokens:  p.InputTokens,
			OutputTokens: p.OutputTokens,
			OutputLen:    len(p.ResponseText),
			HasToolCall:  p.ToolCall != nil,
			IsMultiTurn:  p.IsMultiTurn,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

func newRouter() http.Handler {
	mux := http.NewServeMux()

	// Gemini API endpoints - use prefix matching + manual dispatch
	// because Go ServeMux doesn't support ':' in wildcard paths
	mux.HandleFunc("POST /v1/models/", geminiModelDispatch)
	mux.HandleFunc("POST /v1beta/models/", geminiModelDispatch)

	// Token refresh endpoint
	mux.HandleFunc("POST /oauth2/token", handleTokenRefresh)

	// Management endpoints
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/config", handleConfig)
	mux.HandleFunc("GET /presets", handlePresets)

	// Middleware for logging
	return logMiddleware(mux)
}

func geminiModelDispatch(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, ":generateContent") {
		handleGenerateContent(w, r)
	} else if strings.HasSuffix(path, ":streamGenerateContent") {
		handleStreamGenerateContent(w, r)
	} else {
		http.NotFound(w, r)
	}
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip logging for health checks
		if strings.HasPrefix(r.URL.Path, "/health") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)

		fmt.Printf("[%s] %s %s %d %s\n",
			time.Now().Format("15:04:05"),
			r.Method, r.URL.Path, rw.statusCode, duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Ensure responseWriter also implements http.Flusher
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
