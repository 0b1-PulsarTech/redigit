package shaping

import "github.com/wrapped-owls/redigit/internal/domain/entities"

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=shaping

type Observer interface {
	Moved(pass string, move Move)
	Refused(pass string, added []entities.Edge)
}

type nopObserver struct{}

func (nopObserver) Moved(string, Move) {}

func (nopObserver) Refused(string, []entities.Edge) {}
