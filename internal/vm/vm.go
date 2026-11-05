package vm

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"vim/internal/diagnostics"
	"vim/internal/ir"
)

// Códigos padronizados de diagnósticos de execução (Runtime Errors E03xx).
const (
	ErrRuntimeDivByZero     = "E0301"
	ErrRuntimeIndexBounds   = "E0302"
	ErrRuntimeStackOverflow = "E0303"
	ErrRuntimeReadInt       = "E0304"
)

const DefaultMaxFrames = 1024

// RuntimeError encapsula uma falha ocorrida em tempo de execução preservando o Span exato da instrução.
type RuntimeError struct {
	Code    string
	Message string
	Label   string
	Help    string
	Span    diagnostics.Span
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("erro[%s]: %s (%s)", e.Code, e.Message, e.Span)
}

// ToDiagnostic converte o erro de execução para renderização visual com `diagnostics.Render`.
func (e *RuntimeError) ToDiagnostic() diagnostics.Diagnostic {
	return diagnostics.Diagnostic{
		Severity: diagnostics.SeverityError,
		Code:     e.Code,
		Message:  e.Message,
		Label:    e.Label,
		Help:     e.Help,
		Span:     e.Span,
	}
}

// Config permite injetar fluxos de entrada/saída customizados e limite de recursão para testes e CLI.
type Config struct {
	Stdin     io.Reader
	Stdout    io.Writer
	MaxFrames int
}

// VM implementa a Máquina Virtual de Pilha (Stack VM) dedicada da linguagem vim.
type VM struct {
	mod       *ir.Module
	stack     []Value
	frames    []CallFrame
	globals   []Value
	maxFrames int
	reader    *bufio.Reader
	stdout    io.Writer
	steps     uint64
}

// New instancia uma vim Máquina Virtual pronta para executar o `ir.Module`.
func New(mod *ir.Module, cfg Config) *VM {
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	maxFrames := cfg.MaxFrames
	if maxFrames <= 0 {
		maxFrames = DefaultMaxFrames
	}

	var globals []Value
	if mod != nil && len(mod.GlobalNames) > 0 {
		globals = make([]Value, len(mod.GlobalNames))
	}

	return &VM{
		mod:       mod,
		stack:     make([]Value, 0, 256),
		frames:    make([]CallFrame, 0, 64),
		globals:   globals,
		maxFrames: maxFrames,
		reader:    bufio.NewReader(stdin),
		stdout:    stdout,
	}
}

// ExecutedSteps retorna a quantidade total de instruções despachadas pela VM (útil para `--stats`).
func (vm *VM) ExecutedSteps() uint64 {
	return vm.steps
}

