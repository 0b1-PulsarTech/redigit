package subjecting

import (
	"strconv"
	"unicode/utf8"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const (
	KindDraft             Kind = "draft"
	KindUncovered         Kind = "uncovered"
	KindFormat            Kind = "format"
	KindDescriptionLength Kind = "description_length"
	KindLineLength        Kind = "line_length"
	KindDuplicate         Kind = "duplicate"
	KindWIPMarker         Kind = "wip_marker"
)

const NoPosition = -1

type (
	Limits struct {
		Description int
		Line        int
	}

	Finding struct {
		Position int    `json:"position"`
		Kind     Kind   `json:"kind"`
		Key      string `json:"key,omitempty"`
		Detail   string `json:"detail,omitempty"`
	}
)

func Check(rows []Row, subjects map[string]string, limits Limits) []Finding {
	slots := make([]Slot, 0, len(rows))
	for _, row := range rows {
		slots = append(slots, Slot{Position: row.Position, ID: ID(row.ChunkKeys, row.BlobPaths)})
	}

	hands, problems := Resolve(slots, subjects)
	findings := make([]Finding, 0, len(problems))

	for _, problem := range problems {
		findings = append(findings, Finding{
			Position: NoPosition, Kind: problem.Kind, Key: problem.Key,
		})
	}

	seen := make(map[string]int, len(rows))

	for idx, row := range rows {
		subject := row.Subject

		switch {
		case hands[idx].Found:
			subject = hands[idx].Subject
			findings = append(findings, checkHand(row, subject, limits)...)
		case entities.IsDraft(subject):
			findings = append(findings, Finding{Position: row.Position, Kind: KindUncovered})
		}

		if entities.IsDraft(subject) {
			continue
		}

		line := entities.UnmarkWIP(gitSubject(row.Written(subject)))
		if first, isDuplicate := seen[line]; isDuplicate {
			findings = append(findings, Finding{
				Position: row.Position, Kind: KindDuplicate, Key: strconv.Itoa(first),
			})

			continue
		}

		seen[line] = row.Position
	}

	return findings
}

func checkHand(row Row, subject string, limits Limits) []Finding {
	if entities.IsDraft(subject) {
		return []Finding{{Position: row.Position, Kind: KindDraft}}
	}

	var findings []Finding

	line := gitSubject(row.Written(subject))
	plain := entities.UnmarkWIP(gitSubject(subject))

	prefix, isFormatted := prefixLength(plain)
	if !isFormatted {
		return []Finding{{Position: row.Position, Kind: KindFormat}}
	}

	if !row.IsWIP && entities.IsWIPSubject(gitSubject(subject)) {
		findings = append(findings, Finding{Position: row.Position, Kind: KindWIPMarker})
	}

	if width := utf8.RuneCountInString(line); limits.Line > 0 && width > limits.Line {
		findings = append(findings, Finding{
			Position: row.Position, Kind: KindLineLength, Detail: strconv.Itoa(width),
		})
	}

	if width := utf8.RuneCountInString(line) - prefix; limits.Description > 0 &&
		width > limits.Description {
		findings = append(findings, Finding{
			Position: row.Position, Kind: KindDescriptionLength, Detail: strconv.Itoa(width),
		})
	}

	return findings
}
