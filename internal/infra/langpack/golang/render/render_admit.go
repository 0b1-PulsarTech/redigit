package render

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

var (
	errSpec   = errors.New("import spec does not parse")
	errSource = errors.New("source does not parse for imports")
)

func Needed(source []byte, text string) []string {
	used := qualifiers(text)
	if len(used) == 0 {
		return nil
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", source, parser.ImportsOnly)
	if err != nil {
		return nil
	}

	var specs []string

	for _, spec := range file.Imports {
		// A blank or dot import cannot be seen being used, and "C" needs its cgo preamble.
		if spec.Name != nil && (spec.Name.Name == "_" || spec.Name.Name == ".") {
			continue
		}

		if path, _ := strconv.Unquote(
			spec.Path.Value,
		); path == "C" ||
			!importIsUsed(spec, used) {
			continue
		}

		start, end := fset.Position(spec.Pos()).Offset, fset.Position(spec.End()).Offset
		specs = append(specs, string(source[start:end]))
	}

	return specs
}

func Admit(source []byte, specs []string) ([]byte, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", source, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSource, err)
	}

	held := map[string]bool{}
	for _, spec := range file.Imports {
		held[specKey(spec)] = true
	}

	var missing []string

	for _, text := range specs {
		parsed, parseErr := parser.ParseFile(
			token.NewFileSet(), "", "package p\nimport "+text, parser.ImportsOnly,
		)
		if parseErr != nil || len(parsed.Imports) != 1 {
			return nil, fmt.Errorf("%w: %q", errSpec, text)
		}

		if key := specKey(parsed.Imports[0]); !held[key] {
			held[key] = true

			missing = append(missing, strings.TrimSpace(text))
		}
	}

	if len(missing) == 0 {
		return source, nil
	}

	end := file.Name.End()
	if len(file.Decls) > 0 {
		end = file.Decls[len(file.Decls)-1].End()
	}

	// A name another import already uses stays a compile error, which the probe reports.
	at := fset.Position(end).Offset
	block := "\n\nimport (\n\t" + strings.Join(missing, "\n\t") + "\n)"

	return append(append(append([]byte{}, source[:at]...), block...), source[at:]...), nil
}

func specKey(spec *ast.ImportSpec) string {
	name := ""
	if spec.Name != nil {
		name = spec.Name.Name
	}

	return name + " " + spec.Path.Value
}

func qualifiers(text string) map[string]bool {
	// The text is a declaration, or one spec of a var or type group.
	for _, lead := range []string{"", "var ", "type "} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "", "package p\n"+lead+text, 0)
		if err != nil {
			continue
		}

		used := map[string]bool{}

		ast.Inspect(parsed, func(node ast.Node) bool {
			if sel, isSel := node.(*ast.SelectorExpr); isSel {
				if ident, isIdent := sel.X.(*ast.Ident); isIdent && ident.Obj == nil {
					used[ident.Name] = true
				}
			}

			return true
		})

		return used
	}

	return nil
}
