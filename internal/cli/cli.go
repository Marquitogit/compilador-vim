package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"vim/internal/diagnostics"
	"vim/internal/ir"
	"vim/internal/lexer"
	"vim/internal/parser"
	"vim/internal/semantic"
	"vim/internal/vm"
)

// Mode identifica a ação principal solicitada pela linha de comando.
type Mode int

const (
	ModeNone Mode = iota
	ModeTokens
	ModeAST
	ModeASTDot
	ModeCheck
	ModeDumpIR
	ModeRun
)

type compileStats struct {
	file         string
	byteCount    int
	lineCount    int
	tokens       []lexer.Token
	astNodes     int
	desugaredFor int
	scopeCount   int
	symbolCount  int
	typedExprs   int
	optEnabled   bool
	optStats     ir.OptStats
	irInstCount  int
	irFnCount    int
	vmSteps      uint64
	errCount     int
	lexElapsed   time.Duration
	parseElapsed time.Duration
	semElapsed   time.Duration
	irElapsed    time.Duration
	vmElapsed    time.Duration
	ranParser    bool
	ranSemantic  bool
	ranIR        bool
	ranVM        bool
}

// Run processa os argumentos de linha de comando usando `os.Stdin` como entrada padrão.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInput(args, os.Stdin, stdout, stderr)
}

