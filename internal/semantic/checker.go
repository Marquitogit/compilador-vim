package semantic

import (
	"fmt"

	"vim/internal/diagnostics"
	"vim/internal/lexer"
	"vim/internal/parser"
)

// Códigos padronizados de diagnósticos semânticos (E02xx).
const (
	ErrUndeclared        = "E0201"
	ErrRedeclared        = "E0202"
	ErrTypeMismatch      = "E0203"
	ErrInvalidVoid       = "E0204"
	ErrImmutableAssign   = "E0205"
	ErrInvalidCall       = "E0206"
	ErrReturnOutsideFn   = "E0207"
	ErrMissingReturn     = "E0208"
	ErrAmbiguousEmptyArr = "E0209"
	ErrNotIndexable      = "E0210"
)

// Result encapsula todos os artefatos produzidos pela Análise Semântica.
type Result struct {
	GlobalScope *Scope
	ExprTypes   map[parser.Expr]Type
	ScopeCount  int
	SymbolCount int
}

// Checker executa a validação semântica estática em duas passadas sobre a AST desaçucarada.
type Checker struct {
	diag          *diagnostics.Bag
	global        *Scope
	exprTypes     map[parser.Expr]Type
	currentFnName string
	currentFnRet  Type // nil quando fora de qualquer função (top-level)
}

// Check executa a Análise Semântica completa sobre um `*parser.Program`.
func Check(prog *parser.Program, diag *diagnostics.Bag) *Result {
	if diag == nil {
		diag = diagnostics.NewBag()
	}
	c := &Checker{
		diag:      diag,
		global:    NewScope("global", nil),
		exprTypes: make(map[parser.Expr]Type),
	}
	c.registerIntrinsics()

	if prog != nil {
		// Passada 1: Coleta todas as assinaturas de funções top-level (suporte a recursão mútua)
		c.collectTopLevelFunctions(prog)

		// Passada 2: Valida corpos de funções e comandos/declarações globais
		for _, decl := range prog.Decls {
			c.checkTopLevelDecl(decl)
		}
	}

	return &Result{
		GlobalScope: c.global,
		ExprTypes:   c.exprTypes,
		ScopeCount:  c.global.CountScopes(),
		SymbolCount: c.global.CountUserSymbols(),
	}
}

func (c *Checker) registerIntrinsics() {
	intrinsics := []struct {
		name string
		sig  string
	}{
		{"print", "fn(v1: any, ...) -> void"},
		{"read", "fn() -> string"},
		{"read_int", "fn() -> int"},
		{"len", "fn(x: string | []T) -> int"},
		{"push", "fn(arr: []T, elem: T) -> void"},
	}
	for _, in := range intrinsics {
		_, _ = c.global.Define(&Symbol{
			Name:      in.name,
			Kind:      KindIntrinsic,
			Type:      &IntrinsicType{Name: in.name, Signature: in.sig},
			DefSpan:   diagnostics.Span{File: "<nativo>"},
			IsMutable: false,
		})
	}
}

// ============================================================================
// Passada 1: Registro Antecipado de Funções Top-Level
// ============================================================================

func (c *Checker) collectTopLevelFunctions(prog *parser.Program) {
	for _, decl := range prog.Decls {
		fn, ok := decl.(*parser.FnDecl)
		if !ok {
			continue
		}

		paramTypes := make([]Type, len(fn.Params))
		for i, p := range fn.Params {
			paramTypes[i] = c.resolveTypeNode(p.Type, false, "parâmetro")
		}
		retType := c.resolveTypeNode(fn.ReturnType, true, "retorno de função")

		sym := &Symbol{
			Name:      fn.Name,
			Kind:      KindFn,
			Type:      &FnType{Params: paramTypes, Return: retType},
			DefSpan:   fn.NameSpan,
			IsMutable: false,
		}
		if existing, ok := c.global.Define(sym); !ok {
			c.reportRedeclaration(fn.Name, fn.NameSpan, existing)
		}
	}
}

// ============================================================================
// Passada 2: Verificação de Declarações, Escopos e Comandos
// ============================================================================

func (c *Checker) checkTopLevelDecl(stmt parser.Stmt) {
	if fn, ok := stmt.(*parser.FnDecl); ok {
		c.checkFnBody(fn)
		return
	}
	c.checkStmt(stmt, c.global)
}

