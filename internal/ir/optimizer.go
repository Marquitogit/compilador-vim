package ir

import (
	"math"
	"strconv"

	"vim/internal/diagnostics"
	"vim/internal/lexer"
	"vim/internal/parser"
)

// OptStats contabiliza as transformações aplicadas pelo passe `--opt` para exibição em `--stats`.
type OptStats struct {
	ConstsFolded     int // Expressões constantes dobradas e constantes propagadas
	DeadNodesRemoved int // Comandos inalcançáveis após return e ramos mortos (if false / while false)
}

// Total retorna o total combinado de otimizações efetuadas.
func (s OptStats) Total() int {
	return s.ConstsFolded + s.DeadNodesRemoved
}

type constEnv struct {
	scopes []map[string]parser.Expr // nil indica variável/parâmetro mutável que sombreia eventual `const` externa
}

func newConstEnv() *constEnv {
	return &constEnv{
		scopes: []map[string]parser.Expr{make(map[string]parser.Expr)},
	}
}

func (e *constEnv) pushScope() {
	e.scopes = append(e.scopes, make(map[string]parser.Expr))
}

func (e *constEnv) popScope() {
	if len(e.scopes) > 1 {
		e.scopes = e.scopes[:len(e.scopes)-1]
	}
}

func (e *constEnv) bindConst(name string, lit parser.Expr) {
	e.scopes[len(e.scopes)-1][name] = lit
}

func (e *constEnv) shadowMutable(name string) {
	e.scopes[len(e.scopes)-1][name] = nil
}

func (e *constEnv) lookup(name string) parser.Expr {
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if val, exists := e.scopes[i][name]; exists {
			return val
		}
	}
	return nil
}

// Optimize executa Constant Folding seguro (incluindo propagação de `const` escalares)
// e Dead Code Elimination (simplificação de `if(true)`/`if(false)`/`while(false)` e remoção pós-`return`).
func Optimize(prog *parser.Program) (*parser.Program, OptStats) {
	if prog == nil {
		return nil, OptStats{}
	}

	opt := &astOptimizer{
		env: newConstEnv(),
	}

	finalDecls := make([]parser.Stmt, 0, len(prog.Decls))
	for _, decl := range prog.Decls {
		if s := opt.optStmt(decl); s != nil {
			finalDecls = append(finalDecls, s)
		}
	}

	return &parser.Program{
		Decls:    finalDecls,
		SpanInfo: prog.SpanInfo,
	}, opt.stats
}

type astOptimizer struct {
	env   *constEnv
	stats OptStats
}

