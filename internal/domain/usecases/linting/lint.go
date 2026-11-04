package linting

type totals struct {
	source    int
	generated int
	all       int
}

func Check(rows []Row, limits Limits) Report {
	report := Report{Rows: len(rows), Measures: describe(limits)}

	for _, row := range rows {
		sums := sum(row)
		report.Findings = append(report.Findings, judge(row, sums, limits)...)
	}

	for index := range report.Measures {
		for _, finding := range report.Findings {
			if finding.Measure == report.Measures[index].Name {
				report.Measures[index].Count++
			}
		}
	}

	return report
}

func sum(row Row) totals {
	var sums totals

	for _, file := range row.Files {
		added := max(file.Added, 0)
		sums.all += added

		switch file.Kind {
		case KindSource:
			sums.source += added
		case KindGenerated:
			sums.generated += added
		case KindOther:
		}
	}

	return sums
}

func judge(row Row, sums totals, limits Limits) []Finding {
	var found []Finding

	add := func(measure string, value, limit int) {
		found = append(found, Finding{
			Measure:  measure,
			Position: row.Position,
			Subject:  row.Subject,
			Value:    value,
			Limit:    limit,
		})
	}

	if sums.source > limits.SourceAdded {
		add(MeasureSourceAdded, sums.source, limits.SourceAdded)
	}

	if sums.generated > limits.GeneratedMixed && sums.source > 0 {
		add(MeasureGeneratedMixed, sums.generated, limits.GeneratedMixed)
	}

	if sums.all > limits.RowAdded {
		add(MeasureRowAdded, sums.all, limits.RowAdded)
	}

	return found
}