func (c *Checker) checkFnBody(fn *parser.FnDecl) {
	fnScope := NewScope(fmt.Sprintf("fn %s", fn.Name), c.global)

	// Recupera o tipo de retorno já registrado na Passada 1 (ou resolve localmente se houve duplicata)
	var retType Type
	if sym, ok := c.global.LookupLocal(fn.Name); ok {
		if ft, okFn := sym.Type.(*FnType); okFn && sym.DefSpan == fn.NameSpan {
			retType = ft.Return
		}
	}
	if retType == nil {
		retType = c.resolveTypeInternal(fn.ReturnType, true, "retorno de função", false)
	}

	for _, p := range fn.Params {
		pType := c.resolveTypeInternal(p.Type, false, "parâmetro", false)
		pSym := &Symbol{
			Name:      p.Name,
			Kind:      KindParam,
			Type:      pType,
			DefSpan:   p.NameSpan,
			IsMutable: true,
		}
		if existing, ok := fnScope.Define(pSym); !ok {
			c.reportRedeclaration(p.Name, p.NameSpan, existing)
		}
	}

	prevFnName := c.currentFnName
	prevFnRet := c.currentFnRet
	c.currentFnName = fn.Name
	c.currentFnRet = retType
	defer func() {
		c.currentFnName = prevFnName
		c.currentFnRet = prevFnRet
	}()

	// Os comandos da raiz da função compartilham o escopo dos parâmetros (impedindo redeclarar parâmetro na raiz)
	if fn.Body != nil {
		for _, st := range fn.Body.Stmts {
			c.checkStmt(st, fnScope)
		}
		if !retType.IsError() && !retType.Equals(TypeVoid) && !guaranteesReturn(fn.Body) {
			c.diag.Error(
				ErrMissingReturn,
				fn.NameSpan,
				fmt.Sprintf("a função %q declara retorno %q, mas nem todos os caminhos de execução retornam um valor", fn.Name, retType),
				fmt.Sprintf("retorno do tipo %q não garantido", retType),
				"adicione um 'return ...;' final ou garanta que ambos os ramos 'if' e 'else' retornem",
			)
		}
	}
}

func (c *Checker) checkStmt(stmt parser.Stmt, scope *Scope) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *parser.VarDecl:
		declType := c.resolveTypeNode(s.Type, false, "variável")
		valType := c.checkExpr(s.Value, scope, declType)

		if !declType.IsError() && !valType.IsError() && !declType.Equals(valType) {
			c.diag.Error(
				ErrTypeMismatch,
				s.Value.Span(),
				fmt.Sprintf("tipos incompatíveis na inicialização de %q: esperado %q, mas encontrado %q", s.Name, declType, valType),
				fmt.Sprintf("expressão tem tipo %q, esperado %q", valType, declType),
				fmt.Sprintf("ajuste o valor atribuído ou altere o tipo declarado de %q", s.Name),
			)
		}

		sym := &Symbol{
			Name:      s.Name,
			Kind:      KindVar,
			Type:      declType,
			DefSpan:   s.NameSpan,
			IsMutable: true,
		}
		if existing, ok := scope.Define(sym); !ok {
			c.reportRedeclaration(s.Name, s.NameSpan, existing)
		}

	case *parser.ConstDecl:
		declType := c.resolveTypeNode(s.Type, false, "constante")
		valType := c.checkExpr(s.Value, scope, declType)

		if !declType.IsError() && !valType.IsError() && !declType.Equals(valType) {
			c.diag.Error(
				ErrTypeMismatch,
				s.Value.Span(),
				fmt.Sprintf("tipos incompatíveis na constante %q: esperado %q, mas encontrado %q", s.Name, declType, valType),
				fmt.Sprintf("expressão tem tipo %q, esperado %q", valType, declType),
				"",
			)
		}

		sym := &Symbol{
			Name:      s.Name,
			Kind:      KindConst,
			Type:      declType,
			DefSpan:   s.NameSpan,
			IsMutable: false,
		}
		if existing, ok := scope.Define(sym); !ok {
			c.reportRedeclaration(s.Name, s.NameSpan, existing)
		}

	case *parser.BlockStmt:
		label := fmt.Sprintf("block L%d", s.Span().StartLine)
		if s.IsDesugaredFor {
			label = fmt.Sprintf("for-scope L%d", s.Span().StartLine)
		}
		blockScope := NewScope(label, scope)
		for _, child := range s.Stmts {
			c.checkStmt(child, blockScope)
		}

	case *parser.AssignStmt:
		c.checkAssignStmt(s, scope)

	case *parser.IfStmt:
		condType := c.checkExpr(s.Cond, scope, TypeBool)
		if !condType.IsError() && !condType.Equals(TypeBool) {
			c.diag.Error(
				ErrTypeMismatch,
				s.Cond.Span(),
				fmt.Sprintf("a condição do 'if' deve ser 'bool', mas foi encontrado %q", condType),
				fmt.Sprintf("tipo %q não é booleano", condType),
				"utilize uma comparação explícita, ex: 'if (x != 0)'",
			)
		}
		c.checkStmt(s.Then, scope)
		if s.Else != nil {
			c.checkStmt(s.Else, scope)
		}

	case *parser.WhileStmt:
		condType := c.checkExpr(s.Cond, scope, TypeBool)
		if !condType.IsError() && !condType.Equals(TypeBool) {
			c.diag.Error(
				ErrTypeMismatch,
				s.Cond.Span(),
				fmt.Sprintf("a condição do laço deve ser 'bool', mas foi encontrado %q", condType),
				fmt.Sprintf("tipo %q não é booleano", condType),
				"a condição deve produzir 'true' ou 'false', ex: 'i < 10'",
			)
		}
		c.checkStmt(s.Body, scope)

	case *parser.ForStmt:
		// Suporte direto caso o Checker seja invocado sobre uma AST com `--no-desugar`
		forScope := NewScope(fmt.Sprintf("for-scope L%d", s.Span().StartLine), scope)
		if s.Init != nil {
			c.checkStmt(s.Init, forScope)
		}
		if s.Cond != nil {
			condType := c.checkExpr(s.Cond, forScope, TypeBool)
			if !condType.IsError() && !condType.Equals(TypeBool) {
				c.diag.Error(
					ErrTypeMismatch,
					s.Cond.Span(),
					fmt.Sprintf("a condição do 'for' deve ser 'bool', mas foi encontrado %q", condType),
					fmt.Sprintf("tipo %q não é booleano", condType),
					"",
				)
			}
		}
		if s.Step != nil {
			c.checkStmt(s.Step, forScope)
		}
		c.checkStmt(s.Body, forScope)

	case *parser.ReturnStmt:
		c.checkReturnStmt(s, scope)

	case *parser.ExprStmt:
		_ = c.checkExpr(s.Expr, scope, nil)
	}
}

