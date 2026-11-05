package ir

import (
	"fmt"
	"strings"
)

// Disassemble formata um `Module` inteiro em representação textual didática e alinhada para `--dump-ir`.
func Disassemble(mod *Module) string {
	if mod == nil {
		return "<módulo IR vazio>\n"
	}

	var sb strings.Builder
	sb.WriteString("=== Representação Intermediária: Bytecode da Stack VM ===\n")
	if len(mod.GlobalNames) > 0 {
		sb.WriteString(fmt.Sprintf("Globais registradas (%d): [%s]\n",
			len(mod.GlobalNames), strings.Join(mod.GlobalNames, ", ")))
	}

	for i, fn := range mod.Functions {
		sb.WriteByte('\n')
		disassembleChunk(&sb, mod, fn, fmt.Sprintf("fn #%d: %s", i, fn.Name))
	}

	if mod.Script != nil {
		sb.WriteByte('\n')
		disassembleChunk(&sb, mod, mod.Script, "entry: <script>")
	}

	sb.WriteString(fmt.Sprintf("\nTotal de instruções no módulo: %d\n", mod.TotalInstructions()))
	return sb.String()
}

func disassembleChunk(sb *strings.Builder, mod *Module, fc *FunctionChunk, title string) {
	sb.WriteString(fmt.Sprintf("--- [%s] (aridade=%d, slots_locais=%d, constantes=%d, instruções=%d) ---\n",
		title, fc.Arity, fc.NumLocals, len(fc.Constants), len(fc.Code)))
	sb.WriteString(fmt.Sprintf("  %-6s %-20s %-14s %-28s %s\n", "IP", "OPCODE", "OPERANDOS", "DETALHE SIMBÓLICO", "ORIGEM"))
	sb.WriteString("  " + strings.Repeat("-", 82) + "\n")

	for ip, inst := range fc.Code {
		operands, comment := describeInstruction(mod, fc, inst)
		origin := "-"
		if inst.Span.StartLine > 0 {
			origin = fmt.Sprintf("L%d:%d", inst.Span.StartLine, inst.Span.StartCol)
		}
		sb.WriteString(fmt.Sprintf("  %04d   %-20s %-14s %-28s %s\n",
			ip, inst.Op.String(), operands, comment, origin))
	}
}

func describeInstruction(mod *Module, fc *FunctionChunk, inst Instruction) (operands string, comment string) {
	switch inst.Op {
	case OpConst:
		operands = fmt.Sprintf("%d", inst.ArgA)
		if inst.ArgA >= 0 && inst.ArgA < len(fc.Constants) {
			comment = fmt.Sprintf("; const %s", fc.Constants[inst.ArgA].String())
		}

	case OpGetGlobal, OpSetGlobal:
		operands = fmt.Sprintf("%d", inst.ArgA)
		if inst.ArgA >= 0 && inst.ArgA < len(mod.GlobalNames) {
			comment = fmt.Sprintf("; global %q", mod.GlobalNames[inst.ArgA])
		}

	case OpGetLocal, OpSetLocal:
		operands = fmt.Sprintf("%d", inst.ArgA)
		if inst.ArgA >= 0 && inst.ArgA < len(fc.LocalNames) {
			role := "local"
			if inst.ArgA < fc.Arity {
				role = "param"
			}
			comment = fmt.Sprintf("; %s %q", role, fc.LocalNames[inst.ArgA])
		}

	case OpNewArray:
		operands = fmt.Sprintf("%d", inst.ArgA)
		comment = fmt.Sprintf("; %d elemento(s)", inst.ArgA)

	case OpJump, OpJumpIfFalse, OpJumpIfFalseKeep, OpJumpIfTrueKeep:
		operands = fmt.Sprintf("-> %04d", inst.ArgA)

	case OpCall:
		operands = fmt.Sprintf("fn=%d, argc=%d", inst.ArgA, inst.ArgB)
		if inst.ArgA >= 0 && inst.ArgA < len(mod.Functions) {
			comment = fmt.Sprintf("; %s(%d args)", mod.Functions[inst.ArgA].Name, inst.ArgB)
		}

	case OpIntrinsic:
		id := IntrinsicID(inst.ArgA)
		operands = fmt.Sprintf("id=%d, argc=%d", inst.ArgA, inst.ArgB)
		comment = fmt.Sprintf("; %s(%d args)", id.String(), inst.ArgB)
	}

	if len(comment) > 28 {
		comment = comment[:25] + "..."
	}
	return operands, comment
}
