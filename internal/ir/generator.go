package ir

import (
	"vim/internal/diagnostics"
	"vim/internal/lexer"
	"vim/internal/parser"
)

type localBinding struct {
	slot int
	typ  string // representação textual do tipo (ex: "string", "int", "[]string")
}

type fnMeta struct {
	index   int
	retType string
	chunk   *FunctionChunk
}

type chunkCompiler struct {
	gen      *generator
	chunk    *FunctionChunk
	isScript bool
	scopes   []map[string]localBinding // scopes[0] em funções = parâmetros+raiz; em <script>, scopes só para blocos internos
}

func (cc *chunkCompiler) pushScope() {
	cc.scopes = append(cc.scopes, make(map[string]localBinding))
}

func (cc *chunkCompiler) popScope() {
	cc.scopes = cc.scopes[:len(cc.scopes)-1]
}

func (cc *chunkCompiler) allocLocal(name, typ string) int {
	slot := cc.chunk.NumLocals
	cc.chunk.NumLocals++
	cc.chunk.LocalNames = append(cc.chunk.LocalNames, name)
	cc.scopes[len(cc.scopes)-1][name] = localBinding{slot: slot, typ: typ}
	return slot
}

func (cc *chunkCompiler) lookupLocal(name string) (localBinding, bool) {
	for i := len(cc.scopes) - 1; i >= 0; i-- {
		if b, ok := cc.scopes[i][name]; ok {
			return b, true
		}
	}
	return localBinding{}, false
}

type generator struct {
	mod         *Module
	fnTable     map[string]fnMeta
	globalSlots map[string]int
	globalTypes map[string]string
}

// Generate compila um `*parser.Program` validado semanticamente para um `*Module` de bytecode de pilha.
func Generate(prog *parser.Program) *Module {
	mod := &Module{
		Script: &FunctionChunk{
			Name:  "<script>",
			Arity: 0,
		},
	}
	if prog == nil {
		mod.Script.Emit(OpHalt, 0, 0, diagnostics.Span{})
		return mod
	}

	g := &generator{
		mod:         mod,
		fnTable:     make(map[string]fnMeta),
		globalSlots: make(map[string]int),
		globalTypes: make(map[string]string),
	}

	// Passada 1: Registra todas as funções top-level e slots de variáveis globais de raiz
	for _, decl := range prog.Decls {
		switch d := decl.(type) {
		case *parser.FnDecl:
			idx := len(mod.Functions)
			fc := &FunctionChunk{
				Name:  d.Name,
				Arity: len(d.Params),
			}
			mod.Functions = append(mod.Functions, fc)
			retStr := "void"
			if d.ReturnType != nil {
				retStr = d.ReturnType.TypeString()
			}
			g.fnTable[d.Name] = fnMeta{
				index:   idx,
				retType: retStr,
				chunk:   fc,
			}

		case *parser.VarDecl:
			g.registerGlobal(d.Name, d.Type.TypeString())

		case *parser.ConstDecl:
			g.registerGlobal(d.Name, d.Type.TypeString())
		}
	}

	// Passada 2A: Compila os corpos das funções top-level
	for _, decl := range prog.Decls {
		if fn, ok := decl.(*parser.FnDecl); ok {
			g.compileFn(fn)
		}
	}

	// Passada 2B: Compila os comandos e inicializações globais em `<script>`
	scriptCC := &chunkCompiler{
		gen:      g,
		chunk:    mod.Script,
		isScript: true,
		scopes:   nil, // nível 0 usa globals; blocos `{ ... }` empurram escopos locais!
	}
	for _, decl := range prog.Decls {
		if _, isFn := decl.(*parser.FnDecl); isFn {
			continue
		}
		scriptCC.compileStmt(decl)
	}
	mod.Script.Emit(OpHalt, 0, 0, prog.Span())

	return mod
}

func (g *generator) registerGlobal(name, typ string) int {
	if slot, exists := g.globalSlots[name]; exists {
		return slot
	}
	slot := len(g.mod.GlobalNames)
	g.mod.GlobalNames = append(g.mod.GlobalNames, name)
	g.globalSlots[name] = slot
	g.globalTypes[name] = typ
	return slot
}

