package provider

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EmbedRequest represents a chunk to embed
type EmbedRequest struct {
	ID      int
	Content string
}

// EmbedResult represents an embedding result for a single chunk
type EmbedResult struct {
	ID        int
	Embedding []float32
	Error     error
}

// BatchEmbedResult contains results from a batch embedding operation
type BatchEmbedResult struct {
	Results []EmbedResult
	Errors  int
}

// LLMRequest represents a request to an LLM
type LLMRequest struct {
	Prompt       string
	SystemPrompt string
	MaxTokens    int
	Temperature  float32
}

// LLMResult represents the response from an LLM
type LLMResult struct {
	Text         string
	InputTokens  int
	OutputTokens int
}

// LLMError represents an error from an LLM call
type LLMError struct {
	Message    string
	StatusCode int
}

// Error implements the error interface, so callers can wrap LLM failures with %w
// and recover the status code with errors.As — e.g. to distinguish a 429 that is
// worth retrying from a 401 that never will be.
func (e *LLMError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("llm request failed (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("llm request failed: %s", e.Message)
}

var _ error = (*LLMError)(nil)

// Float32SliceToBytes converts a float32 slice to little-endian bytes.
// This is used to store embeddings as BLOBs in SQLite.
func Float32SliceToBytes(floats []float32) []byte {
	buf := make([]byte, len(floats)*4)
	for i, f := range floats {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// BytesToFloat32Slice converts little-endian bytes to a float32 slice.
// This is used to retrieve embeddings from SQLite BLOBs.
func BytesToFloat32Slice(b []byte) []float32 {
	floats := make([]float32, len(b)/4)
	for i := range floats {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return floats
}
