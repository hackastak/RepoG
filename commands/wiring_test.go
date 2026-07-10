package commands

import "testing"

func TestRootRegistersAllSubcommands(t *testing.T) {
	want := []string{
		"init", "reconfig", "sync", "embed",
		"search", "recommend", "ask", "summarize", "status",
	}
	got := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		got[c.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("subcommand %q is not registered on rootCmd", name)
		}
	}
}

func TestCommandFlagsExist(t *testing.T) {
	cases := []struct {
		cmd   string
		flags []string
	}{
		{"status", []string{"json"}},
		{"sync", []string{"owned", "starred", "full-tree", "verbose"}},
		{"embed", []string{"include-file-tree", "verbose", "batch-size"}},
	}
	for _, tc := range cases {
		var found bool
		for _, c := range rootCmd.Commands() {
			if c.Name() != tc.cmd {
				continue
			}
			found = true
			for _, f := range tc.flags {
				if c.Flags().Lookup(f) == nil {
					t.Errorf("%s: expected flag --%s to exist", tc.cmd, f)
				}
			}
		}
		if !found {
			t.Errorf("command %q not found", tc.cmd)
		}
	}
}

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "****"},
		{"short", "****"},
		{"12345678", "****"},              // boundary: len == 8 is fully masked
		{"123456789", "1234********6789"}, // len == 9 shows first/last 4
		{"ghp_abcdefghijklmnop", "ghp_********mnop"},
	}
	for _, c := range cases {
		if got := maskSecret(c.in); got != c.want {
			t.Errorf("maskSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
