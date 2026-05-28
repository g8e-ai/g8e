package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/tools"
)

func main() {
	ctx := context.Background()
	var llm llms.Model
	var err error

	// Use Ollama if OLLAMA_ENDPOINT is provided, otherwise default to Gemini
	ollamaURL := os.Getenv("OLLAMA_ENDPOINT")
	if ollamaURL != "" {
		ollamaModel := os.Getenv("OLLAMA_MODEL")
		if ollamaModel == "" {
			ollamaModel = "llama3"
		}
		fmt.Printf("Initializing Ollama LLM (model: %s)...\n", ollamaModel)
		llm, err = ollama.New(
			ollama.WithModel(ollamaModel),
			ollama.WithServerURL(ollamaURL),
		)
		if err != nil {
			log.Fatalf("Failed to initialize Ollama: %v", err)
		}
	} else {
		// Default: Google Gemini
		if os.Getenv("GEMINI_API_KEY") == "" {
			fmt.Println("Please set GEMINI_API_KEY environment variable to use the default Gemini model.")
			fmt.Println("Alternatively, set OLLAMA_ENDPOINT to use an Ollama endpoint.")
			os.Exit(1)
		}

		geminiModel := os.Getenv("GEMINI_MODEL")
		if geminiModel == "" {
			geminiModel = "gemini-1.5-flash"
		}
		fmt.Printf("Initializing Google Gemini LLM (model: %s)...\n", geminiModel)
		llm, err = googleai.New(
			ctx,
			googleai.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
			googleai.WithDefaultModel(geminiModel),
		)
		if err != nil {
			log.Fatalf("Failed to initialize Google Gemini: %v", err)
		}
	}

	// 2. Set up the tools the agent can use
	calculator := tools.Calculator{}

	// Initialize the custom Vertex AI Search tool
	// You will need GOOGLE_APPLICATION_CREDENTIALS set for this to authenticate
	vertexProject := os.Getenv("VERTEX_PROJECT_ID")
	vertexLocation := os.Getenv("VERTEX_LOCATION")
	vertexDataStore := os.Getenv("VERTEX_DATA_STORE")

	var agentTools []tools.Tool
	agentTools = append(agentTools, calculator)

	if vertexProject != "" && vertexLocation != "" && vertexDataStore != "" {
		fmt.Println("Adding Vertex AI Search tool...")
		vertexTool := &VertexAISearchTool{
			ProjectID: vertexProject,
			Location:  vertexLocation,
			DataStore: vertexDataStore,
		}
		agentTools = append(agentTools, vertexTool)
	} else {
		fmt.Println("Vertex AI Search credentials not fully set (VERTEX_PROJECT_ID, VERTEX_LOCATION, VERTEX_DATA_STORE).")
		fmt.Println("Continuing with calculator tool only.")
	}

	// 3. Create a Conversational React agent (works well across different LLM providers)
	agent := agents.NewConversationalAgent(
		llm,
		agentTools,
	)

	// 4. Create an executor to run the agent loop
	executor := agents.NewExecutor(
		agent,
		agents.WithMaxIterations(5),
	)

	// 5. Run the agent with a prompt
	question := "Search for the latest internal guidelines on employee benefits. If you can't search, what is 2314 * 123?"
	fmt.Printf("\nQuestion: %s\n\n", question)
	fmt.Println("Agent is thinking (this may take a moment)...")

	result, err := chains.Run(ctx, executor, question)
	if err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}

	fmt.Printf("\nAnswer: %s\n", result)
}
