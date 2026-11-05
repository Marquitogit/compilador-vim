<p align="center">
  <img src="https://raw.githubusercontent.com/Marquitogit/compilador-vim/main/ast_bubblesort.png" alt="Vim AST Logo" width="160" />
</p>

<h1 align="center">Vim Programming Language & Compiler</h1>

<p align="center">
  <strong>Compilador otimizador estaticamente tipado e Máquina Virtual de Pilha (Stack VM) construídos do zero em Go.</strong>
</p>

<p align="center">
  <a href="#-principais-características"><img src="https://img.shields.io/badge/Status-100%25%20Conclu%C3%ADdo-brightgreen?style=for-the-badge" alt="Status"></a>
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version"></a>
  <a href="#-cobertura-de-testes"><img src="https://img.shields.io/badge/Coverage-100%25%20Passing-success?style=for-the-badge" alt="Tests"></a>
  <a href="#-licen%C3%A7a"><img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge" alt="License"></a>
</p>

<p align="center">
  <a href="#-visão-geral">Visão Geral</a> •
  <a href="#-arquitetura-do-compilador">Arquitetura</a> •
  <a href="#-gramática-formal-ebnf">Gramática EBNF</a> •
  <a href="#-sistema-de-tipos--semântica">Sistema de Tipos</a> •
  <a href="#-stack-vm--conjunto-de-instruções-isa">Stack VM & ISA</a> •
  <a href="#-otimizador-de-código---opt">Otimizador</a> •
  <a href="#-diagnósticos-visuais">Diagnósticos</a> •
  <a href="#-como-executar">Como Executar</a> •
  <a href="#-exemplos-de-código">Exemplos</a>
</p>

---

## 🚀 Visão Geral

O **Vim** é uma linguagem de programação imperativa com tipagem estática e semântica expressiva, projetada para aliar rigor conceitual com alto valor didático. 

O compilador e seu motor de execução foram implementados **100% do zero em Go** (utilizando exclusivamente a biblioteca padrão, *zero dependências externas*), abrangendo desde o escaneamento manual de caracteres e análise sintática via **Pratt Parsing**, até a geração de representação intermediária (*bytecode*), passes de otimização em AST e interpretação em uma **Máquina Virtual de Pilha (Stack VM)** de 34 instruções.

### ✨ Destaques de Engenharia
- **Pipeline Modular Desacoplado**: Seis fases estritamente delimitadas em pacotes internos independentes.
- **Pratt Parser Elegante**: Avaliação de precedência por *Binding Power* com 8 níveis, eliminando ambiguidades e recursão à esquerda.
- **Desaçucaramento Sintático (*Desugaring*)**: Transformação canônica de laços `for` em blocos léxicos com `while`.
- **Rastreamento Rigoroso de Posição (`Span`)**: Mapeamento exato de offsets, linhas e colunas para diagnósticos visuais com sublinhado (`^^^^`) estilo Rust/Clang.
- **Otimizador AST Integrado (`--opt`)**: *Constant Folding* seguro com preservação de exceções de runtime e *Dead Code Elimination* (DCE).
- **Semântica de Referência para Vetores (`*ArrayObject`)**: Mutações em vetores (*in-place*, como no Bubble Sort) refletidas no heap da VM.
- **Suíte de Testes Abrangente**: 100% de testes unitários e de integração de ponta a ponta (*golden tests* e equivalência com/sem `--opt`).

---

## 🏛 Arquitetura do Compilador

O código-fonte `.vim` atravessa um pipeline unidirecional de 6 fases:

```mermaid
flowchart LR
    A["Código Fonte (.vim)"] --> B["1. Scanner Manual"]
    B --> C["2. Pratt Parser & AST"]
    C --> D["3. Desugaring (for->while)"]
    D --> E["4. Type Checker & Escopos"]
    E --> F["5. AST Optimizer (--opt)"]
    F --> G["6. Gerador de Bytecode"]
    G --> H["Stack Virtual Machine"]
```

### 📁 Estrutura do Repositório

