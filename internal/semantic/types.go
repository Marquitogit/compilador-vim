package semantic

import (
	"fmt"
	"strings"
)

// Type representa qualquer tipo estático resolvido pelo Analisador Semântico.
type Type interface {
	String() string
	Equals(other Type) bool
	IsError() bool
}

// ============================================================================
// Tipos Primitivos e Sentinela de Erro (Supressão de Cascata)
// ============================================================================

type primitiveKind string

const (
	primInt    primitiveKind = "int"
	primBool   primitiveKind = "bool"
	primString primitiveKind = "string"
	primVoid   primitiveKind = "void"
)

// PrimitiveType representa os tipos escalares fundamentais da linguagem vim.
type PrimitiveType struct {
	kind primitiveKind
}

var (
	TypeInt    Type = &PrimitiveType{kind: primInt}
	TypeBool   Type = &PrimitiveType{kind: primBool}
	TypeString Type = &PrimitiveType{kind: primString}
	TypeVoid   Type = &PrimitiveType{kind: primVoid}
)

func (t *PrimitiveType) String() string { return string(t.kind) }
func (t *PrimitiveType) IsError() bool  { return false }
func (t *PrimitiveType) Equals(other Type) bool {
	o, ok := other.(*PrimitiveType)
	return ok && t.kind == o.kind
}

// ErrorType é o tipo sentinela retornado quando uma subexpressão falha semanticamente.
// Operações pais que recebem TypeError suprimem novos diagnósticos derivados (evitando cascata).
type ErrorType struct{}

var TypeError Type = &ErrorType{}

func (t *ErrorType) String() string         { return "<erro>" }
func (t *ErrorType) IsError() bool          { return true }
func (t *ErrorType) Equals(other Type) bool { _, ok := other.(*ErrorType); return ok }

// ============================================================================
// Tipos Compostos: Arrays Recursivos e Assinaturas de Funções
// ============================================================================

// ArrayType representa um vetor homogêneo `[]Elem` (suportando múltiplas dimensões `[][]int`).
type ArrayType struct {
	Elem Type
}

func NewArrayType(elem Type) *ArrayType {
	return &ArrayType{Elem: elem}
}

func (t *ArrayType) String() string { return "[]" + t.Elem.String() }
func (t *ArrayType) IsError() bool  { return t.Elem.IsError() }
func (t *ArrayType) Equals(other Type) bool {
	o, ok := other.(*ArrayType)
	return ok && t.Elem.Equals(o.Elem)
}

// FnType representa a assinatura estática de uma função declarada no nível superior.
type FnType struct {
	Params []Type
	Return Type
}

func (t *FnType) String() string {
	ps := make([]string, len(t.Params))
	for i, p := range t.Params {
		ps[i] = p.String()
	}
	return fmt.Sprintf("fn(%s) -> %s", strings.Join(ps, ", "), t.Return.String())
}

func (t *FnType) IsError() bool {
	if t.Return.IsError() {
		return true
	}
	for _, p := range t.Params {
		if p.IsError() {
			return true
		}
	}
	return false
}

func (t *FnType) Equals(other Type) bool {
	o, ok := other.(*FnType)
	if !ok || len(t.Params) != len(o.Params) || !t.Return.Equals(o.Return) {
		return false
	}
	for i := range t.Params {
		if !t.Params[i].Equals(o.Params[i]) {
			return false
		}
	}
	return true
}

// IntrinsicType descreve funções nativas polimórficas/embutidas (print, read, read_int, len, push).
type IntrinsicType struct {
	Name      string
	Signature string
}

func (t *IntrinsicType) String() string { return t.Signature }
func (t *IntrinsicType) IsError() bool  { return false }
func (t *IntrinsicType) Equals(other Type) bool {
	o, ok := other.(*IntrinsicType)
	return ok && t.Name == o.Name
}

// IsComparablePrimitive indica se um tipo suporta igualdade direta `==` e `!=` (int, bool, string).
func IsComparablePrimitive(t Type) bool {
	return t.Equals(TypeInt) || t.Equals(TypeBool) || t.Equals(TypeString)
}