func (c *Checker) checkAssignStmt(s *parser.AssignStmt, scope *Scope) {
	// Verifica mutabilidade da raiz do L-Value (cobre tanto `MAX = 1` quanto `arrConst[0] = 1` ou `matConst[0][1] = 1`)
	if root := c.rootSymbol(s.Target, scope); root != nil && !root.IsMutable {
		if root.Kind == KindConst {
			msg := fmt.Sprintf("não é possível reatribuir a constante imutável %q", root.Name)
			if _, isIdx := s.Target.(*parser.IndexExpr); isIdx {
				msg = fmt.Sprintf("não é possível modificar elementos da constante imutável %q", root.Name)
			}
			c.diag.Error(
				ErrImmutableAssign,
				s.Target.Span(),
				msg,
				"alvo declarado como 'const' (imutável)",
				fmt.Sprintf("%q foi declarada como 'const' na linha %d, coluna %d; use 'let' se precisar modificá-la",
					root.Name, root.DefSpan.StartLine, root.DefSpan.StartCol),
			)
		} else {
			c.diag.Error(
				ErrImmutableAssign,
				s.Target.Span(),
				fmt.Sprintf("não é possível atribuir valor a %q (%s não é atribuível)", root.Name, root.Kind),
				"alvo não mutável",
				"",
			)
		}
	}

	targetType := c.checkExpr(s.Target, scope, nil)
	valType := c.checkExpr(s.Value, scope, targetType)

	if !targetType.IsError() && !valType.IsError() && !targetType.Equals(valType) {
		c.diag.Error(
			ErrTypeMismatch,
			s.Value.Span(),
			fmt.Sprintf("tipos incompatíveis na atribuição: o alvo espera %q, mas o valor tem tipo %q", targetType, valType),
			fmt.Sprintf("encontrado %q, esperado %q", valType, targetType),
			"",
		)
	}
}