func (o *astOptimizer) optStmt(stmt parser.Stmt) parser.Stmt {
	if stmt == nil {
		return nil
	}

	switch s := stmt.(type) {
	case *parser.FnDecl:
		o.env.pushScope()
		for _, p := range s.Params {
			o.env.shadowMutable(p.Name)
		}
		body := o.optBlockInCurrentScope(s.Body)
		o.env.popScope()
		return &parser.FnDecl{
			Name:       s.Name,
			NameSpan:   s.NameSpan,
			Params:     s.Params,
			ReturnType: s.ReturnType,
			Body:       body,
			SpanInfo:   s.SpanInfo,
		}

	case *parser.VarDecl:
		val := o.optExpr(s.Value)
		o.env.shadowMutable(s.Name)
		return &parser.VarDecl{
			Name:     s.Name,
			NameSpan: s.NameSpan,
			Type:     s.Type,
			Value:    val,
			SpanInfo: s.SpanInfo,
		}

	case *parser.ConstDecl:
		val := o.optExpr(s.Value)
		if isScalarLiteral(val) {
			o.env.bindConst(s.Name, val)
		} else {
			o.env.shadowMutable(s.Name)
		}
		return &parser.ConstDecl{
			Name:     s.Name,
			NameSpan: s.NameSpan,
			Type:     s.Type,
			Value:    val,
			SpanInfo: s.SpanInfo,
		}

	case *parser.BlockStmt:
		o.env.pushScope()
		res := o.optBlockInCurrentScope(s)
		o.env.popScope()
		return res

	case *parser.AssignStmt:
		return &parser.AssignStmt{
			Target:   o.optLValue(s.Target),
			Value:    o.optExpr(s.Value),
			SpanInfo: s.SpanInfo,
		}

	case *parser.IfStmt:
		cond := o.optExpr(s.Cond)
		if bLit, ok := cond.(*parser.BoolLit); ok {
			o.stats.DeadNodesRemoved++
			if bLit.Value {
				// `if (true)` simplifica para o bloco `then` preservando o isolamento de escopo
				return o.optStmt(s.Then)
			}
			// `if (false)` elimina o ramo `then` e preserva apenas `else` (se existir)
			if s.Else != nil {
				return o.optStmt(s.Else)
			}
			return nil
		}

		thenBlk := o.optStmt(s.Then).(*parser.BlockStmt)
		var elseBranch parser.Stmt
		if s.Else != nil {
			elseBranch = o.optStmt(s.Else)
		}
		return &parser.IfStmt{
			Cond:     cond,
			Then:     thenBlk,
			Else:     elseBranch,
			SpanInfo: s.SpanInfo,
		}

	case *parser.WhileStmt:
		cond := o.optExpr(s.Cond)
		if bLit, ok := cond.(*parser.BoolLit); ok && !bLit.Value {
			// `while (false)` nunca executa
			o.stats.DeadNodesRemoved++
			return nil
		}
		body := o.optStmt(s.Body).(*parser.BlockStmt)
		return &parser.WhileStmt{
			Cond:     cond,
			Body:     body,
			SpanInfo: s.SpanInfo,
		}

	case *parser.ReturnStmt:
		var val parser.Expr
		if s.Value != nil {
			val = o.optExpr(s.Value)
		}
		return &parser.ReturnStmt{
			Value:    val,
			SpanInfo: s.SpanInfo,
		}

	case *parser.ExprStmt:
		return &parser.ExprStmt{
			Expr:     o.optExpr(s.Expr),
			SpanInfo: s.SpanInfo,
		}

	default:
		return stmt
	}
}

func (o *astOptimizer) optBlockInCurrentScope(b *parser.BlockStmt) *parser.BlockStmt {
	if b == nil {
		return nil
	}
	stmts := make([]parser.Stmt, 0, len(b.Stmts))
	for i, st := range b.Stmts {
		optimized := o.optStmt(st)
		if optimized != nil {
			stmts = append(stmts, optimized)
			// Dead Code Elimination: comandos após um retorno garantido no mesmo bloco são inalcançáveis
			if stmtAlwaysReturns(optimized) && i+1 < len(b.Stmts) {
				o.stats.DeadNodesRemoved += len(b.Stmts) - (i + 1)
				break
			}
		}
	}
	return &parser.BlockStmt{
		Stmts:          stmts,
		IsDesugaredFor: b.IsDesugaredFor,
		SpanInfo:       b.SpanInfo,
	}
}

func (o *astOptimizer) optLValue(target parser.Expr) parser.Expr {
	if idx, ok := target.(*parser.IndexExpr); ok {
		return &parser.IndexExpr{
			Collection: o.optLValue(idx.Collection),
			Index:      o.optExpr(idx.Index),
			SpanInfo:   idx.SpanInfo,
		}
	}
	return target
}

