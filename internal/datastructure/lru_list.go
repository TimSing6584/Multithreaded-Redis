// This file defines the doubly linked list used by traditional LRU eviction.
// The head side holds the LRU key, the tail side holds the MRU key. A map from key to node
// makes every operation O(1): touching a key moves its node to the tail, evicting pops the head.
package datastructure

type lruNode struct {
	key        string
	prev, next *lruNode
}

// LRUList uses two sentinel nodes, so inserting/unlinking never has to special-case an empty list.
// head.next is the LRU key, tail.prev is the MRU key.
type LRUList struct {
	head, tail *lruNode
	nodes      map[string]*lruNode
}

func NewLRUList() *LRUList {
	head, tail := &lruNode{}, &lruNode{}
	head.next = tail
	tail.prev = head
	return &LRUList{head: head, tail: tail, nodes: make(map[string]*lruNode)}
}

// Touch marks key as the most recently used: an existing node is moved to the tail,
// a new key gets a new node at the tail.
func (l *LRUList) Touch(key string) {
	if n, ok := l.nodes[key]; ok {
		l.unlink(n)
		l.pushBack(n)
		return
	}
	n := &lruNode{key: key}
	l.nodes[key] = n
	l.pushBack(n)
}

// Remove drops key from the list. It does nothing if key is not tracked.
func (l *LRUList) Remove(key string) {
	if n, ok := l.nodes[key]; ok {
		l.unlink(n)
		delete(l.nodes, key)
	}
}

// PopLRU removes and returns the least recently used key, and false if the list is empty.
func (l *LRUList) PopLRU() (string, bool) {
	n := l.head.next
	if n == l.tail {
		return "", false
	}
	l.unlink(n)
	delete(l.nodes, n.key)
	return n.key, true
}

func (l *LRUList) Len() int {
	return len(l.nodes)
}

func (l *LRUList) unlink(n *lruNode) {
	n.prev.next = n.next
	n.next.prev = n.prev
	n.prev, n.next = nil, nil
}

// pushBack inserts n right before the tail sentinel, i.e. as the MRU key.
func (l *LRUList) pushBack(n *lruNode) {
	n.prev = l.tail.prev
	n.next = l.tail
	l.tail.prev.next = n
	l.tail.prev = n
}
