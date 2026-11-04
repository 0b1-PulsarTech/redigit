package router

import (
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type atomCase struct {
	name  string
	view  View
	group bool
	want  map[entities.DeclKey]string
}

func requireAtoms(t *testing.T, testCases []atomCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.view.atoms(Config{GroupMethods: testCase.group})
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("atoms() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestViewAtomsOfMounts(t *testing.T) {
	t.Parallel()

	first, third := stmtKey("Setup", "1"), stmtKey("Setup", "3")
	users, orders := fnKey("MountUsers"), fnKey("MountOrders")
	byUsers, byOrders := "go-router:"+users.String(), "go-router:"+orders.String()

	testCases := []atomCase{
		{name: "an empty view groups nothing", want: map[entities.DeclKey]string{}},
		{
			name: "a mount meets its first caller",
			view: View{Mounts: []Mount{{Fn: users, Caller: first}}},
			want: map[entities.DeclKey]string{users: byUsers, first: byUsers},
		},
		{
			name: "a mount called from a mount joins one atom with the caller above",
			view: View{Mounts: []Mount{{Fn: users, Caller: first}, {Fn: orders, Caller: users}}},
			want: map[entities.DeclKey]string{users: byOrders, first: byOrders, orders: byOrders},
		},
		{
			name: "a group and a mount sharing a statement are one atom",
			view: View{
				Mounts: []Mount{{Fn: users, Caller: third}},
				Routes: []Route{
					{Method: "GET", Path: "/u", Site: first},
					{Method: "POST", Path: "/u", Site: third},
				},
			},
			group: true,
			want:  map[entities.DeclKey]string{users: byUsers, first: byUsers, third: byUsers},
		},
	}

	requireAtoms(t, testCases)
}

func TestViewAtomsOfGroups(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Setup", "1"), stmtKey("Setup", "2")
	label := "go-router:" + first.String()
	pair := View{Routes: []Route{
		{Method: "GET", Path: "/u", Site: first},
		{Method: "POST", Path: "/u", Site: second},
	}}

	testCases := []atomCase{
		{
			name:  "statements of one path and two methods form the group atom",
			view:  pair,
			group: true,
			want:  map[entities.DeclKey]string{first: label, second: label},
		},
		{
			name: "groupMethods off leaves the statements alone",
			view: pair,
			want: map[entities.DeclKey]string{},
		},
		{
			name: "two paths are two resources",
			view: View{Routes: []Route{
				{Method: "GET", Path: "/u", Site: first},
				{Method: "GET", Path: "/v", Site: second},
			}},
			group: true,
			want:  map[entities.DeclKey]string{},
		},
		{
			name: "a blind path joins no group",
			view: View{Routes: []Route{
				{Method: "GET", Site: first}, {Method: "POST", Site: second},
			}},
			group: true,
			want:  map[entities.DeclKey]string{},
		},
	}

	requireAtoms(t, testCases)
}

func TestViewAtomsOfSites(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Setup", "1"), stmtKey("Setup", "2")
	elsewhere := entities.DeclKey{File: "api/other.go", Kind: entities.KindStmt, Name: "Setup#1"}

	testCases := []atomCase{
		{
			name: "one statement holding two routes is one site",
			view: View{Routes: []Route{
				{Method: "GET", Path: "/u", Site: first},
				{Method: "POST", Path: "/u", Site: first},
			}},
			group: true,
			want:  map[entities.DeclKey]string{},
		},
		{
			name: "statements of two functions are not grouped",
			view: View{Routes: []Route{
				{Method: "GET", Path: "/u", Site: first},
				{Method: "POST", Path: "/u", Site: elsewhere},
			}},
			group: true,
			want:  map[entities.DeclKey]string{},
		},
		{
			name: "a whole func registering the path is no statement",
			view: View{Routes: []Route{
				{Method: "GET", Path: "/u", Site: fnKey("Setup")},
				{Method: "POST", Path: "/u", Site: second},
			}},
			group: true,
			want:  map[entities.DeclKey]string{},
		},
	}

	requireAtoms(t, testCases)
}

func TestViewWithoutSharedCallers(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Setup", "1"), stmtKey("Setup", "2")
	users, orders := fnKey("MountUsers"), fnKey("MountOrders")
	alone := []Mount{{Fn: users, Caller: first}}
	sliced := []entities.Decl{
		{Key: first, Atom: "slice:1"}, {Key: second, Atom: "slice:1"},
	}
	oneResource := []Route{
		{Method: "GET", Path: "/a", Site: first},
		{Method: "POST", Path: "/a", Site: second},
	}
	apart := []Mount{{Fn: users, Caller: first}, {Fn: orders, Caller: second}}

	testCases := []struct {
		name   string
		mounts []Mount
		routes []Route
		cfg    Config
		decls  []entities.Decl
		want   []Mount
	}{
		{name: "a mount alone in its statement stays", mounts: alone, want: alone},
		{
			name:   "two mounts called by one statement both go",
			mounts: []Mount{{Fn: users, Caller: first}, {Fn: orders, Caller: first}},
		},
		{
			name:   "two mounts in statements of one slicer atom both go",
			mounts: apart,
			decls:  sliced,
		},
		{
			name:   "mounts in statements of two atoms stay",
			mounts: apart,
			want:   apart,
		},
		{
			name:   "a mount sharing a slicer atom with no other mount stays",
			mounts: alone,
			decls:  sliced,
			want:   alone,
		},
		{
			name:   "two mounts in statements of one resource both go",
			mounts: apart,
			routes: oneResource,
			cfg:    Config{GroupMethods: true},
		},
		{
			name:   "statements of one path stay apart without groupMethods",
			mounts: apart,
			routes: oneResource,
			want:   apart,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			view := View{Mounts: testCase.mounts, Routes: testCase.routes}

			got := view.withoutSharedCallers(testCase.cfg, testCase.decls).Mounts
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Mounts = %v, want %v", got, testCase.want)
			}
		})
	}
}
