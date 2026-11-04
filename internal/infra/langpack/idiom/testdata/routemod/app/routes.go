package app

import "example.com/routemod/router"

const basePath = "/v1"

func Routes(prefix string) *router.Router {
	mux := router.New()
	svc := &Service{name: "svc"}

	mux.GET("/health", Health)
	mux.GET("/orders", ListOrders)
	mux.POST("/orders", CreateOrder)
	MountUsers(mux)
	mux.Mount("/billing", MountBilling)
	mux.Route("/api", func(api *router.Router) {
		api.GET("/ping", Ping)
	})
	mux.GET(prefix+"/dyn", Dyn)
	mux.GET("/inline", func(ctx *router.Ctx) { ctx.Path = "inline" })
	mux.GET("/svc", svc.List)
	mux.GET("/guarded", Auth, Guarded)
	mux.GET("/orders", ListOrdersV2)
	mux.GET(basePath+"/stats", Stats)
	mux.GET("/wrapped", router.Wrap[int](Wrapped))
	register := mux.GET
	register("/viafunc", ViaFunc)
	grouped := mux.Group("/g1")
	grouped.GET("/x", Assigned)
	other := mux.Group("/g2")
	other.GET("/x", AssignedB)
	mux.Group("/g3").GET("/x", Chained)
	mux.Route("/outer", func(outer *router.Router) {
		outer.Route("/inner", func(inner *router.Router) {
			inner.GET("/leaf", Nested)
		})
	})
	mux.GET("/version", Version)
	mux.GET("/metrics", Metrics)
	MountAudit(mux)

	return mux
}
