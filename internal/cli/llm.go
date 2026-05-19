package cli

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

// guideMD is the canonical "how to use thomctl" doc agents should read at the
// start of a session. It lives in docs/agents/issue-tracker.md so humans can
// edit it in git; `go:embed` pulls it into the binary so the doc shipped with
// `thomctl` always matches the binary's behavior.
//
//go:embed llm_guide.md
var guideMD string

func llmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "llm",
		Short: "Print agent-facing usage instructions (markdown).",
		Long: "Print the canonical 'how an agent should use thomctl' guide. Pipe to an\n" +
			"LLM tool or paste into a chat. The output is markdown — terse, with\n" +
			"copy-pasteable command examples for every common workflow.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(guideMD)
			return nil
		},
	}
}