```text
compilador/
├── cmd/
│   └── vim/
│       └── main.go                  # Ponto de entrada CLI da ferramenta
├── internal/
│   ├── cli/                         # Tratamento de flags (--tokens, --ast, --check, --dump-ir, --run, --opt)
│   ├── diagnostics/                 # Sistema de localização espacial (Span) e renderizador visual de erros
│   ├── lexer/                       # Scanner manual UTF-8, comentários aninhados e acúmulo de diagnósticos
│   ├── parser/                      # Descida recursiva + Pratt Parser, nós AST, desaçucaramento e exportador DOT
│   ├── semantic/                    # Tabela de símbolos encadeada, resolução em 2 passadas e tipagem estática
│   ├── ir/                          # Bytecode ISA (34 opcodes), otimizador (--opt), compilador e desassemblador
│   └── vm/                          # Stack VM, CallFrames, operandos, *ArrayObject e rotinas nativas (I/O)
├── exemplos/
│   ├── validos/                     # Exemplos canônicos (Fatorial, Fibonacci, Bubble Sort, Otimização, etc.)
│   └── invalidos/                   # Casos de teste para as 4 camadas de erro (Léxico, Sintático, Semântico, VM)
├── visualizador_ast.html            # Interface Web interativa para renderização gráfica da AST via Graphviz
├── DEMO_APRESENTACAO.bat            # Script de demonstração rápida interativa para bancas e apresentações
└── examples_test.go                 # Testes de integração de ponta a ponta (Golden & Semantic Equivalence)
```

---

## 📜 Gramática Formal (EBNF)

Declarações de função (`fn`) são estritamente de nível superior (*top-level*), prevenindo complexidades de *closures* não intencionais na Stack VM.

```ebnf
Program        ::= TopLevelDecl* EOF ;

TopLevelDecl   ::= FnDecl | VarDecl | ConstDecl | Statement ;

FnDecl         ::= "fn" IDENT "(" ParamList? ")" ( "->" Type )? Block ;
ParamList      ::= Param ( "," Param )* ","? ;
Param          ::= IDENT ":" Type ;

VarDecl        ::= "let" IDENT ":" Type "=" Expr ";" ;
ConstDecl      ::= "const" IDENT ":" Type "=" Expr ";" ;

Type           ::= "int" | "bool" | "string" | "void" | "[" "]" Type ;

Block          ::= "{" BlockItem* "}" ;
BlockItem      ::= VarDecl | ConstDecl | Statement ;

Statement      ::= Block | IfStmt | WhileStmt | ForStmt | ReturnStmt | AssignOrExprStmt ;
IfStmt         ::= "if" "(" Expr ")" Block ( "else" ( IfStmt | Block ) )? ;
WhileStmt      ::= "while" "(" Expr ")" Block ;
ForStmt        ::= "for" "(" ForInit? ";" Expr? ";" ForPost? ")" Block ;
ReturnStmt     ::= "return" Expr? ";" ;
AssignOrExprStmt ::= Expr ( "=" Expr )? ";" ;

Expr           ::= OrExpr ;
OrExpr         ::= AndExpr ( "||" AndExpr )* ;
AndExpr        ::= EqualityExpr ( "&&" EqualityExpr )* ;
EqualityExpr   ::= RelExpr ( ( "==" | "!=" ) RelExpr )* ;
RelExpr        ::= AddExpr ( ( "<" | "<=" | ">" | ">=" ) AddExpr )* ;
AddExpr        ::= MulExpr ( ( "+" | "-" ) MulExpr )* ;
MulExpr        ::= UnaryExpr ( ( "*" | "/" | "%" ) UnaryExpr )* ;
UnaryExpr      ::= ( "!" | "-" ) UnaryExpr | PostfixExpr ;
PostfixExpr    ::= PrimaryExpr ( "(" ArgList? ")" | "[" Expr "]" )* ;
```

### Precedência de Operadores (Pratt Binding Power)

