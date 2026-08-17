package commands

import (
	"context"
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
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize RepoG with your API credentials",
	Long:  "Initialize RepoG by providing your GitHub Personal Access Token and AI provider API keys.",
	RunE:  runInit,
}

var (
	initGitHubToken   string
	initDBPath        string
	initForce         bool
	initEmbedProvider string
	initGenProvider   string
	initEmbedAPIKey   string
	initGenAPIKey     string
)

func init() {
	initCmd.Flags().StringVar(&initGitHubToken, "github-token", "", "GitHub Personal Access Token")
	initCmd.Flags().StringVar(&initDBPath, "db-path", "", "Custom database path")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing configuration")
	initCmd.Flags().StringVar(&initEmbedProvider, "embed-provider", "", "Embedding provider (gemini, openai, openrouter, voyageai, or ollama)")
	initCmd.Flags().StringVar(&initGenProvider, "gen-provider", "", "Generation provider (gemini, openai, openrouter, anthropic, or ollama)")
	initCmd.Flags().StringVar(&initEmbedAPIKey, "embed-api-key", "", "API key for embedding provider")
	initCmd.Flags().StringVar(&initGenAPIKey, "gen-api-key", "", "API key for generation provider")
}

func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "********" + secret[len(secret)-4:]
}

// selectEmbeddingProvider guides the user through embedding provider configuration
func selectEmbeddingProvider(ctx context.Context, providerFlag, apiKeyFlag string, red, dim, green func(...interface{}) string) (config.ProviderConfig, string) {
	var selectedProvider string
	var apiKey string

	// Select provider
	if providerFlag != "" {
		selectedProvider = providerFlag
	} else {
		fmt.Println()
		fmt.Println(dim("Select an embedding provider:"))
		options := []string{"gemini", "openai", "openrouter", "voyageai", "ollama"}
		prompt := &survey.Select{
			Message: "Embedding Provider:",
			Options: options,
			Default: "gemini",
		}
		if err := survey.AskOne(prompt, &selectedProvider); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get API key (skip for Ollama)
	if selectedProvider == "ollama" {
		apiKey = "" // Ollama doesn't need an API key
	} else if apiKeyFlag != "" {
		apiKey = apiKeyFlag
	} else {
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

	// Build provider config with defaults
	var cfg config.ProviderConfig
	switch selectedProvider {
	case "gemini":
		cfg = config.DefaultEmbeddingConfig()
	case "openai":
		cfg = config.ProviderConfig{
			Provider:   "openai",
			Model:      "text-embedding-3-small",
			Dimensions: 1536,
		}
	case "openrouter":
		cfg = config.ProviderConfig{
			Provider:   "openrouter",
			Model:      "openai/text-embedding-3-small",
			Dimensions: 1536,
		}
	case "voyageai":
		cfg = config.ProviderConfig{
			Provider:   "voyageai",
			Model:      "voyage-code-3",
			Dimensions: 1024,
		}
	case "ollama":
		cfg = config.ProviderConfig{
			Provider:   "ollama",
			Model:      "nomic-embed-text",
			Dimensions: 768,
			BaseURL:    "http://localhost:11434",
		}
	default:
		fmt.Println(red("✗"), "Unknown provider:", selectedProvider)
		os.Exit(1)
	}

	// Validate provider first to ensure credentials work
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

	// Get default max tokens for the selected model
	defaultMaxTokens := embedProvider.MaxTokens()

	// Ask about custom max tokens
	fmt.Println()
	var customizeTokens bool
	tokenPrompt := &survey.Confirm{
		Message: fmt.Sprintf("Customize max token limit? (default: %d)", defaultMaxTokens),
		Default: false,
	}
	if err := survey.AskOne(tokenPrompt, &customizeTokens); err != nil {
		// User cancelled, use default
		customizeTokens = false
	}

	if customizeTokens {
		fmt.Println()
		fmt.Println(dim("Max token limit controls how text is chunked for embedding."))
		fmt.Println()
		yellow := color.New(color.FgYellow).SprintFunc()
		fmt.Println(yellow("⚠️  Warning:"))
		fmt.Println(yellow("   • Too high: May cause embedding API errors if chunks exceed model limits"))
		fmt.Println(yellow("   • Too low:  May reduce search accuracy due to excessive chunking"))
		fmt.Println()

		var maxTokensStr string
		inputPrompt := &survey.Input{
			Message: "Max tokens per chunk:",
			Default: fmt.Sprintf("%d", defaultMaxTokens),
		}
		if err := survey.AskOne(inputPrompt, &maxTokensStr); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}

		var customMaxTokens int
		_, _ = fmt.Sscanf(maxTokensStr, "%d", &customMaxTokens)

		if customMaxTokens > 0 && customMaxTokens != defaultMaxTokens {
			cfg.MaxTokens = customMaxTokens

			// Show additional warning if significantly different
			if customMaxTokens > defaultMaxTokens*2 {
				fmt.Println()
				fmt.Println(yellow("⚠️  Token limit is much higher than model default - embedding errors may occur"))
			} else if customMaxTokens < defaultMaxTokens/4 {
				fmt.Println()
				fmt.Println(yellow("⚠️  Token limit is much lower than model default - search accuracy may be reduced"))
			}
		}
	}

	return cfg, apiKey
}

// selectGenerationProvider guides the user through generation provider configuration
// embedProvider and embedAPIKey are passed so we can offer to reuse the same key if providers match
func selectGenerationProvider(ctx context.Context, providerFlag, apiKeyFlag, embedProvider, embedAPIKey string, red, dim, green func(...interface{}) string) (config.ProviderConfig, string) {
	var selectedProvider string
	var apiKey string

	// Select provider
	if providerFlag != "" {
		selectedProvider = providerFlag
	} else {
		fmt.Println()
		fmt.Println(dim("Select a generation provider:"))
		options := []string{"gemini", "openai", "openrouter", "anthropic", "ollama"}
		prompt := &survey.Select{
			Message: "Generation Provider:",
			Options: options,
			Default: "gemini",
		}
		if err := survey.AskOne(prompt, &selectedProvider); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get API key (skip for Ollama, reuse embedding key if same provider)
	if selectedProvider == "ollama" {
		apiKey = "" // Ollama doesn't need an API key
	} else if apiKeyFlag != "" {
		apiKey = apiKeyFlag
	} else {
		// Only offer to reuse embedding key if providers match
		if selectedProvider == embedProvider && embedAPIKey != "" {
			fmt.Println()
			reuseKey := false
			prompt := &survey.Confirm{
				Message: "Use the same API key for generation?",
				Default: true,
			}
			if err := survey.AskOne(prompt, &reuseKey); err == nil && reuseKey {
				apiKey = embedAPIKey
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
			case "anthropic":
				fmt.Println(dim("Get an Anthropic API key at: https://console.anthropic.com"))
			}
			fmt.Println()

			passPrompt := &survey.Password{
				Message: fmt.Sprintf("%s API Key:", selectedProvider),
			}
			if err := survey.AskOne(passPrompt, &apiKey, survey.WithValidator(survey.Required)); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Build provider config with defaults
	var cfg config.ProviderConfig
	switch selectedProvider {
	case "gemini":
		cfg = config.DefaultGenerationConfig()
	case "openai":
		cfg = config.ProviderConfig{
			Provider: "openai",
			Model:    "gpt-4o",
			Fallback: "gpt-3.5-turbo",
		}
	case "openrouter":
		cfg = config.ProviderConfig{
			Provider: "openrouter",
			Model:    "openai/gpt-4o",
			Fallback: "openai/gpt-3.5-turbo",
		}
	case "anthropic":
		cfg = config.ProviderConfig{
			Provider: "anthropic",
			Model:    "claude-3-5-haiku-20241022",
			Fallback: "claude-3-5-sonnet-20241022",
		}
	case "ollama":
		cfg = config.ProviderConfig{
			Provider: "ollama",
			Model:    "llama3.2",
			Fallback: "llama2",
			BaseURL:  "http://localhost:11434",
		}
	default:
		fmt.Println(red("✗"), "Unknown provider:", selectedProvider)
		os.Exit(1)
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

func runInit(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	fmt.Println()
	fmt.Println(bold("RepoG Setup"))
	fmt.Println()

	// Check if already configured
	if config.IsConfigured() && !initForce {
		cfg, _ := config.LoadConfig()
		pat, _ := config.GetGitHubPAT()

		fmt.Println(yellow("RepoG is already configured."))
		if pat != "" {
			fmt.Println("  GitHub:", green("configured"))
		} else {
			fmt.Println("  GitHub:", red("not set"))
		}

		if cfg != nil {
			fmt.Println("  Embedding Provider:", cfg.Embedding.Provider)
			fmt.Println("  Generation Provider:", cfg.Generation.Provider)
			fmt.Println("  Database:", cfg.DBPath)
		}
		fmt.Println()
		fmt.Println("To update your configuration, use:")
		fmt.Println("  ", cyan("repog reconfig"), "          - Reconfigure all settings")
		fmt.Println("  ", cyan("repog reconfig github"), "   - Update GitHub PAT only")
		fmt.Println("  ", cyan("repog reconfig embedding"), "- Update embedding provider")
		fmt.Println("  ", cyan("repog reconfig generation"), "- Update generation provider")
		fmt.Println()
		fmt.Println(dim("Use --force to reinitialize from scratch."))
		return nil
	}

	// Get GitHub token
	githubToken := initGitHubToken
	if githubToken == "" {
		fmt.Println(bold("RepoG requires a fine-grained GitHub Personal Access Token."))
		fmt.Println()
		fmt.Println(dim("Create one at: https://github.com/settings/personal-access-tokens/new"))
		fmt.Println()
		fmt.Println(dim("Required settings:"))
		fmt.Println(dim("  Resource owner:     Your GitHub account"))
		fmt.Println(dim("  Repository access:  All repositories (or select specific repos)"))
		fmt.Println(dim("  Permissions:"))
		fmt.Println(dim("    Contents:         Read-only"))
		fmt.Println(dim("    Metadata:         Read-only"))
		fmt.Println()

		prompt := &survey.Password{
			Message: "GitHub Personal Access Token:",
		}
		if err := survey.AskOne(prompt, &githubToken, survey.WithValidator(survey.Required)); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Validate GitHub token
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Validating GitHub token..."
	s.Start()

	client := github.NewClient(githubToken)
	patResult := github.ValidatePAT(commandContext(cmd), client)
	s.Stop()

	if !patResult.Valid {
		fmt.Println(red("✗"), "GitHub token invalid:", patResult.Error)
		os.Exit(1)
	}

	fmt.Println(green("✓"), "Fine-grained PAT validated - logged in as @"+patResult.Login)

	// Provider selection and configuration
	embedCfg, embedAPIKey := selectEmbeddingProvider(commandContext(cmd), initEmbedProvider, initEmbedAPIKey, red, dim, green)
	genCfg, genAPIKey := selectGenerationProvider(commandContext(cmd), initGenProvider, initGenAPIKey, embedCfg.Provider, embedAPIKey, red, dim, green)

	// Get database path
	dbPath := initDBPath
	if dbPath == "" {
		defaultPath := config.DefaultDBPath()

		prompt := &survey.Input{
			Message: "Database path:",
			Default: defaultPath,
		}
		if err := survey.AskOne(prompt, &dbPath); err != nil {
			// User cancelled, use default
			dbPath = defaultPath
		}

		if dbPath == "" {
			dbPath = defaultPath
		}
	}

	// Initialize database with selected embedding dimensions
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Initializing database..."
	s.Start()

	database, err := db.Open(dbPath, embedCfg.Dimensions)
	if err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Database init failed:", err)
		os.Exit(1)
	}
	_ = database.Close()
	s.Stop()

	fmt.Println(green("✓"), "Database initialized")

	// Save configuration
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Saving configuration..."
	s.Start()

	cfg := &config.Config{
		DBPath:     dbPath,
		Embedding:  embedCfg,
		Generation: genCfg,
	}

	// Store GitHub PAT
	if err := config.SetAPIKeyForProvider("github", githubToken); err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to save GitHub token:", err)
		os.Exit(1)
	}

	// Store embedding provider API key
	if err := config.SetAPIKeyForProvider(embedCfg.Provider, embedAPIKey); err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to save embedding API key:", err)
		os.Exit(1)
	}

	// Store generation provider API key
	if err := config.SetAPIKeyForProvider(genCfg.Provider, genAPIKey); err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to save generation API key:", err)
		os.Exit(1)
	}

	// Write config file
	if err := config.SaveConfigFile(cfg); err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to save config:", err)
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), "Configuration saved")

	// Summary
	fmt.Println()
	fmt.Println(bold(green("Setup complete!")))
	fmt.Println()

	// Get effective max tokens for display
	var maxTokensLabel string
	if embedCfg.MaxTokens > 0 {
		maxTokensLabel = fmt.Sprintf("%d tokens (custom)", embedCfg.MaxTokens)
	} else {
		// Get model default
		if defaultMaxTokens, err := provider.GetModelDefaultMaxTokens(embedCfg, embedAPIKey); err == nil {
			maxTokensLabel = fmt.Sprintf("%d tokens", defaultMaxTokens)
		} else {
			maxTokensLabel = "default"
		}
	}

	fmt.Println("Configuration:")
	fmt.Println("  GitHub:", cyan(maskSecret(githubToken)), "("+patResult.Login+")")
	fmt.Println("  Embedding:", cyan(embedCfg.Provider), fmt.Sprintf("(%s, %dd, %s)", embedCfg.Model, embedCfg.Dimensions, maxTokensLabel))
	fmt.Println("  Generation:", cyan(genCfg.Provider), fmt.Sprintf("(%s)", genCfg.Model))
	fmt.Println("  Database:", cyan(dbPath))
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println(" ", cyan("repog sync"), "    - Sync your GitHub repositories")
	fmt.Println(" ", cyan("repog status"), "  - Check sync status")
	fmt.Println()

	return nil
}
