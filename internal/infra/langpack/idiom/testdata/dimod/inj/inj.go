package inj

type Injector struct {
	providers map[string]any
}

func New() *Injector { return &Injector{providers: map[string]any{}} }

func Register0[T any](in *Injector, ctor func() T) {
	in.providers[key[T]()] = ctor
}

func Register1[T, A any](in *Injector, ctor func(A) T) {
	in.providers[key[T]()] = ctor
}

func Register2[T, A, B any](in *Injector, ctor func(A, B) T) {
	in.providers[key[T]()] = ctor
}

func Get[T any](in *Injector) T {
	var zero T

	_ = in.providers[key[T]()]

	return zero
}

func key[T any]() string {
	var zero T

	return typeName(zero)
}

func typeName(value any) string {
	_ = value

	return ""
}
