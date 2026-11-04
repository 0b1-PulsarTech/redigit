package subjecting

const (
	TierIdentical   = "identical"
	TierReview      = "review"
	SimilarityFloor = 0.8
)

type (
	Match struct {
		From    int     `json:"from"`
		To      int     `json:"to"`
		ID      string  `json:"id"`
		Tier    string  `json:"tier"`
		Score   float64 `json:"score"`
		Subject string  `json:"subject"`
	}

	Lost struct {
		From    int    `json:"from"`
		Subject string `json:"subject"`
	}

	Carried struct {
		Identical []Match
		Review    []Match
		Lost      []Lost
		Problems  []Problem
		Sources   int
		Fresh     int
	}

	source struct {
		entry   Entry
		subject string
	}
)

func Carry(old []Entry, subjects map[string]string, current []Entry) Carried {
	slots := make([]Slot, 0, len(old))
	for _, entry := range old {
		slots = append(slots, Slot{Position: entry.Position, ID: entry.ID})
	}

	hands, problems := Resolve(slots, subjects)
	result := Carried{Problems: problems}

	var sources []source

	for idx, entry := range old {
		if hand := hands[idx]; hand.Found {
			subject := Unmark(hand.Subject)
			sources = append(sources, source{entry: entry, subject: subject})
		}
	}

	result.Sources = len(sources)

	claimed := make(map[int]bool, len(current))
	// Only an identical id is trusted; a near match is review.
	rest := matchIdentical(&result, sources, old, current, claimed)
	matchSimilar(&result, rest, current, claimed)

	result.Fresh = len(current) - len(result.Identical) - len(result.Review)

	return result
}

func matchIdentical(
	result *Carried,
	sources []source,
	old, current []Entry,
	claimed map[int]bool,
) []source {
	oldCount := countIDs(old)
	currentAt := make(map[string][]int, len(current))

	for idx, entry := range current {
		currentAt[entry.ID] = append(currentAt[entry.ID], idx)
	}

	var rest []source

	for _, src := range sources {
		at := currentAt[src.entry.ID]
		if len(at) != 1 || oldCount[src.entry.ID] != 1 {
			rest = append(rest, src)

			continue
		}

		claimed[at[0]] = true

		result.Identical = append(result.Identical, Match{
			From: src.entry.Position, To: current[at[0]].Position, ID: src.entry.ID,
			Tier: TierIdentical, Score: 1, Subject: src.subject,
		})
	}

	return rest
}

func countIDs(entries []Entry) map[string]int {
	counts := make(map[string]int, len(entries))
	for _, entry := range entries {
		counts[entry.ID]++
	}

	return counts
}
