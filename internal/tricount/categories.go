package tricount

import "strings"

// Category is a standard expense category understood by the Tricount app.
type Category struct {
	ID          string
	Emoji       string
	Description string
}

// ExpenseCategories are the built-in expense categories.
// UNCATEGORIZED is accepted by the API and has no emoji in the app.
func ExpenseCategories() []Category {
	return []Category{
		{ID: "TRAVEL", Emoji: "🛏", Description: "Accommodation"},
		{ID: "ENTERTAINMENT", Emoji: "🎤", Description: "Entertainment"},
		{ID: "GROCERIES", Emoji: "🛒", Description: "Groceries"},
		{ID: "HEALTHCARE", Emoji: "🦷", Description: "Healthcare"},
		{ID: "INSURANCE", Emoji: "🧯", Description: "Insurance"},
		{ID: "RENT_AND_UTILITIES", Emoji: "🏠", Description: "Rent and utilities"},
		{ID: "FOOD_AND_DRINK", Emoji: "🍔", Description: "Restaurants and drinks"},
		{ID: "SHOPPING", Emoji: "🛍", Description: "Shopping"},
		{ID: "TRANSPORT", Emoji: "🚕", Description: "Transport"},
		{ID: "OTHER", Emoji: "✋", Description: "Other"},
		{ID: "UNCATEGORIZED", Emoji: "", Description: "No category"},
	}
}

// GroupCategories are categories stored on the group itself, separate from expense categories.
func GroupCategories() []string {
	return []string{
		"GENERAL",
		"OTHER",
		"TRAVEL",
		"FOOD_AND_DRINK",
		"TRANSPORT",
		"SHOPPING",
		"ENTERTAINMENT",
		"GROCERIES",
	}
}

// NormalizeExpenseCategory uppercases a category id and reports whether it is known.
func NormalizeExpenseCategory(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	for _, c := range ExpenseCategories() {
		if c.ID == s {
			return s, true
		}
	}
	return s, false
}

// NormalizeGroupCategory uppercases a group category and reports whether it is known.
func NormalizeGroupCategory(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	for _, c := range GroupCategories() {
		if c == s {
			return s, true
		}
	}
	return s, false
}

// ExpenseCategoryList is a single line of known expense category ids.
func ExpenseCategoryList() string {
	ids := make([]string, 0, len(ExpenseCategories()))
	for _, c := range ExpenseCategories() {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ", ")
}

// GroupCategoryList is a single line of known group category ids.
func GroupCategoryList() string {
	return strings.Join(GroupCategories(), ", ")
}
