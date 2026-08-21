package entities

import (
	"slices"
	"testing"
)

func keysOf(edges []Edge) []string {
	out := make([]string, 0, len(edges))
	for _, edge := range edges {
		out = append(out, edge.Dependent.String())
	}

	slices.Sort(out)

	return out
}

func sampleGraph() Graph {
	repoInsert := Decl{
		Key:  DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"},
		Unit: "repo",
	}
	repoHelper := Decl{
		Key:  DeclKey{File: "repo/lead.go", Kind: KindFunc, Name: "helper"},
		Unit: "repo",
	}
	port := Decl{
		Key:  DeclKey{File: "usecases/lead.go", Kind: KindType, Name: "Repository"},
		Unit: "usecases",
	}

	return NewGraph(
		[]Decl{repoInsert, repoHelper, port},
		[]Edge{
			{Dependent: repoInsert.Key, Provider: port.Key, Reason: ReasonImplements},
			{Dependent: repoHelper.Key, Provider: repoInsert.Key, Reason: ReasonReference},
			{Dependent: repoInsert.Key, Provider: repoHelper.Key, Reason: ReasonReference},
		},
		nil,
	)
}

func TestGraphNeeds(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()

	testCases := []struct {
		name string
		key  DeclKey
		want int
	}{
		{
			"a declaration that reads two things waits for both",
			DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"},
			2,
		},
		{
			"a provider nobody in this graph feeds waits for nothing",
			DeclKey{File: "usecases/lead.go", Kind: KindType, Name: "Repository"},
			0,
		},
		{"unknown key", DeclKey{File: "nope.go", Kind: KindFunc, Name: "X"}, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := len(graph.Needs(testCase.key)); got != testCase.want {
				t.Fatalf("Needs() = %d edges, want %d", got, testCase.want)
			}
		})
	}
}

func TestGraphExternalConsumers(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()
	insert := DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"}
	port := DeclKey{File: "usecases/lead.go", Kind: KindType, Name: "Repository"}

	testCases := []struct {
		name string
		key  DeclKey
		want []string
	}{
		{
			"a unit's own declarations are not outside readers",
			insert,
			nil,
		},
		{
			"a reader in another unit counts",
			port,
			[]string{insert.String()},
		},
		{"unknown key", DeclKey{File: "nope.go"}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := keysOf(graph.ExternalConsumers(testCase.key))
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("ExternalConsumers() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestGraphConsumersCountsInsiders(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()
	insert := DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"}

	// Consumers is the unfiltered count on purpose. Using it where the
	// external one belongs is what pinned every package to the instant the
	// source history happened to create it.
	if got := len(graph.Consumers(insert)); got != 1 {
		t.Fatalf("Consumers() = %d, want 1", got)
	}

	if got := len(graph.ExternalConsumers(insert)); got != 0 {
		t.Fatalf("ExternalConsumers() = %d, want 0", got)
	}
}

func TestGraphDecl(t *testing.T) {
	t.Parallel()

	graph := sampleGraph()

	testCases := []struct {
		name      string
		key       DeclKey
		wantFound bool
		wantUnit  UnitID
	}{
		{
			"known",
			DeclKey{File: "repo/lead.go", Kind: KindMethod, Name: "Repo.Insert"},
			true,
			"repo",
		},
		{"unknown", DeclKey{File: "nope.go", Kind: KindFunc, Name: "X"}, false, ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decl, found := graph.Decl(testCase.key)
			if found != testCase.wantFound {
				t.Fatalf("Decl() found = %v, want %v", found, testCase.wantFound)
			}

			if decl.Unit != testCase.wantUnit {
				t.Fatalf("Decl() unit = %q, want %q", decl.Unit, testCase.wantUnit)
			}
		})
	}
}
