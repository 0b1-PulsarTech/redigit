package checkrun

import (
	"os"
	"slices"
	"strings"
)

func isDropped(name string) bool {
	const noiseNames = "PWD OLDPWD SHLVL _ TERM COLORTERM DISPLAY WAYLAND_DISPLAY WINDOWID TMUX " +
		"TMUX_PANE STY SSH_CLIENT SSH_CONNECTION SSH_TTY XDG_RUNTIME_DIR XDG_SEAT XDG_VTNR " +
		"DBUS_SESSION_BUS_ADDRESS LS_COLORS PS1 PS2 PROMPT_COMMAND HISTFILE HISTSIZE " +
		"HISTCONTROL MAIL SESSION_MANAGER DESKTOP_SESSION GDMSESSION INVOCATION_ID " +
		"JOURNAL_STREAM MANAGERPID SYSTEMD_EXEC_PID AI_AGENT WT_SESSION GPG_TTY XAUTHORITY " +
		"WINDOWPATH SSH_AGENT_PID MANAGERPIDFDID ZELLIJ VSCODE_IPC_HOOK_CLI ATUIN_SESSION NVIM " +
		"TERM_SESSION_ID SHELL_SESSION_ID"

	const noisePrefixes = "GIT_ TERM_PROGRAM XDG_SESSION_ CLAUDE_ STARSHIP_ P9K_ _P9K_ GHOSTTY_ " +
		"KITTY_ WEZTERM_ ALACRITTY_ KONSOLE_ VTE_ ITERM_ MEMORY_PRESSURE_ GNOME_ ZELLIJ_ " +
		"VSCODE_GIT_ DIRENV_"

	return !isFetchKnob(name) && matchesAny(name, noiseNames, noisePrefixes)
}

func matchesAny(name, names, prefixes string) bool {
	return slices.Contains(strings.Fields(names), name) ||
		slices.ContainsFunc(strings.Fields(prefixes), func(prefix string) bool {
			return strings.HasPrefix(name, prefix)
		})
}

func isFetchKnob(name string) bool {
	const (
		fetchNames = "GIT_SSH GIT_SSH_COMMAND GIT_SSH_VARIANT GIT_ASKPASS GIT_TERMINAL_PROMPT " +
			"GIT_ALLOW_PROTOCOL GIT_PROXY_COMMAND"
		fetchPrefixes = "GIT_CONFIG_ GIT_HTTP_"
	)

	// A fetch must authenticate as the user. GIT_CONFIG_GLOBAL/SYSTEM files can rewrite URLs,
	// but only for fetching, and go.sum pins the fetched content, so these stay unhashed.
	return matchesAny(name, fetchNames, fetchPrefixes)
}

func isUnhashed(name string) bool {
	const unhashedNames = "HOME TMPDIR GOCACHE GOMODCACHE GOPATH XDG_CACHE_HOME XDG_CONFIG_HOME GOENV " +
		"GOPROXY GOSUMDB GONOSUMDB GOPRIVATE GONOPROXY GOINSECURE GOAUTH HTTP_PROXY " +
		"HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy SSH_AUTH_SOCK USER LOGNAME"

	return slices.Contains(strings.Fields(unhashedNames), name) || isFetchKnob(name)
}

func inherited(language string, forced []string) (run, keyed []string, flags string) {
	isGo := language == goLanguage

	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")

		switch {
		case isDropped(name) && !slices.Contains(forced, name):
		case isGo && (name == "GOWORK" || name == "GO111MODULE"):
		case isGo && name == "GOFLAGS":
			flags = value
		default:
			run = append(run, entry)

			// Everything not known to be a location or network knob is keyed. go env does
			// not report these (CPATH, PKG_CONFIG*, AR, GOROOT), yet they decide whether a
			// cgo build passes.
			if !isUnhashed(name) || slices.Contains(forced, name) {
				keyed = append(keyed, entry)
			}
		}
	}

	slices.Sort(keyed)

	return run, keyed, flags
}
