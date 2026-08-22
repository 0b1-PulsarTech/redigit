package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// fakeSession wires a Session to a fakeServer over in-memory pipes, so
// Session's Resolver methods and the initialize handshake are tested
// without spawning real gopls.
func fakeSession(t testing.TB) (*Session, *fakeServer) {
	t.Helper()

	clientWriter, clientReader, server := pipeTransport(t)
	processCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	noop := func() error { return nil }
	session := newSession(processCtx, cancel, clientWriter, clientReader, testLogger(), noop, noop)

	return session, server
}

// wedgedProcessSession wires a Session whose fake process only "exits" once
// kill is invoked, for tests proving Close force-kills instead of hanging
// against an unresponsive language server.
func wedgedProcessSession(
	t testing.TB,
) (session *Session, server *fakeServer, killed chan struct{}) {
	t.Helper()

	clientWriter, clientReader, server := pipeTransport(t)
	processCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	killed = make(chan struct{})
	wait := func() error {
		<-killed // the fake process only "exits" once kill is invoked

		return errors.New("process killed")
	}
	kill := func() error {
		close(killed)

		return nil
	}

	session = newSession(processCtx, cancel, clientWriter, clientReader, testLogger(), wait, kill)

	return session, server, killed
}

func TestServerConfig_mergedInitOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		cfg            ServerConfig
		wantBuildFlags []string
		wantExtraKey   string
	}{
		{
			name:           "build flags only",
			cfg:            ServerConfig{BuildFlags: []string{"-tags=integration"}},
			wantBuildFlags: []string{"-tags=integration"},
		},
		{
			name:           "nil build flags is explicit, not omitted",
			cfg:            ServerConfig{},
			wantBuildFlags: nil,
		},
		{
			name: "extra options merged alongside build flags",
			cfg: ServerConfig{
				BuildFlags: []string{"-tags=integration"},
				InitializationOptions: map[string]any{
					"env": map[string]string{"GOFLAGS": "-mod=mod"},
				},
			},
			wantBuildFlags: []string{"-tags=integration"},
			wantExtraKey:   "env",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.cfg.mergedInitOptions()

			flags, ok := got["buildFlags"]
			if !ok {
				t.Fatalf("mergedInitOptions: missing buildFlags key entirely: %v", got)
			}

			gotFlags, _ := flags.([]string)
			if len(gotFlags) != len(testCase.wantBuildFlags) {
				t.Fatalf("buildFlags = %v, want %v", gotFlags, testCase.wantBuildFlags)
			}

			if testCase.wantExtraKey != "" {
				if _, ok := got[testCase.wantExtraKey]; !ok {
					t.Fatalf(
						"mergedInitOptions: missing extra key %q: %v",
						testCase.wantExtraKey,
						got,
					)
				}
			}
		})
	}
}

func TestSession_Initialize(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	cfg := ServerConfig{
		Command:    "fake",
		RootURI:    "file:///repo",
		BuildFlags: []string{"-tags=integration"},
	}

	initDone := make(chan error, 1)
	go func() { initDone <- session.initialize(context.Background(), cfg) }()

	req := server.next()
	if req.Method != "initialize" {
		t.Fatalf("method = %q, want initialize", req.Method)
	}

	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if !params.Capabilities.TextDocument.DocumentSymbol.HierarchicalDocumentSymbolSupport {
		t.Fatalf("params did not request hierarchical document symbols: %+v", params.Capabilities)
	}

	if flags, ok := params.InitializationOptions["buildFlags"].([]any); !ok || len(flags) != 1 {
		t.Fatalf(
			"initializationOptions.buildFlags = %v, want [-tags=integration]",
			params.InitializationOptions["buildFlags"],
		)
	}

	server.respond(req.ID, InitializeResult{Capabilities: ServerCapabilities{}})

	initNotify := server.next()
	if initNotify.Method != "initialized" {
		t.Fatalf("method = %q, want initialized", initNotify.Method)
	}

	if err := <-initDone; err != nil {
		t.Fatalf("initialize: %v", err)
	}

	if session.PositionEncoding() != "utf-16" {
		t.Fatalf(
			"PositionEncoding = %q, want utf-16 (the unset default)",
			session.PositionEncoding(),
		)
	}
}

