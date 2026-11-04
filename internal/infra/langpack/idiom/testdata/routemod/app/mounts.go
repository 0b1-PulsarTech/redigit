package app

import "example.com/routemod/router"

type Service struct{ name string }

func (svc *Service) List(ctx *router.Ctx) { ctx.Path = svc.name }

func MountUsers(mux *router.Router) {
	mux.GET("/users", ListUsers)
	mux.POST("/users", CreateUser)
	mux.GET("/users/{id}", GetUser)
}

func MountBilling(mux *router.Router) {
	mux.GET("/invoices", Invoices)
	mux.GET("/receipts", Receipts)
	mux.POST("/refunds", Refunds)
}

func MountAudit(mux *router.Router) {
	mux.GET("/audit", Audit)
}

func Unrelated(mux *router.Router) {
	mux.Group("/nothing")
}
