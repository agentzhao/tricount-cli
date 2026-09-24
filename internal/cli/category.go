package cli

import (
	"fmt"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

var categoryCmd = &cobra.Command{
	Use:     "category",
	Aliases: []string{"categories"},
	Short:   "Expense and group categories",
	Long: `Categories label expenses and, separately, the group itself.

Expense categories are passed to tricount expense add --category.
A custom label uses --category-custom and is stored as OTHER.
Group categories are passed to tricount group update --category.
The two lists are not interchangeable.

With --token, the response also lists custom labels already used in that group.

Next:
  tricount category list
  tricount expense add --help`,
	Example: `  tricount category list
  tricount category list --token tABC123xyz`,
	RunE: helpOnly,
}

var categoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List known categories",
	Args:  cobra.NoArgs,
	Long: `List standard expense categories and group categories.

Pass --token to include custom labels already used on that group's transactions.
Each custom label is sent back with --category-custom.

Next:
  tricount expense add --help
  tricount group update --help`,
	Example: `  tricount category list
  tricount category list --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		expenses := make([]map[string]string, 0, len(tricount.ExpenseCategories()))
		var lines []string
		for _, c := range tricount.ExpenseCategories() {
			expenses = append(expenses, map[string]string{
				"id":          c.ID,
				"emoji":       c.Emoji,
				"description": c.Description,
			})
			if c.Emoji != "" {
				lines = append(lines, fmt.Sprintf("%s\t%s\t%s", c.ID, c.Emoji, c.Description))
			} else {
				lines = append(lines, fmt.Sprintf("%s\t%s", c.ID, c.Description))
			}
		}
		data := map[string]any{
			"expense_categories": expenses,
			"group_categories":   tricount.GroupCategories(),
			"notes": []string{
				"Use an expense category id with tricount expense add --category.",
				"Use a group category id with tricount group update --category.",
				"A custom expense label is --category-custom and is stored as OTHER.",
			},
		}
		summary := fmt.Sprintf("%d expense categories and %d group categories.", len(expenses), len(tricount.GroupCategories()))
		next := []NextStep{
			{Command: "tricount expense add --help", Why: "Use an expense category while recording an expense."},
			{Command: "tricount group update --help", Why: "Set a category on the group itself."},
		}
		if cmd.Flags().Changed("token") || cmd.Flags().Changed("id") {
			tc, err := resolveRead(cmd)
			if err != nil {
				return err
			}
			custom := customCategories(tc)
			data["custom_categories"] = custom
			data["group"] = viewSummary(tc)
			summary = fmt.Sprintf("%s %s uses %d custom labels.", summary, tc.Title, len(custom))
			next = append([]NextStep{{
				Command: withToken(tc.Token, "expense list"),
				Why:     "See which transactions use these categories.",
			}}, next...)
		}
		human := ""
		if len(lines) > 0 {
			human = fmt.Sprintf("%s", joinLines(lines))
		}
		return writeResult(cmd, Result{
			Summary: summary,
			Data:    data,
			Next:    next,
			Human:   human,
		})
	},
}

func joinLines(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	return out
}

func init() {
	bindTarget(categoryListCmd)
	categoryCmd.AddCommand(categoryListCmd)
	rootCmd.AddCommand(categoryCmd)
}
