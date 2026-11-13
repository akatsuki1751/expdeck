package core

import "strings"

// Search finds experiments by path or hypothesis, without changing records.
func (k *Kernel) Search(query string) ([]*Experiment, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	experiments, err := k.List()
	if err != nil {
		return nil, err
	}
	var matches []*Experiment
	for _, exp := range experiments {
		if strings.Contains(strings.ToLower(exp.PathString()), query) {
			matches = append(matches, exp)
			continue
		}
		if strings.Contains(strings.ToLower(exp.Hypothesis), query) {
			matches = append(matches, exp)
		}
	}
	return matches, nil
}
