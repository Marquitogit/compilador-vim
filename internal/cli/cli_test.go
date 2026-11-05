package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLITokensAndStatsValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "demo.vim")
	src := "/* cabeçalho /* aninhado */ ok */\nlet x: int = 10 + 20;\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--tokens", "--stats", file}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("esperava exit code 0, obteve %d. stderr:\n%s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "LET") || !strings.Contains(out, "TYPE_INT") {
		t.Fatalf("tabela de tokens incompleta:\n%s", out)
	}
	if !strings.Contains(out, "=== Métricas de Compilação (--stats) ===") || !strings.Contains(out, "Tempo (Lexer)") {
		t.Fatalf("bloco --stats ausente:\n%s", out)
	}
}

func TestCLITokensAccumulatesErrorsAndReturnsNonZero(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "erro.vim")
	src := "let a: int = @;\nlet s: string = \"aberta\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--tokens", file}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("esperava exit code 1 para arquivo com erros léxicos, obteve %d", code)
	}

	errOut := stderr.String()
	if !strings.Contains(errOut, "E0001") || !strings.Contains(errOut, "E0002") {
		t.Fatalf("esperava ambos os erros E0001 e E0002 acumulados no stderr:\n%s", errOut)
	}
}

func TestCLIASTAndASTDotModes(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "for_demo.vim")
	src := "for (let i: int = 0; i < 3; i = i + 1) { print(i); }\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var treeOut, errBuf bytes.Buffer
	if code := Run([]string{"--ast", "--stats", file}, &treeOut, &errBuf); code != 0 {
		t.Fatalf("falha no --ast: %s", errBuf.String())
	}
	if !strings.Contains(treeOut.String(), "BlockStmt [desugared 'for']") || !strings.Contains(treeOut.String(), "WhileStmt") {
		t.Fatalf("esperava saída desaçucarada em --ast:\n%s", treeOut.String())
	}

	var dotOut bytes.Buffer
	if code := Run([]string{"--ast-dot", file}, &dotOut, &errBuf); code != 0 {
		t.Fatalf("falha no --ast-dot: %s", errBuf.String())
	}
	if !strings.HasPrefix(dotOut.String(), "digraph vimAST {") {
		t.Fatalf("esperava grafo DOT válido:\n%s", dotOut.String())
	}
}

func TestCLICheckValidAndInvalidSemantic(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valido.vim")
	validSrc := "fn soma(a: int, b: int) -> int { return a + b; }\nlet r: int = soma(2, 3);\nprint(r);\n"
	if err := os.WriteFile(validFile, []byte(validSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--check", "--stats", validFile}, &stdout, &stderr); code != 0 {
		t.Fatalf("esperava sucesso em --check válido, obteve %d:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "=== Tabela de Símbolos Globais ===") || !strings.Contains(stdout.String(), "Tempo (Semântico)") {
		t.Fatalf("relatório --check ou --stats incompleto:\n%s", stdout.String())
	}

	invalidFile := filepath.Join(tmpDir, "invalido.vim")
	invalidSrc := "const LIMITE: int = 10;\nLIMITE = 20;\n"
	if err := os.WriteFile(invalidFile, []byte(invalidSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"--check", invalidFile}, &stdout, &stderr); code != 1 {
		t.Fatalf("esperava exit code 1 em --check inválido, obteve %d", code)
	}
	if !strings.Contains(stderr.String(), "E0205") {
		t.Fatalf("esperava erro E0205 no stderr:\n%s", stderr.String())
	}
}

func TestCLIDumpIROptAndRunModes(t *testing.T) {
	tmpDir := t.TempDir()
	progFile := filepath.Join(tmpDir, "exec.vim")
	src := `
const FATOR: int = 3;
fn calcular(n: int) -> int {
    return n * (2 + FATOR);
}
let entrada: int = read_int();
print("Resultado:", calcular(entrada));
`
	if err := os.WriteFile(progFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var irOut, errBuf bytes.Buffer
	if code := Run([]string{"--opt", "--dump-ir", "--stats", progFile}, &irOut, &errBuf); code != 0 {
		t.Fatalf("falha em --opt --dump-ir: %s", errBuf.String())
	}
	if !strings.Contains(irOut.String(), "const 5") || !strings.Contains(irOut.String(), "Otimizações (--opt)") {
		t.Fatalf("esperava constante dobrada '5' e métricas --opt:\n%s", irOut.String())
	}

	var runOut bytes.Buffer
	errBuf.Reset()
	code := RunWithInput([]string{"--opt", "--run", "--stats", progFile}, strings.NewReader("8\n"), &runOut, &errBuf)
	if code != 0 {
		t.Fatalf("falha em --run: %s", errBuf.String())
	}
	if !strings.Contains(runOut.String(), "Resultado: 40") || !strings.Contains(runOut.String(), "Execução Stack VM") {
		t.Fatalf("saída inesperada em --run --stats:\n%s", runOut.String())
	}
}