func (c *Checker) checkReturnStmt(s *parser.ReturnStmt, scope *Scope) {
	if c.currentFnRet == nil {
		c.diag.Error(
			ErrReturnOutsideFn,
			s.Span(),
			"comando 'return' fora de função não é permitido",
			"'return' no escopo global",
			"mova este 'return' para dentro do corpo de uma função 'fn'",
		)
		if s.Value != nil {
			_ = c.checkExpr(s.Value, scope, nil)
		}
		return
	}

	if c.currentFnRet.Equals(TypeVoid) {
		if s.Value != nil {
			valType := c.checkExpr(s.Value, scope, nil)
			if !valType.IsError() {
				c.diag.Error(
					ErrTypeMismatch,
					s.Value.Span(),
					fmt.Sprintf("a função %q possui retorno 'void' e não pode retornar valor do tipo %q", c.currentFnName, valType),
					"valor inesperado em função 'void'",
					"use 'return;' sem expressão ou altere o tipo de retorno na assinatura da função",
				)
			}
		}
		return
	}

	if s.Value == nil {
		c.diag.Error(
			ErrTypeMismatch,
			s.Span(),
			fmt.Sprintf("a função %q deve retornar um valor do tipo %q", c.currentFnName, c.currentFnRet),
			fmt.Sprintf("faltou expressão do tipo %q", c.currentFnRet),
			"",
		)
		return
	}

	valType := c.checkExpr(s.Value, scope, c.currentFnRet)
	if !c.currentFnRet.IsError() && !valType.IsError() && !c.currentFnRet.Equals(valType) {
		c.diag.Error(
			ErrTypeMismatch,
			s.Value.Span(),
			fmt.Sprintf("tipo de retorno incompatível em %q: esperado %q, mas encontrado %q", c.currentFnName, c.currentFnRet, valType),
			fmt.Sprintf("encontrado %q, esperado %q", valType, c.currentFnRet),
			"",
		)
	}
}

// ============================================================================
// Checagem de Expressões com Propagação Bidirecional de Tipo Esperado
// ============================================================================

func (c *Checker) checkExpr(expr parser.Expr, scope *Scope, expected Type) Type {
	if expr == nil {
		return TypeError
	}
	res := c.evalExprType(expr, scope, expected)
	c.exprTypes[expr] = res
	return res
}

func (c *Checker) evalExprType(expr parser.Expr, scope *Scope, expected Type) Type {
	switch e := expr.(type) {
	case *parser.IntLit:
		return TypeInt

	case *parser.StringLit:
		return TypeString

	case *parser.BoolLit:
		return TypeBool

	case *parser.IdentExpr:
		sym, ok := scope.Lookup(e.Name)
		if !ok {
			c.diag.Error(
				ErrUndeclared,
				e.Span(),
				fmt.Sprintf("identificador %q não foi declarado neste escopo", e.Name),
				"símbolo desconhecido",
				fmt.Sprintf("declare %q antes de usá-lo (ex: 'let %s: int = ...;')", e.Name, e.Name),
			)
			return TypeError
		}
		return sym.Type

	case *parser.ArrayLit:
		return c.checkArrayLit(e, scope, expected)

	case *parser.UnaryExpr:
		return c.checkUnaryExpr(e, scope)

	case *parser.BinaryExpr:
		return c.checkBinaryExpr(e, scope)

	case *parser.IndexExpr:
		return c.checkIndexExpr(e, scope)

	case *parser.CallExpr:
		return c.checkCallExpr(e, scope)

	default:
		return TypeError
	}
}

