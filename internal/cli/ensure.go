package cli

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

const ensureKeyPrefix = " [tricount-cli:key="

var expenseEnsureCmd = &cobra.Command{
	Use:   "ensure",
	Short: "Create a transaction once for a caller-supplied key",
	Args:  cobra.NoArgs,
	Long: `Create an expense, income, or reimbursement identified by --key.

The key is chosen by the caller, such as rent:2026-10. The CLI does not
derive it from the date, description, amount, or members. It is stored as a
suffix on the description:

  October rent [tricount-cli:key=rent%3A2026-10]

That suffix is visible in the Tricount app. A later run finds it by decoding
the suffix.

Outcomes:
  no match            create the transaction, then read it back and check it
  one exact match     print it with status "existing" and do not write
  one different match fail with idempotency_conflict and do not write
  several matches     fail with idempotency_ambiguous and do not write

This command never edits a keyed transaction. Change one with expense edit,
or delete it and ensure a new key.

--type expense takes one of --among, --share, or --ratio.
--type income is an equal split across --among, received by --receiver.
--type reimbursement moves --amount from --payer to --receiver.

--dry-run resolves members and reports the outcome without writing.
A conflict or an ambiguous key still fails.

Also: tricount transaction ensure and tricount tx ensure.

Next:
  tricount expense list --help
  tricount expense edit --help
  tricount balance show --help`,
	Example: `  tricount transaction ensure --token tABC123xyz --key rent:2026-10 --type expense --description "October rent" --amount 900 --payer Alice --among Alice,Bob
  tricount transaction ensure --token tABC123xyz --key rent:2026-10 --type expense --description "October rent" --amount 100 --payer Alice --share Alice=30 --share Bob=70
  tricount transaction ensure --token tABC123xyz --key dinner:2026-10-01 --type expense --description Dinner --amount 90 --payer Alice --ratio Alice=1 --ratio Bob=2
  tricount transaction ensure --token tABC123xyz --key refund:2026-10 --type income --description Refund --amount 30 --receiver Alice --among Alice,Bob
  tricount transaction ensure --token tABC123xyz --key settle:2026-10 --type reimbursement --description "Settling up" --amount 25 --payer Bob --receiver Alice
  tricount transaction ensure --token tABC123xyz --key rent:2026-10 --type expense --description "October rent" --amount 900 --payer Alice --among Alice,Bob --dry-run`,
	RunE: runEnsure,
}

func init() {
	expenseEnsureCmd.Flags().String("key", "", "Required id for this transaction, chosen by the caller. Example: rent:2026-10. Stored in the description. The CLI does not invent one.")
	expenseEnsureCmd.Flags().String("type", "", "expense, income, or reimbursement.")
	expenseEnsureCmd.Flags().Bool("dry-run", false, "Resolve the key and print the outcome without writing. Conflicts still fail.")
	expenseEnsureCmd.Flags().String("description", "", "Label for the transaction. The CLI appends [tricount-cli:key=...] so a later run can find this key.")
	expenseEnsureCmd.Flags().String("amount", "", flagAmountHelp)
	expenseEnsureCmd.Flags().String("payer", "", flagPayerHelp)
	expenseEnsureCmd.Flags().String("receiver", "", flagReceiverHelp)
	expenseEnsureCmd.Flags().StringSlice("among", nil, flagAmongHelp)
	expenseEnsureCmd.Flags().StringSlice("share", nil, flagShareHelp)
	expenseEnsureCmd.Flags().StringSlice("ratio", nil, flagRatioHelp)
}

type ensureInput struct {
	Key         string
	Type        string
	Description string
	Amount      int64
	AmountSet   bool
	Payer       string
	Receiver    string
	Among       []string
	Shares      []shareSpec
	Ratios      []ratioSpec
}

type ensureDecision struct {
	Action  string
	Match   tricount.Transaction
	Matches []tricount.Transaction
	Diffs   []string
}

type ensurePreview struct {
	Description string      `json:"description"`
	Type        string      `json:"type"`
	TypeMeaning string      `json:"type_meaning"`
	AmountRaw   string      `json:"amount_raw"`
	Currency    string      `json:"currency"`
	Payer       memberView  `json:"payer"`
	Allocations []allocView `json:"allocations"`
}