func TestSession_Initialize_ReadsNegotiatedEncoding(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	initDone := make(chan error, 1)

	go func() { initDone <- session.initialize(context.Background(), ServerConfig{Command: "fake"}) }()

	req := server.next()
	server.respond(
		req.ID,
		InitializeResult{Capabilities: ServerCapabilities{PositionEncoding: "utf-8"}},
	)
	server.next() // initialized notification

	if err := <-initDone; err != nil {
		t.Fatalf("initialize: %v", err)
	}

	if session.PositionEncoding() != "utf-8" {
		t.Fatalf("PositionEncoding = %q, want utf-8", session.PositionEncoding())
	}
}

func TestSession_References(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	resultCh := make(chan error, 1)

	var got []entities.Location

	at := entities.Location{File: "/repo/main.go", Line: 3, Char: 7}

	go func() {
		var err error
		got, err = session.References(context.Background(), at)
		resultCh <- err
	}()

	req := server.next()
	if req.Method != "textDocument/references" {
		t.Fatalf("method = %q, want textDocument/references", req.Method)
	}

	var params ReferenceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if params.TextDocument.URI != "file:///repo/main.go" ||
		params.Position != (Position{Line: 3, Character: 7}) {
		t.Fatalf("params = %+v, want uri file:///repo/main.go position {3 7}", params)
	}

	if params.Context.IncludeDeclaration {
		t.Fatalf("IncludeDeclaration = true, want false")
	}

	server.respond(req.ID, []Location{
		{URI: "file:///repo/caller.go", Range: Range{Start: Position{Line: 10, Character: 2}}},
	})

	if err := <-resultCh; err != nil {
		t.Fatalf("References: %v", err)
	}

	want := entities.Location{File: "/repo/caller.go", Line: 10, Char: 2}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got = %+v, want [%+v]", got, want)
	}
}

func TestSession_References_InvalidPath(t *testing.T) {
	t.Parallel()

	session, _ := fakeSession(t)

	_, err := session.References(context.Background(), entities.Location{File: "relative.go"})
	if !errors.Is(err, ErrRelativePath) {
		t.Fatalf("err = %v, want ErrRelativePath", err)
	}
}

func TestSession_Implementations(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	resultCh := make(chan error, 1)

	var got []entities.Location

	go func() {
		var err error
		got, err = session.Implementations(
			context.Background(),
			entities.Location{File: "/repo/iface.go", Line: 1, Char: 1},
		)
		resultCh <- err
	}()

	req := server.next()
	if req.Method != "textDocument/implementation" {
		t.Fatalf("method = %q, want textDocument/implementation", req.Method)
	}

	server.respond(
		req.ID,
		[]Location{
			{URI: "file:///repo/impl.go", Range: Range{Start: Position{Line: 5, Character: 0}}},
		},
	)

	if err := <-resultCh; err != nil {
		t.Fatalf("Implementations: %v", err)
	}

	want := entities.Location{File: "/repo/impl.go", Line: 5, Char: 0}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got = %+v, want [%+v]", got, want)
	}
}

