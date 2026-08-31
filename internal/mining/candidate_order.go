package mining

import (
	"cmp"
	"slices"
)

func rankCandidates(candidates []Candidate) {
	slices.SortFunc(candidates, compareCandidates)
	for index := range candidates {
		candidates[index].Rank = index + 1
		candidates[index].antecedentName = ""
		candidates[index].consequentName = ""
	}
}

// compareCandidates implements the frozen ranking precedence. The two name
// fields contain raw bytes despite their string type, so cmp.Compare provides
// the required byte ordering even for invalid UTF-8.
func compareCandidates(left, right Candidate) int {
	if c := cmp.Compare(right.WeightedConfidence, left.WeightedConfidence); c != 0 {
		return c
	}
	if c := cmp.Compare(right.WeightedLift, left.WeightedLift); c != 0 {
		return c
	}
	if c := cmp.Compare(right.RawSupport, left.RawSupport); c != 0 {
		return c
	}
	if c := cmp.Compare(right.RawOpportunity, left.RawOpportunity); c != 0 {
		return c
	}
	if c := cmp.Compare(left.antecedentName, right.antecedentName); c != 0 {
		return c
	}

	return cmp.Compare(left.consequentName, right.consequentName)
}

func validatePairIdentities(pairs []Pair) error {
	var (
		previous     pairKey
		havePrevious bool
		seen         map[pairKey]bool
	)

	for index, pair := range pairs {
		if pair.Antecedent == pair.Consequent {
			return &SelfPairError{ComponentID: pair.Antecedent}
		}

		identity := pairKey{antecedent: pair.Antecedent, consequent: pair.Consequent}
		if seen != nil && seen[identity] {
			return duplicatePairError(identity)
		}
		if seen == nil && havePrevious {
			switch comparePairKeys(identity, previous) {
			case 0:
				return duplicatePairError(identity)
			case -1:
				seen = make(map[pairKey]bool, len(pairs))
				for _, prior := range pairs[:index] {
					seen[pairKey{
						antecedent: prior.Antecedent,
						consequent: prior.Consequent,
					}] = true
				}
				if seen[identity] {
					return duplicatePairError(identity)
				}
			}
		}
		if seen != nil {
			seen[identity] = true
		}
		previous = identity
		havePrevious = true
	}

	return nil
}

func comparePairKeys(left, right pairKey) int {
	return cmp.Or(
		cmp.Compare(left.antecedent, right.antecedent),
		cmp.Compare(left.consequent, right.consequent),
	)
}

func duplicatePairError(identity pairKey) error {
	return &DuplicatePairIdentityError{
		Antecedent: identity.antecedent,
		Consequent: identity.consequent,
	}
}
