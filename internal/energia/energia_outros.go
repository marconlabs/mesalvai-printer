//go:build !windows && !darwin

package energia

import "context"

// Linux serve só pra desenvolvimento: sem bandeja, sem pedido de repouso.
func impedir(context.Context) error { return nil }
