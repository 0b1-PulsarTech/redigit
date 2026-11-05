package lsp

// Position is a zero-based line/character pair. Character is a code-unit
// offset in whatever encoding the server negotiated (see
// Session.PositionEncoding); it is never a byte offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open [Start, End) span of positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location addresses a range within one file by URI, the shape every
// textDocument/* result in this subset is built from.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier names the file a request targets.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentPositionParams is the common shape of a request targeting one
// position in one file: documentSymbol excepted, every method in this
// subset embeds it.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// GeneralClientCapabilities declares the position encodings this client can
// consume. gopls ignores it and always uses UTF-16 regardless (verified);
// other servers honor it, so it is sent for their sake.
type GeneralClientCapabilities struct {
	PositionEncodings []string `json:"positionEncodings,omitempty"`
}

// DocumentSymbolClientCapabilities requests the hierarchical DocumentSymbol
// shape (Range/SelectionRange/Children). Without it a server may fall back
// to flat SymbolInformation, which this package's DocumentSymbol type does
// not model - declaring it is what keeps that fallback from happening
// silently.
type DocumentSymbolClientCapabilities struct {
	HierarchicalDocumentSymbolSupport bool `json:"hierarchicalDocumentSymbolSupport"`
}

type TextDocumentClientCapabilities struct {
	DocumentSymbol DocumentSymbolClientCapabilities `json:"documentSymbol"`
}

// ClientCapabilities is the minimal capabilities object this client
// advertises: hierarchical document symbols and UTF-16 position encoding.
// It does not declare definitionLinkSupport/implementationLinkSupport, so a
// spec-conformant server answers definition/implementation with Location or
// Location[], never LocationLink[].
type ClientCapabilities struct {
	General      GeneralClientCapabilities      `json:"general"`
	TextDocument TextDocumentClientCapabilities `json:"textDocument"`
}

// InitializeParams is the initialize request body. InitializationOptions is
// sent as a flat map[string]any: gopls (and this is undocumented behavior
// this client relies on) splits any key containing "." and keeps only the
// last segment, so a nested object like {"build":{"buildFlags":[...]}}
// never reaches the setting it looks like it targets.
type InitializeParams struct {
	ProcessID             int                `json:"processId"`
	RootURI               string             `json:"rootUri"`
	Capabilities          ClientCapabilities `json:"capabilities"`
	InitializationOptions map[string]any     `json:"initializationOptions,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities carries only the one field this client reads back:
// which position encoding the server actually chose.
type ServerCapabilities struct {
	PositionEncoding string `json:"positionEncoding,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DocumentSymbol is one entry of a hierarchical textDocument/documentSymbol
// result. Detail carries a method's receiver (e.g. "func (d *Dog) Sound()")
// but gopls never nests a method under its receiver type here - every
// DocumentSymbol in the returned tree is either a top-level declaration or a
// struct field/nested type, never a method-under-type grouping. Associating
// a method with its receiver from Detail is the caller's job.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type ReferenceParams struct {
	TextDocumentPositionParams

	Context ReferenceContext `json:"context"`
}

// SymbolKind is the LSP-defined symbol taxonomy (3.17 SymbolKind enum).
type SymbolKind int

const (
	SymbolKindFile          SymbolKind = 1
	SymbolKindModule        SymbolKind = 2
	SymbolKindNamespace     SymbolKind = 3
	SymbolKindPackage       SymbolKind = 4
	SymbolKindClass         SymbolKind = 5
	SymbolKindMethod        SymbolKind = 6
	SymbolKindProperty      SymbolKind = 7
	SymbolKindField         SymbolKind = 8
	SymbolKindConstructor   SymbolKind = 9
	SymbolKindEnum          SymbolKind = 10
	SymbolKindInterface     SymbolKind = 11
	SymbolKindFunction      SymbolKind = 12
	SymbolKindVariable      SymbolKind = 13
	SymbolKindConstant      SymbolKind = 14
	SymbolKindString        SymbolKind = 15
	SymbolKindNumber        SymbolKind = 16
	SymbolKindBoolean       SymbolKind = 17
	SymbolKindArray         SymbolKind = 18
	SymbolKindObject        SymbolKind = 19
	SymbolKindKey           SymbolKind = 20
	SymbolKindNull          SymbolKind = 21
	SymbolKindEnumMember    SymbolKind = 22
	SymbolKindStruct        SymbolKind = 23
	SymbolKindEvent         SymbolKind = 24
	SymbolKindOperator      SymbolKind = 25
	SymbolKindTypeParameter SymbolKind = 26
)
