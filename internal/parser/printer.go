package parser

import (
	"fmt"
	"strings"

	"vim/internal/diagnostics"
)

// FormatTree serializa a AST em formato hierárquico indentado (estilo `tree`),
// exibindo tipo do nó, atributos semânticos relevantes e Span compacto.
func FormatTree(prog *Program) string {
	if prog == nil {
		return "<AST vazia>\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Program %s\n", shortSpan(prog.SpanInfo)))
	for i, decl := range prog.Decls {
		renderTreeNode(&sb, "", i == len(prog.Decls)-1, "", decl)
	}
	return sb.String()
}

type labeledChild struct {
	role string
	node Node
}

func renderTreeNode(sb *strings.Builder, prefix string, isLast bool, role string, n Node) {
	if n == nil {
		return
	}

	connector := "├── "
	nextPrefix := prefix + "│   "
	if isLast {
		connector = "└── "
		nextPrefix = prefix + "    "
	}

	rolePrefix := ""
	if role != "" {
		rolePrefix = role + ": "
	}

	label, children := describeNode(n)
	sb.WriteString(fmt.Sprintf("%s%s%s%s %s\n", prefix, connector, rolePrefix, label, shortSpan(n.Span())))

	for i, ch := range children {
		renderTreeNode(sb, nextPrefix, i == len(children)-1, ch.role, ch.node)
	}
}

func describeNode(n Node) (string, []labeledChild) {
	switch v := n.(type) {
	case *FnDecl:
		return fmt.Sprintf("FnDecl [%s]", v.SignatureString()), []labeledChild{
			{role: "body", node: v.Body},
		}

	case *VarDecl:
		return fmt.Sprintf("VarDecl (let %s: %s)", v.Name, v.Type.TypeString()), []labeledChild{
			{role: "init", node: v.Value},
		}

	case *ConstDecl:
		return fmt.Sprintf("ConstDecl (const %s: %s)", v.Name, v.Type.TypeString()), []labeledChild{
			{role: "value", node: v.Value},
		}

	case *BlockStmt:
		tag := "BlockStmt"
		if v.IsDesugaredFor {
			tag = "BlockStmt [desugared 'for']"
		}
		children := make([]labeledChild, len(v.Stmts))
		for i, st := range v.Stmts {
			children[i] = labeledChild{node: st}
		}
		return tag, children

	case *AssignStmt:
		return "AssignStmt (=)", []labeledChild{
			{role: "target", node: v.Target},
			{role: "value", node: v.Value},
		}

	case *IfStmt:
		children := []labeledChild{
			{role: "cond", node: v.Cond},
			{role: "then", node: v.Then},
		}
		if v.Else != nil {
			children = append(children, labeledChild{role: "else", node: v.Else})
		}
		return "IfStmt", children

	case *WhileStmt:
		return "WhileStmt", []labeledChild{
			{role: "cond", node: v.Cond},
			{role: "body", node: v.Body},
		}

	case *ForStmt:
		var children []labeledChild
		if v.Init != nil {
			children = append(children, labeledChild{role: "init", node: v.Init})
		}
		if v.Cond != nil {
			children = append(children, labeledChild{role: "cond", node: v.Cond})
		}
		if v.Step != nil {
			children = append(children, labeledChild{role: "step", node: v.Step})
		}
		children = append(children, labeledChild{role: "body", node: v.Body})
		return "ForStmt", children

	case *ReturnStmt:
		if v.Value == nil {
			return "ReturnStmt (void)", nil
		}
		return "ReturnStmt", []labeledChild{
			{role: "value", node: v.Value},
		}

	case *ExprStmt:
		return "ExprStmt", []labeledChild{
			{node: v.Expr},
		}

	case *BinaryExpr:
		return fmt.Sprintf("BinaryExpr (%s)", v.OpLexeme), []labeledChild{
			{role: "left", node: v.Left},
			{role: "right", node: v.Right},
		}

	case *UnaryExpr:
		return fmt.Sprintf("UnaryExpr (%s)", v.OpLexeme), []labeledChild{
			{role: "operand", node: v.Operand},
		}

	case *CallExpr:
		children := make([]labeledChild, 0, len(v.Args)+1)
		children = append(children, labeledChild{role: "callee", node: v.Callee})
		for i, a := range v.Args {
			children = append(children, labeledChild{role: fmt.Sprintf("arg[%d]", i), node: a})
		}
		return fmt.Sprintf("CallExpr (%d args)", len(v.Args)), children

	case *IndexExpr:
		return "IndexExpr ([])", []labeledChild{
			{role: "collection", node: v.Collection},
			{role: "index", node: v.Index},
		}

	case *ArrayLit:
		children := make([]labeledChild, len(v.Elements))
		for i, el := range v.Elements {
			children[i] = labeledChild{role: fmt.Sprintf("[%d]", i), node: el}
		}
		return fmt.Sprintf("ArrayLit (%d elems)", len(v.Elements)), children

	case *IdentExpr:
		return fmt.Sprintf("IdentExpr %q", v.Name), nil

	case *IntLit:
		return fmt.Sprintf("IntLit %d", v.Value), nil

	case *StringLit:
		return fmt.Sprintf("StringLit %q", v.Value), nil

	case *BoolLit:
		return fmt.Sprintf("BoolLit %t", v.Value), nil

	case *PrimType:
		return fmt.Sprintf("PrimType (%s)", v.Name), nil

	case *ArrayType:
		return fmt.Sprintf("ArrayType (%s)", v.TypeString()), nil

	default:
		return fmt.Sprintf("%T", n), nil
	}
}