func (g *generator) compileFn(fn *parser.FnDecl) {
	meta := g.fnTable[fn.Name]
	cc := &chunkCompiler{
		gen:      g,
		chunk:    meta.chunk,
		isScript: false,
	}
	cc.pushScope() // escopo raiz da função (parâmetros + variáveis da raiz do corpo)

	for _, p := range fn.Params {
		cc.allocLocal(p.Name, p.Type.TypeString())
	}

	if fn.Body != nil {
		for _, st := range fn.Body.Stmts {
			cc.compileStmt(st)
		}
		if !stmtAlwaysReturns(fn.Body) {
			cc.chunk.Emit(OpVoid, 0, 0, fn.Body.Span())
			cc.chunk.Emit(OpReturn, 0, 0, fn.Body.Span())
		}
	} else {
		cc.chunk.Emit(OpVoid, 0, 0, fn.Span())
		cc.chunk.Emit(OpReturn, 0, 0, fn.Span())
	}
}

// ============================================================================
// Compilação de Declarações e Comandos (Invariante de Pilha: delta == 0)
// ============================================================================

func (cc *chunkCompiler) compileStmt(stmt parser.Stmt) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *parser.VarDecl:
		// Importante: compila o inicializador ANTES de registrar o novo slot no escopo atual,
		// garantindo semântica exata em shadowing como `{ let x: int = x + 1; }`!
		cc.compileExpr(s.Value)
		if cc.isScript && len(cc.scopes) == 0 {
			gSlot := cc.gen.registerGlobal(s.Name, s.Type.TypeString())
			cc.chunk.Emit(OpSetGlobal, gSlot, 0, s.Span())
		} else {
			lSlot := cc.allocLocal(s.Name, s.Type.TypeString())
			cc.chunk.Emit(OpSetLocal, lSlot, 0, s.Span())
		}

	case *parser.ConstDecl:
		cc.compileExpr(s.Value)
		if cc.isScript && len(cc.scopes) == 0 {
			gSlot := cc.gen.registerGlobal(s.Name, s.Type.TypeString())
			cc.chunk.Emit(OpSetGlobal, gSlot, 0, s.Span())
		} else {
			lSlot := cc.allocLocal(s.Name, s.Type.TypeString())
			cc.chunk.Emit(OpSetLocal, lSlot, 0, s.Span())
		}

	case *parser.BlockStmt:
		cc.pushScope()
		for _, child := range s.Stmts {
			cc.compileStmt(child)
		}
		cc.popScope()

	case *parser.AssignStmt:
		switch target := s.Target.(type) {
		case *parser.IdentExpr:
			cc.compileExpr(s.Value)
			if loc, ok := cc.lookupLocal(target.Name); ok {
				cc.chunk.Emit(OpSetLocal, loc.slot, 0, s.Span())
			} else {
				gSlot := cc.gen.globalSlots[target.Name]
				cc.chunk.Emit(OpSetGlobal, gSlot, 0, s.Span())
			}

		case *parser.IndexExpr:
			// Ordem na pilha para OpSetIndex: [..., arrayRef, indexInt, novoValor]
			cc.compileExpr(target.Collection)
			cc.compileExpr(target.Index)
			cc.compileExpr(s.Value)
			cc.chunk.Emit(OpSetIndex, 0, 0, target.Span())
		}

	case *parser.IfStmt:
		cc.compileExpr(s.Cond)
		jumpFalseIP := cc.chunk.Emit(OpJumpIfFalse, -1, 0, s.Cond.Span())

		cc.compileStmt(s.Then)

		if s.Else != nil {
			jumpEndIP := cc.chunk.Emit(OpJump, -1, 0, s.Span())
			cc.patchJump(jumpFalseIP, len(cc.chunk.Code))
			cc.compileStmt(s.Else)
			cc.patchJump(jumpEndIP, len(cc.chunk.Code))
		} else {
			cc.patchJump(jumpFalseIP, len(cc.chunk.Code))
		}

	case *parser.WhileStmt:
		loopStartIP := len(cc.chunk.Code)
		cc.compileExpr(s.Cond)
		exitJumpIP := cc.chunk.Emit(OpJumpIfFalse, -1, 0, s.Cond.Span())

		cc.compileStmt(s.Body)
		cc.chunk.Emit(OpJump, loopStartIP, 0, s.Span())

		cc.patchJump(exitJumpIP, len(cc.chunk.Code))

	case *parser.ForStmt:
		// Suporte direto mesmo se `--no-desugar` for utilizado
		cc.pushScope()
		if s.Init != nil {
			cc.compileStmt(s.Init)
		}
		loopStartIP := len(cc.chunk.Code)
		exitJumpIP := -1
		if s.Cond != nil {
			cc.compileExpr(s.Cond)
			exitJumpIP = cc.chunk.Emit(OpJumpIfFalse, -1, 0, s.Cond.Span())
		}
		cc.compileStmt(s.Body)
		if s.Step != nil {
			cc.compileStmt(s.Step)
		}
		cc.chunk.Emit(OpJump, loopStartIP, 0, s.Span())
		if exitJumpIP >= 0 {
			cc.patchJump(exitJumpIP, len(cc.chunk.Code))
		}
		cc.popScope()

	case *parser.ReturnStmt:
		if s.Value != nil {
			cc.compileExpr(s.Value)
		} else {
			cc.chunk.Emit(OpVoid, 0, 0, s.Span())
		}
		cc.chunk.Emit(OpReturn, 0, 0, s.Span())

	case *parser.ExprStmt:
		cc.compileExpr(s.Expr)
		cc.chunk.Emit(OpPop, 0, 0, s.Span())
	}
}

