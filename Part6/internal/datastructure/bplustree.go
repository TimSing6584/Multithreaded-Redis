package datastructure

type bPlusTree struct {
	root   *bNode
	degree int // max children per node; a node splits above degree-1 items
}

// B+ tree node structure:
// Each node contains a slice of items and a slice of child nodes.
// The isLeaf field indicates whether the node is a leaf node or an internal node.
// The parent field points to the parent node.
// The next field is used for leaf nodes to create a linked list of leaves.
// For internal nodes, counts[i] is the number of entries in children[i]'s
// subtree; it is what makes rank queries O(log n).
type bNode struct {
	items    []*bItem
	children []*bNode
	counts   []int // internal-only, parallel to children
	isLeaf   bool
	parent   *bNode
	next     *bNode // leaf-only
}

// Each node can have at most degree - 1 items. Each item contains a score and a member.
type bItem struct {
	score  float64
	member string
}

// less orders items by score, ties broken by member, so every (score,
// member) pair has one unambiguous position in the tree.
func less(score float64, member string, item *bItem) bool {
	if score != item.score {
		return score < item.score
	}
	return member < item.member
}

func newBPlusTree(degree int) *bPlusTree {
	return &bPlusTree{root: &bNode{isLeaf: true}, degree: degree}
}

// size returns the number of entries in n's subtree.
// Entries here mean the number of items at leaf level of that subtree,
// not the number of items in the node itself.
func size(n *bNode) int {
	if n.isLeaf {
		return len(n.items)
	}
	total := 0
	for _, c := range n.counts {
		total += c
	}
	return total
}

func childIndex(parent, child *bNode) int {
	i := 0
	for parent.children[i] != child {
		i++
	}
	return i
}

// adjustCounts adds delta to the subtree count of every ancestor of node,
// walking up through parent pointers.
func adjustCounts(node *bNode, delta int) {
	for node.parent != nil {
		parent := node.parent
		parent.counts[childIndex(parent, node)] += delta
		node = parent
	}
}

func (t *bPlusTree) rank(score float64, member string) (int64, bool) {
	var r int64
	node := t.root
	for !node.isLeaf {
		i := 0
		for i < len(node.items) && !less(score, member, node.items[i]) {
			r += int64(node.counts[i])
			i++
		}
		node = node.children[i]
	}
	for i, it := range node.items {
		if it.score == score && it.member == member {
			return r + int64(i), true
		}
	}
	return 0, false
}

// at returns the entry at 0-based rank k, or nil if k is out of range.
// At each internal node, skip whole children while k is past them.
// Used for commands like ZRANGE and ZREVRANGE,...
func (t *bPlusTree) at(k int64) *bItem {
	if k < 0 {
		return nil
	}
	node := t.root
	for !node.isLeaf {
		i := 0
		for i < len(node.counts)-1 && k >= int64(node.counts[i]) {
			k -= int64(node.counts[i])
			i++
		}
		node = node.children[i]
	}
	if k >= int64(len(node.items)) {
		return nil
	}
	return node.items[k]
}

// findLeaf descends from the root to the leaf where (score, member) belongs.
func (t *bPlusTree) findLeaf(score float64, member string) *bNode {
	node := t.root
	for !node.isLeaf {
		i := 0
		for i < len(node.items) && !less(score, member, node.items[i]) {
			i++
		}
		node = node.children[i]
	}
	return node
}

// add() inserts (score, member) into the tree, splitting nodes on overflow.
// It does not check for an existing entry for member: the caller must
// remove the old (score, member) first if the member is already present.
func (t *bPlusTree) add(score float64, member string) {
	node := t.findLeaf(score, member)

	i := 0
	for i < len(node.items) && !less(score, member, node.items[i]) {
		i++
	}
	node.items = append(node.items, nil)
	copy(node.items[i+1:], node.items[i:])
	node.items[i] = &bItem{score: score, member: member}

	// Update ancestor counts before splitting, while the path is intact;
	// splits then recompute the counts of the nodes they change.
	adjustCounts(node, +1)

	if len(node.items) > t.degree-1 {
		t.splitNode(node)
	}
}

// remove() deletes the entry (score, member) from its leaf, if present,
// and reports whether it was found. No rebalancing is done, meaning that
// internal nodes are untouched, and leaves may become sparse or empty.
func (t *bPlusTree) remove(score float64, member string) bool {
	node := t.findLeaf(score, member)

	for i, it := range node.items {
		if it.score == score && it.member == member {
			copy(node.items[i:], node.items[i+1:])
			node.items[len(node.items)-1] = nil // let the GC free the removed item
			node.items = node.items[:len(node.items)-1]
			adjustCounts(node, -1)
			return true
		}
	}
	return false
}

func (t *bPlusTree) splitNode(node *bNode) {
	if node.parent == nil {
		t.splitRoot()
		return
	}
	if node.isLeaf {
		t.splitLeaf(node)
	} else {
		t.splitInternal(node)
	}
}

func (t *bPlusTree) splitLeaf(node *bNode) {
	mid := len(node.items) / 2
	sibling := &bNode{isLeaf: true, parent: node.parent, next: node.next}
	sibling.items = append(sibling.items, node.items[mid:]...)
	node.items = node.items[:mid]
	node.next = sibling

	// Promote a copy of the new leaf's first item: leaves keep every item,
	// internal nodes only route, and the two must not share a pointer.
	first := sibling.items[0]
	t.insertIntoParent(node, sibling, &bItem{score: first.score, member: first.member})
}

func (t *bPlusTree) splitInternal(node *bNode) {
	mid := len(node.items) / 2
	promoted := node.items[mid]

	sibling := &bNode{parent: node.parent}
	sibling.items = append(sibling.items, node.items[mid+1:]...)
	sibling.children = append(sibling.children, node.children[mid+1:]...)
	sibling.counts = append(sibling.counts, node.counts[mid+1:]...)
	node.items = node.items[:mid]
	node.children = node.children[:mid+1]
	node.counts = node.counts[:mid+1]
	for _, child := range sibling.children {
		child.parent = sibling
	}

	t.insertIntoParent(node, sibling, promoted)
}

func (t *bPlusTree) splitRoot() {
	oldRoot := t.root
	newRoot := &bNode{}
	t.root = newRoot
	oldRoot.parent = newRoot
	newRoot.children = append(newRoot.children, oldRoot)
	newRoot.counts = append(newRoot.counts, size(oldRoot))

	if oldRoot.isLeaf {
		t.splitLeaf(oldRoot)
	} else {
		t.splitInternal(oldRoot)
	}
}

// insertIntoParent inserts sibling as node's right neighbor in their
// shared parent, promoting key as the routing item between them, and
// recursively splits the parent if that overflows it.
func (t *bPlusTree) insertIntoParent(node, sibling *bNode, key *bItem) {
	parent := node.parent
	idx := 0
	for idx < len(parent.children) && parent.children[idx] != node {
		idx++
	}

	parent.items = append(parent.items, nil)
	copy(parent.items[idx+1:], parent.items[idx:])
	parent.items[idx] = key

	parent.children = append(parent.children, nil)
	copy(parent.children[idx+2:], parent.children[idx+1:])
	parent.children[idx+1] = sibling

	// The split divided node's entries between node and sibling, so
	// recompute both counts. The parent's own total is unchanged.
	parent.counts = append(parent.counts, 0)
	copy(parent.counts[idx+2:], parent.counts[idx+1:])
	parent.counts[idx] = size(node)
	parent.counts[idx+1] = size(sibling)

	if len(parent.items) > t.degree-1 {
		t.splitNode(parent)
	}
}
