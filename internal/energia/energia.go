// Package energia impede que o computador durma enquanto o programa roda:
// com a tampa aberta e na tomada, sem repouso automático, o stream com a
// Entregas e a fila continuam vivos (P1). Fechar a tampa do notebook ainda
// suspende: isso é do hardware e a instalação avisa ("deixe aberto").
package energia

import "context"

// Impedir pede ao sistema para não entrar em repouso até `ctx` acabar (ou o
// programa fechar). Devolve erro se o sistema não deixou; o programa segue
// funcionando e a janela mostra o aviso.
func Impedir(ctx context.Context) error {
	return impedir(ctx)
}