// ============================================================================
// Compilação de Expressões (Invariante de Pilha: delta == +1)
// ============================================================================

func (cc *chunkCompiler) compileExpr(expr parser.Expr) {
	switch e := expr.(type) {
	case *parser.IntLit:
		idx := cc.chunk.AddConstant(Constant{Kind: ConstInt, IntVal: e.Value})
		cc.chunk.Emit(OpConst, idx, 0, e.Span())

	case *parser.StringLit:
		idx := cc.chunk.AddConstant(Constant{Kind: ConstString, StrVal: e.Value})
		cc.chunk.Emit(OpConst, idx, 0, e.Span())

	case *parser.BoolLit:
		if e.Value {
			cc.chunk.Emit(OpTrue, 0, 0, e.Span())
		} else {
			cc.chunk.Emit(OpFalse, 0, 0, e.Span())
		}

	case *parser.IdentExpr:
		if loc, ok := cc.lookupLocal(e.Name); ok {
			cc.chunk.Emit(OpGetLocal, loc.slot, 0, e.Span())
		} else {
			gSlot := cc.gen.globalSlots[e.Name]
			cc.chunk.Emit(OpGetGlobal, gSlot, 0, e.Span())
		}

	case *parser.ArrayLit:
		for _, el := range e.Elements {
			cc.compileExpr(el)
		}
		cc.chunk.Emit(OpNewArray, len(e.Elements), 0, e.Span())

	case *parser.IndexExpr:
		cc.compileExpr(e.Collection)
		cc.compileExpr(e.Index)
		cc.chunk.Emit(OpGetIndex, 0, 0, e.Span())

	case *parser.UnaryExpr:
		cc.compileExpr(e.Operand)
		switch e.Op {
		case lexer.TokenMinus:
			cc.chunk.Emit(OpNeg, 0, 0, e.Span())
		case lexer.TokenBang:
			cc.chunk.Emit(OpNot, 0, 0, e.Span())
		}

	case *parser.BinaryExpr:
		// Curto-circuito real para && e || (jamais avalia o lado direito se decidido à esquerda)
		if e.Op == lexer.TokenAnd {
			cc.compileExpr(e.Left)
			shortJump := cc.chunk.Emit(OpJumpIfFalseKeep, -1, 0, e.Span())
			cc.chunk.Emit(OpPop, 0, 0, e.Span())
			cc.compileExpr(e.Right)
			cc.patchJump(shortJump, len(cc.chunk.Code))
			return
		}
		if e.Op == lexer.TokenOr {
			cc.compileExpr(e.Left)
			shortJump := cc.chunk.Emit(OpJumpIfTrueKeep, -1, 0, e.Span())
			cc.chunk.Emit(OpPop, 0, 0, e.Span())
			cc.compileExpr(e.Right)
			cc.patchJump(shortJump, len(cc.chunk.Code))
			return
		}

		cc.compileExpr(e.Left)
		cc.compileExpr(e.Right)

		switch e.Op {
		case lexer.TokenPlus:
			if cc.inferExprType(e) == "string" {
				cc.chunk.Emit(OpConcat, 0, 0, e.Span())
			} else {
				cc.chunk.Emit(OpAdd, 0, 0, e.Span())
			}
		case lexer.TokenMinus:
			cc.chunk.Emit(OpSub, 0, 0, e.Span())
		case lexer.TokenStar:
			cc.chunk.Emit(OpMul, 0, 0, e.Span())
		case lexer.TokenSlash:
			cc.chunk.Emit(OpDiv, 0, 0, e.Span())
		case lexer.TokenPercent:
			cc.chunk.Emit(OpMod, 0, 0, e.Span())
		case lexer.TokenEq:
			cc.chunk.Emit(OpEq, 0, 0, e.Span())
		case lexer.TokenNotEq:
			cc.chunk.Emit(OpNotEq, 0, 0, e.Span())
		case lexer.TokenLt:
			cc.chunk.Emit(OpLt, 0, 0, e.Span())
		case lexer.TokenLtEq:
			cc.chunk.Emit(OpLtEq, 0, 0, e.Span())
		case lexer.TokenGt:
			cc.chunk.Emit(OpGt, 0, 0, e.Span())
		case lexer.TokenGtEq:
			cc.chunk.Emit(OpGtEq, 0, 0, e.Span())
		}

	case *parser.CallExpr:
		for _, arg := range e.Args {
			cc.compileExpr(arg)
		}
		ident := e.Callee.(*parser.IdentExpr)
		if id, isIntrinsic := lookupIntrinsic(ident.Name); isIntrinsic {
			cc.chunk.Emit(OpIntrinsic, int(id), len(e.Args), e.Span())
			return
		}
		meta := cc.gen.fnTable[ident.Name]
		cc.chunk.Emit(OpCall, meta.index, len(e.Args), e.Span())
	}
}

