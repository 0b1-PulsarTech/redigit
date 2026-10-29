package fixture

//go:generate mockgen -source=generate_directive.go -destination=generate_directive_mock.go

// Source defines the mockable surface.
type Source interface {
	Do() error
}

// Helper is unrelated to the directive above.
func Helper() int {
	return 1
}
