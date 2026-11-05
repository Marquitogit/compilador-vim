// Caso de Rejeição Semântica: Variáveis/Funções Não Declaradas, Redeclaração e Cascata Suprimida

let base: int = 50;
let base: int = 99; // Redeclaração no mesmo escopo (aponta para a linha 3)

// 'fatorSecreto' não existe: emite APENAS E0201 e suprime erros em cascata no '+' e '*'!
let calculo: int = (fatorSecreto + base) * 2;

let resultado: int = funcaoInexistente(10, 20);
