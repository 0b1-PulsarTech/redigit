package atlassum

import "testing"

func TestDerive(t *testing.T) {
	t.Parallel()

	inputs := map[string][]byte{
		"database/migrations/0002_b.sql": []byte("CREATE TABLE \"b\" ();\n"),
		"database/migrations/0001_a.sql": []byte("-- a\nCREATE TABLE \"a\" ();\n"),
	}

	want := "h1:bwF19B5Y9sfEKbVY7RGMWQhJc5pLQY2dCFlDE9X3IBo=\n" +
		"0001_a.sql h1:p79wCdh6ByrVmpecwgO4RRTAWzX0R0RuCfotUFwVCsk=\n" +
		"0002_b.sql h1:bnTXdVCWiQblHUrQgwL6NUYhBHm+uoG2sZ7brlwvPTE=\n"

	got, err := Deriver{}.Derive("database/migrations/atlas.sum", inputs)
	if err != nil || string(got) != want {
		t.Fatalf("Derive = %q, %v; want %q", got, err, want)
	}
}

func TestOutputAndFeeds(t *testing.T) {
	t.Parallel()

	d := Deriver{}
	out := "database/migrations/atlas.sum"

	tests := []struct {
		name         string
		output, feed string
		isOut, feeds bool
	}{
		{"sum", out, "database/migrations/0001.sql", true, true},
		{"other dir", out, "database/queries/a.sql", true, false},
		{"not sql", out, "database/migrations/embed.go", true, false},
		{"not output", "database/migrations/embed.go", "", false, false},
	}

	for _, tt := range tests {
		if got := d.Output(tt.output); got != tt.isOut {
			t.Errorf("%s: Output = %v", tt.name, got)
		}

		if tt.feed != "" && d.Feeds(tt.output, tt.feed) != tt.feeds {
			t.Errorf("%s: Feeds = %v", tt.name, !tt.feeds)
		}
	}
}
