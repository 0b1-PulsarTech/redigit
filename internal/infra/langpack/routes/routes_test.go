package routes

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestInProse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
		want []string
	}{
		{
			"a method and a path in code ticks",
			"`GET /conversations/{id}/messages` returns",
			[]string{"/conversations/{id}/messages"},
		},
		{"a query is dropped", "`GET /conversations?status=open`", []string{"/conversations"}},
		{"a trailing full stop is dropped", "see /roles/{id}.", []string{"/roles/{id}"}},
		{"a fraction is no path", "about 1/2 of them", nil},
		{
			"two paths",
			"`POST /templates` and `PUT /templates/{id}`",
			[]string{"/templates", "/templates/{id}"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := InProse(testCase.text); !slices.Equal(got, testCase.want) {
				t.Fatalf("InProse() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestGroupsOf(t *testing.T) {
	t.Parallel()

	settings := entities.DeclKey{File: "s.go", Name: "settings"}
	appearance := entities.DeclKey{File: "a.go", Name: "appearance"}
	groups := Groups{"/settings": {settings}, "/settings/appearance": {appearance}}

	testCases := []struct {
		route string
		want  []entities.DeclKey
	}{
		{"/settings/appearance/theme", []entities.DeclKey{appearance}},
		{"/settings/schedule", []entities.DeclKey{settings}},
		{"/settingsx", nil},
		{"/unknown", nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.route, func(t *testing.T) {
			t.Parallel()

			if got := groups.Of(testCase.route); !slices.Equal(got, testCase.want) {
				t.Fatalf("Of() = %v, want %v", got, testCase.want)
			}
		})
	}
}