func (o *astOptimizer) optExpr(expr parser.Expr) parser.Expr {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *parser.IdentExpr:
		if lit := o.env.lookup(e.Name); lit != nil {
			o.stats.ConstsFolded++
			return cloneLiteralWithSpan(lit, e.SpanInfo)
		}
		return e

	case *parser.ArrayLit:
		elems := make([]parser.Expr, len(e.Elements))
		for i, el := range e.Elements {
			elems[i] = o.optExpr(el)
		}
		return &parser.ArrayLit{
			Elements: elems,
			SpanInfo: e.SpanInfo,
		}

	case *parser.IndexExpr:
		return &parser.IndexExpr{
			Collection: o.optExpr(e.Collection),
			Index:      o.optExpr(e.Index),
			SpanInfo:   e.SpanInfo,
		}

	case *parser.CallExpr:
		args := make([]parser.Expr, len(e.Args))
		for i, a := range e.Args {
			args[i] = o.optExpr(a)
		}
		return &parser.CallExpr{
			Callee:   e.Callee,
			Args:     args,
			SpanInfo: e.SpanInfo,
		}

	case *parser.UnaryExpr:
		op := o.optExpr(e.Operand)
		switch e.Op {
		case lexer.TokenBang:
			if b, ok := op.(*parser.BoolLit); ok {
				o.stats.ConstsFolded++
				return &parser.BoolLit{Value: !b.Value, SpanInfo: e.SpanInfo}
			}
		case lexer.TokenMinus:
			// Proteção contra overflow: -math.MinInt64 excede int64 positivo
			if n, ok := op.(*parser.IntLit); ok && n.Value != math.MinInt64 {
				o.stats.ConstsFolded++
				val := -n.Value
				return &parser.IntLit{Value: val, Raw: strconv.FormatInt(val, 10), SpanInfo: e.SpanInfo}
			}
		}
		return &parser.UnaryExpr{
			Op:       e.Op,
			OpLexeme: e.OpLexeme,
			Operand:  op,
			SpanInfo: e.SpanInfo,
		}

	case *parser.BinaryExpr:
		left := o.optExpr(e.Left)
		right := o.optExpr(e.Right)

		// 1. Curto-circuito algébrico em tempo de compilação
		if e.Op == lexer.TokenAnd {
			if lb, ok := left.(*parser.BoolLit); ok {
				o.stats.ConstsFolded++
				if !lb.Value {
					return &parser.BoolLit{Value: false, SpanInfo: e.SpanInfo}
				}
				right.SetSpan(e.SpanInfo)
				return right
			}
		}
		if e.Op == lexer.TokenOr {
			if lb, ok := left.(*parser.BoolLit); ok {
				o.stats.ConstsFolded++
				if lb.Value {
					return &parser.BoolLit{Value: true, SpanInfo: e.SpanInfo}
				}
				right.SetSpan(e.SpanInfo)
				return right
			}
		}

		// 2. Dobramento aritmético e relacional de inteiros
		if li, okL := left.(*parser.IntLit); okL {
			if ri, okR := right.(*parser.IntLit); okR {
				if folded := foldIntBinary(e.Op, li.Value, ri.Value, e.SpanInfo); folded != nil {
					o.stats.ConstsFolded++
					return folded
				}
			}
		}

		// Simplificações algébricas neutras seguras (preservam avaliação de efeitos colaterais de `expr`):
		// x + 0 -> x, 0 + x -> x, x - 0 -> x, x * 1 -> x, 1 * x -> x
		if ri, okR := right.(*parser.IntLit); okR {
			if (e.Op == lexer.TokenPlus || e.Op == lexer.TokenMinus) && ri.Value == 0 {
				o.stats.ConstsFolded++
				left.SetSpan(e.SpanInfo)
				return left
			}
			if e.Op == lexer.TokenStar && ri.Value == 1 {
				o.stats.ConstsFolded++
				left.SetSpan(e.SpanInfo)
				return left
			}
		}
		if li, okL := left.(*parser.IntLit); okL {
			if e.Op == lexer.TokenPlus && li.Value == 0 {
				o.stats.ConstsFolded++
				right.SetSpan(e.SpanInfo)
				return right
			}
			if e.Op == lexer.TokenStar && li.Value == 1 {
				o.stats.ConstsFolded++
				right.SetSpan(e.SpanInfo)
				return right
			}
		}

		// 3. Dobramento de strings (concatenação e igualdade)
		if ls, okL := left.(*parser.StringLit); okL {
			if rs, okR := right.(*parser.StringLit); okR {
				switch e.Op {
				case lexer.TokenPlus:
					o.stats.ConstsFolded++
					joined := ls.Value + rs.Value
					return &parser.StringLit{Value: joined, Raw: strconv.Quote(joined), SpanInfo: e.SpanInfo}
				case lexer.TokenEq:
					o.stats.ConstsFolded++
					return &parser.BoolLit{Value: ls.Value == rs.Value, SpanInfo: e.SpanInfo}
				case lexer.TokenNotEq:
					o.stats.ConstsFolded++
					return &parser.BoolLit{Value: ls.Value != rs.Value, SpanInfo: e.SpanInfo}
				}
			}
		}

		// 4. Dobramento de igualdade booleana
		if lb, okL := left.(*parser.BoolLit); okL {
			if rb, okR := right.(*parser.BoolLit); okR {
				switch e.Op {
				case lexer.TokenEq:
					o.stats.ConstsFolded++
					return &parser.BoolLit{Value: lb.Value == rb.Value, SpanInfo: e.SpanInfo}
				case lexer.TokenNotEq:
					o.stats.ConstsFolded++
					return &parser.BoolLit{Value: lb.Value != rb.Value, SpanInfo: e.SpanInfo}
				}
			}
		}

		return &parser.BinaryExpr{
			Left:     left,
			Op:       e.Op,
			OpLexeme: e.OpLexeme,
			Right:    right,
			SpanInfo: e.SpanInfo,
		}

	default:
		return expr
	}
}

