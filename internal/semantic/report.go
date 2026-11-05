package semantic

import (
	"fmt"
	"strings"
)

// FormatReport gera um relatório didático detalhando a Tabela de Símbolos,
// a hierarquia de escopos encadeados e os tipos resolvidos para exibição em `vim --check`.
func FormatReport(res *Result) string {
	var sb strings.Builder

	sb.WriteString("✓ Análise Semântica concluída com sucesso (nenhum erro de tipo ou escopo).\n\n")

	if res == nil || res.GlobalScope == nil {
		return sb.String()
	}

	// 1. Tabela de Símbolos Globais (Funções, Variáveis, Constantes e Intrínsecos)
	sb.WriteString("=== Tabela de Símbolos Globais ===\n")
	sb.WriteString(fmt.Sprintf("%-16s %-12s %-10s %-34s %s\n", "SÍMBOLO", "CATEGORIA", "MUTÁVEL", "TIPO ESTÁTICO", "ORIGEM"))
	sb.WriteString(strings.Repeat("-", 88) + "\n")

	for _, sym := range res.GlobalScope.Order {
		mut := "não"
		if sym.IsMutable {
			mut = "sim"
		}
		origin := "<nativo>"
		if sym.DefSpan.StartLine > 0 {
			origin = fmt.Sprintf("linha %d:%d", sym.DefSpan.StartLine, sym.DefSpan.StartCol)
		}
		sb.WriteString(fmt.Sprintf("%-16s %-12s %-10s %-34s %s\n",
			sym.Name, sym.Kind, mut, sym.Type.String(), origin))
	}

	// 2. Árvore de Escopos Aninhados (Funções e Blocos)
	if len(res.GlobalScope.Children) > 0 {
		sb.WriteString("\n=== Hierarquia de Escopos Aninhados ===\n")
		sb.WriteString("global\n")
		for i, ch := range res.GlobalScope.Children {
			renderScopeTree(&sb, "", i == len(res.GlobalScope.Children)-1, ch)
		}
	}

	sb.WriteString(fmt.Sprintf("\nResumo Semântico: %d escopo(s) verificado(s), %d símbolo(s) do usuário, %d expressão(ões) tipada(s).\n",
		res.ScopeCount, res.SymbolCount, len(res.ExprTypes)))

	return sb.String()
}

func renderScopeTree(sb *strings.Builder, prefix string, isLast bool, sc *Scope) {
	conn := "├── "
	nextPrefix := prefix + "│   "
	if isLast {
		conn = "└── "
		nextPrefix = prefix + "    "
	}

	var symDescs []string
	for _, sym := range sc.Order {
		mutTag := ""
		if sym.Kind == KindConst {
			mutTag = " [const]"
		} else if sym.Kind == KindParam {
			mutTag = " [param]"
		}
		symDescs = append(symDescs, fmt.Sprintf("%s: %s%s", sym.Name, sym.Type.String(), mutTag))
	}

	if len(symDescs) > 0 {
		sb.WriteString(fmt.Sprintf("%s%s[%s] -> { %s }\n", prefix, conn, sc.Name, strings.Join(symDescs, ", ")))
	} else {
		sb.WriteString(fmt.Sprintf("%s%s[%s] -> { (sem declarações locais) }\n", prefix, conn, sc.Name))
	}

	for i, ch := range sc.Children {
		renderScopeTree(sb, nextPrefix, i == len(sc.Children)-1, ch)
	}
}
