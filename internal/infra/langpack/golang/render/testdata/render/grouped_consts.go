package fixture

// Weekday is a day of the week.
type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
	Wednesday
)

const (
	DefaultTimeout = 5
	MaxRetries     = 3
)
