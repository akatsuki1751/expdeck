package core

import "strings"

// Search finds experiments by path or saved note, without changing records.
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
		note, _, err := k.ReadNote(exp)
		if err != nil {
			return nil, err
		}
		if strings.Contains(strings.ToLower(note), query) {
			matches = append(matches, exp)
		}
	}
	return matches, nil
}
