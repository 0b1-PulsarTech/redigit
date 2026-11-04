package router

type Ctx struct{ Path string }

type HandlerFunc func(*Ctx)

type Reg struct{ Name string }

type Router struct {
	table  map[string][]HandlerFunc
	prefix string
}

func New() *Router { return &Router{table: map[string][]HandlerFunc{}} }

func (mux *Router) GET(path string, handlers ...HandlerFunc) *Reg {
	return mux.add("GET", path, handlers)
}

func (mux *Router) POST(path string, handlers ...HandlerFunc) *Reg {
	return mux.add("POST", path, handlers)
}

func (mux *Router) Route(prefix string, build func(*Router)) {
	build(mux.child(prefix))
}

func (mux *Router) Mount(prefix string, build func(*Router)) {
	build(mux.child(prefix))
}

func (mux *Router) child(prefix string) *Router {
	return &Router{table: mux.table, prefix: mux.prefix + prefix}
}

func (mux *Router) add(method, path string, handlers []HandlerFunc) *Reg {
	mux.table[method+" "+mux.prefix+path] = handlers

	return &Reg{}
}

func Named(name string) HandlerFunc {
	return func(ctx *Ctx) { ctx.Path = name }
}