func (c *Checker) checkArrayLit(lit *parser.ArrayLit, scope *Scope, expected Type) Type {
	var expectedElem Type
	if arrExp, ok := expected.(*ArrayType); ok {
		expectedElem = arrExp.Elem
	}

	if len(lit.Elements) == 0 {
		if expected != nil && expected.IsError() {
			return TypeError
		}
		if expectedElem != nil {
			return NewArrayType(expectedElem)
		}
		if expected != nil {
			// O contexto exige um tipo não-array (ex: `let x: int = [];`)
			c.diag.Error(
				ErrTypeMismatch,
				lit.Span(),
				fmt.Sprintf("tipos incompatíveis: esperado %q, mas encontrado literal de array '[]'", expected),
				"array não atribuível a tipo escalar",
				"",
			)
			return TypeError
		}
		c.diag.Error(
			ErrAmbiguousEmptyArr,
			lit.Span(),
			"não é possível inferir o tipo dos elementos do array vazio '[]'",
			"tipo do array vazio indeterminado",
			"atribua '[]' a uma variável com anotação de tipo explícita, ex: 'let v: []int = [];'",
		)
		return TypeError
	}

	// Array não-vazio: usa o tipo esperado pelo contexto ou infere a partir do primeiro elemento
	elemType := expectedElem
	anyError := false
	for i, el := range lit.Elements {
		t := c.checkExpr(el, scope, elemType)
		if t.IsError() {
			anyError = true
			continue
		}
		if t.Equals(TypeVoid) {
			c.diag.Error(
				ErrInvalidVoid,
				el.Span(),
				"elemento de array não pode ser uma expressão do tipo 'void'",
				"expressão sem valor",
				"",
			)
			anyError = true
			continue
		}
		if elemType == nil {
			elemType = t
		} else if !elemType.IsError() && !t.Equals(elemType) {
			c.diag.Error(
				ErrTypeMismatch,
				el.Span(),
				fmt.Sprintf("elementos de array devem ser homogêneos: elemento [%d] tem tipo %q, mas esperado %q", i, t, elemType),
				fmt.Sprintf("encontrado %q, esperado %q", t, elemType),
				"todos os itens de um array na linguagem vim devem possuir o mesmo tipo estático",
			)
			anyError = true
		}
	}

	if anyError || elemType == nil {
		return TypeError
	}
	return NewArrayType(elemType)
}

func (c *Checker) checkUnaryExpr(e *parser.UnaryExpr, scope *Scope) Type {
	switch e.Op {
	case lexer.TokenBang:
		opType := c.checkExpr(e.Operand, scope, TypeBool)
		if opType.IsError() {
			return TypeError
		}
		if !opType.Equals(TypeBool) {
			c.diag.Error(
				ErrTypeMismatch,
				e.Span(),
				fmt.Sprintf("operador lógico '!' requer operando 'bool', mas recebeu %q", opType),
				fmt.Sprintf("operando tem tipo %q", opType),
				"",
			)
			return TypeError
		}
		return TypeBool

	case lexer.TokenMinus:
		opType := c.checkExpr(e.Operand, scope, TypeInt)
		if opType.IsError() {
			return TypeError
		}
		if !opType.Equals(TypeInt) {
			c.diag.Error(
				ErrTypeMismatch,
				e.Span(),
				fmt.Sprintf("operador unário '-' requer operando 'int', mas recebeu %q", opType),
				fmt.Sprintf("operando tem tipo %q", opType),
				"",
			)
			return TypeError
		}
		return TypeInt

	default:
		return TypeError
	}
}

func (c *Checker) checkBinaryExpr(e *parser.BinaryExpr, scope *Scope) Type {
	leftType := c.checkExpr(e.Left, scope, nil)
	rightType := c.checkExpr(e.Right, scope, nil)

	// Supressão de cascata: se qualquer operando já falhou, propaga TypeError silenciosamente.
	if leftType.IsError() || rightType.IsError() {
		return TypeError
	}

	switch e.Op {
	case lexer.TokenPlus:
		if leftType.Equals(TypeInt) && rightType.Equals(TypeInt) {
			return TypeInt
		}
		if leftType.Equals(TypeString) && rightType.Equals(TypeString) {
			return TypeString
		}
		c.diag.Error(
			ErrTypeMismatch,
			e.Span(),
			fmt.Sprintf("operador '+' inválido entre %q e %q", leftType, rightType),
			fmt.Sprintf("não é possível somar/concatenar %q com %q", leftType, rightType),
			"o operador '+' aceita apenas 'int + int' ou 'string + string'",
		)
		return TypeError

	case lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		if leftType.Equals(TypeInt) && rightType.Equals(TypeInt) {
			return TypeInt
		}
		c.diag.Error(
			ErrTypeMismatch,
			e.Span(),
			fmt.Sprintf("operador aritmético %q requer 'int' e 'int', mas recebeu %q e %q", e.OpLexeme, leftType, rightType),
			fmt.Sprintf("operandos de tipos %q e %q", leftType, rightType),
			"",
		)
		return TypeError

	case lexer.TokenLt, lexer.TokenLtEq, lexer.TokenGt, lexer.TokenGtEq:
		if leftType.Equals(TypeInt) && rightType.Equals(TypeInt) {
			return TypeBool
		}
		c.diag.Error(
			ErrTypeMismatch,
			e.Span(),
			fmt.Sprintf("operador relacional %q requer 'int' e 'int', mas recebeu %q e %q", e.OpLexeme, leftType, rightType),
			fmt.Sprintf("comparação inválida entre %q e %q", leftType, rightType),
			"",
		)
		return TypeError

	case lexer.TokenEq, lexer.TokenNotEq:
		if leftType.Equals(rightType) && IsComparablePrimitive(leftType) {
			return TypeBool
		}
		c.diag.Error(
			ErrTypeMismatch,
			e.Span(),
			fmt.Sprintf("operador de igualdade %q requer operandos do mesmo tipo primitivo ('int', 'bool' ou 'string'), mas recebeu %q e %q",
				e.OpLexeme, leftType, rightType),
			fmt.Sprintf("comparação incompatível entre %q e %q", leftType, rightType),
			"",
		)
		return TypeError

	case lexer.TokenAnd, lexer.TokenOr:
		if leftType.Equals(TypeBool) && rightType.Equals(TypeBool) {
			return TypeBool
		}
		c.diag.Error(
			ErrTypeMismatch,
			e.Span(),
			fmt.Sprintf("operador lógico %q requer 'bool' e 'bool', mas recebeu %q e %q", e.OpLexeme, leftType, rightType),
			"coerção implícita para 'bool' não é permitida",
			"converta os operandos em expressões booleanas explícitas",
		)
		return TypeError

	default:
		return TypeError
	}
}

