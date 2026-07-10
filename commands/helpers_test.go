package commands

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/github"
)

// mockKeyring is an in-memory config.KeyringBackend for tests. It never touches
// the real OS keyring.
type mockKeyring struct {
	mu    sync.Mutex
	store map[string]string
}

func newMockKeyring() *mockKeyring { return &mockKeyring{store: map[string]string{}} }

func (m *mockKeyring) k(service, key string) string { return service + "\x00" + key }

func (m *mockKeyring) Set(service, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[m.k(service, key)] = value
	return nil
}

func (m *mockKeyring) Get(service, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.store[m.k(service, key)]
	if !ok {
		return "", errors.New("keyring: not found")
	}
	return v, nil
}

func (m *mockKeyring) Delete(service, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, m.k(service, key))
	return nil
}

// setupCmdTest wires up an isolated environment so a command's RunE can execute
// end-to-end without any real network, keyring, or user config:
//   - a temp config directory (os.UserConfigDir reads HOME / XDG_CONFIG_HOME)
//   - a mock keyring seeded with a GitHub PAT (ollama providers need no API key)
//   - a mock GitHub API returning empty repo lists and a healthy rate limit
//   - a mock ollama server for embeddings (only hit when there are chunks)
//   - a config on disk pointing both providers at the ollama mock
//
// It also disables color so captured output is plain text.
func setupCmdTest(t *testing.T) {
	t.Helper()
	color.NoColor = true

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdgconfig"))

	kr := newMockKeyring()
	config.SetKeyringBackend(kr)
	if err := kr.Set(config.KeyringService, config.KeyringGitHubPAT, "fake-pat"); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}

	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rate_limit") {
			reset := time.Now().Add(time.Hour).Unix()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources": map[string]any{
					"core": map[string]any{"limit": 5000, "remaining": 4999, "reset": reset},
				},
			})
			return
		}
		// /user/repos, /user/starred, and anything else: empty JSON array.
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(gh.Close)
	github.SetDefaultBaseURL(gh.URL)
	t.Cleanup(github.ResetDefaultBaseURL)

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		emb := make([]float64, 768)
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": emb})
	}))
	t.Cleanup(ollama.Close)

	dbPath := filepath.Join(tmp, "repog.db")
	cfg := &config.Config{
		DBPath: dbPath,
		Embedding: config.ProviderConfig{
			Provider:   "ollama",
			Model:      "nomic-embed-text",
			Dimensions: 768,
			BaseURL:    ollama.URL,
		},
		Generation: config.ProviderConfig{
			Provider: "ollama",
			Model:    "llama3",
			BaseURL:  ollama.URL,
		},
	}
	if err := config.SaveConfigFile(cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns whatever
// was written. It drains the pipe concurrently so a spinner or large output can
// never deadlock on a full pipe buffer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

// openTestDB opens a fresh migrated database at the given dimensions.
func openTestDB(t *testing.T, dimensions int) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, dimensions)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// countRows returns the row count of a table, failing the test on error.
func countRows(t *testing.T, database *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
