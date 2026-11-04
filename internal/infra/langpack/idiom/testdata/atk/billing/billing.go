package billing

import "example.com/atk/router"

type Config struct{ Prefix string }

func (cfg Config) withDefaults() Config {
	if cfg.Prefix == "" {
		cfg.Prefix = "/billing"
	}

	return cfg
}

func H1(ctx *router.Ctx) { ctx.Path = "billing1" }
func H2(ctx *router.Ctx) { ctx.Path = "billing2" }
func H3(ctx *router.Ctx) { ctx.Path = "billing3" }
func H4(ctx *router.Ctx) { ctx.Path = "billing4" }
func H5(ctx *router.Ctx) { ctx.Path = "billing5" }

func Mount(mux *router.Router, cfg Config) {
	cfg = cfg.withDefaults()

	mux.GET(cfg.Prefix+"/1", H1)
	mux.GET(cfg.Prefix+"/2", H2)
	mux.GET(cfg.Prefix+"/3", H3)
	mux.GET(cfg.Prefix+"/4", H4)
	mux.GET(cfg.Prefix+"/5", H5)
}
