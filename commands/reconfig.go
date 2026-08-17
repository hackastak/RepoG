package commands

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/github"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/anthropic"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	_ "github.com/hackastak/repog/internal/provider/ollama"
	_ "github.com/hackastak/repog/internal/provider/openai"
	_ "github.com/hackastak/repog/internal/provider/openrouter"
	_ "github.com/hackastak/repog/internal/provider/voyageai"
	"github.com/hackastak/repog/internal/sync"
)

var reconfigCmd = &cobra.Command{
	Use:   "reconfig [github|embedding|generation]",
	Short: "Reconfigure RepoG settings",
	Long: `Reconfigure RepoG settings. Use without arguments to reconfigure all settings.

Examples:
  repog reconfig           # Reconfigure all settings
  repog reconfig github    # Update GitHub Personal Access Token only
  repog reconfig embedding # Reconfigure embedding provider
  repog reconfig generation # Reconfigure generation provider`,
	RunE: runReconfig,
}

var (
	reconfigProvider    string
	reconfigModel       string
	reconfigDimensions  int
	reconfigMaxTokens   int
	reconfigBaseURL     string
	reconfigFallback    string
	reconfigAPIKey      string
	reconfigGitHubToken string
)

func init() {
	reconfigCmd.Flags().StringVar(&reconfigProvider, "provider", "", "Provider name (gemini, openai, openrouter, voyageai, anthropic, or ollama)")
	reconfigCmd.Flags().StringVar(&reconfigModel, "model", "", "Model name")
	reconfigCmd.Flags().IntVar(&reconfigDimensions, "dimensions", 0, "Embedding dimensions (embedding only)")
	reconfigCmd.Flags().IntVar(&reconfigMaxTokens, "max-tokens", 0, "Max token limit per chunk (embedding only, 0 = use model default)")
	reconfigCmd.Flags().StringVar(&reconfigBaseURL, "base-url", "", "Custom base URL (ollama only)")
	reconfigCmd.Flags().StringVar(&reconfigFallback, "fallback", "", "Fallback model (generation only)")
	reconfigCmd.Flags().StringVar(&reconfigAPIKey, "api-key", "", "API key for the provider")
	reconfigCmd.Flags().StringVar(&reconfigGitHubToken, "github-token", "", "GitHub Personal Access Token (for github reconfiguration)")
}

