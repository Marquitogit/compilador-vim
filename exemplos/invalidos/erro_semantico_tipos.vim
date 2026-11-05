// Caso de Rejeição Semântica: Incompatibilidade de Tipos e Uso Ilegal de 'void'

let idade: int = "vinte e cinco";
let somaInvalida: string = "total: " + 10;
let misturaArray: []int = [1, 2, "tres"];
let proibidoVoid: void = 0;

if (10 + 20) {
    print("condicao nao booleana");
}