func runEnsure(cmd *cobra.Command, args []string) error {
	input, err := readEnsureInput(cmd)
	if err != nil {
		return err
	}
	dry, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}
	tc, err := prepareExpenseGroup(cmd)
	if err != nil {
		return err
	}
	entry, err := input.entry(tc)
	if err != nil {
		return err
	}
	decision := decideEnsure(tc.Transactions, input.Key, entry, input.Description)
	switch decision.Action {
	case "conflict":
		return ensureConflict(input.Key, decision.Match, decision.Diffs)
	case "ambiguous":
		return ensureAmbiguous(input.Key, decision.Matches)
	case "existing":
		if dry {
			return writeEnsureDry(cmd, tc, input.Key, false, viewTransaction(tc, decision.Match))
		}
		return writeEnsureExisting(cmd, tc, input.Key, decision.Match)
	default:
		if dry {
			return writeEnsureDry(cmd, tc, input.Key, true, previewEnsure(tc, entry))
		}
		return createAndVerifyEnsure(cmd, tc, entry, input)
	}
}

func readEnsureInput(cmd *cobra.Command) (ensureInput, error) {
	key, err := parseEnsureKey(flagString(cmd, "key"), cmd.Flags().Changed("key"))
	if err != nil {
		return ensureInput{}, err
	}
	typeName := flagString(cmd, "type")
	if typeName == "" {
		return ensureInput{}, coded("invalid_request", "pass --type expense, income, or reimbursement", "")
	}
	txType, err := normalizeTxType(typeName)
	if err != nil {
		return ensureInput{}, err
	}
	description := flagString(cmd, "description")
	if description == "" {
		return ensureInput{}, coded("invalid_request", "pass --description, the label people should see", "The CLI appends the key suffix itself.")
	}
	if _, _, ok := descriptionKey(description); ok || strings.Contains(description, "[tricount-cli:key=") {
		return ensureInput{}, coded("invalid_request", "--description must not contain a tricount-cli key suffix", "Pass the plain label. The CLI appends the key.")
	}
	mode, err := ensureSplitMode(cmd)
	if err != nil {
		return ensureInput{}, err
	}
	input := ensureInput{Key: key, Type: txType, Description: description}
	if cmd.Flags().Changed("amount") {
		input.Amount, err = positiveAmount(cmd)
		if err != nil {
			return ensureInput{}, err
		}
		input.AmountSet = true
	}
	payerFallback, receiverFallback := "", ""
	if profile, ok := profileValue(cmd); ok {
		payerFallback = profile.Payer
		receiverFallback = profile.Receiver
	}
	input.Payer = defaultedFlag(cmd, "payer", payerFallback)
	input.Receiver = defaultedFlag(cmd, "receiver", receiverFallback)
	if mode == "among" {
		input.Among, err = stringSlice(cmd, "among")
		if err != nil {
			return ensureInput{}, err
		}
	}
	switch txType {
	case "NORMAL":
		if input.Receiver != "" && cmd.Flags().Changed("receiver") {
			return ensureInput{}, coded("invalid_request", "an expense uses --payer and a split, not --receiver", "Use --type reimbursement to name both people.")
		}
		if mode == "" {
			refs, err := stringSlice(cmd, "among")
			if err != nil {
				return ensureInput{}, err
			}
			refs = defaultedAmong(cmd, refs)
			if len(refs) > 0 {
				mode = "among"
				input.Among = refs
			}
		}
		switch mode {
		case "share":
			input.Shares, err = parseShareFlags(mustSlice(cmd, "share"))
		case "ratio":
			input.Ratios, err = parseRatioFlags(mustSlice(cmd, "ratio"))
		case "among":
		default:
			return ensureInput{}, coded("invalid_request", "an expense needs one of --among, --share, or --ratio", "")
		}
	case "INCOME":
		if mode == "share" || mode == "ratio" {
			return ensureInput{}, coded("invalid_request", "income is split equally with --among", "")
		}
		if cmd.Flags().Changed("payer") {
			return ensureInput{}, coded("invalid_request", "income uses --receiver, not --payer", "")
		}
		if mode == "" {
			refs, err := stringSlice(cmd, "among")
			if err != nil {
				return ensureInput{}, err
			}
			refs = defaultedAmong(cmd, refs)
			if len(refs) > 0 {
				mode = "among"
				input.Among = refs
			}
		}
		if mode != "among" {
			return ensureInput{}, coded("invalid_request", "income needs --among, the members who share the credit", "")
		}
	default:
		if mode != "" {
			return ensureInput{}, coded("invalid_request", "a reimbursement names --payer and --receiver, not a split", "")
		}
	}
	if err != nil {
		return ensureInput{}, err
	}
	return input, nil
}

