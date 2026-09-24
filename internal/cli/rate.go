package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rateCmd = &cobra.Command{
	Use:     "rate",
	Aliases: []string{"rates", "exchange"},
	Short:   "Exchange rates used for foreign expenses",
	Long: `Exchange rates convert a foreign --amount into the group currency.

rate list returns every target currency for one source. rate get returns
one pair. The rate means 1 unit of --from equals rate units of --to.
Pass that number to tricount expense add --exchange-rate, or omit
--exchange-rate and the expense command looks it up.

Next:
  tricount rate list --help
  tricount expense add --help`,
	Example: `  tricount rate list --from USD
  tricount rate get --from USD --to JPY`,
	RunE: helpOnly,
}

var rateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List exchange rates from one currency",
	Args:  cobra.NoArgs,
	Long: `List rates from --from into the other currencies the API knows.

rate is a decimal string. 1 unit of source equals rate units of target.
decimals is how many decimal places the target currency uses.

Next:
  tricount rate get --help
  tricount expense add --help`,
	Example: `  tricount rate list --from USD`,
	RunE: func(cmd *cobra.Command, args []string) error {
		from, err := normalizeCurrency(flagString(cmd, "from"))
		if err != nil {
			return fmt.Errorf("pass --from, a 3-letter currency such as USD. %w", err)
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		rates, err := s.client.Rates(cmdCtx(cmd), from)
		if err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("%d exchange rates from %s.", len(rates), from),
			Data: map[string]any{
				"from":  from,
				"rates": rates,
				"count": len(rates),
				"note":  "1 unit of from equals rate units of target.",
			},
		})
	},
}

var rateGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get one exchange rate",
	Args:  cobra.NoArgs,
	Long: `Get the rate from --from to --to.

The value is how many units of --to equal 1 unit of --from. Use it as
tricount expense add --exchange-rate when you want to pin the rate.

Next:
  tricount expense add --help
  tricount rate list --help`,
	Example: `  tricount rate get --from USD --to JPY`,
	RunE: func(cmd *cobra.Command, args []string) error {
		from, err := normalizeCurrency(flagString(cmd, "from"))
		if err != nil {
			return fmt.Errorf("pass --from. %w", err)
		}
		to, err := normalizeCurrency(flagString(cmd, "to"))
		if err != nil {
			return fmt.Errorf("pass --to. %w", err)
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		rate, err := s.client.Rate(cmdCtx(cmd), from, to)
		if err != nil {
			return fmt.Errorf("%w. List pairs with: tricount rate list --from %s", err, from)
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("1 %s = %s %s.", from, rate.Rate, to),
			Data:    rate,
		})
	},
}

func init() {
	rateListCmd.Flags().String("from", "", "Source currency, such as USD.")
	if err := rateListCmd.MarkFlagRequired("from"); err != nil {
		panic(err)
	}
	rateGetCmd.Flags().String("from", "", "Source currency, such as USD.")
	rateGetCmd.Flags().String("to", "", "Target currency, such as JPY.")
	if err := rateGetCmd.MarkFlagRequired("from"); err != nil {
		panic(err)
	}
	if err := rateGetCmd.MarkFlagRequired("to"); err != nil {
		panic(err)
	}
	rateCmd.AddCommand(rateListCmd, rateGetCmd)
	rootCmd.AddCommand(rateCmd)
}
