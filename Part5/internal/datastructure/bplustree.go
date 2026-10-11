package datastructure

type bPlusTree struct {
	root   *bNode
	degree int // max children per node; a node splits above degree-1 items
}
// B+ tree node structure:
// Each node contains a slice of items and a slice of child nodes. 
// The isLeaf field indicates whether the node is a leaf node or an internal node. 
// The parent field points to the parent node 
// The next field is used for leaf nodes to create a linked list of leaves
type bNode struct {
	items    []*bItem
	children []*bNode
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

// This function finds the index at the leaves level where the new item should be inserted.
// Then split node if overflow occurs. The caller must ensure that the member is not already present in the tree,
// as this implementation does not handle duplicates. So call removeMember first
func (t *bPlusTree) add(score float64, member string) {
	node := t.root
	for !node.isLeaf {
		i := 0
		for i < len(node.items) && !less(score, member, node.items[i]) {
			i++
		}
		node = node.children[i]
	}
		i := 0
	for i < len(node.items) && !less(score, member, node.items[i]) {
		i++
	}
	node.items = append(node.items, nil)
	copy(node.items[i+1:], node.items[i:])
	node.items[i] = &bItem{score: score, member: member}

	if len(node.items) > t.degree - 1 {
		t.splitNode(node)
	}
}
// This function moves to the leftmost leaf node and traverses the linked list of leaves to find and remove the specified member.
// This is simple version as no balancing is done after removal.
func (t *bPlusTree) removeMember(member string) {
	node := t.root
	for !node.isLeaf {
		node = node.children[0]
	}
	for node != nil {
		for i, it := range node.items {
			if it.member == member {
				node.items = append(node.items[:i], node.items[i+1:]...)
				return
			}
		}
		node = node.next
	}
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

	// The new leaf's first key is promoted as-is (not removed from the
	// leaf): leaves hold every item, internal nodes just route.
	t.insertIntoParent(node, sibling, sibling.items[0])
}

func (t *bPlusTree) splitInternal(node *bNode) {
	mid := len(node.items) / 2
	promoted := node.items[mid]

	sibling := &bNode{parent: node.parent}
	sibling.items = append(sibling.items, node.items[mid+1:]...)
	sibling.children = append(sibling.children, node.children[mid+1:]...)
	node.items = node.items[:mid]
	node.children = node.children[:mid+1]
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

	if len(parent.items) > t.degree-1 {
		t.splitNode(parent)
	}
}


