package commands

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRunStatusJSON exercises the full status RunE against an empty database and
// a mocked GitHub rate-limit endpoint, asserting it emits valid JSON.
func TestRunStatusJSON(t *testing.T) {
	setupCmdTest(t)

	statusJSON = true
	t.Cleanup(func() { statusJSON = false })

	out := captureStdout(t, func() {
		if err := runStatus(statusCmd, nil); err != nil {
			t.Errorf("runStatus returned error: %v", err)
		}
	})

	out = strings.TrimSpace(out)
	if out == "" {
		t.Fatal("runStatus --json produced no output")
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("status --json output is not a valid JSON object: %v\noutput: %s", err, out)
	}
	if len(result) == 0 {
		t.Errorf("status --json object is empty: %s", out)
	}
}

// TestRunSyncEmpty exercises the full sync RunE with a mocked GitHub API that
// returns no repositories. It should complete without error.
func TestRunSyncEmpty(t *testing.T) {
	setupCmdTest(t)

	syncOwned, syncStarred, syncVerbose = true, true, true // verbose disables the spinner
	t.Cleanup(func() { syncOwned, syncStarred, syncFullTree, syncVerbose = false, false, false, false })

	out := captureStdout(t, func() {
		if err := runSync(syncCmd, nil); err != nil {
			t.Errorf("runSync returned error: %v", err)
		}
	})

	if !strings.Contains(out, "Sync complete") {
		t.Errorf("expected sync completion message, got: %q", out)
	}
}

// TestRunEmbedEmpty exercises the full embed RunE with no pending chunks. The
// pipeline should report completion without ever calling the embedding API.
func TestRunEmbedEmpty(t *testing.T) {
	setupCmdTest(t)

	embedVerbose = true // disables the spinner
	t.Cleanup(func() { embedVerbose = false })

	out := captureStdout(t, func() {
		if err := runEmbed(embedCmd, nil); err != nil {
			t.Errorf("runEmbed returned error: %v", err)
		}
	})

	if !strings.Contains(out, "Embedding complete") {
		t.Errorf("expected embedding completion message, got: %q", out)
	}
}