func ensureSplitMode(cmd *cobra.Command) (string, error) {
	among := cmd.Flags().Changed("among")
	share := cmd.Flags().Changed("share")
	ratio := cmd.Flags().Changed("ratio")
	n := 0
	if among {
		n++
	}
	if share {
		n++
	}
	if ratio {
		n++
	}
	if n > 1 {
		return "", coded("invalid_request", "pass only one of --among, --share, or --ratio", "")
	}
	switch {
	case share:
		return "share", nil
	case ratio:
		return "ratio", nil
	case among:
		return "among", nil
	default:
		return "", nil
	}
}

func parseEnsureKey(key string, set bool) (string, error) {
	if !set {
		return "", coded("invalid_idempotency_key", "--key is required", "Example: --key rent:2026-10")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", coded("invalid_idempotency_key", "--key is empty", "Example: --key rent:2026-10")
	}
	if len(key) > 200 {
		return "", coded("invalid_idempotency_key", "--key is longer than 200 characters", "")
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return "", coded("invalid_idempotency_key", "--key cannot contain control characters", "")
		}
	}
	if _, err := ensureStoredDescription("label", key); err != nil {
		return "", err
	}
	return key, nil
}

func ensureStoredDescription(description, key string) (string, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return "", coded("invalid_request", "pass --description, the label people should see", "")
	}
	encoded := url.QueryEscape(key)
	if encoded == "" || strings.ContainsAny(encoded, " []") {
		return "", coded("invalid_idempotency_key", "--key cannot be stored in the description suffix", "")
	}
	return description + ensureKeyPrefix + encoded + "]", nil
}

func descriptionKey(description string) (visible, key string, ok bool) {
	i := strings.LastIndex(description, ensureKeyPrefix)
	if i < 0 {
		return "", "", false
	}
	encoded := description[i+len(ensureKeyPrefix):]
	if !strings.HasSuffix(encoded, "]") {
		return "", "", false
	}
	encoded = strings.TrimSuffix(encoded, "]")
	if encoded == "" || strings.ContainsAny(encoded, " []") {
		return "", "", false
	}
	decoded, err := url.QueryUnescape(encoded)
	if err != nil || decoded == "" || url.QueryEscape(decoded) != encoded {
		return "", "", false
	}
	return strings.TrimSpace(description[:i]), decoded, true
}