| Nível | Operadores | Categoria | Associatividade |
| :---: | :--- | :--- | :---: |
| **8** | `f(...)`, `arr[...]` | Chamada e Indexação | Esquerda |
| **7** | `!`, `-` (unário) | Negação lógica e aritmética | Direita (Prefixo) |
| **6** | `*`, `/`, `%` | Multiplicação, Divisão, Módulo | Esquerda |
| **5** | `+`, `-` | Adição, Subtração, Concatenação | Esquerda |
| **4** | `<`, `<=`, `>`, `>=` | Operadores Relacionais | Esquerda |
| **3** | `==`, `!=` | Igualdade e Desigualdade | Esquerda |
| **2** | `&&` | Conjunção Lógica (Curto-Circuito) | Esquerda |
| **1** | `\|\|` | Disjunção Lógica (Curto-Circuito) | Esquerda |

---

## 🛡 Sistema de Tipos & Semântica

1. **Tipos Estáticos Nativos**: `int` (inteiro 64-bit com sinal), `bool` (`true`/`false`), `string` (UTF-8) e `void`.
2. **Tipos Compostos Recursivos**: Vetores homogêneos unidimensionais (`[]int`) e multidimensionais (`[][]string`, matrizes).
3. **Imutabilidade Profunda de `const`**: Bloqueia reatribuição escalar direta (`C = 10`), mutação indexada em qualquer profundidade (`MATRIZ[i][j] = 0`) e mutações por intrínsecos (`push(C, 42)`).
4. **Resolução de Escopos em 2 Passadas**: Habilita **recursão mútua** e chamadas de funções declaradas posteriormente sem necessidade de *forward declarations*.
5. **Inferência Contextual de Array Vazio**: Literais `[]` adotam o tipo contextual esperado da variável, retorno ou argumento (`let a: []int = [];`).
6. **Supressão de Erros em Cascata**: Implementada através do tipo sentinela `TypeError`, impedindo poluição do terminal com mensagens redundantes.
7. **Análise de Retorno Garantido**: Funções com retorno não-void exigem `return` válido em todos os ramos de execução.

---

## ⚙ Stack VM & Conjunto de Instruções (ISA)

A **Stack Virtual Machine** do Vim é uma máquina de pilha baseada em quadros de ativação (`CallFrame`), com alocação estática de variáveis globais e slots locais indexados em $O(1)$.

```text
+--------------------------------------------------------------+
|                        CallFrame                             |
|  Function: bubbleSort  |  IP: 0014  |  BaseSlot: 0002        |
+--------------------------------------------------------------+
|  [Slot 0] arg "arr" (*ArrayObject)                           |
|  [Slot 1] local "n" (int)                                    |
|  [Slot 2] local "i" (int)                                    |
+--------------------------------------------------------------+
|                     Pilha de Operandos                       |
|  [... | valor_a | valor_b ]  -->  ADD  -->  [... | resultado]|
+--------------------------------------------------------------+
```

### Conjunto de Instruções (34 OpCodes)

| OpCode | Mnemônico | Operandos | Descrição |
| :--- | :--- | :--- | :--- |
| `00` | `CONST` | `constIdx` | Empilha literal da tabela de constantes |
| `01..03` | `VOID`, `TRUE`, `FALSE` | — | Empilha constantes primitivas |
| `04` | `POP` | — | Descarta o topo da pilha |
| `05..06` | `GET_GLOBAL`, `SET_GLOBAL` | `slot` | Leitura e escrita de variáveis globais |
| `07..08` | `GET_LOCAL`, `SET_LOCAL` | `slot` | Leitura e escrita no quadro atual |
| `09` | `NEW_ARRAY` | `count` | Aloca novo vetor de referências `*ArrayObject` |
| `10..11` | `GET_INDEX`, `SET_INDEX` | — | Leitura/escrita indexada com checagem de limites |
| `12..17` | `ADD`, `CONCAT`, `SUB`, `MUL`, `DIV`, `MOD` | — | Aritmética e concatenação |
| `18..19` | `NEG`, `NOT` | — | Operações unárias |
| `20..25` | `EQ`, `NOT_EQ`, `LT`, `LT_EQ`, `GT`, `GT_EQ` | — | Comparações e igualdade |
| `26` | `JUMP` | `targetIP` | Salto incondicional |
| `27` | `JUMP_IF_FALSE` | `targetIP` | Salto condicional |
| `28..29` | `JUMP_IF_FALSE_KEEP`, `JUMP_IF_TRUE_KEEP` | `targetIP` | Saltos com preservação para curto-circuito (`&&`, `\|\|`) |
| `30` | `CALL` | `fnIdx, argc` | Invoca função e empilha novo `CallFrame` |
| `31` | `INTRINSIC` | `id, argc` | Despacha rotinas nativas (`print`, `read`, `read_int`, `len`, `push`) |
| `32` | `RETURN` | — | Finaliza quadro atual e entrega valor de retorno |
| `33` | `HALT` | — | Interrompe a execução do módulo principal |

