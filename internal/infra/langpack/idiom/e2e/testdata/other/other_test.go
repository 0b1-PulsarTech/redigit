package other

import "testing"

func get(url string) string { return url }

func TestHealth(t *testing.T) { _ = get("/health") }