func TestSession_Definition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		result  locations
		want    entities.Location
		wantErr error
	}{
		{
			name: "single location",
			result: locations{
				{URI: "file:///repo/def.go", Range: Range{Start: Position{Line: 4, Character: 2}}},
			},
			want: entities.Location{File: "/repo/def.go", Line: 4, Char: 2},
		},
		{
			name:    "empty result is no definition",
			result:  locations{},
			wantErr: ErrNoDefinition,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			session, server := fakeSession(t)
			resultCh := make(chan error, 1)

			var got entities.Location

			go func() {
				var err error
				got, err = session.Definition(
					context.Background(),
					entities.Location{File: "/repo/use.go", Line: 1, Char: 1},
				)
				resultCh <- err
			}()

			req := server.next()
			server.respond(req.ID, []Location(testCase.result))

			err := <-resultCh
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestSession_DocumentSymbols(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	resultCh := make(chan error, 1)

	var got []DocumentSymbol

	go func() {
		var err error
		got, err = session.DocumentSymbols(context.Background(), "/repo/main.go")
		resultCh <- err
	}()

	req := server.next()
	if req.Method != "textDocument/documentSymbol" {
		t.Fatalf("method = %q, want textDocument/documentSymbol", req.Method)
	}

	server.respond(req.ID, []DocumentSymbol{{Name: "main", Kind: SymbolKindFunction}})

	if err := <-resultCh; err != nil {
		t.Fatalf("DocumentSymbols: %v", err)
	}

	if len(got) != 1 || got[0].Name != "main" {
		t.Fatalf("got = %+v, want one symbol named main", got)
	}
}

func TestSession_Close(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)

	closeDone := make(chan error, 1)
	go func() { closeDone <- session.Close(context.Background()) }()

	shutdownReq := server.next()
	if shutdownReq.Method != "shutdown" {
		t.Fatalf("method = %q, want shutdown", shutdownReq.Method)
	}

	server.respond(shutdownReq.ID, nil)

	exitNotify := server.next()
	if exitNotify.Method != "exit" {
		t.Fatalf("method = %q, want exit", exitNotify.Method)
	}

	if err := <-closeDone; err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSession_Close_TimesOutAndKills(t *testing.T) {
	t.Parallel()

	session, server, killed := wedgedProcessSession(t)

	// A fake server that never answers "shutdown" stands in for an
	// unresponsive language server: Close must not hang on it forever.
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer closeCancel()

	closeDone := make(chan error, 1)
	go func() { closeDone <- session.Close(closeCtx) }()

	shutdownReq := server.next()
	if shutdownReq.Method != "shutdown" {
		t.Fatalf("method = %q, want shutdown", shutdownReq.Method)
	}
	// No server.respond: the fake server never answers, so Close blocks
	// until closeCtx's deadline fires, which makes call send its own
	// $/cancelRequest - drain it too or Close would block on cancel's timeout.
	cancelMsg := server.next()
	if cancelMsg.Method != "$/cancelRequest" {
		t.Fatalf("method = %q, want $/cancelRequest", cancelMsg.Method)
	}

	err := <-closeDone
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close: err = %v, want context.DeadlineExceeded", err)
	}

	select {
	case <-killed:
	default:
		t.Fatalf("Close: kill was not invoked after ctx timed out on an unresponsive server")
	}
}

// TestSession_Close_BoundsUndeadlinedContext proves round 3's finding: a
// ctx with no deadline (context.Background() is legal here) must not let
// Close hang forever behind an unresponsive "shutdown" - pre-fix code
// trusted the caller's ctx as-is and would block indefinitely here.
func TestSession_Close_BoundsUndeadlinedContext(t *testing.T) {
	t.Parallel()

	session, server, killed := wedgedProcessSession(t)

	closeDone := make(chan error, 1)
	go func() { closeDone <- session.Close(context.Background()) }()

	shutdownReq := server.next()
	if shutdownReq.Method != "shutdown" {
		t.Fatalf("method = %q, want shutdown", shutdownReq.Method)
	}
	// No server.respond: closeTimeout's internal ceiling is what eventually
	// fires here (same mechanism as TestSession_Close_TimesOutAndKills, but
	// with no caller-supplied bound at all), making call send its own
	// $/cancelRequest - drain it too or Close would block on cancel's timeout.
	cancelMsg := server.next()
	if cancelMsg.Method != "$/cancelRequest" {
		t.Fatalf("method = %q, want $/cancelRequest", cancelMsg.Method)
	}

	// Close must still return well before a generous outer bound.
	select {
	case err := <-closeDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Close: err = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(closeTimeout + 5*time.Second):
		t.Fatalf("Close: still blocked well past closeTimeout given an undeadlined ctx")
	}

	select {
	case <-killed:
	default:
		t.Fatalf("Close: kill was not invoked once closeTimeout fired on an unresponsive server")
	}
}

func TestStartSession_EmptyCommand(t *testing.T) {
	t.Parallel()

	_, err := StartSession(context.Background(), ServerConfig{})
	if !errors.Is(err, ErrEmptyCommand) {
		t.Fatalf("err = %v, want ErrEmptyCommand", err)
	}
}