func (cc *chunkCompiler) patchJump(ip, target int) {
	cc.chunk.Code[ip].ArgA = target
}

func (cc *chunkCompiler) inferExprType(expr parser.Expr) string {
	switch e := expr.(type) {
	case *parser.StringLit:
		return "string"
	case *parser.IntLit:
		return "int"
	case *parser.BoolLit:
		return "bool"
	case *parser.IdentExpr:
		if loc, ok := cc.lookupLocal(e.Name); ok {
			return loc.typ
		}
		return cc.gen.globalTypes[e.Name]
	case *parser.BinaryExpr:
		if e.Op == lexer.TokenPlus {
			if cc.inferExprType(e.Left) == "string" || cc.inferExprType(e.Right) == "string" {
				return "string"
			}
			return "int"
		}
		return "int"
	case *parser.CallExpr:
		if ident, ok := e.Callee.(*parser.IdentExpr); ok {
			if ident.Name == "read" {
				return "string"
			}
			if meta, found := cc.gen.fnTable[ident.Name]; found {
				return meta.retType
			}
		}
	case *parser.IndexExpr:
		colT := cc.inferExprType(e.Collection)
		if len(colT) >= 2 && colT[:2] == "[]" {
			return colT[2:]
		}
	}
	return ""
}

func lookupIntrinsic(name string) (IntrinsicID, bool) {
	switch name {
	case "print":
		return IntrinsicPrint, true
	case "read":
		return IntrinsicRead, true
	case "read_int":
		return IntrinsicReadInt, true
	case "len":
		return IntrinsicLen, true
	case "push":
		return IntrinsicPush, true
	default:
		return 0, false
	}
}
