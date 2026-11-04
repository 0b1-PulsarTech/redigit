package app

import "example.com/routemod/router"

func Health(ctx *router.Ctx)    { ctx.Path = "health" }
func Ping(ctx *router.Ctx)      { ctx.Path = "ping" }
func ListUsers(ctx *router.Ctx) { ctx.Path = "users" }
func CreateUser(ctx *router.Ctx) {
	ctx.Path = "create-user"
}
func GetUser(ctx *router.Ctx)     { ctx.Path = "user" }
func ListOrders(ctx *router.Ctx)  { ctx.Path = "orders" }
func CreateOrder(ctx *router.Ctx) { ctx.Path = "create-order" }
func ListOrdersV2(ctx *router.Ctx) {
	ctx.Path = "orders-v2"
}
func Invoices(ctx *router.Ctx)  { ctx.Path = "invoices" }
func Receipts(ctx *router.Ctx)  { ctx.Path = "receipts" }
func Refunds(ctx *router.Ctx)   { ctx.Path = "refunds" }
func Dyn(ctx *router.Ctx)       { ctx.Path = "dyn" }
func Guarded(ctx *router.Ctx)   { ctx.Path = "guarded" }
func Auth(ctx *router.Ctx)      { ctx.Path = "auth" }
func Wrapped(ctx *router.Ctx)   { ctx.Path = "wrapped" }
func ViaFunc(ctx *router.Ctx)   { ctx.Path = "viafunc" }
func Chained(ctx *router.Ctx)   { ctx.Path = "chained" }
func Assigned(ctx *router.Ctx)  { ctx.Path = "assigned" }
func AssignedB(ctx *router.Ctx) { ctx.Path = "assigned-b" }
func Nested(ctx *router.Ctx)    { ctx.Path = "nested" }
func Stats(ctx *router.Ctx)     { ctx.Path = "stats" }
func Version(ctx *router.Ctx)   { ctx.Path = "version" }
func Metrics(ctx *router.Ctx)   { ctx.Path = "metrics" }
func Audit(ctx *router.Ctx)     { ctx.Path = "audit" }
