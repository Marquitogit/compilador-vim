package parser

import (
	"fmt"
	"strings"

	"vim/internal/diagnostics"
	"vim/internal/lexer"
)

// Node é a interface base de todos os elementos da Árvore de Sintaxe Abstrata (AST).
type Node interface {
	Span() diagnostics.Span
	nodeMarker()
}

// Stmt representa declarações de topo e comandos executáveis.
type Stmt interface {
	Node
	stmtMarker()
}

// Expr representa expressões que produzem valores.
type Expr interface {
	Node
	SetSpan(diagnostics.Span)
	exprMarker()
}

// TypeNode representa uma anotação estática de tipo na gramática.
type TypeNode interface {
	Node
	TypeString() string
	typeMarker()
}

// ============================================================================
// Raiz do Programa e Parâmetros
// ============================================================================

// Program representa a unidade completa de compilação (sequência de TopLevelDecls).
type Program struct {
	Decls    []Stmt
	SpanInfo diagnostics.Span
}

func (p *Program) Span() diagnostics.Span { return p.SpanInfo }
func (p *Program) nodeMarker()            {}

// Param representa um parâmetro formal tipado em uma declaração de função.
type Param struct {
	Name     string
	NameSpan diagnostics.Span
	Type     TypeNode
	SpanInfo diagnostics.Span
}

// ============================================================================
// Anotações Sintáticas de Tipo (TypeNode)
// ============================================================================

// PrimType representa um tipo primitivo: int, bool, string ou void.
type PrimType struct {
	Name     string
	SpanInfo diagnostics.Span
}

func (t *PrimType) Span() diagnostics.Span { return t.SpanInfo }
func (t *PrimType) TypeString() string     { return t.Name }
func (t *PrimType) nodeMarker()            {}
func (t *PrimType) typeMarker()            {}

// ArrayType representa um tipo de array homogêneo prefixado: []T (ex: []int, [][]string).
type ArrayType struct {
	Elem     TypeNode
	SpanInfo diagnostics.Span
}

func (t *ArrayType) Span() diagnostics.Span { return t.SpanInfo }
func (t *ArrayType) TypeString() string     { return "[]" + t.Elem.TypeString() }
func (t *ArrayType) nodeMarker()            {}
func (t *ArrayType) typeMarker()            {}

// ============================================================================
// Declarações e Comandos (Stmt)
// ============================================================================

// FnDecl representa uma declaração de função estritamente top-level.
type FnDecl struct {
	Name       string
	NameSpan   diagnostics.Span
	Params     []Param
	ReturnType TypeNode
	Body       *BlockStmt
	SpanInfo   diagnostics.Span
}

func (s *FnDecl) Span() diagnostics.Span { return s.SpanInfo }
func (s *FnDecl) nodeMarker()            {}
func (s *FnDecl) stmtMarker()            {}

func (s *FnDecl) SignatureString() string {
	params := make([]string, len(s.Params))
	for i, p := range s.Params {
		params[i] = fmt.Sprintf("%s: %s", p.Name, p.Type.TypeString())
	}
	ret := "void"
	if s.ReturnType != nil {
		ret = s.ReturnType.TypeString()
	}
	return fmt.Sprintf("fn %s(%s) -> %s", s.Name, strings.Join(params, ", "), ret)
}

// VarDecl representa uma declaração de variável mutável: `let id: Tipo = expr;`.
type VarDecl struct {
	Name     string
	NameSpan diagnostics.Span
	Type     TypeNode
	Value    Expr
	SpanInfo diagnostics.Span
}

func (s *VarDecl) Span() diagnostics.Span { return s.SpanInfo }
func (s *VarDecl) nodeMarker()            {}
func (s *VarDecl) stmtMarker()            {}

// ConstDecl representa uma declaração de constante imutável: `const id: Tipo = expr;`.
type ConstDecl struct {
	Name     string
	NameSpan diagnostics.Span
	Type     TypeNode
	Value    Expr
	SpanInfo diagnostics.Span
}

func (s *ConstDecl) Span() diagnostics.Span { return s.SpanInfo }
func (s *ConstDecl) nodeMarker()            {}
func (s *ConstDecl) stmtMarker()            {}

// BlockStmt representa um bloco léxico delimitado por chaves `{ ... }`.
type BlockStmt struct {
	Stmts          []Stmt
	IsDesugaredFor bool // Indica se este bloco resultou do desaçucaramento de um laço `for`
	SpanInfo       diagnostics.Span
}

func (s *BlockStmt) Span() diagnostics.Span { return s.SpanInfo }
func (s *BlockStmt) nodeMarker()            {}
func (s *BlockStmt) stmtMarker()            {}

