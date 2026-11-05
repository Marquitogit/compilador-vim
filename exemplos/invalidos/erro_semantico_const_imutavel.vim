// Caso de Rejeição Semântica: Imutabilidade de Constantes ('const')

const TAXA_FIXA: int = 15;
TAXA_FIXA = 20;

const PRIMOS: []int = [2, 3, 5, 7];
PRIMOS[0] = 11;
push(PRIMOS, 13);
