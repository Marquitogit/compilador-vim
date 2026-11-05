/* ====================================================================
   Exemplo 04: Ordenação Bubble Sort em Array Tipado ([]int)
   Demonstra:
   - Anotação de tipo de array `[]int` e literais `[...]`
   - Laços `for` aninhados desaçucarados automaticamente para `while`
   - Acesso indexado `arr[j]` e atribuição em L-Value indexado `arr[j] = ...`
   ==================================================================== */

fn bubbleSort(arr: []int) -> void {
    let n: int = len(arr);
    for (let i: int = 0; i < n - 1; i = i + 1) {
        for (let j: int = 0; j < n - 1 - i; j = j + 1) {
            if (arr[j] > arr[j + 1]) {
                let temp: int = arr[j];
                arr[j] = arr[j + 1];
                arr[j + 1] = temp;
            }
        }
    }
}

let valores: []int = [64, 34, 25, 12, 22, 11, 90];
print("Vetor antes da ordenacao:");
print(valores);

bubbleSort(valores);

print("Vetor ordenado:");
print(valores);
