package datastructure

type ZSet struct {
	tree *bPlusTree
	scores map[string]float64 // maps member to score for O(1) score lookup
}

func NewSortedSet(degree int) *ZSet {
	return &ZSet{
		tree: newBPlusTree(degree), // degree 4 for example; can be tuned
		scores: make(map[string]float64),
	}
}
// When insert a new element with score to the set, we need to check if the member already exists
// with different score or not, if yes, remove it and insert the new one.
func (zset *ZSet) Add(score float64, member string) int {
	if curr, exists := zset.scores[member]; exists {
		if curr != score {
			zset.tree.removeMember(member)
			zset.tree.add(score, member)
			zset.scores[member] = score
		}
		return 0 // score updated, but not a new member
	}
	zset.tree.add(score, member)
	zset.scores[member] = score
	return 1
}

func (zset *ZSet) Score(member string) (float64, bool) {
	score, exists := zset.scores[member]
	return score, exists
}
// For simplicity, we just implement sequential search at the leave level
// Later improvement can use the idea of skip list to speed up the rank calculation.
func (zset *ZSet) Rank(member string) (int64, bool) {
	if _, ok := zset.scores[member]; !ok {
		return 0, false
	}
	node := zset.tree.root
	for !node.isLeaf {
		node = node.children[0]
	}
	var rank int64
	for node != nil {
		for _, item := range node.items {
			if item.member == member {
				return rank, true
			}
			rank++
		}
		node = node.next
	}
	return 0, false
}