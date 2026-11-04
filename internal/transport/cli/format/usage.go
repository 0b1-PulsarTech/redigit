package format

const Usage = `
  redigit format --manifest FILE --out FILE --blob-dir DIR --out-blob-dir DIR --config FILE [--strict]
      Format the intermediate blobs of an emit manifest with the external
      formatters of the config's "format" section, before write. The last
      version of each path is never formatted, so the final tree stays what
      the source holds. Only blobs[].handle changes; every other field is
      copied. The input manifest, config and blob dir are never written: the
      new manifest goes to --out and every handle it names to --out-blob-dir.
      A usage error refuses, before any work and with symlinks resolved, an
      --out that is an input file or lies in either blob dir, and blob dirs
      that are the same, nested or one inside the other.
      A blob a formatter fails on stays raw with a warning, or fails the verb
      under --strict. A path no language of the config claims passes through.
      A cancelled run fails and publishes nothing.
      Formatting intermediates while the last version stays raw means a
      formatter that disagrees with the source's own style adds churn at the
      commit holding each path's last version: the formatter should match the
      source's own style.
      Prints one JSON line: blobs, handles, claimed, changed, unchanged,
      failed, passed_through, final_left_alone. blobs counts the blob spots
      of all rows and final_left_alone those holding a path's last version;
      handles counts distinct handles. The rest count distinct (handle, path)
      units of regular, non-final blobs, so they do not sum to blobs: claimed
      are those handed to the engine, including ones a skip marker or if_wider
      leaves alone, and changed + unchanged + failed = claimed; passed_through
      are those no language claims.
`