// RunWithInput permite injetar `stdin` customizado em testes automatizados do CLI.
func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	var (
		mode       Mode
		showStats  bool
		enableOpt  bool
		noDesugar  bool
		showHelp   bool
		filePath   string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			showHelp = true
		case "--stats":
			showStats = true
		case "--opt":
			enableOpt = true
		case "--no-desugar":
			noDesugar = true
		case "--tokens":
			mode = ModeTokens
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				filePath = args[i+1]
				i++
			}
		case "--ast":
			mode = ModeAST
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				filePath = args[i+1]
				i++
			}
		case "--ast-dot":
			mode = ModeASTDot
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				filePath = args[i+1]
				i++
			}
		case "--check":
			mode = ModeCheck
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				filePath = args[i+1]
				i++
			}
		case "--dump-ir":
			mode = ModeDumpIR
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				filePath = args[i+1]
				i++
			}
		case "--run":
			mode = ModeRun
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				filePath = args[i+1]
				i++
			}
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "erro: flag desconhecida %q\n\n", arg)
				printUsage(stderr)
				return 2
			}
			if filePath == "" {
				filePath = arg
			} else {
				fmt.Fprintf(stderr, "erro: múltiplos arquivos informados (%q e %q)\n", filePath, arg)
				return 2
			}
		}
	}

	if showHelp {
		printUsage(stdout)
		return 0
	}

	if mode == ModeNone && showStats && filePath != "" {
		mode = ModeCheck
	}

	if mode == ModeNone {
		fmt.Fprintln(stderr, "erro: especifique uma flag de fase (--tokens, --ast, --ast-dot, --check, --dump-ir ou --run).")
		printUsage(stderr)
		return 2
	}

	if filePath == "" {
		fmt.Fprintln(stderr, "erro: caminho do arquivo '.vim' não informado.")
		return 2
	}

	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "erro ao ler arquivo %q: %v\n", filePath, err)
		return 1
	}
	src := string(rawBytes)
	sm := diagnostics.NewSourceMap(filePath, src)
	diag := diagnostics.NewBag()

	// Fase 1: Analisador Léxico
	lexTimer := startTimer()
	scanner := lexer.New(filePath, src, diag)
	tokens := scanner.ScanAll()
	lexElapsed := lexTimer.Elapsed()

	if mode == ModeTokens {
		printTokenTable(stdout, tokens)
		if showStats {
			printStats(stdout, compileStats{
				file:       filePath,
				byteCount:  len(rawBytes),
				lineCount:  sm.LineCount(),
				tokens:     tokens,
				errCount:   diag.ErrorCount(),
				lexElapsed: lexElapsed,
			})
		}
		if diag.HasErrors() {
			fmt.Fprintln(stderr)
			fmt.Fprint(stderr, diagnostics.Render(sm, diag))
			return 1
		}
		return 0
	}

	// Fase 2: Analisador Sintático + Desaçucaramento
	parseTimer := startTimer()
	p := parser.New(tokens, diag)
	prog := p.ParseProgram()
	desugaredCount := 0
	if !noDesugar {
		prog, desugaredCount = parser.DesugarWithCount(prog)
	}
	parseElapsed := parseTimer.Elapsed()

	if mode == ModeAST || mode == ModeASTDot {
		var optSt ir.OptStats
		if enableOpt && !diag.HasErrors() {
			prog, optSt = ir.Optimize(prog)
		}

		if !diag.HasErrors() {
			if mode == ModeASTDot {
				fmt.Fprint(stdout, parser.FormatDOT(prog))
			} else {
				fmt.Fprint(stdout, parser.FormatTree(prog))
			}
		} else if mode == ModeAST && len(prog.Decls) > 0 {
			fmt.Fprintln(stdout, "// AST parcial recuperada após sincronização de erros:")
			fmt.Fprint(stdout, parser.FormatTree(prog))
		}

		if showStats {
			printStats(stdout, compileStats{
				file:         filePath,
				byteCount:    len(rawBytes),
				lineCount:    sm.LineCount(),
				tokens:       tokens,
				astNodes:     parser.CountNodes(prog),
				desugaredFor: desugaredCount,
				optEnabled:   enableOpt,
				optStats:     optSt,
				errCount:     diag.ErrorCount(),
				lexElapsed:   lexElapsed,
				parseElapsed: parseElapsed,
				ranParser:    true,
			})
		}

		if diag.HasErrors() {
			fmt.Fprintln(stderr)
			fmt.Fprint(stderr, diagnostics.Render(sm, diag))
			return 1
		}
		return 0
	}

	// Fase 3: Analisador Semântico (executado sobre AST sintaticamente íntegra)
	var semRes *semantic.Result
	var semElapsed time.Duration
	ranSem := false
	if !diag.HasErrors() {
		semTimer := startTimer()
		semRes = semantic.Check(prog, diag)
		semElapsed = semTimer.Elapsed()
		ranSem = true
	}

	st := compileStats{
		file:         filePath,
		byteCount:    len(rawBytes),
		lineCount:    sm.LineCount(),
		tokens:       tokens,
		astNodes:     parser.CountNodes(prog),
		desugaredFor: desugaredCount,
		optEnabled:   enableOpt,
		lexElapsed:   lexElapsed,
		parseElapsed: parseElapsed,
		semElapsed:   semElapsed,
		ranParser:    true,
		ranSemantic:  ranSem,
	}
	if semRes != nil {
		st.scopeCount = semRes.ScopeCount
		st.symbolCount = semRes.SymbolCount
		st.typedExprs = len(semRes.ExprTypes)
	}

	if mode == ModeCheck {
		if !diag.HasErrors() && semRes != nil {
			fmt.Fprint(stdout, semantic.FormatReport(semRes))
		}
		st.errCount = diag.ErrorCount()
		if showStats {
			printStats(stdout, st)
		}
		if diag.HasErrors() {
			fmt.Fprintln(stderr)
			fmt.Fprint(stderr, diagnostics.Render(sm, diag))
			return 1
		}
		return 0
	}

	// Abortamos antes da geração de código caso haja qualquer erro léxico, sintático ou semântico
	if diag.HasErrors() {
		st.errCount = diag.ErrorCount()
		if showStats {
			printStats(stdout, st)
		}
		fmt.Fprintln(stderr)
		fmt.Fprint(stderr, diagnostics.Render(sm, diag))
		return 1
	}

	// Fase 4A: Otimizador Opcional (`--opt`) + Gerador de Bytecode IR
	irTimer := startTimer()
	if enableOpt {
		prog, st.optStats = ir.Optimize(prog)
		st.astNodes = parser.CountNodes(prog)
	}
	mod := ir.Generate(prog)
	st.irElapsed = irTimer.Elapsed()
	st.ranIR = true
	st.irInstCount = mod.TotalInstructions()
	st.irFnCount = len(mod.Functions)

	if mode == ModeDumpIR {
		fmt.Fprint(stdout, ir.Disassemble(mod))
		if showStats {
			printStats(stdout, st)
		}
		return 0
	}

	// Fase 4B: Execução na Máquina Virtual de Pilha (`--run`)
	vmTimer := startTimer()
	machine := vm.New(mod, vm.Config{
		Stdin:  stdin,
		Stdout: stdout,
	})
	rtErr := machine.Run()
	st.vmElapsed = vmTimer.Elapsed()
	st.ranVM = true
	st.vmSteps = machine.ExecutedSteps()

	if rtErr != nil {
		diag.Add(rtErr.ToDiagnostic())
		st.errCount = diag.ErrorCount()
		if showStats {
			printStats(stdout, st)
		}
		fmt.Fprintln(stderr)
		fmt.Fprint(stderr, diagnostics.Render(sm, diag))
		return 1
	}

	if showStats {
		printStats(stdout, st)
	}
	return 0
}

func printTokenTable(w io.Writer, tokens []lexer.Token) {
	fmt.Fprintf(w, "%-5s %-14s %-26s %-16s %s\n", "#", "TIPO", "LEXEMA / LITERAL", "LINHA:COL", "BYTES [ini..fim)")
	fmt.Fprintln(w, strings.Repeat("-", 80))

	for i, tok := range tokens {
		display := fmt.Sprintf("%q", tok.Lexeme)
		if tok.Type == lexer.TokenEOF {
			display = "<EOF>"
		} else if len(display) > 24 {
			display = display[:21] + "...\""
		}

		loc := fmt.Sprintf("%d:%d..%d:%d", tok.Span.StartLine, tok.Span.StartCol, tok.Span.EndLine, tok.Span.EndCol)
		if tok.Span.StartLine == tok.Span.EndLine {
			loc = fmt.Sprintf("%d:%d..%d", tok.Span.StartLine, tok.Span.StartCol, tok.Span.EndCol)
		}

		offsets := fmt.Sprintf("[%d..%d)", tok.Span.StartOffset, tok.Span.EndOffset)
		fmt.Fprintf(w, "%-5d %-14s %-26s %-16s %s\n", i, tok.Type, display, loc, offsets)
	}
}

