package write

const Usage = `
  redigit write  --repo PATH --manifest FILE --ref REF --date-ref REV [flags]
      Replay an emit manifest as real commits on REF. Rows are sorted by
      position, every commit is authored and committed by the configured
      identity, and dates are sampled from the author dates of --date-ref,
      then forced strictly increasing, each keeping its own zone. The same
      input gives the same shas. Prints {"ref","commit","commits"}.
      --blob-dir DIR  handles are files at DIR/<handle>; without it they are
                      blob object ids already in the repository
      --subjects FILE JSON object of position -> full message
      --allow-draft   write subjects still marked DRAFT
      --verify REV    fail unless the written tree equals REV's tree; REF is
                      moved only after the check passes
      --author-name N --author-email E  identity (flags win over config)
      --config FILE   a redigit.json whose "write" section may hold
                      {"identity":{"name","email"},"dateRef","subjects"}
`
