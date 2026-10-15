package datastructure

// SimpleSet is an unordered collection of unique string members, backed by
// a native Go map - Add/Rem/IsMember are all O(1).
type SimpleSet struct {
	members map[string]struct{}
}

func NewSimpleSet() *SimpleSet {
	return &SimpleSet{members: make(map[string]struct{})}
}

func (s *SimpleSet) Add(members ...string) int{
	added := 0
	for _, member := range members {
		if _, exists := s.members[member]; !exists {
			s.members[member] = struct{}{}
			added++
		}
	}
	return added
}

func (s *SimpleSet) Remove(members ...string) int {
	removed := 0
	for _, member := range members {
		if _, exists := s.members[member]; exists {
			delete(s.members, member)
			removed++
		}
	}
	return removed
}

func (s *SimpleSet) IsMember(member string) bool{
	_, exists := s.members[member]
	return exists
}

func (s *SimpleSet) Len() int {
	return len(s.members)
}

// Members returns every member in unspecified order, matching real Redis:
// SMEMBERS makes no ordering guarantee for a plain set.
func (s *SimpleSet) Members() []string {
	res := make([]string, 0, len(s.members))
	for member := range s.members {
		res = append(res, member)
	}
	return res
}