package converter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OpenAI request/response types
type OpenAIRequest struct {
	Model            string           `json:"model"`
	Messages         []OpenAIMessage  `json:"messages"`
	Temperature      *float64         `json:"temperature,omitempty"`
	TopP             *float64         `json:"top_p,omitempty"`
	MaxTokens        *int             `json:"max_tokens,omitempty"`
	Stop             []string         `json:"stop,omitempty"`
	Stream           bool             `json:"stream"`
	Tools            []OpenAITool     `json:"tools,omitempty"`
	ToolChoice       any              `json:"tool_choice,omitempty"`
}

type OpenAIMessage struct {
	Role       string          `json:"role"`
	Content    any             `json:"content"` // string or []ContentPart
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type OpenAIToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function OpenAIFunctionCall  `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []OpenAIChoice  `json:"choices"`
	Usage   *OpenAIUsage    `json:"usage,omitempty"`
}

type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      *OpenAIMessage `json:"message,omitempty"`
	Delta        *OpenAIMessage `json:"delta,omitempty"`
	FinishReason *string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Gemini request/response types
type GeminiRequest struct {
	Contents         []GeminiContent        `json:"contents"`
	GenerationConfig map[string]any         `json:"generationConfig,omitempty"`
	Tools            []GeminiToolDef        `json:"tools,omitempty"`
	ToolConfig       *GeminiToolConfig      `json:"toolConfig,omitempty"`
	SystemInstruction *GeminiContent        `json:"systemInstruction,omitempty"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type GeminiPart struct {
	Text         string                 `json:"text,omitempty"`
	FunctionCall *GeminiFunctionCall    `json:"functionCall,omitempty"`
	FunctionResp *GeminiFunctionResp    `json:"functionResponse,omitempty"`
}

type GeminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type GeminiFunctionResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type GeminiToolDef struct {
	FunctionDeclarations []GeminiFuncDecl `json:"functionDeclarations,omitempty"`
}

type GeminiFuncDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type GeminiToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type FunctionCallingConfig struct {
	Mode string `json:"mode"`
}

type GeminiResponse struct {
	Candidates    []GeminiCandidate `json:"candidates"`
	UsageMetadata *GeminiUsage      `json:"usageMetadata,omitempty"`
}

type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index"`
}

type GeminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// OpenAIToGemini converts an OpenAI chat completion request to Gemini format.
func OpenAIToGemini(req *OpenAIRequest) (*GeminiRequest, error) {
	gemReq := &GeminiRequest{}

	// Convert messages to contents
	var contents []GeminiContent
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			text := ExtractTextContent(msg.Content)
			gemReq.SystemInstruction = &GeminiContent{
				Parts: []GeminiPart{{Text: text}},
				Role:  "user",
			}
		case "user":
			text := ExtractTextContent(msg.Content)
			contents = append(contents, GeminiContent{
				Parts: []GeminiPart{{Text: text}},
				Role:  "user",
			})
		case "assistant":
			parts := []GeminiPart{}
			text := ExtractTextContent(msg.Content)
			if text != "" {
				parts = append(parts, GeminiPart{Text: text})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
				parts = append(parts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				})
			}
			if len(parts) > 0 {
				contents = append(contents, GeminiContent{
					Parts: parts,
					Role:  "model",
				})
			}
		case "tool":
			var respData map[string]any
			text := ExtractTextContent(msg.Content)
			if err := json.Unmarshal([]byte(text), &respData); err != nil {
				respData = map[string]any{"result": text}
			}
			contents = append(contents, GeminiContent{
				Parts: []GeminiPart{{
					FunctionResp: &GeminiFunctionResp{
						Name:     msg.Name,
						Response: respData,
					},
				}},
				Role: "user",
			})
		}
	}
	gemReq.Contents = contents

	// Convert generation config
	genConfig := map[string]any{}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		genConfig["topP"] = *req.TopP
	}
	if req.MaxTokens != nil {
		genConfig["maxOutputTokens"] = *req.MaxTokens
	}
	if len(req.Stop) > 0 {
		genConfig["stopSequences"] = req.Stop
	}
	if len(genConfig) > 0 {
		gemReq.GenerationConfig = genConfig
	}

	// Convert tools
	if len(req.Tools) > 0 {
		var decls []GeminiFuncDecl
		for _, tool := range req.Tools {
			params := CleanSchemaForGemini(tool.Function.Parameters)
			decls = append(decls, GeminiFuncDecl{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  params,
			})
		}
		gemReq.Tools = []GeminiToolDef{{FunctionDeclarations: decls}}
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		mode := "AUTO"
		switch v := req.ToolChoice.(type) {
		case string:
			switch v {
			case "auto":
				mode = "AUTO"
			case "none":
				mode = "NONE"
			case "required":
				mode = "ANY"
			}
		}
		gemReq.ToolConfig = &GeminiToolConfig{
			FunctionCallingConfig: &FunctionCallingConfig{Mode: mode},
		}
	}

	return gemReq, nil
}