// Run executa o módulo desde o entry-point `<script>` até `OpHalt` ou até um `RuntimeError`.
func (vm *VM) Run() *RuntimeError {
	if vm.mod == nil || vm.mod.Script == nil {
		return nil
	}

	// Inicializa o frame raiz `<script>` reservando seus slots locais (para blocos top-level)
	vm.stack = vm.stack[:0]
	vm.frames = vm.frames[:0]
	for i := 0; i < vm.mod.Script.NumLocals; i++ {
		vm.stack = append(vm.stack, VoidVal())
	}
	vm.frames = append(vm.frames, CallFrame{
		Fn:       vm.mod.Script,
		IP:       0,
		BaseSlot: 0,
	})

	for len(vm.frames) > 0 {
		frame := &vm.frames[len(vm.frames)-1]
		inst := frame.Fn.Code[frame.IP]
		frame.IP++
		vm.steps++

		switch inst.Op {
		case ir.OpConst:
			c := frame.Fn.Constants[inst.ArgA]
			if c.Kind == ir.ConstString {
				vm.push(StringVal(c.StrVal))
			} else {
				vm.push(IntVal(c.IntVal))
			}

		case ir.OpVoid:
			vm.push(VoidVal())

		case ir.OpTrue:
			vm.push(BoolVal(true))

		case ir.OpFalse:
			vm.push(BoolVal(false))

		case ir.OpPop:
			_ = vm.pop()

		case ir.OpGetGlobal:
			vm.push(vm.globals[inst.ArgA])

		case ir.OpSetGlobal:
			vm.globals[inst.ArgA] = vm.pop()

		case ir.OpGetLocal:
			vm.push(vm.stack[frame.BaseSlot+inst.ArgA])

		case ir.OpSetLocal:
			vm.stack[frame.BaseSlot+inst.ArgA] = vm.pop()

		case ir.OpNewArray:
			count := inst.ArgA
			elems := make([]Value, count)
			startIdx := len(vm.stack) - count
			copy(elems, vm.stack[startIdx:])
			vm.stack = vm.stack[:startIdx]
			vm.push(ArrayVal(&ArrayObject{Elements: elems}))

		case ir.OpGetIndex:
			idx := vm.pop().Int
			arr := vm.pop().Arr
			if idx < 0 || idx >= int64(len(arr.Elements)) {
				return &RuntimeError{
					Code:    ErrRuntimeIndexBounds,
					Message: fmt.Sprintf("índice %d fora dos limites para array de tamanho %d", idx, len(arr.Elements)),
					Label:   fmt.Sprintf("índice inválido (%d)", idx),
					Help:    fmt.Sprintf("índices válidos para este array vão de 0 até %d", max(0, len(arr.Elements)-1)),
					Span:    inst.Span,
				}
			}
			vm.push(arr.Elements[idx])

		case ir.OpSetIndex:
			val := vm.pop()
			idx := vm.pop().Int
			arr := vm.pop().Arr
			if idx < 0 || idx >= int64(len(arr.Elements)) {
				return &RuntimeError{
					Code:    ErrRuntimeIndexBounds,
					Message: fmt.Sprintf("atribuição no índice %d fora dos limites para array de tamanho %d", idx, len(arr.Elements)),
					Label:   fmt.Sprintf("índice fora dos limites (%d)", idx),
					Help:    "use 'push(arr, valor)' se desejar expandir o array ao final",
					Span:    inst.Span,
				}
			}
			arr.Elements[idx] = val

		case ir.OpAdd:
			b := vm.pop()
			a := vm.pop()
			if a.Kind == ValString && b.Kind == ValString {
				vm.push(StringVal(a.Str + b.Str))
			} else {
				vm.push(IntVal(a.Int + b.Int))
			}

		case ir.OpConcat:
			b := vm.pop().Str
			a := vm.pop().Str
			vm.push(StringVal(a + b))

		case ir.OpSub:
			b := vm.pop().Int
			a := vm.pop().Int
			vm.push(IntVal(a - b))

		case ir.OpMul:
			b := vm.pop().Int
			a := vm.pop().Int
			vm.push(IntVal(a * b))

		case ir.OpDiv:
			b := vm.pop().Int
			a := vm.pop().Int
			if b == 0 {
				return &RuntimeError{
					Code:    ErrRuntimeDivByZero,
					Message: "divisão inteira por zero em tempo de execução",
					Label:   "divisor avaliou para 0",
					Help:    "verifique se o denominador é diferente de zero antes de dividir",
					Span:    inst.Span,
				}
			}
			if a == math.MinInt64 && b == -1 {
				return &RuntimeError{
					Code:    ErrRuntimeDivByZero,
					Message: "estouro aritmético de 64 bits na divisão (-9223372036854775808 / -1)",
					Label:   "quociente excede MaxInt64",
					Span:    inst.Span,
				}
			}
			vm.push(IntVal(a / b))

		case ir.OpMod:
			b := vm.pop().Int
			a := vm.pop().Int
			if b == 0 {
				return &RuntimeError{
					Code:    ErrRuntimeDivByZero,
					Message: "operação de módulo ('%') por zero em tempo de execução",
					Label:   "divisor avaliou para 0",
					Help:    "verifique se o divisor é diferente de zero antes de calcular o resto",
					Span:    inst.Span,
				}
			}
			if a == math.MinInt64 && b == -1 {
				vm.push(IntVal(0))
			} else {
				vm.push(IntVal(a % b))
			}

		case ir.OpNeg:
			a := vm.pop().Int
			vm.push(IntVal(-a))

		case ir.OpNot:
			b := vm.pop().Bool
			vm.push(BoolVal(!b))

		case ir.OpEq:
			b := vm.pop()
			a := vm.pop()
			vm.push(BoolVal(a.Equals(b)))

		case ir.OpNotEq:
			b := vm.pop()
			a := vm.pop()
			vm.push(BoolVal(!a.Equals(b)))

		case ir.OpLt:
			b := vm.pop().Int
			a := vm.pop().Int
			vm.push(BoolVal(a < b))

		case ir.OpLtEq:
			b := vm.pop().Int
			a := vm.pop().Int
			vm.push(BoolVal(a <= b))

		case ir.OpGt:
			b := vm.pop().Int
			a := vm.pop().Int
			vm.push(BoolVal(a > b))

		case ir.OpGtEq:
			b := vm.pop().Int
			a := vm.pop().Int
			vm.push(BoolVal(a >= b))

		case ir.OpJump:
			frame.IP = inst.ArgA

		case ir.OpJumpIfFalse:
			if !vm.pop().Bool {
				frame.IP = inst.ArgA
			}

		case ir.OpJumpIfFalseKeep:
			if !vm.peek().Bool {
				frame.IP = inst.ArgA
			}

		case ir.OpJumpIfTrueKeep:
			if vm.peek().Bool {
				frame.IP = inst.ArgA
			}

		case ir.OpCall:
			fnIdx := inst.ArgA
			argCount := inst.ArgB
			if len(vm.frames) >= vm.maxFrames {
				return &RuntimeError{
					Code:    ErrRuntimeStackOverflow,
					Message: fmt.Sprintf("estouro da pilha de chamadas (stack overflow): limite de %d ativações excedido", vm.maxFrames),
					Label:   "recursão profunda ou infinita detectada aqui",
					Help:    "verifique o caso base de parada da função recursiva",
					Span:    inst.Span,
				}
			}

			callee := vm.mod.Functions[fnIdx]
			baseSlot := len(vm.stack) - argCount
			extraLocals := callee.NumLocals - argCount
			for i := 0; i < extraLocals; i++ {
				vm.stack = append(vm.stack, VoidVal())
			}
			vm.frames = append(vm.frames, CallFrame{
				Fn:       callee,
				IP:       0,
				BaseSlot: baseSlot,
			})

		case ir.OpIntrinsic:
			if err := vm.execIntrinsic(ir.IntrinsicID(inst.ArgA), inst.ArgB, inst.Span); err != nil {
				return err
			}

		case ir.OpReturn:
			retVal := vm.pop()
			finished := vm.frames[len(vm.frames)-1]
			vm.frames = vm.frames[:len(vm.frames)-1]
			vm.stack = vm.stack[:finished.BaseSlot]
			vm.push(retVal)

		case ir.OpHalt:
			return nil
		}
	}

	return nil
}