---

## ⚡ Otimizador de Código (`--opt`)

O compilador inclui um otimizador AST de alto impacto executado antes da geração de bytecode:

- **Constant Folding**: Avaliação estática de expressões constantes inteiras, booleanas e strings (`24 * 60 * 60` $\rightarrow$ `86400`).
- **Segurança Numérica**: **Nunca dobra divisões por zero** nem `MinInt64 / -1`, garantindo que exceções legítimas de tempo de execução sejam reportadas com precisão na VM.
- **Dead Code Elimination (DCE)**: Poda ramos mortos `if (false)`, simplifica `if (true)` e descarta instruções inalcançáveis após `return`.
- **Propagação de Constantes**: Substituição em cascata de identificadores `const` escalares nos blocos de código.

### Impacto Real no Exemplo `05_otimizacao_constantes.vim`
A função `segundosEmDias` é drasticamente reduzida no bytecode:

```text
SEM OTIMIZAÇÃO: 57 instruções (12 constantes, 3 variáveis locais e ramos mortos)
COM OTIMIZAÇÃO:  4 instruções (redução de 93% do tamanho da função!)

--- [fn #0: segundosEmDias] (aridade=1, slots_locais=1, constantes=1, instruções=4) ---
  IP     OPCODE       OPERANDOS    DETALHE SIMBÓLICO       ORIGEM
  0000   GET_LOCAL    0            ; param "dias"          L23:16
  0001   CONST        0            ; const 86400           L23:23
  0002   MUL                                               L23:16
  0003   RETURN                                            L23:9
```

---

## 🔍 Diagnósticos Visuais

Inspirado nos compiladores modernos (*Rustc*, *Clang*), o compilador aponta para a localização exata do problema com contexto do código fonte:

```text
erro[E0301]: divisão inteira por zero em tempo de execução
  --> exemplos/invalidos/erro_runtime_divisao_zero.vim:8:12
  |
8 |     return somaTotal / quantidadeItens;
  |            ^^^^^^^^^^^^^^^^^^^^^^^^^^^ divisor avaliou para 0
  |
  = ajuda: verifique se o denominador é diferente de zero antes de dividir
```

O compilador cataloga 26 diagnósticos padronizados em 4 camadas:
- **Léxicos (`E0001`–`E0006`)**: Caracteres inválidos, strings abertas, escapes desconhecidos, comentários aninhados sem fechamento e overflow de literais.
- **Sintáticos (`E0101`–`E0106`)**: Funções aninhadas não permitidas, alvos de atribuição inválidos (*L-Value*), tokens esperados e recuperação *Panic Mode*.
- **Semânticos (`E0201`–`E0210`)**: Identificadores não declarados, redeclarações, incompatibilidade de tipos, mutação de constantes e ausência de retorno garantido.
- **Runtime (`E0301`–`E0304`)**: Divisão por zero, índice fora dos limites do vetor, estouro de pilha (*Stack Overflow*) e falha de conversão em `read_int()`.

---

## 🛠 Como Executar