func (in ensureInput) entry(tc tricount.Tricount) (tricount.Entry, error) {
	stored, err := ensureStoredDescription(in.Description, in.Key)
	if err != nil {
		return tricount.Entry{}, err
	}
	entry := tricount.Entry{
		Description:   stored,
		When:          time.Now(),
		Type:          in.Type,
		GroupCurrency: tc.Currency,
	}
	switch in.Type {
	case "NORMAL":
		payer, err := resolveMember(tc, in.Payer)
		if err != nil {
			return tricount.Entry{}, err
		}
		entry.PayerUUID = payer.UUID
		switch {
		case len(in.Shares) > 0:
			if err := fillShareEntry(&entry, tc, in); err != nil {
				return tricount.Entry{}, err
			}
		case len(in.Ratios) > 0:
			if !in.AmountSet {
				return tricount.Entry{}, coded("missing_amount", "pass --amount as a positive decimal in major units, such as 12.50 or 1500", "Do not pass cents.")
			}
			if err := fillRatioEntry(&entry, tc, in); err != nil {
				return tricount.Entry{}, err
			}
		default:
			if !in.AmountSet {
				return tricount.Entry{}, coded("missing_amount", "pass --amount as a positive decimal in major units, such as 12.50 or 1500", "Do not pass cents.")
			}
			members, err := resolveMembers(tc, in.Among)
			if err != nil {
				return tricount.Entry{}, err
			}
			if err := rejectDuplicateMembers(members); err != nil {
				return tricount.Entry{}, err
			}
			allocs, err := equalAllocs(uuidsOf(members), in.Amount, 0, false)
			if err != nil {
				return tricount.Entry{}, err
			}
			entry.GroupMinor = in.Amount
			entry.Allocations = allocs
		}
	case "INCOME":
		receiver, err := resolveMember(tc, in.Receiver)
		if err != nil {
			return tricount.Entry{}, err
		}
		if !in.AmountSet {
			return tricount.Entry{}, coded("missing_amount", "pass --amount as a positive decimal in major units, such as 12.50 or 1500", "Do not pass cents.")
		}
		members, err := resolveMembers(tc, in.Among)
		if err != nil {
			return tricount.Entry{}, err
		}
		if err := rejectDuplicateMembers(members); err != nil {
			return tricount.Entry{}, err
		}
		allocs, err := equalAllocs(uuidsOf(members), in.Amount, 0, false)
		if err != nil {
			return tricount.Entry{}, err
		}
		entry.PayerUUID = receiver.UUID
		entry.GroupMinor = in.Amount
		entry.Allocations = allocs
	default:
		payer, err := resolveMember(tc, in.Payer)
		if err != nil {
			return tricount.Entry{}, err
		}
		receiver, err := resolveMember(tc, in.Receiver)
		if err != nil {
			return tricount.Entry{}, err
		}
		if payer.UUID == receiver.UUID {
			return tricount.Entry{}, fmt.Errorf("payer and receiver are both %s. A reimbursement moves money from one member to another", payer.Name)
		}
		if !in.AmountSet {
			return tricount.Entry{}, coded("missing_amount", "pass --amount as a positive decimal in major units, such as 12.50 or 1500", "Do not pass cents.")
		}
		entry.PayerUUID = payer.UUID
		entry.GroupMinor = in.Amount
		entry.Allocations = []tricount.Alloc{
			{UUID: receiver.UUID, GroupMinor: in.Amount, Kind: "AMOUNT"},
			{UUID: payer.UUID, GroupMinor: 0, Kind: "AMOUNT"},
		}
	}
	return entry, nil
}

func fillShareEntry(entry *tricount.Entry, tc tricount.Tricount, in ensureInput) error {
	allocs := make([]tricount.Alloc, 0, len(in.Shares))
	members := make([]tricount.Member, 0, len(in.Shares))
	var sum int64
	for _, share := range in.Shares {
		member, err := resolveMember(tc, share.ref)
		if err != nil {
			return err
		}
		members = append(members, member)
		allocs = append(allocs, tricount.Alloc{UUID: member.UUID, GroupMinor: share.minor, Kind: "AMOUNT"})
		sum += share.minor
	}
	if err := rejectDuplicateMembers(members); err != nil {
		return err
	}
	if in.AmountSet && in.Amount != sum {
		return fmt.Errorf("--amount is %s and the shares sum to %s. Make them equal, or omit --amount to use the share total", tricount.FormatMajor(in.Amount), tricount.FormatMajor(sum))
	}
	entry.GroupMinor = sum
	entry.Allocations = allocs
	return nil
}

func fillRatioEntry(entry *tricount.Entry, tc tricount.Tricount, in ensureInput) error {
	members := make([]tricount.Member, 0, len(in.Ratios))
	weights := make([]int, 0, len(in.Ratios))
	for _, ratio := range in.Ratios {
		member, err := resolveMember(tc, ratio.ref)
		if err != nil {
			return err
		}
		members = append(members, member)
		weights = append(weights, ratio.ratio)
	}
	if err := rejectDuplicateMembers(members); err != nil {
		return err
	}
	parts, err := tricount.SplitRatio(in.Amount, weights)
	if err != nil {
		return err
	}
	ratios := append([]int(nil), weights...)
	allocs := make([]tricount.Alloc, len(members))
	for i, member := range members {
		allocs[i] = tricount.Alloc{UUID: member.UUID, GroupMinor: parts[i], Kind: "RATIO", Ratio: &ratios[i]}
	}
	entry.GroupMinor = in.Amount
	entry.Allocations = allocs
	return nil
}

