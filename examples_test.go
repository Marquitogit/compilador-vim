package vim_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vim/internal/cli"
	"vim/internal/diagnostics"
	"vim/internal/ir"
	"vim/internal/parser"
	"vim/internal/semantic"
)

func TestGoldenValidExamplesEquivalenceAndOptimization(t *testing.T) {
	goldenOutputs := map[string]string{
		"01_fatorial_recursivo.vim":    "Fatorial calculado:\n720\n",
		"02_fibonacci_iterativo.vim":   "Fibonacci de 0 ate 10:\n[0, 1, 1, 2, 3, 5, 8, 13, 21, 34, 55]\nF(10) = 55\n",
		"03_escopos_e_sombreamento.vim": "Analise de Escopos - Bloco Nivel 1\nBloco Nivel 2: true\nValor global intacto: 100\n",
		"04_arrays_bubblesort.vim":     "Vetor antes da ordenacao:\n[64, 34, 25, 12, 22, 11, 90]\nVetor ordenado:\n[11, 12, 22, 25, 34, 64, 90]\n",
		"05_otimizacao_constantes.vim": "Segundos em 7 dias: 604800\n",
	}

	validDir := filepath.Join("exemplos", "validos")
	entries, err := os.ReadDir(validDir)
	if err != nil {
		t.Fatalf("erro ao ler %s: %v", validDir, err)
	}

	seen := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".vim") {
			continue
		}
		seen++
		filePath := filepath.Join(validDir, entry.Name())

		t.Run(entry.Name(), func(t *testing.T) {
			wantOut, hasGolden := goldenOutputs[entry.Name()]
			if !hasGolden {
				t.Fatalf("arquivo %s não possui saída golden cadastrada no teste de integração", entry.Name())
			}

			// 1. Inspeciona todas as flags estáticas (--tokens, --ast, --ast-dot, --check, --dump-ir)
			for _, flag := range []string{"--tokens", "--ast", "--ast-dot", "--check", "--dump-ir"} {
				var out, errBuf bytes.Buffer
				if code := cli.Run([]string{flag, filePath}, &out, &errBuf); code != 0 {
					t.Fatalf("falha em %s para %s (exit=%d):\n%s", flag, entry.Name(), code, errBuf.String())
				}
			}

			// 2. Executa sem --opt e com --opt via CLI e compara saídas
			var outRaw, errRaw bytes.Buffer
			if code := cli.Run([]string{"--run", filePath}, &outRaw, &errRaw); code != 0 {
				t.Fatalf("falha em --run sem --opt (exit=%d):\n%s", code, errRaw.String())
			}

			var outOpt, errOpt bytes.Buffer
			if code := cli.Run([]string{"--opt", "--run", filePath}, &outOpt, &errOpt); code != 0 {
				t.Fatalf("falha em --opt --run (exit=%d):\n%s", code, errOpt.String())
			}

			gotRaw := strings.ReplaceAll(outRaw.String(), "\r\n", "\n")
			gotOpt := strings.ReplaceAll(outOpt.String(), "\r\n", "\n")

			if gotRaw != gotOpt {
				t.Errorf("divergência semântica entre sem --opt e com --opt:\nSem --opt:\n%s\nCom --opt:\n%s", gotRaw, gotOpt)
			}
			if gotRaw != wantOut {
				t.Errorf("saída divergente do esperado (golden):\nEsperado:\n%q\nObtido:\n%q", wantOut, gotRaw)
			}

			// 3. Verifica que o número de instruções de bytecode com --opt é <= sem --opt
			rawSrc, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatal(err)
			}
			diag := diagnostics.NewBag()
			prog, _ := parser.ParseSource(filePath, string(rawSrc), diag)
			prog = parser.Desugar(prog)
			_ = semantic.Check(prog, diag)
			if diag.HasErrors() {
				t.Fatalf("erro estático inesperado: %+v", diag.Items())
			}

			modRaw := ir.Generate(prog)
			progOpt, _ := ir.Optimize(prog)
			modOpt := ir.Generate(progOpt)

			if modOpt.TotalInstructions() > modRaw.TotalInstructions() {
				t.Errorf("--opt aumentou instruções (%d > %d)", modOpt.TotalInstructions(), modRaw.TotalInstructions())
			}
			if entry.Name() == "05_otimizacao_constantes.vim" && modOpt.TotalInstructions() >= modRaw.TotalInstructions() {
				t.Errorf("05_otimizacao_constantes.vim não reduziu bytecode (%d vs %d)",
					modOpt.TotalInstructions(), modRaw.TotalInstructions())
			}
		})
	}

	if seen != len(goldenOutputs) {
		t.Fatalf("esperava validar %d arquivos válidos, mas encontrou %d", len(goldenOutputs), seen)
	}
}

