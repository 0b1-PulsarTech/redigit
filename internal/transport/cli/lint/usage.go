package lint

const (
	Usage = `
  redigit lint --manifest FILE [--blob-dir DIR] [--config FILE] [--fail-over]
               [--repo R [--rev REV] [--tags TAGS] [--mode MODE] [--resolver R] [--from-cache]]
      Measure the shape of an emit manifest and print one JSON line per
      finding and a final {"summary":true,...} line. It reports; it never
      changes a plan. Exit 0 whenever it ran, whatever the measures, unless
      --fail-over is given (then exit 1 when any finding exists, or when
      nothing was measured: no blob read and no --repo; an unused_at_birth
      finding counts only when it is never read or first read more than the
      stub gap rows on: the rest is printed with "notCounted":true). A manifest with no row
      is refused. It decides nothing else. These numbers are this scanner's,
      not parity with terectek. A manifest records bytes, not lines, so lines
      are counted from the blobs in --blob-dir (handles are files at
      DIR/<handle>); without it no line measure runs and the summary says so.
      The measure is net line growth: a path adds its lines now minus its lines
      in the last row that held it (never below 0), so a rewrite at equal
      length adds 0. Generated is the Code generated marker on the blob, not
      the directory.
      Measures that need only the manifest (each summary entry lists what it
      does not see):
        source_added_over  source lines (not generated) grown by a row > 900
        generated_mixed    generated lines grown by a row > 900, row also grew source
        rows_over          lines grown by a row over every path > 850
      With --repo the graph of that revision is built by the same session as
      plan (--from-cache reuses its cache, --resolver as there; --mode reaches
      only the cache key, it does not change what is measured) and five more
      run. Give --config the file the manifest was emitted with: its slice
      section changes the graph. A manifest the graph does not match is a usage
      error that names it, when it holds a chunk the graph does not know, holds
      one twice, or places only some, or none, of a function the graph cuts.
      These are not detected: an altered span, an altered unit, and a manifest
      emitted under other build tags (it measures as if it matched). No
      layers, features, chunk map, server or virtual flag here.
        shells_born_empty    a cut function whose head row holds none of its statements
        shells_without_tail  a cut function whose last statement is not in the head's row
        stub_gap             a cut function whose rows span more than 20 (last - first)
        errcheck_split       an err assignment and its if err != nil in different rows
        unused_at_birth      a production declaration read by nothing born in its row or earlier
      Only Go has a slicer: a repo of another language reports 0 cut functions
      and says so. Without --repo these five are listed as not measured, under
      the names errcheck_pairs_split, shells_without_tail, shells_born_empty,
      unused_at_birth and born_as_stubs_gap_over_20_rows (stub_gap and
      errcheck_split above are the same measures; the 20 is the stub gap limit).
      --config FILE  a redigit.json whose "lint" section may hold
                     {"sourceAdded","generatedMixed","rowAdded","stubGap"}
                     (positive ints)
`
)
