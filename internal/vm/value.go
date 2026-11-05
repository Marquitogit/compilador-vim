package vm

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueKind identifica a tag dinâmica segura de um valor armazenado na Stack VM.
type ValueKind uint8

const (
	ValVoid ValueKind = iota
	ValInt
	ValBool
	ValString
	ValArray
)

// ArrayObject armazena os elementos de um array por referência (heap da VM),
// garantindo que `push(arr, x)` e mutações `arr[i] = x` reflitam em todas as referências ao mesmo vetor.
type ArrayObject struct {
	Elements []Value
}

// Value representa um valor em tempo de execução na pilha de operandos, slots locais ou globais.
type Value struct {
	Kind ValueKind
	Int  int64
	Bool bool
	Str  string
	Arr  *ArrayObject
}

func VoidVal() Value               { return Value{Kind: ValVoid} }
func IntVal(v int64) Value         { return Value{Kind: ValInt, Int: v} }
func BoolVal(v bool) Value         { return Value{Kind: ValBool, Bool: v} }
func StringVal(v string) Value     { return Value{Kind: ValString, Str: v} }
func ArrayVal(a *ArrayObject) Value { return Value{Kind: ValArray, Arr: a} }

// String formata o valor para saída padrão (`print`), exibindo arrays como `[e1, e2, ...]`.
func (v Value) String() string {
	return v.format(false)
}

func (v Value) format(quoteStrings bool) string {
	switch v.Kind {
	case ValVoid:
		return "<void>"
	case ValInt:
		return strconv.FormatInt(v.Int, 10)
	case ValBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case ValString:
		if quoteStrings {
			return fmt.Sprintf("%q", v.Str)
		}
		return v.Str
	case ValArray:
		if v.Arr == nil {
			return "[]"
		}
		parts := make([]string, len(v.Arr.Elements))
		for i, el := range v.Arr.Elements {
			parts[i] = el.format(true)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return "<desconhecido>"
	}
}

// Equals compara dois valores escalares em `OpEq` / `OpNotEq`.
func (v Value) Equals(other Value) bool {
	if v.Kind != other.Kind {
		return false
	}
	switch v.Kind {
	case ValInt:
		return v.Int == other.Int
	case ValBool:
		return v.Bool == other.Bool
	case ValString:
		return v.Str == other.Str
	case ValVoid:
		return true
	default:
		return v.Arr == other.Arr
	}
}
