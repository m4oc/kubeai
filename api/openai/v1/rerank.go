package v1

import (
	"github.com/go-json-experiment/json/jsontext"
)

// RerankRequest represents a request to rerank a set of documents given a query.
type RerankRequest struct {
	// Query is the search query used to score documents.
	Query string `json:"query"`
	// Documents is the list of documents to score.
	Documents []string `json:"documents,omitempty"`
	// Model is the ID of the model to use.
	Model string `json:"model"`
	// TopN limits the number of results returned.
	TopN int `json:"top_n,omitzero"`

	// Unknown fields should be preserved to fully support the extended set of fields that backends may support.
	Unknown jsontext.Value `json:",unknown"`
}

func (r *RerankRequest) GetModel() string {
	return r.Model
}

func (r *RerankRequest) SetModel(m string) {
	r.Model = m
}

// Prefix returns the first n characters of the query for load balancing strategies that require it.
func (r *RerankRequest) Prefix(n int) string {
	return firstNChars(r.Query, n)
}

// RerankResult represents a single reranked document.
type RerankResult struct {
	// Index of the document in the original request.
	Index int `json:"index"`
	// Document text as returned by the backend.
	Document string `json:"document,omitempty"`
	// RelevanceScore indicates the model's confidence that the document matches the query.
	RelevanceScore float32 `json:"relevance_score"`
}

// RerankResponse is returned from a rerank request.
type RerankResponse struct {
	Object string         `json:"object"`
	Model  string         `json:"model"`
	Data   []RerankResult `json:"data"`

	// Unknown fields should be preserved to fully support the extended set of fields that backends may support.
	Unknown jsontext.Value `json:",unknown"`
}
