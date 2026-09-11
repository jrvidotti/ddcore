package config

import (
	"os"
	"testing"
)

// O arquivo que as pessoas leem é o .env.example do repositório; a constante é
// só o que `ddcore init` escreve. Se os dois divergirem, quem consulta o
// repositório recebe uma lista de variáveis que não é a que o servidor lê.
func TestEnvExampleMatchesTheCommittedFile(t *testing.T) {
	b, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("lendo .env.example do repositório: %v", err)
	}
	if string(b) != EnvExample {
		t.Error(".env.example e config.EnvExample divergiram — atualize os dois")
	}
}