func TestInvalidExamplesAllFourDiagnosticLayers(t *testing.T) {
	type expectedFailure struct {
		passesCheck bool     // Verdadeiro para erros exclusivos de runtime (E03xx)
		codes       []string // Códigos esperados na saída diagnóstica
	}

	expected := map[string]expectedFailure{
		// Camada 1: Léxica (E00xx)
		"erro_lexico_completo.vim": {
			passesCheck: false,
			codes:       []string{"E0001", "E0006", "E0003", "E0002", "E0004"},
		},
		// Camada 2: Sintática (E01xx)
		"erro_sintatico_expressao.vim": {
			passesCheck: false,
			codes:       []string{"E0101", "E0102", "E0104"},
		},
		// Camada 3: Semântica (E02xx)
		"erro_semantico_tipos.vim": {
			passesCheck: false,
			codes:       []string{"E0203", "E0204"},
		},
		"erro_semantico_nao_declarada.vim": {
			passesCheck: false,
			codes:       []string{"E0202", "E0201"},
		},
		"erro_semantico_const_imutavel.vim": {
			passesCheck: false,
			codes:       []string{"E0205"},
		},
		"erro_semantico_retorno_faltante.vim": {
			passesCheck: false,
			codes:       []string{"E0208", "E0207"},
		},
		// Camada 4: Runtime na Stack VM (E03xx)
		"erro_runtime_divisao_zero.vim": {
			passesCheck: true,
			codes:       []string{"E0301"},
		},
		"erro_runtime_indice_fora.vim": {
			passesCheck: true,
			codes:       []string{"E0302"},
		},
	}

	invalidDir := filepath.Join("exemplos", "invalidos")
	entries, err := os.ReadDir(invalidDir)
	if err != nil {
		t.Fatalf("erro ao ler %s: %v", invalidDir, err)
	}

	seen := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".vim") {
			continue
		}
		seen++
		filePath := filepath.Join(invalidDir, entry.Name())

		t.Run(entry.Name(), func(t *testing.T) {
			spec, ok := expected[entry.Name()]
			if !ok {
				t.Fatalf("arquivo inválido %s não está mapeado na suíte de integração", entry.Name())
			}

			// Verifica comportamento em --check
			var chkOut, chkErr bytes.Buffer
			chkCode := cli.Run([]string{"--check", filePath}, &chkOut, &chkErr)
			if spec.passesCheck && chkCode != 0 {
				t.Fatalf("%s deveria passar na validação estática (--check), mas falhou:\n%s",
					entry.Name(), chkErr.String())
			}
			if !spec.passesCheck && chkCode == 0 {
				t.Fatalf("%s deveria ser rejeitado em --check, mas retornou exit code 0", entry.Name())
			}

			// Verifica comportamento em --run (e também --opt --run para casos de runtime)
			modesToTest := [][]string{{"--run", filePath}}
			if spec.passesCheck {
				modesToTest = append(modesToTest, []string{"--opt", "--run", filePath})
			}

			for _, args := range modesToTest {
				var runOut, runErr bytes.Buffer
				runCode := cli.Run(args, &runOut, &runErr)
				if runCode == 0 {
					t.Fatalf("execução %v para %s deveria falhar com erro, mas obteve exit code 0", args, entry.Name())
				}

				diagText := runErr.String()
				// Valida elementos visuais estilo Rust/Clang (`-->`, `|`, `^`)
				if !strings.Contains(diagText, "-->") || !strings.Contains(diagText, "^") {
					t.Errorf("formatação visual ausente para %s:\n%s", entry.Name(), diagText)
				}
				for _, code := range spec.codes {
					tag := "erro[" + code + "]"
					if !strings.Contains(diagText, tag) {
						t.Errorf("esperava diagnóstico %s em %s, mas obteve:\n%s", tag, entry.Name(), diagText)
					}
				}
			}
		})
	}

	if seen != len(expected) {
		t.Fatalf("esperava validar %d arquivos inválidos, mas encontrou %d", len(expected), seen)
	}
}
