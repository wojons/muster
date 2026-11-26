package builtin

import (
	"github.com/spf13/cobra"
)

// silenceRuntimeUsage applies the DF-019 usage policy to a hand-written
// command — the same pattern newRequestCommand uses in request.go (REVIEW-
// MUSTER-32 extends it to grpc, protocol graphql, protocol jsonrpc, protocol
// soap and inventory):
//
//   - A RUNTIME failure (a RunE error: a bad discovery source, an unreachable
//     endpoint, a mapped protocol fault) reports ONLY the error line — cobra
//     must not bury it under the twelve-line Usage/Flags block.
//   - Flag and argument misuse still surfaces usage: the FlagErrorFunc and the
//     wrapped positional-args validator temporarily un-silence usage for those
//     two error classes, mirroring request.go.
//
// validate may be nil for commands that take no positional args.
func silenceRuntimeUsage(cmd *cobra.Command, validate cobra.PositionalArgs) {
	cmd.SilenceUsage = true
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		c.SilenceUsage = false
		return err
	})
	if validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := validate(c, args); err != nil {
				c.SilenceUsage = false
				return err
			}
			return nil
		}
	}
}
