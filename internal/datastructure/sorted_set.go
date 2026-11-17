package datastructure

type ZSet struct {
	tree   *bPlusTree
	scores map[string]float64 // maps member to score for O(1) score lookup
}

func NewSortedSet(degree int) *ZSet {
	return &ZSet{
		tree:   newBPlusTree(degree), // degree 4 for example; can be tuned
		scores: make(map[string]float64),
	}
}

// When insert a new element with score to the set, we need to check if the member already exists
// with different score or not, if yes, remove it and insert the new one.
func (zset *ZSet) Add(score float64, member string) int {
	if curr, exists := zset.scores[member]; exists {
		if curr != score {
			zset.tree.remove(curr, member) // descend using the old score: O(log n)
			zset.tree.add(score, member)
			zset.scores[member] = score
		}
		return 0 // score updated (or unchanged), but not a new member
	}
	zset.tree.add(score, member)
	zset.scores[member] = score
	return 1
}

// Rem removes member from the set.
func (zset *ZSet) Rem(member string) int {
	score, exists := zset.scores[member]
	if !exists {
		return 0
	}
	zset.tree.remove(score, member)
	delete(zset.scores, member)
	return 1
}

// Len returns the number of members in the set.
func (zset *ZSet) Len() int {
	return len(zset.scores)
}

func (zset *ZSet) Score(member string) (float64, bool) {
	score, exists := zset.scores[member]
	return score, exists
}

// Rank returns the 0-based position of member in (score, member) order.
func (zset *ZSet) Rank(member string) (int64, bool) {
	score, ok := zset.scores[member]
	if !ok {
		return 0, false
	}
	return zset.tree.rank(score, member)
}