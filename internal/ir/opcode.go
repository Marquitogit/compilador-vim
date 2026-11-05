package ir

import "fmt"

// OpCode representa uma instrução de máquina da Stack VM da linguagem vim.
type OpCode uint8

const (
	// Constantes e Pilha Básica
	OpConst OpCode = iota // [A=constIdx] Empilha chunk.Constants[A]
	OpVoid                // Empilha valor vazio (void)
	OpTrue                // Empilha literal booleano true
	OpFalse               // Empilha literal booleano false
	OpPop                 // Descarta o valor no topo da pilha

	// Variáveis Globais e Locais (baseadas em slots O(1))
	OpGetGlobal // [A=globalSlot] Empilha globals[A]
	OpSetGlobal // [A=globalSlot] Desempilha valor e grava em globals[A]
	OpGetLocal  // [A=localSlot]  Empilha stack[frame.baseSlot + A]
	OpSetLocal  // [A=localSlot]  Desempilha valor e grava em stack[frame.baseSlot + A]

	// Estruturas de Array por Referência (*ArrayObject)
	OpNewArray // [A=elemCount] Desempilha A elementos e empilha novo *ArrayObject
	OpGetIndex // Desempilha índice e array; empilha array[índice] (valida E0302)
	OpSetIndex // Desempilha valor, índice e array; faz array[índice] = valor (valida E0302)

	// Operações Aritméticas, Texto e Unárias
	OpAdd    // int + int -> int
	OpConcat // string + string -> string
	OpSub    // int - int -> int
	OpMul    // int * int -> int
	OpDiv    // int / int -> int (valida divisão por zero E0301)
	OpMod    // int % int -> int (valida módulo por zero E0301)
	OpNeg    // -int -> int
	OpNot    // !bool -> bool

	// Operações Relacionais e de Igualdade
	OpEq    // == -> bool
	OpNotEq // != -> bool
	OpLt    // <  -> bool
	OpLtEq  // <= -> bool
	OpGt    // >  -> bool
	OpGtEq  // >= -> bool

	// Desvios e Curto-Circuito Booleano
	OpJump            // [A=targetIP] Salto incondicional para instrução A
	OpJumpIfFalse     // [A=targetIP] Desempilha bool; se false, salta para A
	OpJumpIfFalseKeep // [A=targetIP] Inspeciona topo sem desempilhar; se false, salta para A (curto-circuito &&)
	OpJumpIfTrueKeep  // [A=targetIP] Inspeciona topo sem desempilhar; se true, salta para A  (curto-circuito ||)

	// Chamadas de Função, Intrínsecos e Retorno
	OpCall      // [A=fnIdx, B=argCount] Invoca Module.Functions[A] com B argumentos
	OpIntrinsic // [A=intrinsicID, B=argCount] Executa função nativa A com B argumentos
	OpReturn    // Finaliza CallFrame atual retornando o valor do topo da pilha
	OpHalt      // Encerra a execução do módulo (<script>)
)

// IntrinsicID identifica em O(1) qual rotina nativa `OpIntrinsic` deve despachar na VM.
type IntrinsicID int

const (
	IntrinsicPrint IntrinsicID = iota
	IntrinsicRead
	IntrinsicReadInt
	IntrinsicLen
	IntrinsicPush
)

func (id IntrinsicID) String() string {
	switch id {
	case IntrinsicPrint:
		return "print"
	case IntrinsicRead:
		return "read"
	case IntrinsicReadInt:
		return "read_int"
	case IntrinsicLen:
		return "len"
	case IntrinsicPush:
		return "push"
	default:
		return fmt.Sprintf("intrinsic(%d)", int(id))
	}
}

var opNames = map[OpCode]string{
	OpConst:           "CONST",
	OpVoid:            "VOID",
	OpTrue:            "TRUE",
	OpFalse:           "FALSE",
	OpPop:             "POP",
	OpGetGlobal:       "GET_GLOBAL",
	OpSetGlobal:       "SET_GLOBAL",
	OpGetLocal:        "GET_LOCAL",
	OpSetLocal:        "SET_LOCAL",
	OpNewArray:        "NEW_ARRAY",
	OpGetIndex:        "GET_INDEX",
	OpSetIndex:        "SET_INDEX",
	OpAdd:             "ADD",
	OpConcat:          "CONCAT",
	OpSub:             "SUB",
	OpMul:             "MUL",
	OpDiv:             "DIV",
	OpMod:             "MOD",
	OpNeg:             "NEG",
	OpNot:             "NOT",
	OpEq:              "EQ",
	OpNotEq:           "NOT_EQ",
	OpLt:              "LT",
	OpLtEq:            "LT_EQ",
	OpGt:              "GT",
	OpGtEq:            "GT_EQ",
	OpJump:            "JUMP",
	OpJumpIfFalse:     "JUMP_IF_FALSE",
	OpJumpIfFalseKeep: "JUMP_IF_FALSE_KEEP",
	OpJumpIfTrueKeep:  "JUMP_IF_TRUE_KEEP",
	OpCall:            "CALL",
	OpIntrinsic:       "INTRINSIC",
	OpReturn:          "RETURN",
	OpHalt:            "HALT",
}

func (op OpCode) String() string {
	if s, ok := opNames[op]; ok {
		return s
	}
	return fmt.Sprintf("OP_%d", uint8(op))
}