func (c *Checker) checkIndexExpr(e *parser.IndexExpr, scope *Scope) Type {
	colType := c.checkExpr(e.Collection, scope, nil)
	idxType := c.checkExpr(e.Index, scope, TypeInt)

	if !idxType.IsError() && !idxType.Equals(TypeInt) {
		c.diag.Error(
			ErrTypeMismatch,
			e.Index.Span(),
			fmt.Sprintf("índice de array deve ser do tipo 'int', mas foi encontrado %q", idxType),
			fmt.Sprintf("índice tem tipo %q", idxType),
			"",
		)
	}

	if colType.IsError() {
		return TypeError
	}

	arrType, ok := colType.(*ArrayType)
	if !ok {
		c.diag.Error(
			ErrNotIndexable,
			e.Collection.Span(),
			fmt.Sprintf("não é possível indexar uma expressão do tipo %q com '[...]'", colType),
			fmt.Sprintf("tipo %q não é um array", colType),
			"apenas valores do tipo '[]T' suportam indexação",
		)
		return TypeError
	}

	if idxType.IsError() {
		return TypeError
	}
	return arrType.Elem
}

func (c *Checker) checkCallExpr(e *parser.CallExpr, scope *Scope) Type {
	// Caso o alvo seja identificador direto de função intrínseca
	if ident, ok := e.Callee.(*parser.IdentExpr); ok {
		if sym, found := scope.Lookup(ident.Name); found && sym.Kind == KindIntrinsic {
			c.exprTypes[e.Callee] = sym.Type
			return c.checkIntrinsicCall(ident.Name, e, scope)
		}
	}

	calleeType := c.checkExpr(e.Callee, scope, nil)
	if calleeType.IsError() {
		for _, arg := range e.Args {
			_ = c.checkExpr(arg, scope, nil)
		}
		return TypeError
	}

	fnType, ok := calleeType.(*FnType)
	if !ok {
		c.diag.Error(
			ErrInvalidCall,
			e.Callee.Span(),
			fmt.Sprintf("expressão do tipo %q não pode ser chamada como função", calleeType),
			"alvo não é uma função",
			"",
		)
		for _, arg := range e.Args {
			_ = c.checkExpr(arg, scope, nil)
		}
		return TypeError
	}

	if len(e.Args) != len(fnType.Params) {
		c.diag.Error(
			ErrInvalidCall,
			e.Span(),
			fmt.Sprintf("número incorreto de argumentos: esperado %d, mas foram passados %d", len(fnType.Params), len(e.Args)),
			fmt.Sprintf("assinatura esperada: %s", fnType),
			"",
		)
		for i, arg := range e.Args {
			var exp Type
			if i < len(fnType.Params) {
				exp = fnType.Params[i]
			}
			_ = c.checkExpr(arg, scope, exp)
		}
		return fnType.Return
	}

	for i, arg := range e.Args {
		expectedParam := fnType.Params[i]
		argType := c.checkExpr(arg, scope, expectedParam)
		if !expectedParam.IsError() && !argType.IsError() && !expectedParam.Equals(argType) {
			c.diag.Error(
				ErrTypeMismatch,
				arg.Span(),
				fmt.Sprintf("argumento %d incompatível na chamada: esperado %q, mas encontrado %q", i+1, expectedParam, argType),
				fmt.Sprintf("encontrado %q, esperado %q", argType, expectedParam),
				"",
			)
		}
	}

	return fnType.Return
}