func rejectDuplicateMembers(members []tricount.Member) error {
	seen := map[string]string{}
	for _, member := range members {
		if _, ok := seen[strings.ToLower(member.UUID)]; ok {
			return coded("invalid_request", fmt.Sprintf("%s is listed more than once", member.Name), "Pass each member once.")
		}
		seen[strings.ToLower(member.UUID)] = member.Name
	}
	return nil
}

func decideEnsure(txs []tricount.Transaction, key string, entry tricount.Entry, visible string) ensureDecision {
	matches := transactionsForKey(txs, key)
	switch len(matches) {
	case 0:
		return ensureDecision{Action: "create"}
	case 1:
		diffs := ensureDifferences(matches[0], entry, visible, key)
		if len(diffs) > 0 {
			return ensureDecision{Action: "conflict", Match: matches[0], Diffs: diffs}
		}
		return ensureDecision{Action: "existing", Match: matches[0]}
	default:
		return ensureDecision{Action: "ambiguous", Matches: matches}
	}
}

func transactionsForKey(txs []tricount.Transaction, key string) []tricount.Transaction {
	var out []tricount.Transaction
	for _, tx := range txs {
		_, got, ok := descriptionKey(tx.Description)
		if ok && got == key {
			out = append(out, tx)
		}
	}
	return out
}

func ensureDifferences(tx tricount.Transaction, entry tricount.Entry, visible, key string) []string {
	var diffs []string
	txType := storedTxType(tx.Type)
	entryType := entry.Type
	if entryType == "" {
		entryType = "NORMAL"
	}
	if txType != entryType {
		diffs = append(diffs, "type")
	}
	gotVisible, gotKey, ok := descriptionKey(tx.Description)
	if !ok || gotKey != key || gotVisible != visible {
		diffs = append(diffs, "description")
	}
	if !strings.EqualFold(tx.OwnerUUID, entry.PayerUUID) {
		diffs = append(diffs, "payer")
	}
	if absMinor(tx.Amount.Value) != entry.GroupMinor {
		diffs = append(diffs, "amount_raw")
	}
	if cur := strings.TrimSpace(tx.Amount.Currency); cur != "" && !strings.EqualFold(cur, entry.GroupCurrency) {
		diffs = append(diffs, "currency")
	}
	if !sameAllocations(tx.Allocations, entry.Allocations) || ratiosDiffer(tx.Allocations, entry.Allocations) {
		diffs = append(diffs, "allocations")
	}
	return diffs
}

func ratiosDiffer(got []tricount.Allocation, want []tricount.Alloc) bool {
	for _, w := range want {
		if w.Ratio == nil {
			continue
		}
		for _, g := range got {
			if strings.EqualFold(g.MemberUUID, w.UUID) && g.ShareRatio != nil && *g.ShareRatio != *w.Ratio {
				return true
			}
		}
	}
	return false
}

func ensureConflict(key string, tx tricount.Transaction, diffs []string) error {
	return &Error{
		Code:          "idempotency_conflict",
		Message:       fmt.Sprintf("Key %q already identifies transaction %d, but its %s.", key, tx.ID, diffClause(diffs)),
		Hint:          "Repeat the original request, or delete the transaction and use a new key. This command does not edit a keyed transaction.",
		TransactionID: tx.ID,
		Differences:   append([]string(nil), diffs...),
	}
}

func ensureAmbiguous(key string, txs []tricount.Transaction) error {
	ids := make([]string, len(txs))
	for i, tx := range txs {
		ids[i] = strconv.Itoa(tx.ID)
	}
	return &Error{
		Code:    "idempotency_ambiguous",
		Message: fmt.Sprintf("Key %q matches transactions %s.", key, strings.Join(ids, ", ")),
		Hint:    "Delete the extras or use a new key. This command does not choose one or edit them.",
	}
}

func diffClause(diffs []string) string {
	labels := make([]string, len(diffs))
	for i, diff := range diffs {
		if diff == "amount_raw" {
			labels[i] = "amount"
			continue
		}
		labels[i] = diff
	}
	if len(labels) == 1 {
		return labels[0] + " differs"
	}
	if len(labels) == 0 {
		return "content differs"
	}
	return strings.Join(labels, ", ") + " differ"
}

