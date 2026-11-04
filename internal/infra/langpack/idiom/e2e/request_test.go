package e2e

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func callTo(site entities.DeclKey, callee, receiver, literal string) []facts.Call {
	return []facts.Call{{
		Site: site, Callee: callee, Receiver: facts.TypeRef{Name: receiver},
		Literals: []string{literal},
	}}
}

func TestRuleEdgesSkipCallsThatAreNoRequest(t *testing.T) {
	t.Parallel()

	var (
		set   = newRouteSet()
		suite = suiteKey("test", "TestCall")
		own   = `{"type":"go-e2e","router":{"handle":["*.GET","*.HandleFunc"],` +
			`"mount":["*.Route"]},"dirs":["test/"]}`
	)

	requireEdges(t, set.known, set.decls, []edgeCase{
		{
			name:  "a log line",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "testing.T.Log", "testing.T", "/plain"),
		},
		{
			name:  "a subtest name",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "testing.T.Run", "testing.T", "/plain"),
		},
		{
			name:  "a file open",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "os.Open", "", "/plain"),
		},
		{
			name:  "a joined file path",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "path/filepath.Join", "", "/plain"),
		},
		{
			name:  "a formatted string",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "fmt.Sprintf", "", "/plain"),
		},
		{
			name:  "a log call",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "log/slog.Info", "", "/plain"),
		},
		{
			name:  "a string helper",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "strings.Contains", "", "/plain"),
		},
		{
			name:  "a test's own server route",
			cfg:   own,
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "net/http.ServeMux.HandleFunc", "net/http.ServeMux", "/plain"),
		},
		{
			name:  "a test's own mount call",
			cfg:   own,
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "m/router.Router.Route", "m/router.Router", "/plain"),
		},
	})
}

func TestRuleEdgesCountTheCallsThatCanBeARequest(t *testing.T) {
	t.Parallel()

	var (
		set    = newRouteSet()
		suite  = suiteKey("test", "TestCall")
		narrow = `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["test/"],` +
			`"requests":["Get","*.Do"]}`
	)

	requireEdges(t, set.known, set.decls, []edgeCase{
		{
			name:  "any other call counts when no requests are listed",
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "m/test.get", "", "/plain"),
			want:  []entities.Edge{edgeTo(suite, set.plain)},
		},
		{
			name:  "a listed function counts",
			cfg:   narrow,
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "net/http.Get", "", "/health"),
			want:  []entities.Edge{edgeTo(suite, set.health)},
		},
		{
			name:  "a listed method counts",
			cfg:   narrow,
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "net/http.Client.Do", "net/http.Client", "/health"),
			want:  []entities.Edge{edgeTo(suite, set.health)},
		},
		{
			name:  "a call the list does not name",
			cfg:   narrow,
			suite: entities.Decl{Key: suite},
			calls: callTo(suite, "m/test.get", "", "/health"),
		},
	})
}
