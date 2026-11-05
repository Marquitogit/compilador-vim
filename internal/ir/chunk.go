package ir

import (
	"fmt"

	"vim/internal/diagnostics"
)

// ConstKind discrimina o tipo armazenado no pool de constantes de um FunctionChunk.
type ConstKind uint8

const (
	ConstInt ConstKind = iota
	ConstString
)

// Constant representa um literal imutável armazenado no pool de constantes do bytecode.
type Constant struct {
	Kind   ConstKind
	IntVal int64
	StrVal string
}

func (c Constant) String() string {
	if c.Kind == ConstString {
		return fmt.Sprintf("%q", c.StrVal)
	}
	return fmt.Sprintf("%d", c.IntVal)
}

// Instruction representa uma instrução individual da Stack VM acompanhada de seus operandos
// e do Span exato de origem para emissão de diagnósticos visuais em tempo de execução.
type Instruction struct {
	Op   OpCode
	ArgA int
	ArgB int
	Span diagnostics.Span
}

// FunctionChunk agrupa o bytecode e os metadados de uma função compilada (ou do corpo `<script>`).
type FunctionChunk struct {
	Name       string
	Arity      int
	NumLocals  int
	LocalNames []string // Nomes para inspeção simbólica em `--dump-ir` (índice == slot local)
	Constants  []Constant
	Code       []Instruction
}

// AddConstant insere (com deduplicação) uma constante no pool do chunk e retorna seu índice.
func (fc *FunctionChunk) AddConstant(c Constant) int {
	for i, existing := range fc.Constants {
		if existing.Kind == c.Kind && existing.IntVal == c.IntVal && existing.StrVal == c.StrVal {
			return i
		}
	}
	idx := len(fc.Constants)
	fc.Constants = append(fc.Constants, c)
	return idx
}

// Emit anexa uma instrução ao final do chunk e retorna seu endereço (índice IP).
func (fc *FunctionChunk) Emit(op OpCode, argA, argB int, span diagnostics.Span) int {
	ip := len(fc.Code)
	fc.Code = append(fc.Code, Instruction{
		Op:   op,
		ArgA: argA,
		ArgB: argB,
		Span: span,
	})
	return ip
}

// Module representa a unidade completa de código intermediário pronta para execução na Stack VM.
type Module struct {
	Script      *FunctionChunk   // Ponto de entrada top-level (`<script>`)
	Functions   []*FunctionChunk // Funções top-level indexadas por fnIdx
	GlobalNames []string         // Nomes simbólicos das variáveis globais (índice == globalSlot)
}

// TotalInstructions contabiliza todas as instruções emitidas no módulo (`<script>` + funções).
func (m *Module) TotalInstructions() int {
	if m == nil {
		return 0
	}
	total := 0
	if m.Script != nil {
		total += len(m.Script.Code)
	}
	for _, fn := range m.Functions {
		total += len(fn.Code)
	}
	return total
}
