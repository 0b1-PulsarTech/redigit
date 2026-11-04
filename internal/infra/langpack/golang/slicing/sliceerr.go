package slicing

import (
	"go/ast"
	"go/token"
	"go/types"
)

func (cut bodyCut) joinReassigns(flow bodyFlow, set groups) {
	for index, stmt := range cut.stmts {
		assign, isAssign := stmt.(*ast.AssignStmt)
		if !isAssign || assign.Tok != token.ASSIGN {
			continue
		}

		for obj := range flow.writes[index] {
			if obj.Name() == "err" {
				joinNextReader(flow, set, index, obj)
			}
		}
	}
}

func joinNextReader(flow bodyFlow, set groups, writer int, obj types.Object) {
	for index := writer + 1; index < len(flow.reads); index++ {
		if flow.reads[index][obj] {
			set.join(writer, index)

			return
		}
	}
}