func runReconfig(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	// Load current config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Run `repog init` first."))
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println(bold("RepoG Reconfiguration"))
	fmt.Println()

	// Determine what to reconfigure
	var reconfigureEmbedding, reconfigureGeneration, reconfigureGitHub bool

	if len(args) == 0 {
		// No arguments - reconfigure everything
		reconfigureEmbedding = true
		reconfigureGeneration = true
		fmt.Println(dim("Reconfiguring all settings. Current configuration will be prefilled."))
		fmt.Println()
	} else {
		switch args[0] {
		case "github":
			reconfigureGitHub = true
		case "embedding":
			reconfigureEmbedding = true
		case "generation":
			reconfigureGeneration = true
		default:
			fmt.Fprintln(os.Stderr, red("Unknown target:", args[0]))
			fmt.Fprintln(os.Stderr, "Usage: repog reconfig [github|embedding|generation]")
			os.Exit(1)
		}
	}

	// Handle GitHub PAT reconfiguration separately (doesn't need full config)
	if reconfigureGitHub {
		return runReconfigGitHub(commandContext(cmd), reconfigGitHubToken, green, red, dim, bold)
	}

	// Store original embedding config to detect changes
	originalEmbedding := cfg.Embedding

	// Track embedding provider and API key for potential reuse in generation config
	var currentEmbedProvider string
	var currentEmbedAPIKey string

	// Reconfigure embedding
	if reconfigureEmbedding {
		newEmbedCfg, newEmbedAPIKey := reconfigureEmbeddingProvider(commandContext(cmd), cfg.Embedding, reconfigProvider, reconfigModel, reconfigDimensions, reconfigMaxTokens, reconfigBaseURL, reconfigAPIKey, red, dim, green, yellow)
		currentEmbedProvider = newEmbedCfg.Provider
		currentEmbedAPIKey = newEmbedAPIKey

		// Calculate chunk sizes to detect if they changed
		var oldChunkSize, newChunkSize int

		// Get old provider's token limit (uses config MaxTokens override if set)
		oldAPIKey, _ := config.GetAPIKeyForProvider(originalEmbedding.Provider)
		if oldProvider, err := provider.NewEmbeddingProvider(originalEmbedding, oldAPIKey); err == nil {
			oldChunkSize = sync.CalculateMaxCharsFromTokens(oldProvider.MaxTokens())
		}

		// Get new provider's token limit (uses config MaxTokens override if set)
		if newProvider, err := provider.NewEmbeddingProvider(newEmbedCfg, newEmbedAPIKey); err == nil {
			newChunkSize = sync.CalculateMaxCharsFromTokens(newProvider.MaxTokens())
		}

		chunkSizeChanged := oldChunkSize != newChunkSize && oldChunkSize > 0 && newChunkSize > 0

		// Check if embedding config changed (provider, model, or dimensions)
		embeddingChanged := originalEmbedding.Provider != newEmbedCfg.Provider ||
			originalEmbedding.Model != newEmbedCfg.Model ||
			originalEmbedding.Dimensions != newEmbedCfg.Dimensions

		// Handle different change scenarios
		if embeddingChanged || chunkSizeChanged {
			fmt.Println()

			if embeddingChanged {
				fmt.Println(yellow("⚠️  Warning: Embedding configuration has changed"))
				fmt.Println()
				fmt.Println("  Previous:", fmt.Sprintf("%s (%s, %dd)", originalEmbedding.Provider, originalEmbedding.Model, originalEmbedding.Dimensions))
				fmt.Println("  New:     ", fmt.Sprintf("%s (%s, %dd)", newEmbedCfg.Provider, newEmbedCfg.Model, newEmbedCfg.Dimensions))
				fmt.Println()
			}

			if chunkSizeChanged {
				fmt.Println(yellow("  ⚠️  Chunk size will change:"))
				fmt.Printf("     Previous: %d characters (from %d tokens)\n", oldChunkSize, originalEmbedding.MaxTokens)
				fmt.Printf("     New:      %d characters (from %d tokens)\n", newChunkSize, newEmbedCfg.MaxTokens)
				fmt.Println()
			}

			if embeddingChanged && chunkSizeChanged {
				fmt.Println(yellow("  This will delete ALL existing embeddings AND chunks."))
				fmt.Println(dim("  You'll need to run:"))
				fmt.Println(dim("    1. repog sync  (to re-chunk with new size)"))
				fmt.Println(dim("    2. repog embed                   (to generate new embeddings)"))
			} else if embeddingChanged {
				fmt.Println(yellow("  This will delete ALL existing embeddings."))
				fmt.Println(dim("  You'll need to run `repog embed` to regenerate them."))
			} else if chunkSizeChanged {
				fmt.Println(yellow("  This will delete ALL existing chunks (max token limit changed)."))
				fmt.Println(dim("  You'll need to run:"))
				fmt.Println(dim("    1. repog sync  (to re-chunk with new size)"))
				fmt.Println(dim("    2. repog embed                   (to generate new embeddings)"))
			}
			fmt.Println()

			var confirmed bool
			prompt := &survey.Confirm{
				Message: "Continue with reconfiguration?",
				Default: false,
			}
			if err := survey.AskOne(prompt, &confirmed); err != nil || !confirmed {
				fmt.Println(dim("Reconfiguration cancelled."))
				return nil
			}

			// Open database
			database, err := db.Open(cfg.DBPath, originalEmbedding.Dimensions)
			if err != nil {
				fmt.Fprintln(os.Stderr, red("Failed to open database:", err))
				os.Exit(1)
			}

			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)

			// Clear embeddings if embedding config changed
			if embeddingChanged {
				s.Suffix = " Clearing embeddings..."
				s.Start()

				if err := clearEmbeddings(database, newEmbedCfg.Dimensions); err != nil {
					s.Stop()
					fmt.Fprintln(os.Stderr, red("Failed to clear embeddings:", err))
					_ = database.Close()
					os.Exit(1)
				}

				s.Stop()
				fmt.Println(green("✓"), "Embeddings cleared")
			}

			// Clear chunks if chunk size changed
			if chunkSizeChanged {
				s.Suffix = " Clearing chunks..."
				s.Start()

				if err := clearChunks(database); err != nil {
					s.Stop()
					fmt.Fprintln(os.Stderr, red("Failed to clear chunks:", err))
					_ = database.Close()
					os.Exit(1)
				}

				s.Stop()
				fmt.Println(green("✓"), "Chunks cleared")
			}

			_ = database.Close()
		}

		// Update config
		cfg.Embedding = newEmbedCfg

		// Store API key
		if err := config.SetAPIKeyForProvider(newEmbedCfg.Provider, newEmbedAPIKey); err != nil {
			fmt.Fprintln(os.Stderr, red("Failed to save embedding API key:", err))
			os.Exit(1)
		}
	}

	// Reconfigure generation
	if reconfigureGeneration {
		// If embedding wasn't reconfigured, get current embedding info from config
		if currentEmbedProvider == "" {
			currentEmbedProvider = cfg.Embedding.Provider
			currentEmbedAPIKey, _ = config.GetAPIKeyForProvider(cfg.Embedding.Provider)
		}
		newGenCfg, newGenAPIKey := reconfigureGenerationProvider(commandContext(cmd), cfg.Generation, reconfigProvider, reconfigModel, reconfigFallback, reconfigBaseURL, reconfigAPIKey, currentEmbedProvider, currentEmbedAPIKey, red, dim, green)

		// Update config
		cfg.Generation = newGenCfg

		// Store API key
		if err := config.SetAPIKeyForProvider(newGenCfg.Provider, newGenAPIKey); err != nil {
			fmt.Fprintln(os.Stderr, red("Failed to save generation API key:", err))
			os.Exit(1)
		}
	}

	// Save config file
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Saving configuration..."
	s.Start()

	if err := config.SaveConfigFile(cfg); err != nil {
		s.Stop()
		fmt.Fprintln(os.Stderr, red("Failed to save config:", err))
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), "Configuration saved")

	// Summary
	fmt.Println()
	fmt.Println(bold(green("Reconfiguration complete!")))
	fmt.Println()
	if reconfigureEmbedding {
		// Get effective max tokens for display
		var maxTokensLabel string
		if cfg.Embedding.MaxTokens > 0 {
			maxTokensLabel = fmt.Sprintf("%d tokens (custom)", cfg.Embedding.MaxTokens)
		} else {
			// Get model default
			apiKey, _ := config.GetAPIKeyForProvider(cfg.Embedding.Provider)
			if defaultMaxTokens, err := provider.GetModelDefaultMaxTokens(cfg.Embedding, apiKey); err == nil {
				maxTokensLabel = fmt.Sprintf("%d tokens", defaultMaxTokens)
			} else {
				maxTokensLabel = "default"
			}
		}
		fmt.Println("Embedding:", cfg.Embedding.Provider, fmt.Sprintf("(%s, %dd, %s)", cfg.Embedding.Model, cfg.Embedding.Dimensions, maxTokensLabel))
	}
	if reconfigureGeneration {
		fmt.Println("Generation:", cfg.Generation.Provider, fmt.Sprintf("(%s)", cfg.Generation.Model))
	}
	fmt.Println()

	return nil
}