// AssignStmt representa uma atribuição a um L-Value válido (`x = expr;` ou `arr[i] = expr;`).
type AssignStmt struct {
	Target   Expr // Garantido pelo parser como *IdentExpr ou *IndexExpr
	Value    Expr
	SpanInfo diagnostics.Span
}

func (s *AssignStmt) Span() diagnostics.Span { return s.SpanInfo }
func (s *AssignStmt) nodeMarker()            {}
func (s *AssignStmt) stmtMarker()            {}

// IfStmt representa uma estrutura condicional `if (cond) { ... } else ...`.
// O campo Else pode ser nil, *BlockStmt (else) ou *IfStmt (else if).
type IfStmt struct {
	Cond     Expr
	Then     *BlockStmt
	Else     Stmt
	SpanInfo diagnostics.Span
}

func (s *IfStmt) Span() diagnostics.Span { return s.SpanInfo }
func (s *IfStmt) nodeMarker()            {}
func (s *IfStmt) stmtMarker()            {}

// WhileStmt representa um laço condicional `while (cond) { ... }`.
type WhileStmt struct {
	Cond     Expr
	Body     *BlockStmt
	SpanInfo diagnostics.Span
}

func (s *WhileStmt) Span() diagnostics.Span { return s.SpanInfo }
func (s *WhileStmt) nodeMarker()            {}
func (s *WhileStmt) stmtMarker()            {}

// ForStmt representa o açúcar sintático `for (init; cond; step) { ... }` antes do desugaring.
type ForStmt struct {
	Init     Stmt // nil, *VarDecl ou *AssignStmt
	Cond     Expr // nil ou expressão booleana
	Step     Stmt // nil, *AssignStmt ou *ExprStmt
	Body     *BlockStmt
	SpanInfo diagnostics.Span
}

func (s *ForStmt) Span() diagnostics.Span { return s.SpanInfo }
func (s *ForStmt) nodeMarker()            {}
func (s *ForStmt) stmtMarker()            {}

// ReturnStmt representa o comando `return expr?;`.
type ReturnStmt struct {
	Value    Expr // nil quando `return;`
	SpanInfo diagnostics.Span
}

func (s *ReturnStmt) Span() diagnostics.Span { return s.SpanInfo }
func (s *ReturnStmt) nodeMarker()            {}
func (s *ReturnStmt) stmtMarker()            {}

// ExprStmt representa uma expressão avaliada como comando (ex: `print(x);` ou `f();`).
type ExprStmt struct {
	Expr     Expr
	SpanInfo diagnostics.Span
}

func (s *ExprStmt) Span() diagnostics.Span { return s.SpanInfo }
func (s *ExprStmt) nodeMarker()            {}
func (s *ExprStmt) stmtMarker()            {}

// ============================================================================
// Expressões (Expr)
// ============================================================================

// IntLit representa um literal inteiro de 64 bits.
type IntLit struct {
	Value            int64
	Raw              string
	RequiresNegation bool // Verdadeiro para 9223372036854775808 caso ainda não tenha sido negado por '-'
	SpanInfo         diagnostics.Span
}

func (e *IntLit) Span() diagnostics.Span       { return e.SpanInfo }
func (e *IntLit) SetSpan(sp diagnostics.Span)  { e.SpanInfo = sp }
func (e *IntLit) nodeMarker()                  {}
func (e *IntLit) exprMarker()                  {}

// StringLit representa um literal de texto já decodificado.
type StringLit struct {
	Value    string
	Raw      string
	SpanInfo diagnostics.Span
}

func (e *StringLit) Span() diagnostics.Span      { return e.SpanInfo }
func (e *StringLit) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *StringLit) nodeMarker()                 {}
func (e *StringLit) exprMarker()                 {}

// BoolLit representa os literais booleanos `true` e `false`.
type BoolLit struct {
	Value    bool
	SpanInfo diagnostics.Span
}

func (e *BoolLit) Span() diagnostics.Span      { return e.SpanInfo }
func (e *BoolLit) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *BoolLit) nodeMarker()                 {}
func (e *BoolLit) exprMarker()                 {}

// IdentExpr representa a referência a um identificador (variável, constante ou função).
type IdentExpr struct {
	Name     string
	SpanInfo diagnostics.Span
}

func (e *IdentExpr) Span() diagnostics.Span      { return e.SpanInfo }
func (e *IdentExpr) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *IdentExpr) nodeMarker()                 {}
func (e *IdentExpr) exprMarker()                 {}

// ArrayLit representa um literal de array homogêneo: `[e1, e2, ...]`.
type ArrayLit struct {
	Elements []Expr
	SpanInfo diagnostics.Span
}

func (e *ArrayLit) Span() diagnostics.Span      { return e.SpanInfo }
func (e *ArrayLit) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *ArrayLit) nodeMarker()                 {}
func (e *ArrayLit) exprMarker()                 {}

