package infra

// Blank imports pin the adapter dependencies here so a parallel change to one
// adapter never has to touch the module file and collide with another.
import (
	_ "github.com/dave/dst"
	_ "github.com/scip-code/scip/bindings/go/scip"
	_ "golang.org/x/sync/errgroup"
	_ "google.golang.org/protobuf/proto"
)
