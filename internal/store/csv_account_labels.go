package store

const csvAccountLabelsKey = "csv-account-labels"

// CSVAccountLabels maps the IBAN of an account known only from CSV imports to
// the name the user gave it.
func (s *Store) CSVAccountLabels() (map[string]string, error) {
	labels := map[string]string{}
	if err := s.readSetting(csvAccountLabelsKey, &labels); err != nil {
		return map[string]string{}, err
	}
	if labels == nil {
		labels = map[string]string{}
	}
	return labels, nil
}

// SetCSVAccountLabel names a CSV account; an empty label removes the name.
func (s *Store) SetCSVAccountLabel(id, label string) error {
	labels, err := s.CSVAccountLabels()
	if err != nil {
		return err
	}
	if label == "" {
		delete(labels, id)
	} else {
		labels[id] = label
	}
	return s.writeSetting(csvAccountLabelsKey, labels)
}
