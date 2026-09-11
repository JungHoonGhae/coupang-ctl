package core

import "errors"

// ValidateCategoryScope checks the typed contract after the source adapter has
// matched an explicit click, selected label and native destination breadcrumb.
// It cannot manufacture the source evidence from a URL or caller-supplied ID.
func (c ProductCoverage) ValidateCategoryScope(request ProductSearchRequest) error {
	if c.AppliedCategoryID == "" {
		return nil
	}
	if request.Query != "" || !NumericProductIdentifier(request.CategoryID) || !NumericProductIdentifier(c.AppliedCategoryID) {
		return errors.New("invalid applied category scope")
	}
	selections := 0
	label := ""
	for _, selection := range request.FacetSelections {
		if selection.Name == "카테고리" {
			selections++
			label = selection.Label
		}
	}
	if selections != 1 {
		return errors.New("category transition requires one explicit sidebar category choice")
	}
	groups, choices := 0, 0
	selected := false
	for _, group := range c.Facets {
		if group.Name != "카테고리" {
			continue
		}
		groups++
		for _, option := range group.Options {
			if option.Label == label {
				choices++
				selected = option.Selected && !option.Disabled
			}
		}
	}
	if groups != 1 || choices != 1 || !selected {
		return errors.New("applied category choice was not verified")
	}
	return nil
}
