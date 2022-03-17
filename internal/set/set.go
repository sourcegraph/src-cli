package set

type Set[E comparable] map[E]struct{}

func New[E comparable]() Set[E] {
	return Set[E]{}
}

func (s Set[E]) Add(v E) {
	s[v] = struct{}{}
}

func (s Set[E]) Contains(v E) bool {
	_, ok := s[v]
	return ok
}
