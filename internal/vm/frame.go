package vm

import "vim/internal/ir"

// CallFrame representa um registro de ativação (stack frame) na pilha de chamadas da VM.
type CallFrame struct {
	Fn       *ir.FunctionChunk // Função em execução (ou `<script>` no frame 0)
	IP       int               // Instruction Pointer (próxima instrução a buscar)
	BaseSlot int               // Índice base na pilha de valores onde começam os parâmetros/locais deste frame
}