func foldIntBinary(op lexer.TokenType, a, b int64, span diagnostics.Span) parser.Expr {
	switch op {
	case lexer.TokenPlus:
		v := a + b
		return &parser.IntLit{Value: v, Raw: strconv.FormatInt(v, 10), SpanInfo: span}
	case lexer.TokenMinus:
		v := a - b
		return &parser.IntLit{Value: v, Raw: strconv.FormatInt(v, 10), SpanInfo: span}
	case lexer.TokenStar:
		v := a * b
		return &parser.IntLit{Value: v, Raw: strconv.FormatInt(v, 10), SpanInfo: span}
	case lexer.TokenSlash:
		// NUNCA dobra divisão por 0 nem MinInt64 / -1 (preserva para diagnóstico em runtime)
		if b == 0 || (a == math.MinInt64 && b == -1) {
			return nil
		}
		v := a / b
		return &parser.IntLit{Value: v, Raw: strconv.FormatInt(v, 10), SpanInfo: span}
	case lexer.TokenPercent:
		// NUNCA dobra módulo por 0 nem MinInt64 % -1
		if b == 0 || (a == math.MinInt64 && b == -1) {
			return nil
		}
		v := a % b
		return &parser.IntLit{Value: v, Raw: strconv.FormatInt(v, 10), SpanInfo: span}
	case lexer.TokenLt:
		return &parser.BoolLit{Value: a < b, SpanInfo: span}
	case lexer.TokenLtEq:
		return &parser.BoolLit{Value: a <= b, SpanInfo: span}
	case lexer.TokenGt:
		return &parser.BoolLit{Value: a > b, SpanInfo: span}
	case lexer.TokenGtEq:
		return &parser.BoolLit{Value: a >= b, SpanInfo: span}
	case lexer.TokenEq:
		return &parser.BoolLit{Value: a == b, SpanInfo: span}
	case lexer.TokenNotEq:
		return &parser.BoolLit{Value: a != b, SpanInfo: span}
	default:
		return nil
	}
}

func isScalarLiteral(e parser.Expr) bool {
	switch e.(type) {
	case *parser.IntLit, *parser.BoolLit, *parser.StringLit:
		return true
	default:
		return false
	}
}

func cloneLiteralWithSpan(lit parser.Expr, sp diagnostics.Span) parser.Expr {
	switch v := lit.(type) {
	case *parser.IntLit:
		return &parser.IntLit{Value: v.Value, Raw: v.Raw, SpanInfo: sp}
	case *parser.BoolLit:
		return &parser.BoolLit{Value: v.Value, SpanInfo: sp}
	case *parser.StringLit:
		return &parser.StringLit{Value: v.Value, Raw: v.Raw, SpanInfo: sp}
	default:
		return lit
	}
}

func stmtAlwaysReturns(stmt parser.Stmt) bool {
	switch s := stmt.(type) {
	case *parser.ReturnStmt:
		return true
	case *parser.BlockStmt:
		for _, ch := range s.Stmts {
			if stmtAlwaysReturns(ch) {
				return true
			}
		}
		return false
	case *parser.IfStmt:
		return s.Else != nil && stmtAlwaysReturns(s.Then) && stmtAlwaysReturns(s.Else)
	default:
		return false
	}
}