func printStats(w io.Writer, st compileStats) {
	illegalCount := 0
	for _, t := range st.tokens {
		if t.Type == lexer.TokenIllegal {
			illegalCount++
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "=== Métricas de Compilação (--stats) ===")
	fmt.Fprintf(w, "  Arquivo            : %s (%d bytes)\n", st.file, st.byteCount)
	fmt.Fprintf(w, "  Linhas processadas : %d\n", st.lineCount)
	fmt.Fprintf(w, "  Tokens gerados     : %d (válidos: %d, ilegais: %d)\n", len(st.tokens), len(st.tokens)-illegalCount, illegalCount)
	if st.ranParser {
		fmt.Fprintf(w, "  Nós na AST         : %d (laços 'for' desaçucarados: %d)\n", st.astNodes, st.desugaredFor)
	}
	if st.ranSemantic {
		fmt.Fprintf(w, "  Análise Semântica  : %d escopo(s), %d símbolo(s), %d expressão(ões) tipada(s)\n",
			st.scopeCount, st.symbolCount, st.typedExprs)
	}
	if st.optEnabled {
		fmt.Fprintf(w, "  Otimizações (--opt): %d aplicadas (constantes dobradas: %d, código morto removido: %d)\n",
			st.optStats.Total(), st.optStats.ConstsFolded, st.optStats.DeadNodesRemoved)
	}
	if st.ranIR {
		fmt.Fprintf(w, "  Bytecode IR        : %d instruções (%d função(ões) + <script>)\n", st.irInstCount, st.irFnCount)
	}
	if st.ranVM {
		fmt.Fprintf(w, "  Execução Stack VM  : %d instruções despachadas\n", st.vmSteps)
	}
	fmt.Fprintf(w, "  Erros acumulados   : %d\n", st.errCount)

	lexNs := st.lexElapsed.Nanoseconds()
	totalNs := lexNs
	fmt.Fprintf(w, "  Tempo (Lexer)      : %.4f ms (%d ns)\n", float64(lexNs)/1e6, lexNs)

	if st.ranParser {
		parseNs := st.parseElapsed.Nanoseconds()
		totalNs += parseNs
		fmt.Fprintf(w, "  Tempo (Parser+AST) : %.4f ms (%d ns)\n", float64(parseNs)/1e6, parseNs)
	}
	if st.ranSemantic {
		semNs := st.semElapsed.Nanoseconds()
		totalNs += semNs
		fmt.Fprintf(w, "  Tempo (Semântico)  : %.4f ms (%d ns)\n", float64(semNs)/1e6, semNs)
	}
	if st.ranIR {
		irNs := st.irElapsed.Nanoseconds()
		totalNs += irNs
		label := "Tempo (Gerador IR)"
		if st.optEnabled {
			label = "Tempo (Opt + IR)  "
		}
		fmt.Fprintf(w, "  %s : %.4f ms (%d ns)\n", label, float64(irNs)/1e6, irNs)
	}
	if st.ranVM {
		vmNs := st.vmElapsed.Nanoseconds()
		totalNs += vmNs
		fmt.Fprintf(w, "  Tempo (Stack VM)   : %.4f ms (%d ns)\n", float64(vmNs)/1e6, vmNs)
	}
	if st.ranParser {
		fmt.Fprintf(w, "  Tempo Total        : %.4f ms (%d ns)\n", float64(totalNs)/1e6, totalNs)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Compilador vim — Linguagem Imperativa Estaticamente Tipada

Uso:
  vim <fase> [--opt] [--stats] <arquivo.vim>

Flags de Inspeção Didática:
  --tokens <arquivo>    Executa o Analisador Léxico e exibe a tabela de tokens com spans
  --ast <arquivo>       Executa Lexer + Parser + Desugaring e imprime a AST hierárquica
  --ast-dot <arquivo>   Exporta a AST em formato Graphviz (.dot) para visualização gráfica
  --check <arquivo>     Executa até o Analisador Semântico e exibe a Tabela de Símbolos/Escopos
  --dump-ir <arquivo>   Exibe o Bytecode desmontado da Stack VM por função
  --run <arquivo>       Compila e executa o programa na Máquina Virtual de Pilha

Otimização e Instrumentação:
  --opt                 Ativa Constant Folding, propagação de 'const' e Dead Code Elimination
  --stats               Exibe métricas detalhadas da compilação e execução em milissegundos
  --no-desugar          Preserva nós ForStmt brutos ao inspecionar --ast / --ast-dot
  -h, --help            Exibe esta mensagem de ajuda`)
}