// ============================================================================
// Validação das 5 Funções Intrínsecas Nativas
// ============================================================================

func (c *Checker) checkIntrinsicCall(name string, call *parser.CallExpr, scope *Scope) Type {
	switch name {
	case "print":
		if len(call.Args) == 0 {
			c.diag.Error(
				ErrInvalidCall,
				call.Span(),
				"a função intrínseca 'print' requer ao menos 1 argumento",
				"chamada vazia a 'print()'",
				"exemplo: 'print(\"ola\");' ou 'print(x, y);'",
			)
			return TypeVoid
		}
		for i, arg := range call.Args {
			t := c.checkExpr(arg, scope, nil)
			if t.IsError() {
				continue
			}
			if t.Equals(TypeVoid) {
				c.diag.Error(
					ErrTypeMismatch,
					arg.Span(),
					fmt.Sprintf("o argumento %d de 'print' tem tipo 'void' e não produz valor imprimível", i+1),
					"expressão 'void'",
					"",
				)
			} else if _, isFn := t.(*FnType); isFn {
				c.diag.Error(
					ErrTypeMismatch,
					arg.Span(),
					fmt.Sprintf("não é possível imprimir diretamente a função %q sem invocá-la", t),
					"referência de função não invocada",
					"adicione '(...)' para chamar a função e imprimir seu resultado",
				)
			}
		}
		return TypeVoid

	case "read":
		if len(call.Args) != 0 {
			c.diag.Error(
				ErrInvalidCall,
				call.Span(),
				fmt.Sprintf("'read()' não aceita argumentos, mas recebeu %d", len(call.Args)),
				"aridade incorreta",
				"",
			)
			for _, a := range call.Args {
				_ = c.checkExpr(a, scope, nil)
			}
		}
		return TypeString

	case "read_int":
		if len(call.Args) != 0 {
			c.diag.Error(
				ErrInvalidCall,
				call.Span(),
				fmt.Sprintf("'read_int()' não aceita argumentos, mas recebeu %d", len(call.Args)),
				"aridade incorreta",
				"",
			)
			for _, a := range call.Args {
				_ = c.checkExpr(a, scope, nil)
			}
		}
		return TypeInt

	case "len":
		if len(call.Args) != 1 {
			c.diag.Error(
				ErrInvalidCall,
				call.Span(),
				fmt.Sprintf("'len(x)' requer exatamente 1 argumento, mas recebeu %d", len(call.Args)),
				"aridade incorreta",
				"",
			)
			for _, a := range call.Args {
				_ = c.checkExpr(a, scope, nil)
			}
			return TypeInt
		}
		argType := c.checkExpr(call.Args[0], scope, nil)
		if !argType.IsError() {
			_, isArr := argType.(*ArrayType)
			if !isArr && !argType.Equals(TypeString) {
				c.diag.Error(
					ErrTypeMismatch,
					call.Args[0].Span(),
					fmt.Sprintf("'len' aceita apenas 'string' ou array '[]T', mas recebeu %q", argType),
					fmt.Sprintf("tipo %q não possui comprimento", argType),
					"",
				)
			}
		}
		return TypeInt

	case "push":
		if len(call.Args) != 2 {
			c.diag.Error(
				ErrInvalidCall,
				call.Span(),
				fmt.Sprintf("'push(arr, elem)' requer exatamente 2 argumentos, mas recebeu %d", len(call.Args)),
				"aridade incorreta",
				"",
			)
			for _, a := range call.Args {
				_ = c.checkExpr(a, scope, nil)
			}
			return TypeVoid
		}

		// Verifica mutabilidade caso o 1º argumento seja constante (ou indexado em constante)
		if root := c.rootSymbol(call.Args[0], scope); root != nil && !root.IsMutable {
			c.diag.Error(
				ErrImmutableAssign,
				call.Args[0].Span(),
				fmt.Sprintf("não é possível executar 'push' sobre a constante imutável %q", root.Name),
				"array constante ('const')",
				fmt.Sprintf("declare %q com 'let' na linha %d se desejar adicionar elementos", root.Name, root.DefSpan.StartLine),
			)
		}

		arrType := c.checkExpr(call.Args[0], scope, nil)
		if arrType.IsError() {
			_ = c.checkExpr(call.Args[1], scope, nil)
			return TypeVoid
		}
		at, ok := arrType.(*ArrayType)
		if !ok {
			c.diag.Error(
				ErrTypeMismatch,
				call.Args[0].Span(),
				fmt.Sprintf("o 1º argumento de 'push' deve ser um array '[]T', mas recebeu %q", arrType),
				fmt.Sprintf("tipo %q não é array", arrType),
				"",
			)
			_ = c.checkExpr(call.Args[1], scope, nil)
			return TypeVoid
		}

		elemType := c.checkExpr(call.Args[1], scope, at.Elem)
		if !at.Elem.IsError() && !elemType.IsError() && !elemType.Equals(at.Elem) {
			c.diag.Error(
				ErrTypeMismatch,
				call.Args[1].Span(),
				fmt.Sprintf("'push' em %q exige elemento do tipo %q, mas recebeu %q", at, at.Elem, elemType),
				fmt.Sprintf("encontrado %q, esperado %q", elemType, at.Elem),
				"",
			)
		}
		return TypeVoid

	default:
		return TypeError
	}
}

