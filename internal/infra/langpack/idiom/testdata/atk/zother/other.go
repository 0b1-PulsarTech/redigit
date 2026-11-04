package zother

import (
	"example.com/atk/router"
	"example.com/atk/users"
)

func Early() {
	users.Mount(router.New(), users.Config{})
}
