package parser

// Desugar percorre toda a AST convertendo laços `*ForStmt` em blocos léxicos `*BlockStmt`
// contendo a inicialização seguida de um `*WhileStmt` equivalente, preservando todos os Spans originais.
func Desugar(prog *Program) *Program {
	out, _ := DesugarWithCount(prog)
	return out
}

// DesugarWithCount executa o desaçucaramento e retorna também a quantidade de laços `for` transformados.
func DesugarWithCount(prog *Program) (*Program, int) {
	if prog == nil {
		return nil, 0
	}
	count := 0
	decls := make([]Stmt, len(prog.Decls))
	for i, d := range prog.Decls {
		decls[i] = desugarStmt(d, &count)
	}
	return &Program{
		Decls:    decls,
		SpanInfo: prog.SpanInfo,
	}, count
}

func desugarStmt(stmt Stmt, count *int) Stmt {
	if stmt == nil {
		return nil
	}

	switch s := stmt.(type) {
	case *FnDecl:
		return &FnDecl{
			Name:       s.Name,
			NameSpan:   s.NameSpan,
			Params:     s.Params,
			ReturnType: s.ReturnType,
			Body:       desugarBlock(s.Body, count),
			SpanInfo:   s.SpanInfo,
		}

	case *BlockStmt:
		return desugarBlock(s, count)

	case *IfStmt:
		var elseDesugared Stmt
		if s.Else != nil {
			elseDesugared = desugarStmt(s.Else, count)
		}
		return &IfStmt{
			Cond:     s.Cond,
			Then:     desugarBlock(s.Then, count),
			Else:     elseDesugared,
			SpanInfo: s.SpanInfo,
		}

	case *WhileStmt:
		return &WhileStmt{
			Cond:     s.Cond,
			Body:     desugarBlock(s.Body, count),
			SpanInfo: s.SpanInfo,
		}

	case *ForStmt:
		return desugarFor(s, count)

	default:
		// VarDecl, ConstDecl, AssignStmt, ReturnStmt e ExprStmt não contêm sub-blocos
		return s
	}
}

func desugarBlock(b *BlockStmt, count *int) *BlockStmt {
	if b == nil {
		return nil
	}
	stmts := make([]Stmt, len(b.Stmts))
	for i, s := range b.Stmts {
		stmts[i] = desugarStmt(s, count)
	}
	return &BlockStmt{
		Stmts:          stmts,
		IsDesugaredFor: b.IsDesugaredFor,
		SpanInfo:       b.SpanInfo,
	}
}

// desugarFor aplica a equivalência formal:
//
//	for (Init; Cond; Step) { Body }
//	  ===>
//	{
//	    Init;
//	    while (Cond) {
//	        Body...
//	        Step;
//	    }
//	}
func desugarFor(f *ForStmt, count *int) *BlockStmt {
	*count++

	bodyDesugared := desugarBlock(f.Body, count)

	whileStmts := make([]Stmt, 0, len(bodyDesugared.Stmts)+1)
	whileStmts = append(whileStmts, bodyDesugared.Stmts...)
	if f.Step != nil {
		whileStmts = append(whileStmts, desugarStmt(f.Step, count))
	}

	whileBody := &BlockStmt{
		Stmts:    whileStmts,
		SpanInfo: bodyDesugared.SpanInfo,
	}

	cond := f.Cond
	if cond == nil {
		cond = &BoolLit{
			Value:    true,
			SpanInfo: f.SpanInfo,
		}
	}

	whileNode := &WhileStmt{
		Cond:     cond,
		Body:     whileBody,
		SpanInfo: f.SpanInfo,
	}

	outerStmts := make([]Stmt, 0, 2)
	if f.Init != nil {
		outerStmts = append(outerStmts, desugarStmt(f.Init, count))
	}
	outerStmts = append(outerStmts, whileNode)

	return &BlockStmt{
		Stmts:          outerStmts,
		IsDesugaredFor: true,
		SpanInfo:       f.SpanInfo,
	}
}