### Pré-requisitos
Apenas o [Go 1.23+](https://golang.org/dl/) instalado no sistema (dispensa ferramentas externas de build).

### Compilação do Executável

```bash
# Clone o repositório
git clone https://github.com/Marquitogit/compilador-vim.git
cd compilador-vim

# Compilar o binário (gera vim.exe no Windows ou vim em Linux/macOS)
go build -o vim.exe ./cmd/vim
```

### Executando Programas

```powershell
# Execução direta na Stack VM
.\vim.exe --run exemplos/validos/01_fatorial_recursivo.vim

# Executar com otimizador ligado e telemetria de performance
.\vim.exe --opt --run --stats exemplos/validos/04_arrays_bubblesort.vim
```

### Inspeção das Fases do Compilador

```powershell
# Inspecionar fluxo de tokens (Léxico)
.\vim.exe --tokens exemplos/validos/01_fatorial_recursivo.vim

# Inspecionar Árvore de Sintaxe Abstrata formatada (Parser)
.\vim.exe --ast exemplos/validos/01_fatorial_recursivo.vim

# Exibir Tabela de Símbolos e Árvore de Escopos (Semântico)
.\vim.exe --check exemplos/validos/03_escopos_e_sombreamento.vim

# Desmontar o Bytecode da VM (Backend IR)
.\vim.exe --dump-ir exemplos/validos/05_otimizacao_constantes.vim
```

---

## 🧪 Cobertura de Testes

A suíte automatizada do projeto valida todas as fases da compilação:

```powershell
go vet ./...
go test -v ./...
```

A suíte automatizada cobre:
- Testes unitários dedicados por pacote (`diagnostics`, `lexer`, `parser`, `semantic`, `ir`, `vm`, `cli`);
- Suíte de integração ponta a ponta (`examples_test.go`) validando saídas *golden*, equivalência semântica estrita entre `--run` e `--opt --run`, redução comprovada de bytecode e cobertura das 4 camadas de erro (`E00xx`, `E01xx`, `E02xx`, `E03xx`).

### 6.3 Flags da Linha de Comando

| Flag | Descrição |
| :--- | :--- |
| `--tokens <arq.vim>` | Executa a Fase 1 (Scanner) e imprime a tabela alinhada de tokens e spans. |
| `--ast <arq.vim>` | Executa até a Fase 2 e imprime a AST hierárquica indentada (pós-desaçucaramento). |
| `--ast-dot <arq.vim>` | Exporta a AST no formato Graphviz (`.dot`) para renderização visual em PNG/SVG. |
| `--no-desugar` | Combinada com `--ast` ou `--ast-dot`, preserva o nó `ForStmt` original antes da reescrita em `while`. |
| `--check <arq.vim>` | Executa até a Fase 3 e exibe a Tabela de Símbolos Globais e a Hierarquia de Escopos. |
| `--dump-ir <arq.vim>` | Executa até a Fase 4a e exibe o *disassembly* simbólico completo do bytecode gerado. |
| `--run <arq.vim>` | Compila e executa o programa na Stack VM (Fase 4b). |
| `--opt` | Ativa o otimizador seguro de AST (Constant Folding + propagação de `const` + DCE) em `--dump-ir` ou `--run`. |
| `--stats` | Anexa ao final um painel detalhado de métricas e tempos de alta precisão por fase. |
| `-h`, `--help` | Exibe o guia rápido de uso da ferramenta. |

---

## 💻 Exemplos de Código

### Bubble Sort com Mutações In-Place (`04_arrays_bubblesort.vim`)

```rust
fn bubbleSort(arr: []int) -> void {
    let n: int = len(arr);
    for (let i: int = 0; i < n - 1; i = i + 1) {
        for (let j: int = 0; j < n - i - 1; j = j + 1) {
            if (arr[j] > arr[j + 1]) {
                let temp: int = arr[j];
                arr[j] = arr[j + 1];
                arr[j + 1] = temp;
            }
        }
    }
}

let valores: []int = [64, 34, 25, 12, 22, 11, 90];
print("Vetor antes da ordenacao:", valores);
bubbleSort(valores);
print("Vetor ordenado:", valores);
```

### Recursão Mútua e Sombreamento de Escopo (`03_escopos_e_sombreamento.vim`)

```rust
let x: int = 100;

fn ehPar(n: int) -> bool {
    if (n == 0) { return true; }
    return ehImpar(n - 1); // Resolução adiantada (forward call)
}

fn ehImpar(n: int) -> bool {
    if (n == 0) { return false; }
    return ehPar(n - 1);
}

{
    // Sombreamento léxico sem afetar a variável global 'x'
    let x: string = "Analise de Escopos - Bloco Nivel 1";
    print(x);
}

print("Valor global intacto:", x);
```

---

## 📄 Licença

Distribuído sob a licença **MIT**. Consulte `LICENSE` para mais detalhes.
