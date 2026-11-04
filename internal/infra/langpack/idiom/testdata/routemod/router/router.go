package router

type Ctx struct{ Path string }

type HandlerFunc func(*Ctx)

type Router struct {
	table  map[string][]HandlerFunc
	prefix string
}

func New() *Router { return &Router{table: map[string][]HandlerFunc{}} }

func (mux *Router) GET(path string, handlers ...HandlerFunc) {
	mux.add("GET", path, handlers)
}

func (mux *Router) POST(path string, handlers ...HandlerFunc) {
	mux.add("POST", path, handlers)
}

func (mux *Router) Route(prefix string, build func(*Router)) {
	build(mux.child(prefix))
}

func (mux *Router) Group(prefix string) *Router {
	return mux.child(prefix)
}

func (mux *Router) Mount(prefix string, mount func(*Router)) {
	mount(mux.child(prefix))
}

func (mux *Router) child(prefix string) *Router {
	return &Router{table: mux.table, prefix: mux.prefix + prefix}
}

func (mux *Router) add(method, path string, handlers []HandlerFunc) {
	mux.table[method+" "+mux.prefix+path] = handlers
}

func Wrap[Kind any](handler HandlerFunc) HandlerFunc {
	var zero Kind

	_ = zero

	return handler
}