// UnaryExpr representa uma operação unária prefixa (`!expr` ou `-expr`).
type UnaryExpr struct {
	Op       lexer.TokenType
	OpLexeme string
	Operand  Expr
	SpanInfo diagnostics.Span
}

func (e *UnaryExpr) Span() diagnostics.Span      { return e.SpanInfo }
func (e *UnaryExpr) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *UnaryExpr) nodeMarker()                 {}
func (e *UnaryExpr) exprMarker()                 {}

// BinaryExpr representa uma operação binária infixa gerada pelo Pratt Parser.
type BinaryExpr struct {
	Left     Expr
	Op       lexer.TokenType
	OpLexeme string
	Right    Expr
	SpanInfo diagnostics.Span
}

func (e *BinaryExpr) Span() diagnostics.Span      { return e.SpanInfo }
func (e *BinaryExpr) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *BinaryExpr) nodeMarker()                 {}
func (e *BinaryExpr) exprMarker()                 {}

// CallExpr representa uma chamada de função: `callee(arg1, arg2, ...)`.
type CallExpr struct {
	Callee   Expr
	Args     []Expr
	SpanInfo diagnostics.Span
}

func (e *CallExpr) Span() diagnostics.Span      { return e.SpanInfo }
func (e *CallExpr) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *CallExpr) nodeMarker()                 {}
func (e *CallExpr) exprMarker()                 {}

// IndexExpr representa o acesso indexado a um array: `collection[index]`.
type IndexExpr struct {
	Collection Expr
	Index      Expr
	SpanInfo   diagnostics.Span
}

func (e *IndexExpr) Span() diagnostics.Span      { return e.SpanInfo }
func (e *IndexExpr) SetSpan(sp diagnostics.Span) { e.SpanInfo = sp }
func (e *IndexExpr) nodeMarker()                 {}
func (e *IndexExpr) exprMarker()                 {}

// IsValidLValue verifica se uma expressão pode figurar no lado esquerdo de uma atribuição (`=`).
func IsValidLValue(expr Expr) bool {
	switch expr.(type) {
	case *IdentExpr, *IndexExpr:
		return true
	default:
		return false
	}
}

// CountNodes contabiliza recursivamente o total de nós presentes na AST (útil para `--stats`).
func CountNodes(n Node) int {
	if n == nil {
		return 0
	}
	total := 1
	switch v := n.(type) {
	case *Program:
		for _, d := range v.Decls {
			total += CountNodes(d)
		}
	case *PrimType:
		// folha
	case *ArrayType:
		total += CountNodes(v.Elem)
	case *FnDecl:
		for _, p := range v.Params {
			total += CountNodes(p.Type)
		}
		if v.ReturnType != nil {
			total += CountNodes(v.ReturnType)
		}
		if v.Body != nil {
			total += CountNodes(v.Body)
		}
	case *VarDecl:
		total += CountNodes(v.Type)
		total += CountNodes(v.Value)
	case *ConstDecl:
		total += CountNodes(v.Type)
		total += CountNodes(v.Value)
	case *BlockStmt:
		for _, s := range v.Stmts {
			total += CountNodes(s)
		}
	case *AssignStmt:
		total += CountNodes(v.Target)
		total += CountNodes(v.Value)
	case *IfStmt:
		total += CountNodes(v.Cond)
		if v.Then != nil {
			total += CountNodes(v.Then)
		}
		if v.Else != nil {
			total += CountNodes(v.Else)
		}
	case *WhileStmt:
		total += CountNodes(v.Cond)
		if v.Body != nil {
			total += CountNodes(v.Body)
		}
	case *ForStmt:
		if v.Init != nil {
			total += CountNodes(v.Init)
		}
		if v.Cond != nil {
			total += CountNodes(v.Cond)
		}
		if v.Step != nil {
			total += CountNodes(v.Step)
		}
		if v.Body != nil {
			total += CountNodes(v.Body)
		}
	case *ReturnStmt:
		if v.Value != nil {
			total += CountNodes(v.Value)
		}
	case *ExprStmt:
		total += CountNodes(v.Expr)
	case *ArrayLit:
		for _, el := range v.Elements {
			total += CountNodes(el)
		}
	case *UnaryExpr:
		total += CountNodes(v.Operand)
	case *BinaryExpr:
		total += CountNodes(v.Left)
		total += CountNodes(v.Right)
	case *CallExpr:
		total += CountNodes(v.Callee)
		for _, a := range v.Args {
			total += CountNodes(a)
		}
	case *IndexExpr:
		total += CountNodes(v.Collection)
		total += CountNodes(v.Index)
	}
	return total
}
