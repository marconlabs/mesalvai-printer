//go:build !windows

package impressora

import "testing"

func TestInterpretarCups(t *testing.T) {
	casos := map[string]string{
		"printer X is idle.  enabled since Mon\n\tAlerts: none": Pronta,
		"printer X is idle.\n\tAlerts: media-empty-warning":     SemPapel,
		"printer X is idle.\n\tAlerts: cover-open-report":       TampaAberta,
		"printer X disabled since Mon -\n\treason unknown":      SemResposta,
		"printer X is idle.\n\tAlerts: offline-report":          SemResposta,
	}
	for saida, quer := range casos {
		if l := interpretarCups(saida); l.Situacao != quer {
			t.Errorf("%q: %+v, queria %s", saida, l, quer)
		}
	}
}
