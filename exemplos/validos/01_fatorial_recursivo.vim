/* ====================================================================
   Exemplo 01: Fatorial Recursivo
   Demonstra comentários de bloco aninhados:
   /* bloco interno: fatorial(n) = n * fatorial(n - 1) */
   ==================================================================== */

fn fatorial(n: int) -> int {
    if (n <= 1) {
        return 1;
    }
    return n * fatorial(n - 1);
}

let entrada: int = 6;
let resultado: int = fatorial(entrada);
print("Fatorial calculado:");
print(resultado);
