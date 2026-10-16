package divergence

import (
	"fmt"
	"strings"

	"github.com/adams-shaun/gorge/decision"
)

// ChoiceSummary renders one intent's answer against the decision it
// answered: the chosen options' labels, or the raw indices when the
// decision does not offer labels for them. It is the renderer cmd/repro
// prints intent timelines with, moved here (design §5.6) so the host's
// divergence capture and the review UI render the same strings the
// reproduction tool does.
func ChoiceSummary(d *decision.Decision, it decision.Intent) string {
	chosen := d.Chosen(it)
	if len(chosen) == 0 {
		return fmt.Sprintf("choices %v — %s", it.Choices, d.Prompt)
	}
	labels := make([]string, len(chosen))
	for i, o := range chosen {
		labels[i] = o.Label
	}
	return fmt.Sprintf("%s — %s", strings.Join(labels, "; "), d.Prompt)
}
