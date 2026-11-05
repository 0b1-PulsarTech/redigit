package cli

const usage = `redigit rebuilds a repository's history so every commit creates exactly what it needs.

  redigit audit  --repo PATH [--rev REF] [--tags TAGS] [--far N]
      Index, order, and report. Says which declarations are born far from
      whatever reads them, what each number does not look at, and which files
      a build constraint left out of the main build.

  redigit emit   --repo PATH [--rev REF] [--tags TAGS] [--blob-dir DIR]
                 [--state-dir DIR]
      Materialise the plan: one JSON object per commit with the declarations
      born there, the full content of every file it changes, and the forward
      references it can see. Writes no commit and no ref - that is the calling
      agent's to do. Blobs go into the repository's object database unless
      --blob-dir names a directory to write them to instead.

  redigit chunks --repo PATH [--rev REF]
      One JSON object per line, per piece of every file a chunker claims: a
      sqlc "-- name:" block, a go.mod require or replace line, a go.work use
      line. born_at is the position of the published history that first held
      the piece, read by re-cutting the file at each commit that touched it,
      and -1 for a piece that history never held. Starts no language server.

  redigit report --repo PATH [--rev REF] [--tags TAGS]
      One JSON object per line, per declaration: where it was born in this
      history, every reader outside its own unit with the reason, and the gap
      to the nearest one. Birth is read per declaration, not per file.

  redigit graph  --repo PATH [--rev REF] [--tags TAGS]
      Emit the declaration graph: every declaration and every edge, with the
      reason each edge exists.

  redigit plan   --repo PATH [--rev REF] [--tags TAGS] [--state-dir DIR]
      Emit a commit plan, cut along features where a feature owns the work and
      along layers where none does. Subjects come out marked DRAFT: nothing
      writes a history from an unreviewed plan.

Flags:
  --repo PATH   repository to read (required)
  --rev  REF    revision holding the tree to read (default HEAD)
  --tags TAGS   comma-separated build tags. Passing none is a real choice: a
                file behind a build constraint is left out of the main build.
                The typed resolver checks it in an extra pass and, if it
                type-checks, answers its reference queries; under --resolver
                lsp it may not, and its declarations reach the graph with no
                edges, like code nothing reads. Every command names those
                files on stderr, and audit lists them under "unseen", so the
                choice is at least reported.
  --config FILE a redigit.json: per-language tags and virtual modules, plus
                rules and a slice section. A flag replaces the file's whole
                setting for it (a --virtual drops every virtual module the
                file lists). A file that names an unknown rule type, a file
                glob matching nothing in the tree, or a slice section it
                cannot read is refused before the language server starts; a
                declaration glob matching nothing (a pin target, a
                slice.functions glob) fails after indexing.

                  {"languages": {
                    "go":  {"tags": ["integration"]},
                    "web": {"virtual": {"virtual:x": "web/x.dart"}}}}

                The slice section cuts a function into the statements it is
                made of, each born with what it reads. Go only.

                  {"slice": {"functions": ["Mount*", "Server.Run"],
                             "errcheck": true}}

                "functions" are globs on declaration names. A method is
                spelled Type.Method: Run matches the function Run and *.Run
                the methods, and a bare name matches in every package and
                module. Generated and test-side functions are never cut, and a
                function the slicer cannot cut safely (a label or goto, an
                early return, a body of one statement, a name that occurs
                twice in its file such as init) is left whole. A glob that
                cuts nothing fails the run after indexing, rc 1, naming the
                glob. "errcheck" is off by default; on, an "err = ..." is born
                with the next statement that reads err. An unknown key, a null
                section or a bad glob is refused with rc 2 before anything
                starts. verify checks compile and vet, not runtime, and a cut
                is not proven safe in general: docs/PLANO.md, section 3.11.
  --resolver R  how reference queries are answered: typed (default) answers
                from the type-check the rules already ran and starts the
                language server only for a query it cannot cover; lsp starts
                the server first and asks it everything.
  --mode M      what the plan's order follows: history (default) seeds each
                declaration at the commit that first touched its file;
                lineage orders by the commit that first held the declaration
                and still exists, following renames. It scans the history
                once, so it costs seconds more; a cached plan skips that.
  --far  N      positions a declaration may sit ahead of its first outside
                reader before audit reports it (default 25)
  --layers FILE which path prefixes come first when the graph is indifferent.
                One rule per line, the layer number then the path prefix, "#"
                starting a comment; lower layers are born first. It is a
                tiebreak and never a constraint: a dependency always wins, so
                a layer only chooses among placements the graph allows.

                  1  libs/tereckernel
                  4  database        # imports nothing, so it can go early
                  6  libs/infra      # persists module entities: after them

  --virtual ID=PATH the file of the tree a bundler plugin compiles the module
                ID from, such as virtual:app-client=web/tool/client_entry.dart.
                No import names that file, so without this a component reading
                the module waits only for the .d.ts declaring it. Repeatable.

  --features FILE the vertical slices a layer may reorder inside. One slice
                per line: the set of positions it covers, then the name. A
                comma separates, a hyphen marks an inclusive range, so a
                capability the history built in distant pieces is one slice
                without swallowing what lies between. WITHOUT this,
                a layer reaches across the whole history and orders every
                kernel declaration before every engine, which builds the
                repository horizontally and leaves it with no working product
                until the end. A feature says what gets built; a layer says
                in which order that feature's own pieces are born.

                  0-12         bootstrap
                  17,258,575   tags de lead   # schema, domain, use case
                  243-245      agendamento    # engine and module together
`

const runUsage = `
Run flags (every command):
  --cpuprofile FILE  write a CPU profile of the whole run; read it with
                     go tool pprof
  --memprofile FILE  write a heap profile when the run ends
  --trace FILE       write a runtime execution trace; read it with
                     go tool trace
  --mem-limit SIZE   soft memory limit for the Go runtime, in GOMEMLIMIT
                     syntax: 512MiB, 2GiB, or off
  --timings          print one JSON line on stderr with the milliseconds each
                     stage took, {"timings":{"graph":48912.5}}; the stages are
                     start, key, births, graph, order, plan, audit, emit, close
                     and total
  --from-cache       reuse the graph, ordering and plan an earlier run saved for
                     the same commit, flags, binary and language server, and
                     save them after a run that found none. graph and plan then
                     start nothing; emit, audit and report start the pipeline
                     only when they need it. A corrupt entry is ignored.
                     Entries live under the user cache dir, redigit/plancache.
`
