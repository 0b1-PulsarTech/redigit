package checkrun

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func rustIdentity(t *testing.T) string {
	t.Helper()

	cfg := Config{Languages: map[string]Language{"rust": {Steps: []Step{fake("pass")}}}}

	return identityOf(t, cfg, rustUnit())
}

func TestIdentityDefaultHashesTheEnvironment(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		wantSame bool
	}{
		{"CPATH", "CPATH", false},
		{"C_INCLUDE_PATH", "C_INCLUDE_PATH", false},
		{"an unknown variable", "REDIGIT_TEST_UNKNOWN", false},
		{"PATH", "PATH", false},
		{"PWD", "PWD", true},
		{"SHLVL", "SHLVL", true},
		{"TERM", "TERM", true},
		{"TERM_PROGRAM_VERSION", "TERM_PROGRAM_VERSION", true},
		{"XDG_SESSION_ID", "XDG_SESSION_ID", true},
		{"GIT_DIR", "GIT_DIR", true},
		{"TMPDIR", "TMPDIR", true},
		{"GOCACHE", "GOCACHE", true},
		{"HOME", "HOME", true},
		{"LC_TIME", "LC_TIME", false},
		{"LANG", "LANG", false},
		{"LANGUAGE", "LANGUAGE", false},
		{"HTTPS_PROXY", "HTTPS_PROXY", true},
		{"CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_SESSION_ID", true},
		{"AI_AGENT", "AI_AGENT", true},
		{"STARSHIP_SESSION_KEY", "STARSHIP_SESSION_KEY", true},
		{"P9K_TTY", "P9K_TTY", true},
		{"_P9K_TTY", "_P9K_TTY", true},
		{"GHOSTTY_RESOURCES_DIR", "GHOSTTY_RESOURCES_DIR", true},
		{"KITTY_WINDOW_ID", "KITTY_WINDOW_ID", true},
		{"WEZTERM_PANE", "WEZTERM_PANE", true},
		{"ALACRITTY_SOCKET", "ALACRITTY_SOCKET", true},
		{"KONSOLE_VERSION", "KONSOLE_VERSION", true},
		{"VTE_VERSION", "VTE_VERSION", true},
		{"ITERM_SESSION_ID", "ITERM_SESSION_ID", true},
		{"WT_SESSION", "WT_SESSION", true},
		{"GPG_TTY", "GPG_TTY", true},
		{"XAUTHORITY", "XAUTHORITY", true},
		{"WINDOWPATH", "WINDOWPATH", true},
		{"SSH_AGENT_PID", "SSH_AGENT_PID", true},
		{"MEMORY_PRESSURE_WATCH", "MEMORY_PRESSURE_WATCH", true},
		{"MANAGERPIDFDID", "MANAGERPIDFDID", true},
		{"GNOME_TERMINAL_SCREEN", "GNOME_TERMINAL_SCREEN", true},
		{"ZELLIJ_SESSION_NAME", "ZELLIJ_SESSION_NAME", true},
		{"ZELLIJ", "ZELLIJ", true},
		{"VSCODE_IPC_HOOK_CLI", "VSCODE_IPC_HOOK_CLI", true},
		{"VSCODE_GIT_ASKPASS_NODE", "VSCODE_GIT_ASKPASS_NODE", true},
		{"ATUIN_SESSION", "ATUIN_SESSION", true},
		{"NVIM", "NVIM", true},
		{"TERM_SESSION_ID", "TERM_SESSION_ID", true},
		{"DIRENV_DIFF", "DIRENV_DIFF", true},
		{"SHELL_SESSION_ID", "SHELL_SESSION_ID", true},
		{"GIT_SSH_COMMAND", "GIT_SSH_COMMAND", true},
		{"GH_TOKEN", "GH_TOKEN", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(testCase.key, "before")

			before := rustIdentity(t)

			t.Setenv(testCase.key, "after")

			got := rustIdentity(t)
			if (got == before) != testCase.wantSame {
				t.Fatalf("%s: same = %v, want %v", testCase.key, got == before, testCase.wantSame)
			}
		})
	}
}

func TestIdentityTellsAnEmptyHashedVariableFromAnUnsetOne(t *testing.T) {
	t.Setenv("REDIGIT_TEST_UNKNOWN", "")

	if err := os.Unsetenv("REDIGIT_TEST_UNKNOWN"); err != nil {
		t.Fatalf("unset: %v", err)
	}

	unset := rustIdentity(t)

	t.Setenv("REDIGIT_TEST_UNKNOWN", "")

	if empty := rustIdentity(t); empty == unset {
		t.Fatal("a variable set to empty has the identity of an unset one")
	}
}

