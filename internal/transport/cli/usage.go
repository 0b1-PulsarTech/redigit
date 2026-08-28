package cli

const usage = `redigit rebuilds a repository's history so every commit creates exactly what it needs.

  redigit audit  --repo PATH [--rev REF] [--tags TAGS] [--far N]
      Index, order, and report. Says which declarations are born far from
      whatever reads them, what each number does not look at, and which files
      a build constraint hid from the reference queries.

  redigit emit   --repo PATH [--rev REF] [--tags TAGS] [--blob-dir DIR]
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

  redigit plan   --repo PATH [--rev REF] [--tags TAGS]
      Emit a commit plan, cut along features where a feature owns the work and
      along layers where none does. Subjects come out marked DRAFT: nothing
      writes a history from an unreviewed plan.

Flags:
  --repo PATH   repository to read (required)
  --rev  REF    revision holding the tree to read (default HEAD)
  --tags TAGS   comma-separated build tags. Passing none is a real choice: a
                file behind a build constraint answers a syntactic query but is
                invisible to reference queries, so its declarations reach the
                graph with no edges and look exactly like code nothing reads.
                Every command now names those files on stderr, and audit lists
                them under "unseen", so the choice is at least reported.
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
