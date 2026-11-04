package app

import (
	"example.com/atk/billing"
	"example.com/atk/orders"
	"example.com/atk/router"
	"example.com/atk/users"
)

func Routes() *router.Router {
	mux := router.New()

	mux.Route("/api", func(api *router.Router) {
		users.Mount(api, users.Config{})
		orders.Mount(api, orders.Config{})
		billing.Mount(api, billing.Config{})
	})

	return mux
}