// runReconfigGitHub handles GitHub PAT reconfiguration
func runReconfigGitHub(ctx context.Context, tokenFlag string, green, red, dim, bold func(...interface{}) string) error {
	fmt.Println(bold("GitHub Token Update"))
	fmt.Println()

	// Get current PAT info if available
	currentPAT, err := config.GetGitHubPAT()
	if err == nil && currentPAT != "" {
		fmt.Println(dim("Current token: " + maskSecret(currentPAT)))
		fmt.Println()
	}

	// Get new token
	githubToken := tokenFlag
	if githubToken == "" {
		fmt.Println(dim("Create a new token at: https://github.com/settings/personal-access-tokens/new"))
		fmt.Println()
		fmt.Println(dim("Required settings:"))
		fmt.Println(dim("  Resource owner:     Your GitHub account"))
		fmt.Println(dim("  Repository access:  All repositories (or select specific repos)"))
		fmt.Println(dim("  Permissions:"))
		fmt.Println(dim("    Contents:         Read-only"))
		fmt.Println(dim("    Metadata:         Read-only"))
		fmt.Println()

		prompt := &survey.Password{
			Message: "New GitHub Personal Access Token:",
		}
		if err := survey.AskOne(prompt, &githubToken, survey.WithValidator(survey.Required)); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Validate the new token
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Validating GitHub token..."
	s.Start()

	client := github.NewClient(githubToken)
	patResult := github.ValidatePAT(ctx, client)
	s.Stop()

	if !patResult.Valid {
		fmt.Println(red("✗"), "GitHub token invalid:", patResult.Error)
		os.Exit(1)
	}

	fmt.Println(green("✓"), "Token validated - logged in as @"+patResult.Login)

	// Save the new token
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Saving token..."
	s.Start()

	if err := config.SetAPIKeyForProvider("github", githubToken); err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to save token:", err)
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), "GitHub token updated")
	fmt.Println()

	return nil
}

