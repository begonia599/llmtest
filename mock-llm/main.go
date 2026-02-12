package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

func main() {
	port := flag.Int("port", 8081, "Server port")
	flag.Parse()

	router := newRouter()

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Mock LLM server starting on %s\n", addr)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  POST /v1/models/{model}:generateContent\n")
	fmt.Printf("  POST /v1/models/{model}:streamGenerateContent\n")
	fmt.Printf("  POST /oauth2/token\n")
	fmt.Printf("  GET  /health\n")
	fmt.Printf("  GET  /presets\n")
	fmt.Printf("  GET/POST /config\n")

	if err := http.ListenAndServe(addr, router); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