// GeminiToOpenAI converts a Gemini response to OpenAI format.
func GeminiToOpenAI(gemResp *GeminiResponse, model string, reqID string) *OpenAIResponse {
	resp := &OpenAIResponse{
		ID:      reqID,
		Object:  "chat.completion",
		Created: 0, // Will be set by caller
		Model:   model,
	}

	for _, cand := range gemResp.Candidates {
		choice := OpenAIChoice{
			Index: cand.Index,
		}

		msg := &OpenAIMessage{Role: "assistant"}
		var textParts []string

		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				msg.ToolCalls = append(msg.ToolCalls, OpenAIToolCall{
					ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}

		if len(textParts) > 0 {
			combined := strings.Join(textParts, "")
			msg.Content = combined
		}

		choice.Message = msg

		if cand.FinishReason != "" {
			fr := mapFinishReason(cand.FinishReason)
			choice.FinishReason = &fr
		}

		resp.Choices = append(resp.Choices, choice)
	}

	if gemResp.UsageMetadata != nil {
		resp.Usage = &OpenAIUsage{
			PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gemResp.UsageMetadata.TotalTokenCount,
		}
	}

	return resp
}

// GeminiChunkToOpenAIChunk converts a single Gemini streaming chunk to OpenAI SSE format.
func GeminiChunkToOpenAIChunk(gemResp *GeminiResponse, model string, reqID string) *OpenAIResponse {
	resp := &OpenAIResponse{
		ID:      reqID,
		Object:  "chat.completion.chunk",
		Created: 0,
		Model:   model,
	}

	for _, cand := range gemResp.Candidates {
		choice := OpenAIChoice{
			Index: cand.Index,
		}

		delta := &OpenAIMessage{Role: "assistant"}
		var textParts []string

		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				delta.ToolCalls = append(delta.ToolCalls, OpenAIToolCall{
					ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}

		if len(textParts) > 0 {
			combined := strings.Join(textParts, "")
			delta.Content = combined
		}

		choice.Delta = delta

		if cand.FinishReason != "" {
			fr := mapFinishReason(cand.FinishReason)
			choice.FinishReason = &fr
		}

		resp.Choices = append(resp.Choices, choice)
	}

	if gemResp.UsageMetadata != nil {
		resp.Usage = &OpenAIUsage{
			PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gemResp.UsageMetadata.TotalTokenCount,
		}
	}

	return resp
}

func mapFinishReason(geminiReason string) string {
	switch geminiReason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	case "RECITATION":
		return "content_filter"
	default:
		return "stop"
	}
}

func ExtractTextContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		// Handle []ContentPart format
		data, _ := json.Marshal(v)
		var parts []map[string]any
		if err := json.Unmarshal(data, &parts); err == nil {
			var texts []string
			for _, p := range parts {
				if t, ok := p["text"].(string); ok {
					texts = append(texts, t)
				}
			}
			return strings.Join(texts, "")
		}
		return fmt.Sprintf("%v", v)
	}
}