func (vm *VM) execIntrinsic(id ir.IntrinsicID, argCount int, span diagnostics.Span) *RuntimeError {
	switch id {
	case ir.IntrinsicPrint:
		start := len(vm.stack) - argCount
		parts := make([]string, argCount)
		for i := 0; i < argCount; i++ {
			parts[i] = vm.stack[start+i].String()
		}
		vm.stack = vm.stack[:start]
		fmt.Fprintln(vm.stdout, strings.Join(parts, " "))
		vm.push(VoidVal())
		return nil

	case ir.IntrinsicRead:
		line, err := vm.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			line = ""
		}
		line = strings.TrimRight(line, "\r\n")
		vm.push(StringVal(line))
		return nil

	case ir.IntrinsicReadInt:
		line, err := vm.reader.ReadString('\n')
		if err != nil && err != io.EOF && len(line) == 0 {
			return &RuntimeError{
				Code:    ErrRuntimeReadInt,
				Message: "erro de leitura na entrada padrão durante 'read_int()'",
				Label:   "falha de I/O",
				Span:    span,
			}
		}
		trimmed := strings.TrimSpace(line)
		val, parseErr := strconv.ParseInt(trimmed, 10, 64)
		if parseErr != nil {
			shown := fmt.Sprintf("%q", trimmed)
			if trimmed == "" {
				shown = "<entrada vazia/EOF>"
			}
			return &RuntimeError{
				Code:    ErrRuntimeReadInt,
				Message: fmt.Sprintf("falha ao converter %s em inteiro de 64 bits em 'read_int()'", shown),
				Label:   "entrada não numérica ou fora do intervalo",
				Help:    "digite apenas dígitos inteiros válidos (ex: 42 ou -10)",
				Span:    span,
			}
		}
		vm.push(IntVal(val))
		return nil

	case ir.IntrinsicLen:
		v := vm.pop()
		if v.Kind == ValString {
			vm.push(IntVal(int64(utf8.RuneCountInString(v.Str))))
		} else {
			vm.push(IntVal(int64(len(v.Arr.Elements))))
		}
		return nil

	case ir.IntrinsicPush:
		elem := vm.pop()
		arr := vm.pop().Arr
		arr.Elements = append(arr.Elements, elem)
		vm.push(VoidVal())
		return nil
	}
	return nil
}

func (vm *VM) push(v Value) {
	vm.stack = append(vm.stack, v)
}

func (vm *VM) pop() Value {
	last := len(vm.stack) - 1
	v := vm.stack[last]
	vm.stack = vm.stack[:last]
	return v
}

func (vm *VM) peek() Value {
	return vm.stack[len(vm.stack)-1]
}