func TestStepEnvPassesAndHashesADeclaredNameOfTheDroppedClass(t *testing.T) {
	testCases := []struct {
		name     string
		forced   []string
		wantSeen bool
	}{
		{"declared GIT_X", []string{"GIT_X"}, true},
		{"undeclared GIT_X", nil, false},
		{"another name declared", []string{"GIT_Y"}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("GIT_X", "x")

			run, keyed := stepEnv(rustUnit(), testCase.forced, "")

			if got := slices.Contains(run, "GIT_X=x"); got != testCase.wantSeen {
				t.Fatalf("passed = %v, want %v", got, testCase.wantSeen)
			}

			if got := slices.Contains(keyed, "GIT_X=x"); got != testCase.wantSeen {
				t.Fatalf("hashed = %v, want %v", got, testCase.wantSeen)
			}
		})
	}
}

func TestStepEnvPassesTheGitFetchKnobsUnhashedAndDropsTheRest(t *testing.T) {
	testCases := []struct {
		name      string
		key       string
		wantFetch bool
	}{
		{"GIT_SSH", "GIT_SSH", true},
		{"GIT_SSH_COMMAND", "GIT_SSH_COMMAND", true},
		{"GIT_SSH_VARIANT", "GIT_SSH_VARIANT", true},
		{"GIT_ASKPASS", "GIT_ASKPASS", true},
		{"GIT_TERMINAL_PROMPT", "GIT_TERMINAL_PROMPT", true},
		{"GIT_ALLOW_PROTOCOL", "GIT_ALLOW_PROTOCOL", true},
		{"GIT_PROXY_COMMAND", "GIT_PROXY_COMMAND", true},
		{"GIT_CONFIG_COUNT", "GIT_CONFIG_COUNT", true},
		{"GIT_CONFIG_KEY_0", "GIT_CONFIG_KEY_0", true},
		{"GIT_CONFIG_VALUE_0", "GIT_CONFIG_VALUE_0", true},
		{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_GLOBAL", true},
		{"GIT_CONFIG_SYSTEM", "GIT_CONFIG_SYSTEM", true},
		{"GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_NOSYSTEM", true},
		{"GIT_HTTP_LOW_SPEED_LIMIT", "GIT_HTTP_LOW_SPEED_LIMIT", true},
		{"GIT_DIR", "GIT_DIR", false},
		{"GIT_WORK_TREE", "GIT_WORK_TREE", false},
		{"GIT_INDEX_FILE", "GIT_INDEX_FILE", false},
		{"GIT_OBJECT_DIRECTORY", "GIT_OBJECT_DIRECTORY", false},
		{"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_ALTERNATE_OBJECT_DIRECTORIES", false},
		{"GIT_COMMON_DIR", "GIT_COMMON_DIR", false},
		{"GIT_NAMESPACE", "GIT_NAMESPACE", false},
		{"GIT_CEILING_DIRECTORIES", "GIT_CEILING_DIRECTORIES", false},
		{"GIT_EXEC_PATH", "GIT_EXEC_PATH", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(testCase.key, "x")

			run, keyed := stepEnv(rustUnit(), nil, "")

			if got := slices.Contains(run, testCase.key+"=x"); got != testCase.wantFetch {
				t.Fatalf("passed = %v, want %v", got, testCase.wantFetch)
			}

			if slices.Contains(keyed, testCase.key+"=x") {
				t.Fatal("a git knob is hashed")
			}
		})
	}
}

func TestCheckPassesHashedAndUnhashedButNoDroppedVariables(t *testing.T) {
	useFakeGo(t)

	passed := map[string]string{
		"REDIGIT_TEST_UNKNOWN": "u", "CPATH": "/inc", "TMPDIR": "/tmp/x",
		"HOME": "/home/x", "LC_TIME": "C", "HTTP_PROXY": "http://p",
	}
	dropped := map[string]string{
		"PWD": "/p", "SHLVL": "3", "TERM": "xterm", "TERM_PROGRAM": "tp", "XDG_SESSION_ID": "9",
		"GIT_DIR": "/g", "DBUS_SESSION_BUS_ADDRESS": "unix:x", "INVOCATION_ID": "abc",
	}

	t.Setenv("GOCACHE", "/gc")

	for key, value := range passed {
		t.Setenv(key, value)
	}

	for key, value := range dropped {
		t.Setenv(key, value)
	}

	got := "\n" + dumpOf(t, goUnit())

	for key, value := range passed {
		if !strings.Contains(got, "\n"+key+"="+value+"\n") {
			t.Errorf("step env lacks %s=%s", key, value)
		}
	}

	if strings.Contains(got, "\nGOCACHE=/gc\n") {
		t.Error("step env still holds the inherited GOCACHE")
	}

	for key := range dropped {
		if strings.Contains(got, "\n"+key+"=") {
			t.Errorf("step env still holds %s", key)
		}
	}
}
