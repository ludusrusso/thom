package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// issueLinkCmd builds `thomctl issue link add|list|remove`.
//
// Pocock's `to-issues` skill expresses dependencies as "Blocked by: <KEY>" in
// the slice body. With this subcommand the agent can promote that to a real
// Jira issue link so JQL queries like `issuekey in linkedIssues(...)` work.
func issueLinkCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "link",
		Short: "Create, list, or remove Jira issue links (Blocks, Relates, Duplicate, ...).",
	}
	c.AddCommand(issueLinkAddCmd(), issueLinkListCmd(), issueLinkRemoveCmd())
	return c
}

func issueLinkAddCmd() *cobra.Command {
	var (
		blockedBy   string
		blocks      string
		relatesTo   string
		duplicateOf string
		linkType    string
		toKey       string
	)
	c := &cobra.Command{
		Use:   "add <KEY>",
		Short: "Create a link from KEY to another issue.",
		Long: "Express a directional Jira link from <KEY> to another issue. The most\n" +
			"common case for the Pocock `to-issues` skill is --blocked-by, which\n" +
			"records that this slice cannot start until another finishes.\n\n" +
			"Shortcut flags are mutually exclusive. For uncommon link types use\n" +
			"--type and --to.",
		Example: "  thomctl issue link add OPE-744 --blocked-by OPE-743\n" +
			"  thomctl issue link add OPE-743 --blocks OPE-744\n" +
			"  thomctl issue link add OPE-745 --relates-to OPE-744\n" +
			"  thomctl issue link add OPE-746 --duplicate-of OPE-742\n" +
			"  thomctl issue link add OPE-747 --type Cloners --to OPE-740",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			out, in, typ, err := resolveLink(key, blockedBy, blocks, relatesTo, duplicateOf, linkType, toKey)
			if err != nil {
				return err
			}
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			return be.AddLink(out, in, typ)
		},
	}
	c.Flags().StringVar(&blockedBy, "blocked-by", "", "OTHER blocks KEY (KEY can't start until OTHER is done).")
	c.Flags().StringVar(&blocks, "blocks", "", "KEY blocks OTHER.")
	c.Flags().StringVar(&relatesTo, "relates-to", "", "Symmetric 'Relates' link between KEY and OTHER.")
	c.Flags().StringVar(&duplicateOf, "duplicate-of", "", "KEY duplicates OTHER (OTHER is the canonical one).")
	c.Flags().StringVar(&linkType, "type", "", "Advanced: link type name (e.g. Cloners). Use with --to.")
	c.Flags().StringVar(&toKey, "to", "", "Advanced: target issue key. Use with --type. KEY is the outward side.")
	return c
}

// resolveLink turns the shortcut flags into the (out, in, type) triple acli
// expects. Returns an error when multiple shortcuts are set or none match.
func resolveLink(key, blockedBy, blocks, relatesTo, duplicateOf, linkType, toKey string) (out, in, typ string, err error) {
	set := []string{}
	if blockedBy != "" {
		set = append(set, "--blocked-by")
	}
	if blocks != "" {
		set = append(set, "--blocks")
	}
	if relatesTo != "" {
		set = append(set, "--relates-to")
	}
	if duplicateOf != "" {
		set = append(set, "--duplicate-of")
	}
	if linkType != "" || toKey != "" {
		set = append(set, "--type/--to")
	}
	if len(set) == 0 {
		return "", "", "", errors.New("one of --blocked-by / --blocks / --relates-to / --duplicate-of / --type+--to is required")
	}
	if len(set) > 1 {
		return "", "", "", fmt.Errorf("only one of %s may be set", strings.Join(set, ", "))
	}
	switch {
	case blockedBy != "":
		return blockedBy, key, "Blocks", nil
	case blocks != "":
		return key, blocks, "Blocks", nil
	case relatesTo != "":
		return key, relatesTo, "Relates", nil
	case duplicateOf != "":
		return key, duplicateOf, "Duplicate", nil
	default:
		if linkType == "" || toKey == "" {
			return "", "", "", errors.New("--type and --to must both be set")
		}
		return key, toKey, linkType, nil
	}
}

func issueLinkListCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "list <KEY>",
		Short: "List all issue links involving KEY (in either direction).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			links, err := be.ListLinks(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return emitJSON(links)
			}
			if len(links) == 0 {
				return nil
			}
			maxPhrase, maxKey, maxStatus := len("RELATIONSHIP"), len("KEY"), len("STATUS")
			for _, l := range links {
				if n := len(l.Phrase); n > maxPhrase {
					maxPhrase = n
				}
				if n := len(l.OtherKey); n > maxKey {
					maxKey = n
				}
				if n := len(l.Status); n > maxStatus {
					maxStatus = n
				}
			}
			fmt.Printf("%-*s  %-*s  %-*s  %-9s  %s\n",
				maxPhrase, "RELATIONSHIP", maxKey, "KEY", maxStatus, "STATUS", "LINK-ID", "SUMMARY")
			for _, l := range links {
				fmt.Printf("%-*s  %-*s  %-*s  %-9s  %s\n",
					maxPhrase, l.Phrase, maxKey, l.OtherKey, maxStatus, l.Status, l.ID, l.Summary)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "Print JSON to stdout.")
	return c
}

func issueLinkRemoveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "remove <LINK-ID>",
		Short: "Delete a link by its Jira link ID (see `link list`).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			be, _, err := resolveBackend()
			if err != nil {
				return err
			}
			return be.RemoveLink(args[0])
		},
	}
	return c
}
