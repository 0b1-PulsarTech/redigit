package fixture

// Stack is a generic LIFO container.
type Stack[T any] struct {
	items []T
}

// Push adds an item to the stack.
func (s *Stack[T]) Push(item T) {
	s.items = append(s.items, item)
}

// Pop removes and returns the top item.
func (s *Stack[T]) Pop() (T, bool) {
	var zero T

	if len(s.items) == 0 {
		return zero, false
	}

	last := len(s.items) - 1
	item := s.items[last]
	s.items = s.items[:last]

	return item, true
}

// Map applies f to every element of in.
func Map[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}

	return out
}