func previewEnsure(tc tricount.Tricount, entry tricount.Entry) ensurePreview {
	sign := int64(1)
	if entry.Type == "NORMAL" || entry.Type == "" {
		sign = -1
	}
	payer, _ := memberByUUID(tc, entry.PayerUUID)
	allocs := make([]allocView, 0, len(entry.Allocations))
	for _, alloc := range entry.Allocations {
		member, _ := memberByUUID(tc, alloc.UUID)
		raw := tricount.FormatMinor(sign * alloc.GroupMinor)
		kind := alloc.Kind
		if kind == "" {
			kind = "AMOUNT"
		}
		allocs = append(allocs, allocView{
			Name:        member.Name,
			UUID:        alloc.UUID,
			AmountRaw:   raw,
			AmountMajor: majorOf(raw),
			Type:        kind,
			ShareRatio:  alloc.Ratio,
		})
	}
	raw := tricount.FormatMinor(sign * entry.GroupMinor)
	return ensurePreview{
		Description: entry.Description,
		Type:        entry.Type,
		TypeMeaning: typeMeaning(entry.Type),
		AmountRaw:   raw,
		Currency:    entry.GroupCurrency,
		Payer:       viewMember(payer),
		Allocations: allocs,
	}
}

func createAndVerifyEnsure(cmd *cobra.Command, tc tricount.Tricount, entry tricount.Entry, input ensureInput) error {
	payload, err := entry.Payload()
	if err != nil {
		return err
	}
	s, err := loadSession(cmdCtx(cmd))
	if err != nil {
		return err
	}
	id, err := s.client.CreateEntry(cmdCtx(cmd), tc.ID, payload)
	if err != nil {
		return err
	}
	updated, err := reloadGroup(cmd, tc)
	if err != nil {
		return &Error{
			Code:          "verification_failed",
			Message:       fmt.Sprintf("Created transaction %d but reading the group back failed: %v", id, err),
			Hint:          "Inspect it before retrying. This command will not edit it.",
			TransactionID: id,
		}
	}
	tx, findErr := findTransaction(updated, strconv.Itoa(id))
	if findErr != nil {
		return &Error{
			Code:          "verification_failed",
			Message:       fmt.Sprintf("Created transaction %d but it was not found when the group was read back.", id),
			Hint:          "Inspect the group with tricount expense list before retrying. This command will not edit it.",
			TransactionID: id,
		}
	}
	if diffs := ensureDifferences(tx, entry, input.Description, input.Key); len(diffs) > 0 {
		return &Error{
			Code:          "verification_failed",
			Message:       fmt.Sprintf("Created transaction %d but the stored entry does not match this request.", id),
			Hint:          fmt.Sprintf("Inspect it with: tricount expense get --transaction %d. This command will not change it.", id),
			TransactionID: id,
			Differences:   diffs,
		}
	}
	report := viewBalanceReport(updated)
	return writeResult(cmd, Result{
		Summary: fmt.Sprintf("Created transaction %d for key %s.", id, input.Key),
		Data: map[string]any{
			"status":          "created",
			"idempotency_key": input.Key,
			"transaction":     viewTransaction(updated, tx),
			"balances":        report,
		},
		Human: formatBalances(report),
	})
}

func writeEnsureExisting(cmd *cobra.Command, tc tricount.Tricount, key string, tx tricount.Transaction) error {
	report := viewBalanceReport(tc)
	return writeResult(cmd, Result{
		Summary: fmt.Sprintf("Transaction %d already exists for key %s.", tx.ID, key),
		Data: map[string]any{
			"status":          "existing",
			"idempotency_key": key,
			"transaction":     viewTransaction(tc, tx),
			"balances":        report,
		},
		Human: formatBalances(report),
	})
}

func writeEnsureDry(cmd *cobra.Command, tc tricount.Tricount, key string, wouldCreate bool, transaction any) error {
	summary := fmt.Sprintf("Dry run: key %s would create a transaction in %s.", key, tc.Title)
	if !wouldCreate {
		summary = fmt.Sprintf("Dry run: key %s already exists in %s.", key, tc.Title)
	}
	return writeResult(cmd, Result{
		Summary: summary,
		Data: map[string]any{
			"status":          "dry_run",
			"idempotency_key": key,
			"would_create":    wouldCreate,
			"transaction":     transaction,
		},
	})
}