// ============================================================================
// Resolução de Anotações de Tipo, Análise de Retorno e Utilitários
// ============================================================================

// resolveTypeNode converte um `parser.TypeNode` em um `Type` semântico validado.
// Na Passada 2 das assinaturas de função, `emitErrors` evita duplicar diagnósticos já emitidos na Passada 1.
func (c *Checker) resolveTypeNode(tn parser.TypeNode, allowVoid bool, context string) Type {
	return c.resolveTypeInternal(tn, allowVoid, context, true)
}

func (c *Checker) resolveTypeInternal(tn parser.TypeNode, allowVoid bool, context string, emit bool) Type {
	if tn == nil {
		return TypeVoid
	}
	switch t := tn.(type) {
	case *parser.PrimType:
		switch t.Name {
		case "int":
			return TypeInt
		case "bool":
			return TypeBool
		case "string":
			return TypeString
		case "void":
			if !allowVoid {
				if emit {
					c.diag.Error(
						ErrInvalidVoid,
						t.Span(),
						fmt.Sprintf("%s não pode ter tipo 'void'", context),
						"uso inválido de 'void'",
						"'void' é exclusivo para retorno de funções que não devolvem valor",
					)
				}
				return TypeError
			}
			return TypeVoid
		default:
			return TypeError
		}

	case *parser.ArrayType:
		// Elementos de array NUNCA podem ser `void` (ex: `[]void` ou `[][]void`)
		elem := c.resolveTypeInternal(t.Elem, false, "elemento de array", emit)
		if elem.IsError() {
			return TypeError
		}
		return NewArrayType(elem)

	default:
		return TypeError
	}
}

func (c *Checker) rootSymbol(expr parser.Expr, scope *Scope) *Symbol {
	switch e := expr.(type) {
	case *parser.IdentExpr:
		sym, _ := scope.Lookup(e.Name)
		return sym
	case *parser.IndexExpr:
		return c.rootSymbol(e.Collection, scope)
	default:
		return nil
	}
}

func (c *Checker) reportRedeclaration(name string, span diagnostics.Span, existing *Symbol) {
	help := "escolha outro nome ou utilize atribuição '=' se desejar atualizar o valor"
	if existing.Kind == KindIntrinsic {
		help = fmt.Sprintf("%q é uma função intrínseca reservada da linguagem vim", name)
	} else if existing.DefSpan.StartLine > 0 {
		help = fmt.Sprintf("declaração anterior de %q neste mesmo escopo está na linha %d, coluna %d",
			name, existing.DefSpan.StartLine, existing.DefSpan.StartCol)
	}
	c.diag.Error(
		ErrRedeclared,
		span,
		fmt.Sprintf("símbolo %q já foi declarado neste escopo", name),
		"redeclaração no mesmo escopo léxico",
		help,
	)
}

// guaranteesReturn verifica estaticamente se um comando garante a execução de `return` em todos os ramos.
func guaranteesReturn(stmt parser.Stmt) bool {
	if stmt == nil {
		return false
	}
	switch s := stmt.(type) {
	case *parser.ReturnStmt:
		return true
	case *parser.BlockStmt:
		for _, child := range s.Stmts {
			if guaranteesReturn(child) {
				return true
			}
		}
		return false
	case *parser.IfStmt:
		return s.Else != nil && guaranteesReturn(s.Then) && guaranteesReturn(s.Else)
	default:
		return false
	}
}
