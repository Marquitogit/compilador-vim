package vm

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vim/internal/diagnostics"
	"vim/internal/ir"
	"vim/internal/parser"
	"vim/internal/semantic"
)

func runSource(t *testing.T, src string, stdin string, optimize bool) (string, *RuntimeError, *ir.Module) {
	t.Helper()
	diag := diagnostics.NewBag()
	prog, _ := parser.ParseSource("vm_test.vim", src, diag)
	prog = parser.Desugar(prog)
	_ = semantic.Check(prog, diag)
	if diag.HasErrors() {
		t.Fatalf("erro estático inesperado: %+v", diag.Items())
	}
	if optimize {
		prog, _ = ir.Optimize(prog)
	}
	mod := ir.Generate(prog)

	var out bytes.Buffer
	machine := New(mod, Config{
		Stdin:     strings.NewReader(stdin),
		Stdout:    &out,
		MaxFrames: 128,
	})
	rtErr := machine.Run()
	return out.String(), rtErr, mod
}

func TestAllValidAcademicExamplesMatchWithAndWithoutOpt(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "exemplos", "validos")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("falha ao listar exemplos válidos: %v", err)
	}
	if len(entries) < 5 {
		t.Fatalf("esperava ao menos 5 exemplos válidos em exemplos/validos, encontrou %d", len(entries))
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".vim") {
			continue
		}
		path := filepath.Join(examplesDir, entry.Name())
		rawBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(rawBytes)

		outNoOpt, errNoOpt, modNoOpt := runSource(t, src, "", false)
		if errNoOpt != nil {
			t.Fatalf("[%s sem --opt] erro de execução inesperado: %v", entry.Name(), errNoOpt)
		}

		outOpt, errOpt, modOpt := runSource(t, src, "", true)
		if errOpt != nil {
			t.Fatalf("[%s com --opt] erro de execução inesperado: %v", entry.Name(), errOpt)
		}

		if outNoOpt != outOpt {
			t.Errorf("[%s] divergência de saída entre execução sem --opt e com --opt:\nSem opt:\n%s\nCom opt:\n%s",
				entry.Name(), outNoOpt, outOpt)
		}

		if entry.Name() == "05_otimizacao_constantes.vim" {
			if modOpt.TotalInstructions() >= modNoOpt.TotalInstructions() {
				t.Errorf("05_otimizacao_constantes.vim deveria reduzir instruções com --opt (%d vs %d)",
					modOpt.TotalInstructions(), modNoOpt.TotalInstructions())
			}
		}
	}
}

func TestBubbleSortMutatesArrayObjectInPlace(t *testing.T) {
	path := filepath.Join("..", "..", "exemplos", "validos", "04_arrays_bubblesort.vim")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	out, rtErr, _ := runSource(t, string(raw), "", false)
	if rtErr != nil {
		t.Fatalf("erro inesperado no bubbleSort: %v", rtErr)
	}
	if !strings.Contains(out, "[11, 12, 22, 25, 34, 64, 90]") {
		t.Fatalf("vetor não foi ordenado in-place corretamente. Saída:\n%s", out)
	}
}

func TestShortCircuitPreventsOutOfBoundsAccess(t *testing.T) {
	src := `
fn verificarIndice(arr: []int, i: int) -> bool {
    if (i >= 0 && i < len(arr) && arr[i] == 10) {
        return true;
    }
    if (i < 0 || i >= len(arr) || arr[i] == 99) {
        return false;
    }
    return false;
}

let dados: []int = [5, 10, 15];
print(verificarIndice(dados, -1));
print(verificarIndice(dados, 50));
print(verificarIndice(dados, 1));
`
	out, rtErr, _ := runSource(t, src, "", false)
	if rtErr != nil {
		t.Fatalf("curto-circuito falhou! Avaliou arr[i] indevidamente gerando: %v", rtErr)
	}
	want := "false\nfalse\ntrue\n"
	if out != want {
		t.Errorf("saída incorreta: esperado %q, obtido %q", want, out)
	}
}

func TestFunctionLocalShadowingAndSequentialForLoops(t *testing.T) {
	src := `
fn testarSlots(x: int) -> int {
    let soma: int = x;
    {
        let x: int = x + 100;
        soma = soma + x;
    }
    // x externo deve continuar valendo o argumento original (5)!
    for (let i: int = 0; i < 3; i = i + 1) {
        soma = soma + i;
    }
    for (let i: int = 10; i < 12; i = i + 1) {
        soma = soma + i;
    }
    return soma + x;
}
print(testarSlots(5));
`
	// soma inicial = 5; bloco interno x = 105 -> soma = 110; 1º for (+0+1+2) -> 113; 2º for (+10+11) -> 134; + x(5) = 139
	out, rtErr, _ := runSource(t, src, "", false)
	if rtErr != nil {
		t.Fatalf("erro inesperado: %v", rtErr)
	}
	if strings.TrimSpace(out) != "139" {
		t.Errorf("esperava 139 preservando slots de escopos isolados, obteve %q", strings.TrimSpace(out))
	}
}

func TestStdinReadAndReadIntIntrinsics(t *testing.T) {
	src := `
let nome: string = read();
let a: int = read_int();
let b: int = read_int();
print(nome, a + b);
`
	out, rtErr, _ := runSource(t, src, "Marco\n17\n25\n", false)
	if rtErr != nil {
		t.Fatalf("erro inesperado de I/O: %v", rtErr)
	}
	if strings.TrimSpace(out) != "Marco 42" {
		t.Errorf("esperava 'Marco 42', obteve %q", strings.TrimSpace(out))
	}
}

func TestRuntimeErrorsVisualDiagnostics(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		stdin    string
		wantCode string
	}{
		{
			name:     "divisao_por_zero",
			src:      "let z: int = 0;\nlet r: int = 50 / z;\nprint(r);\n",
			wantCode: ErrRuntimeDivByZero,
		},
		{
			name:     "indice_fora_dos_limites",
			src:      "let v: []int = [1, 2];\nprint(v[5]);\n",
			wantCode: ErrRuntimeIndexBounds,
		},
		{
			name:     "stack_overflow_recursao_infinita",
			src:      "fn inf(n: int) -> int { return inf(n + 1); }\nprint(inf(0));\n",
			wantCode: ErrRuntimeStackOverflow,
		},
		{
			name:     "read_int_invalido",
			src:      "let x: int = read_int();\nprint(x);\n",
			stdin:    "nao_eh_numero\n",
			wantCode: ErrRuntimeReadInt,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, rtErr, _ := runSource(t, tc.src, tc.stdin, false)
			if rtErr == nil {
				t.Fatalf("esperava RuntimeError %s, mas executou sem erro", tc.wantCode)
			}
			if rtErr.Code != tc.wantCode {
				t.Errorf("código esperado %s, obtido %s", tc.wantCode, rtErr.Code)
			}

			// Garante compatibilidade visual total com `diagnostics.Render` (setas `^` na linha exata)
			sm := diagnostics.NewSourceMap("vm_test.vim", tc.src)
			bag := diagnostics.NewBag()
			bag.Add(rtErr.ToDiagnostic())
			rendered := diagnostics.Render(sm, bag)
			if !strings.Contains(rendered, "^") || !strings.Contains(rendered, tc.wantCode) {
				t.Errorf("diagnóstico visual incompleto:\n%s", rendered)
			}
		})
	}
}
