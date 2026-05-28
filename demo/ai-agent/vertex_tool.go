package main

import (
	"context"
	"fmt"

	discoveryengine "cloud.google.com/go/discoveryengine/apiv1"
	discoveryenginepb "cloud.google.com/go/discoveryengine/apiv1/discoveryenginepb"
	"github.com/tmc/langchaingo/callbacks"
	"google.golang.org/api/iterator"
)

type VertexAISearchTool struct {
	ProjectID  string
	Location   string
	DataStore  string
	CallbacksHandler callbacks.Handler
}

func (t *VertexAISearchTool) Description() string {
	return "Useful for searching company data and documents using Google Cloud Vertex AI Search."
}

func (t *VertexAISearchTool) Name() string {
	return "vertex_ai_search"
}

func (t *VertexAISearchTool) Call(ctx context.Context, input string) (string, error) {
	if t.CallbacksHandler != nil {
		t.CallbacksHandler.HandleToolStart(ctx, input)
	}

	client, err := discoveryengine.NewSearchClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create discoveryengine client: %w", err)
	}
	defer client.Close()

	req := &discoveryenginepb.SearchRequest{
		ServingConfig: fmt.Sprintf("projects/%s/locations/%s/collections/default_collection/dataStores/%s/servingConfigs/default_search", t.ProjectID, t.Location, t.DataStore),
		Query:         input,
		PageSize:      3,
	}

	it := client.Search(ctx, req)
	var results string
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error during search: %w", err)
		}

		results += fmt.Sprintf("Title: %s\nSnippet: %s\n\n", resp.Document.Name, resp.Document.Id) // Simplified for the tool
	}
	
	if results == "" {
		results = "No results found."
	}

	if t.CallbacksHandler != nil {
		t.CallbacksHandler.HandleToolEnd(ctx, results)
	}

	return results, nil
}
