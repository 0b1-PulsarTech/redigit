package auditing

// Unclaimed is one final-tree path no language pack's extensions match. Unlike
// Unseen, which is read and contributes edgeless declarations, an unclaimed path
// is never read at all, so no plan from this run can reproduce it.
type Unclaimed struct {
	File string
}