func shortSpan(sp diagnostics.Span) string {
	if sp.StartLine == sp.EndLine {
		return fmt.Sprintf("[%d:%d..%d]", sp.StartLine, sp.StartCol, sp.EndCol)
	}
	return fmt.Sprintf("[%d:%d..%d:%d]", sp.StartLine, sp.StartCol, sp.EndLine, sp.EndCol)
}

// ============================================================================
// Exportador Graphviz (.dot)
// ============================================================================

// FormatDOT exporta a AST completa como um grafo direcionado Graphviz (.dot) estilizado por categoria.
func FormatDOT(prog *Program) string {
	var sb strings.Builder
	sb.WriteString("digraph vimAST {\n")
	sb.WriteString("  rankdir=TB;\n")
	sb.WriteString("  node [shape=box, style=\"rounded,filled\", fontname=\"Consolas,Menlo,monospace\", fontsize=10];\n")
	sb.WriteString("  edge [fontname=\"Consolas,Menlo,monospace\", fontsize=9, color=\"#555555\"];\n\n")

	if prog != nil {
		id := 0
		rootID := nextDOTID(&id)
		sb.WriteString(fmt.Sprintf("  %s [label=%q, fillcolor=\"#dbeafe\", color=\"#2563eb\"];\n",
			rootID, fmt.Sprintf("Program\n%s", shortSpan(prog.SpanInfo))))

		for _, decl := range prog.Decls {
			childID := emitDOTNode(&sb, decl, &id)
			sb.WriteString(fmt.Sprintf("  %s -> %s;\n", rootID, childID))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

func emitDOTNode(sb *strings.Builder, n Node, counter *int) string {
	myID := nextDOTID(counter)
	label, children := describeNode(n)
	fill, border := dotColors(n)

	fullLabel := fmt.Sprintf("%s\n%s", label, shortSpan(n.Span()))
	sb.WriteString(fmt.Sprintf("  %s [label=%s, fillcolor=%q, color=%q];\n",
		myID, quoteDOT(fullLabel), fill, border))

	for _, ch := range children {
		if ch.node == nil {
			continue
		}
		childID := emitDOTNode(sb, ch.node, counter)
		if ch.role != "" {
			sb.WriteString(fmt.Sprintf("  %s -> %s [label=%s];\n", myID, childID, quoteDOT(ch.role)))
		} else {
			sb.WriteString(fmt.Sprintf("  %s -> %s;\n", myID, childID))
		}
	}
	return myID
}

func nextDOTID(counter *int) string {
	*counter++
	return fmt.Sprintf("n%d", *counter)
}

func quoteDOT(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return `"` + escaped + `"`
}

func dotColors(n Node) (fill, border string) {
	switch n.(type) {
	case *FnDecl, *VarDecl, *ConstDecl:
		return "#e0e7ff", "#4f46e5" // Índigo suave (Declarações)
	case *BlockStmt, *IfStmt, *WhileStmt, *ForStmt, *ReturnStmt, *AssignStmt, *ExprStmt:
		return "#fef3c7", "#d97706" // Âmbar suave (Comandos e Controle)
	case *BinaryExpr, *UnaryExpr, *CallExpr, *IndexExpr:
		return "#dcfce7", "#16a34a" // Verde suave (Operações e Chamadas)
	default:
		return "#f3f4f6", "#6b7280" // Cinza neutro (Literais e Identificadores)
	}
}
