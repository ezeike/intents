package laya

import (
	"bytes"
	"encoding/json"
)

// Criterion is one option of a "choice" question's Criteria: Key is the wire value Laya returns
// as the winning choice, Description is the natural-language text shown to the model.
type Criterion struct {
	Key         string
	Description string
}

// OrderedCriteria is a "choice" question's option list, marshaled in the caller-declared order.
//
// encoding/json sorts a map[string]string's keys alphabetically before marshaling it, which
// silently reorders any criteria built from a map into alphabetical wire order regardless of the
// order the caller wrote them in. That matters here specifically: Laya has a measured, real
// first-slot position bias (laya skill, "position bias" — k=3 identical-text options score
// 0.47/0.32/0.21 by slot; k=15 slot 0 alone takes 0.62). OrderedCriteria exists so option order on
// the wire is always what the caller wrote, never an accident of Go map iteration.
type OrderedCriteria []Criterion

// MarshalJSON writes c as a JSON object, one key per Criterion, in slice order.
func (c OrderedCriteria) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, crit := range c {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(crit.Key)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(crit.Description)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Keys returns c's option keys in declared order.
func (c OrderedCriteria) Keys() []string {
	keys := make([]string, len(c))
	for i, crit := range c {
		keys[i] = crit.Key
	}
	return keys
}
