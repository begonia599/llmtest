package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"gateway-go/converter"
	"gateway-go/credential"
	"gateway-go/proxy"
	"gateway-go/token"
)

func main() {
	port := flag.Int("port", 8080, "Gateway port")
	upstreamURL := flag.String("upstream", "http://localhost:8081", "Upstream LLM URL")
	credCount := flag.Int("creds", 20, "Number of mock credentials")
	flag.Parse()

	refreshURL := *upstreamURL + "/oauth2/token"
	credManager := credential.NewManager(*credCount, refreshURL)
	tokenStats := token.NewStats()
	proxyHandler := proxy.NewProxy(*upstreamURL, credManager, tokenStats)

	mux := http.NewServeMux()

	// OpenAI-compatible chat completions
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req converter.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}

		reqID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

		if req.Stream {
			proxyHandler.HandleStreaming(w, &req, reqID)
		} else {
			proxyHandler.HandleNonStreaming(w, &req, reqID)
		}
	})

	// Model list
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		models := []map[string]any{
			{"id": "gemini-2.0-flash", "object": "model", "owned_by": "google"},
			{"id": "gemini-1.5-pro", "object": "model", "owned_by": "google"},
			{"id": "gemini-2.0-flash-thinking", "object": "model", "owned_by": "google"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": models})
	})

	// Metrics endpoint
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tokens":      tokenStats.GetSummary(),
			"credentials": credManager.GetStats(),
		})
	})

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "gateway": "go"})
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Go LLM Gateway starting on %s\n", addr)
	fmt.Printf("Upstream: %s\n", *upstreamURL)
	fmt.Printf("Credentials: %d\n", *credCount)

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