// reconfigureEmbeddingProvider handles embedding provider reconfiguration
func reconfigureEmbeddingProvider(ctx context.Context, current config.ProviderConfig, providerFlag, modelFlag string, dimensionsFlag, maxTokensFlag int, baseURLFlag, apiKeyFlag string, red, dim, green, yellow func(...interface{}) string) (config.ProviderConfig, string) {
	var selectedProvider string
	var apiKey string
	var model string
	var dimensions int
	var maxTokens int
	var baseURL string

	// Select provider (prefill with current)
	if providerFlag != "" {
		selectedProvider = providerFlag
	} else {
		fmt.Println()
		fmt.Println(dim("Select an embedding provider:"))
		options := []string{"gemini", "openai", "openrouter", "voyageai", "ollama"}
		prompt := &survey.Select{
			Message: "Embedding Provider:",
			Options: options,
			Default: current.Provider,
		}
		if err := survey.AskOne(prompt, &selectedProvider); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get model
	if modelFlag != "" {
		model = modelFlag
	} else {
		// Determine default based on provider
		var defaultModel string
		if selectedProvider == current.Provider {
			defaultModel = current.Model
		} else {
			switch selectedProvider {
			case "gemini":
				defaultModel = "gemini-embedding-2-preview"
			case "openai":
				defaultModel = "text-embedding-3-small"
			case "openrouter":
				defaultModel = "openai/text-embedding-3-small"
			case "voyageai":
				defaultModel = "voyage-code-3"
			case "ollama":
				defaultModel = "nomic-embed-text"
			}
		}

		fmt.Println()
		prompt := &survey.Input{
			Message: "Model:",
			Default: defaultModel,
		}
		if err := survey.AskOne(prompt, &model); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get dimensions
	if dimensionsFlag != 0 {
		dimensions = dimensionsFlag
	} else {
		// Determine default based on provider
		var defaultDimensions int
		if selectedProvider == current.Provider {
			defaultDimensions = current.Dimensions
		} else {
			switch selectedProvider {
			case "gemini":
				defaultDimensions = 768
			case "openai", "openrouter":
				defaultDimensions = 1536
			case "voyageai":
				defaultDimensions = 1024
			case "ollama":
				defaultDimensions = 768
			}
		}

		fmt.Println()
		var dimStr string
		prompt := &survey.Input{
			Message: "Dimensions:",
			Default: fmt.Sprintf("%d", defaultDimensions),
		}
		if err := survey.AskOne(prompt, &dimStr); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
		_, _ = fmt.Sscanf(dimStr, "%d", &dimensions)
	}

	// Get base URL for Ollama
	if selectedProvider == "ollama" {
		if baseURLFlag != "" {
			baseURL = baseURLFlag
		} else {
			fmt.Println()
			defaultURL := "http://localhost:11434"
			if current.Provider == "ollama" && current.BaseURL != "" {
				defaultURL = current.BaseURL
			}
			prompt := &survey.Input{
				Message: "Ollama Base URL:",
				Default: defaultURL,
			}
			if err := survey.AskOne(prompt, &baseURL); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Get API key
	if selectedProvider == "ollama" {
		apiKey = "" // Ollama doesn't need an API key
	} else if apiKeyFlag != "" {
		apiKey = apiKeyFlag
	} else {
		// Try to get existing key from keyring
		existingKey, err := config.GetAPIKeyForProvider(selectedProvider)
		if err == nil && existingKey != "" {
			fmt.Println()
			var reuseKey bool
			prompt := &survey.Confirm{
				Message: fmt.Sprintf("Use existing %s API key?", selectedProvider),
				Default: true,
			}
			if err := survey.AskOne(prompt, &reuseKey); err == nil && reuseKey {
				apiKey = existingKey
			}
		}

		if apiKey == "" {
			fmt.Println()
			switch selectedProvider {
			case "gemini":
				fmt.Println(dim("Get a Gemini API key at: https://aistudio.google.com/apikey"))
			case "openai":
				fmt.Println(dim("Get an OpenAI API key at: https://platform.openai.com/api-keys"))
			case "openrouter":
				fmt.Println(dim("Get an OpenRouter API key at: https://openrouter.ai/keys"))
			case "voyageai":
				fmt.Println(dim("Get a Voyage AI API key at: https://dash.voyageai.com"))
			case "anthropic":
				fmt.Println(dim("Get an Anthropic API key at: https://console.anthropic.com"))
			}
			fmt.Println()

			prompt := &survey.Password{
				Message: fmt.Sprintf("%s API Key:", selectedProvider),
			}
			if err := survey.AskOne(prompt, &apiKey, survey.WithValidator(survey.Required)); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Get max tokens - now that we have the API key, we can get model defaults
	tempCfg := config.ProviderConfig{
		Provider:   selectedProvider,
		Model:      model,
		Dimensions: dimensions,
		BaseURL:    baseURL,
	}

	// Get model's default max tokens
	var defaultMaxTokens int
	if modelDefault, err := provider.GetModelDefaultMaxTokens(tempCfg, apiKey); err == nil {
		defaultMaxTokens = modelDefault
	} else {
		// Fallback defaults if we can't determine
		defaultMaxTokens = 2048
	}

	// Determine the current max tokens setting
	var currentMaxTokens int
	if current.MaxTokens > 0 {
		currentMaxTokens = current.MaxTokens
	} else {
		currentMaxTokens = defaultMaxTokens
	}

	if maxTokensFlag != 0 {
		maxTokens = maxTokensFlag
	} else {
		fmt.Println()
		var customizeTokens bool
		tokenPrompt := &survey.Confirm{
			Message: fmt.Sprintf("Customize max token limit? (current: %d, model default: %d)", currentMaxTokens, defaultMaxTokens),
			Default: false,
		}
		if err := survey.AskOne(tokenPrompt, &customizeTokens); err != nil {
			// User cancelled, keep current
			customizeTokens = false
		}

		if customizeTokens {
			fmt.Println()
			fmt.Println(dim("Max token limit controls how text is chunked for embedding."))
			fmt.Println()
			fmt.Println(yellow("⚠️  Warning:"))
			fmt.Println(yellow("   • Too high: May cause embedding API errors if chunks exceed model limits"))
			fmt.Println(yellow("   • Too low:  May reduce search accuracy due to excessive chunking"))
			fmt.Println()

			var maxTokensStr string
			inputPrompt := &survey.Input{
				Message: "Max tokens per chunk:",
				Default: fmt.Sprintf("%d", currentMaxTokens),
			}
			if err := survey.AskOne(inputPrompt, &maxTokensStr); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}

			_, _ = fmt.Sscanf(maxTokensStr, "%d", &maxTokens)

			// Show additional warning if significantly different from default
			if maxTokens > defaultMaxTokens*2 {
				fmt.Println()
				fmt.Println(yellow("⚠️  Token limit is much higher than model default - embedding errors may occur"))
			} else if maxTokens < defaultMaxTokens/4 && maxTokens > 0 {
				fmt.Println()
				fmt.Println(yellow("⚠️  Token limit is much lower than model default - search accuracy may be reduced"))
			}
		} else {
			// Keep the current setting
			maxTokens = current.MaxTokens
		}
	}

	// Build config
	cfg := config.ProviderConfig{
		Provider:   selectedProvider,
		Model:      model,
		Dimensions: dimensions,
		MaxTokens:  maxTokens,
		BaseURL:    baseURL,
	}

	// Validate provider
	fmt.Println()
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Validating %s embedding...", selectedProvider)
	s.Start()

	embedProvider, err := provider.NewEmbeddingProvider(cfg, apiKey)
	if err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to create embedding provider:", err)
		os.Exit(1)
	}

	if err := embedProvider.Validate(ctx); err != nil {
		s.Stop()
		fmt.Println(red("✗"), fmt.Sprintf("%s embedding validation failed:", selectedProvider), err)
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), fmt.Sprintf("%s embedding validated", selectedProvider))

	return cfg, apiKey
}

// reconfigureGenerationProvider handles generation provider reconfiguration
// embedProvider and embedAPIKey are passed so we can offer to reuse the same key if providers match
func reconfigureGenerationProvider(ctx context.Context, current config.ProviderConfig, providerFlag, modelFlag, fallbackFlag, baseURLFlag, apiKeyFlag, embedProvider, embedAPIKey string, red, dim, green func(...interface{}) string) (config.ProviderConfig, string) {
	var selectedProvider string
	var apiKey string
	var model string
	var fallback string
	var baseURL string

	// Select provider (prefill with current)
	if providerFlag != "" {
		selectedProvider = providerFlag
	} else {
		fmt.Println()
		fmt.Println(dim("Select a generation provider:"))
		options := []string{"gemini", "openai", "openrouter", "ollama"}
		prompt := &survey.Select{
			Message: "Generation Provider:",
			Options: options,
			Default: current.Provider,
		}
		if err := survey.AskOne(prompt, &selectedProvider); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get model
	if modelFlag != "" {
		model = modelFlag
	} else {
		// Determine default based on provider
		var defaultModel string
		if selectedProvider == current.Provider {
			defaultModel = current.Model
		} else {
			switch selectedProvider {
			case "gemini":
				defaultModel = "gemini-2.5-flash"
			case "openai":
				defaultModel = "gpt-4o"
			case "openrouter":
				defaultModel = "openai/gpt-4o"
			case "ollama":
				defaultModel = "llama3.2"
			}
		}

		fmt.Println()
		prompt := &survey.Input{
			Message: "Model:",
			Default: defaultModel,
		}
		if err := survey.AskOne(prompt, &model); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get fallback model
	if fallbackFlag != "" {
		fallback = fallbackFlag
	} else {
		// Determine default based on provider
		var defaultFallback string
		if selectedProvider == current.Provider {
			defaultFallback = current.Fallback
		} else {
			switch selectedProvider {
			case "gemini":
				defaultFallback = "gemini-3.0-flash"
			case "openai":
				defaultFallback = "gpt-3.5-turbo"
			case "openrouter":
				defaultFallback = "openai/gpt-3.5-turbo"
			case "ollama":
				defaultFallback = "llama2"
			}
		}

		fmt.Println()
		prompt := &survey.Input{
			Message: "Fallback Model:",
			Default: defaultFallback,
		}
		if err := survey.AskOne(prompt, &fallback); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get base URL for Ollama
	if selectedProvider == "ollama" {
		if baseURLFlag != "" {
			baseURL = baseURLFlag
		} else {
			fmt.Println()
			defaultURL := "http://localhost:11434"
			if current.Provider == "ollama" && current.BaseURL != "" {
				defaultURL = current.BaseURL
			}
			prompt := &survey.Input{
				Message: "Ollama Base URL:",
				Default: defaultURL,
			}
			if err := survey.AskOne(prompt, &baseURL); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Get API key
	if selectedProvider == "ollama" {
		apiKey = "" // Ollama doesn't need an API key
	} else if apiKeyFlag != "" {
		apiKey = apiKeyFlag
	} else {
		// First, check if we can reuse the embedding key (same provider, just entered)
		if selectedProvider == embedProvider && embedAPIKey != "" {
			fmt.Println()
			var reuseKey bool
			prompt := &survey.Confirm{
				Message: "Use the same API key as embedding?",
				Default: true,
			}
			if err := survey.AskOne(prompt, &reuseKey); err == nil && reuseKey {
				apiKey = embedAPIKey
			}
		}

		// If not reusing embedding key, try to get existing key from keyring
		if apiKey == "" {
			existingKey, err := config.GetAPIKeyForProvider(selectedProvider)
			if err == nil && existingKey != "" {
				fmt.Println()
				var reuseKey bool
				prompt := &survey.Confirm{
					Message: fmt.Sprintf("Use existing %s API key?", selectedProvider),
					Default: true,
				}
				if err := survey.AskOne(prompt, &reuseKey); err == nil && reuseKey {
					apiKey = existingKey
				}
			}
		}

		if apiKey == "" {
			fmt.Println()
			switch selectedProvider {
			case "gemini":
				fmt.Println(dim("Get a Gemini API key at: https://aistudio.google.com/apikey"))
			case "openai":
				fmt.Println(dim("Get an OpenAI API key at: https://platform.openai.com/api-keys"))
			case "openrouter":
				fmt.Println(dim("Get an OpenRouter API key at: https://openrouter.ai/keys"))
			case "voyageai":
				fmt.Println(dim("Get a Voyage AI API key at: https://dash.voyageai.com"))
			case "anthropic":
				fmt.Println(dim("Get an Anthropic API key at: https://console.anthropic.com"))
			}
			fmt.Println()

			prompt := &survey.Password{
				Message: fmt.Sprintf("%s API Key:", selectedProvider),
			}
			if err := survey.AskOne(prompt, &apiKey, survey.WithValidator(survey.Required)); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Build config
	cfg := config.ProviderConfig{
		Provider: selectedProvider,
		Model:    model,
		Fallback: fallback,
		BaseURL:  baseURL,
	}

	// Validate provider
	fmt.Println()
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Validating %s generation...", selectedProvider)
	s.Start()

	llmProvider, err := provider.NewLLMProvider(cfg, apiKey)
	if err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to create LLM provider:", err)
		os.Exit(1)
	}

	if err := llmProvider.Validate(ctx); err != nil {
		s.Stop()
		fmt.Println(red("✗"), fmt.Sprintf("%s generation validation failed:", selectedProvider), err)
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), fmt.Sprintf("%s generation validated", selectedProvider))

	return cfg, apiKey
}

// clearEmbeddings drops and recreates the embeddings table with new dimensions
func clearEmbeddings(database *sql.DB, newDimensions int) error {
	// Drop existing embeddings table
	if _, err := database.Exec("DROP TABLE IF EXISTS chunk_embeddings"); err != nil {
		return err
	}

	// Recreate with new dimensions
	createSQL := db.CreateChunkEmbeddingsTableSQL(newDimensions)
	if _, err := database.Exec(createSQL); err != nil {
		return err
	}

	// Mark all repos as needing re-embedding
	if _, err := database.Exec("UPDATE repos SET embedded_hash = NULL, embedded_at = NULL"); err != nil {
		return err
	}

	// Update stored dimensions in meta table
	if err := db.SetEmbeddingDimensions(database, newDimensions); err != nil {
		return err
	}

	return nil
}

// clearChunks deletes all chunks and resets repo sync state
func clearChunks(database *sql.DB) error {
	// Delete all chunks
	if _, err := database.Exec("DELETE FROM chunks"); err != nil {
		return err
	}

	// Reset repo sync state to force re-sync
	if _, err := database.Exec("UPDATE repos SET pushed_at_hash = NULL, embedded_hash = NULL, embedded_at = NULL"); err != nil {
		return err
	}

	// Clear sync state
	if _, err := database.Exec("DELETE FROM sync_state"); err != nil {
		return err
	}

	return nil
}
