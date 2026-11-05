/* ====================================================================
   Exemplo 02: Sequência de Fibonacci Iterativa
   Demonstra:
   - Laço `while` iterativo em tempo O(n) e espaço O(1) no cálculo pontual
   - Construção dinâmica de série em array tipado `[]int` via `push`
   ==================================================================== */

fn fibonacci(n: int) -> int {
    if (n <= 0) {
        return 0;
    }
    if (n == 1) {
        return 1;
    }

    let anterior: int = 0;
    let atual: int = 1;
    let passo: int = 2;

    while (passo <= n) {
        let proximo: int = anterior + atual;
        anterior = atual;
        atual = proximo;
        passo = passo + 1;
    }
    return atual;
}

let sequencia: []int = [];
for (let i: int = 0; i <= 10; i = i + 1) {
    push(sequencia, fibonacci(i));
}

print("Fibonacci de 0 ate 10:");
print(sequencia);
print("F(10) =", fibonacci(10));
