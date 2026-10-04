package entities

// EdgeReason names why one declaration has to wait for another. Naming it is
// not decoration: every ordering defect in the rewrite that preceded this tool
// was an edge that existed in the code and nothing in the graph represented, so
// a cycle or a suspicious wait has to be explainable by the reason that made it.
type EdgeReason string

const (
	// ReasonReference is the language server answering who reads a symbol. It
	// replaces identifier matching, which put 24 of 53 destinations in the wrong
	// place: collisions on common names, mentions inside comments, and a
	// standard-library symbol matching a same-named one nothing ever calls.
	ReasonReference EdgeReason = "reference"
	// ReasonImplements is structural interface satisfaction, invisible to the
	// import graph: an implementation package need not import the package whose
	// interface it satisfies.
	ReasonImplements EdgeReason = "implements"
	// ReasonMethodSet is a value that reaches a method through a field or an
	// interface it is stored in, where no reference names the method's owner.
	ReasonMethodSet EdgeReason = "method-set"
	// ReasonGeneratedBy links generated output to the directive that produces
	// it. No reference runs that way: generated code never names its generator.
	ReasonGeneratedBy EdgeReason = "generated-by"
	// ReasonMirrors links a structural double to what it mirrors, recorded only
	// in a header comment the mock generator wrote.
	ReasonMirrors EdgeReason = "mirrors"
	// ReasonTestOf is a test waiting for the declaration it exercises.
	ReasonTestOf EdgeReason = "test-of"
	// ReasonEncloses keeps a value or constructor with the type it belongs to,
	// so a type and the constants that enumerate it are never split apart.
	ReasonEncloses EdgeReason = "encloses"
	// ReasonSchema places a migration after the schema a generator read to
	// produce it. Fixing only the query side pushed 9 of 12 migrations ahead of
	// their own source.
	ReasonSchema EdgeReason = "schema"
	// ReasonQuery places generated query code after the query text it came
	// from, which lives in a file the language graph does not cover at all.
	ReasonQuery EdgeReason = "query"
	// ReasonHosts keeps a declaration behind the manifest that makes its unit
	// importable at all: a module file, a workspace entry, a build file.
	ReasonHosts EdgeReason = "hosts"
	// ReasonDescribes keeps prose beside what it describes, for the case where
	// a document belongs to one unit - a package README - rather than to the
	// finished tree.
	ReasonDescribes EdgeReason = "describes"
	// ReasonModule ties code to the go.mod that makes its directory a module.
	// Without it nothing in the tree compiles, and no reference query sees it:
	// a package's declarations name their own module never, and the module
	// file names them never either.
	ReasonModule EdgeReason = "module"
	// ReasonWorkspace ties a go.work "use" line, and a go.mod "replace"
	// pointing at a local path, to the module directory it names. Go reads a
	// replaced directory even when nothing imports the module, so a replace
	// line landing before its target exists breaks every build until it does.
	ReasonWorkspace EdgeReason = "workspace"
	// ReasonImport ties a declaration to the go.mod require providing a package
	// its file imports. No reference names the module that ships a package, so
	// without it the first importer lands long before the require it needs.
	ReasonImport EdgeReason = "import"
	// ReasonChecksum ties a go.mod require to the go.sum holding the hashes the
	// go command verifies it against; without them the build stops on "missing
	// go.sum entry" even though the require is present.
	ReasonChecksum EdgeReason = "checksum"
	// ReasonEmbed ties a //go:embed declaration to a file its pattern matches.
	// The pattern is a string, not a reference, and go refuses a pattern that
	// matches nothing.
	ReasonEmbed EdgeReason = "embed"
	// ReasonProse holds back what is only true of the finished tree: general
	// documentation, a pipeline definition, an agent instruction. Written about
	// a whole that does not exist until the end, it belongs at the end.
	ReasonProse EdgeReason = "prose"
)

// Edge records that Dependent cannot be born before Provider. The field names
// carry the direction so no caller has to remember which end is which.
type Edge struct {
	Dependent DeclKey
	Provider  DeclKey
	Reason    EdgeReason
}

// Anchor pulls a declaration back toward another instead of merely keeping it
// behind. It is the second side of the test rule: "not before the function" is
// a floor, and with only the floor a package's whole suite arrived six hundred
// commits late. An Anchor lowers the base; the fixed point still delays it for
// anything it reads that is not there yet.
type Anchor struct {
	Decl   DeclKey
	To     DeclKey
	Reason EdgeReason
}
