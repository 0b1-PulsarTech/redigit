package subjects

const Usage = `
  redigit subjects digest --manifest FILE
      One JSON line per commit of an emit manifest: position, id, the draft
      subject, shape, feature, reason, is_wip, is_draft, risks, units, and the
      chunk keys and blob paths the id is made of. A subject that is not
      DRAFT needs no work. id hashes the sorted chunk keys then the sorted
      blob paths (first 16 hex digits); spans are left out because lines move.
  redigit subjects check --manifest FILE --subjects FILE [flags]
      Check a subjects file (position or id:<hex> -> full message) against the
      manifest. One JSON line per finding: draft, uncovered, format,
      description_length, line_length, duplicate, wip_marker, stale_key,
      unknown_id, ambiguous_id, key_conflict. Exit is non-zero on any finding.
      --max-description N  limit after "type(scope): ", counting [wip] (72)
      --max-line N         limit of line one (100); 0 turns a limit off
      The emoji no-repeat rule is per project and is not checked here.
      A wip row gets "[wip] " first in the description when written (the slot,
      after DRAFT:, :emoji: and type(scope): ; the scope may hold spaces), so
      it is measured with it; "[wip] " in the slot, or in the old place ahead
      of the type or the emoji, on a row that is not wip is refused. The same
      text anywhere else is words. check reads the subject as git shows it
      (lines up to the first blank, joined by a space). Duplicates compare
      with the marker left out. Keys are canonical positions ("7", not "07").
  redigit subjects carry --from DIGEST --subjects FILE --manifest FILE --out FILE [flags]
      After a re-plan, move hand subjects from the old digest to the new
      manifest. identical (same id) goes to --out keyed id:<hex>. A unique
      best Jaccard >= 0.8 is "review": it goes to --review, never to --out.
      Anything else stays DRAFT. Prints a JSON report with counts. Any key
      problem (stale, unknown, ambiguous, conflict) refuses like check: exit 1,
      the report names them, nothing is written. Every "[wip] " in the slot
      (and the old place ahead of the type or emoji) is stripped from carried
      values: write puts one on rows that are wip. Words elsewhere are kept.
      --review FILE        subjects of the review tier, same shape as --out
`
