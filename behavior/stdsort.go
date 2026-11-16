package behavior

import "math/bits"

// The limits of libc++'s std::sort, which BDS's Linux build uses.
const (
	sortInsertionLimit = 24  // shorter ranges are insertion sorted
	sortNintherLimit   = 128 // longer ranges pick the pivot from nine
	sortIncompleteMax  = 8   // moves before the quick insertion sort gives up
)

// stdSort sorts s in the exact order libc++'s std::sort leaves it, so equal
// elements end where BDS puts them. It is introsort: median-of-three or
// ninther pivots, insertion sort under 24 elements, heap sort past the
// depth limit. The algorithm follows LLVM libc++ (Apache-2.0 WITH
// LLVM-exception); see CREDITS.md.
func stdSort[T any](s []T, less func(a, b T) bool) {
	if len(s) < 2 {
		return
	}
	introsort(s, 0, len(s), less, 2*(bits.Len(uint(len(s)))-1), true)
}

// introsort sorts s[first:last].
func introsort[T any](s []T, first, last int, less func(a, b T) bool, depth int, leftmost bool) {
	for {
		n := last - first
		switch n {
		case 0, 1:
			return
		case 2:
			if less(s[last-1], s[first]) {
				s[first], s[last-1] = s[last-1], s[first]
			}
			return
		case 3:
			sort3(s, first, first+1, last-1, less)
			return
		case 4:
			sort4(s, first, first+1, first+2, last-1, less)
			return
		case 5:
			sort5(s, first, first+1, first+2, first+3, last-1, less)
			return
		}
		if n < sortInsertionLimit {
			if leftmost {
				insertionSort(s, first, last, less)
			} else {
				insertionSortUnguarded(s, first, last, less)
			}
			return
		}
		if depth == 0 {
			heapSort(s[first:last], less)
			return
		}
		depth--
		choosePivot(s, first, last, less)
		if !leftmost && !less(s[first-1], s[first]) {
			first = partitionEqualsLeft(s, first, last, less)
			continue
		}
		i, done := partitionEqualsRight(s, first, last, less)
		if done {
			fs := insertionSortIncomplete(s, first, i, less)
			if insertionSortIncomplete(s, i+1, last, less) {
				if fs {
					return
				}
				last = i
				continue
			}
			if fs {
				first = i + 1
				continue
			}
		}
		introsort(s, first, i, less, depth, leftmost)
		leftmost = false
		first = i + 1
	}
}

// choosePivot moves the pivot to first.
func choosePivot[T any](s []T, first, last int, less func(a, b T) bool) {
	half := (last - first) / 2
	if last-first > sortNintherLimit {
		sort3(s, first, first+half, last-1, less)
		sort3(s, first+1, first+half-1, last-2, less)
		sort3(s, first+2, first+half+1, last-3, less)
		sort3(s, first+half-1, first+half, first+half+1, less)
		s[first], s[first+half] = s[first+half], s[first]
		return
	}
	sort3(s, first+half, first, last-1, less)
}

// partitionEqualsRight puts the elements below the pivot on its left and
// returns where the pivot ends; done reports no swap was needed.
func partitionEqualsRight[T any](s []T, begin, last int, less func(a, b T) bool) (int, bool) {
	pivot := s[begin]
	first := begin + 1
	for less(s[first], pivot) {
		first++
	}
	if first-1 == begin {
		for first < last {
			last--
			if less(s[last], pivot) {
				break
			}
		}
	} else {
		last--
		for !less(s[last], pivot) {
			last--
		}
	}
	done := first >= last
	for first < last {
		s[first], s[last] = s[last], s[first]
		first++
		for less(s[first], pivot) {
			first++
		}
		last--
		for !less(s[last], pivot) {
			last--
		}
	}
	p := first - 1
	if p != begin {
		s[begin] = s[p]
	}
	s[p] = pivot
	return p, done
}

// partitionEqualsLeft puts the elements equal to the pivot on its left and
// returns the start of the rest.
func partitionEqualsLeft[T any](s []T, begin, last int, less func(a, b T) bool) int {
	pivot := s[begin]
	first := begin
	if less(pivot, s[last-1]) {
		first++
		for !less(pivot, s[first]) {
			first++
		}
	} else {
		first++
		for first < last && !less(pivot, s[first]) {
			first++
		}
	}
	if first < last {
		last--
		for less(pivot, s[last]) {
			last--
		}
	}
	for first < last {
		s[first], s[last] = s[last], s[first]
		first++
		for !less(pivot, s[first]) {
			first++
		}
		last--
		for less(pivot, s[last]) {
			last--
		}
	}
	p := first - 1
	if p != begin {
		s[begin] = s[p]
	}
	s[p] = pivot
	return first
}

func sort3[T any](s []T, x, y, z int, less func(a, b T) bool) {
	if !less(s[y], s[x]) {
		if !less(s[z], s[y]) {
			return
		}
		s[y], s[z] = s[z], s[y]
		if less(s[y], s[x]) {
			s[x], s[y] = s[y], s[x]
		}
		return
	}
	if less(s[z], s[y]) {
		s[x], s[z] = s[z], s[x]
		return
	}
	s[x], s[y] = s[y], s[x]
	if less(s[z], s[y]) {
		s[y], s[z] = s[z], s[y]
	}
}

func sort4[T any](s []T, a, b, c, d int, less func(x, y T) bool) {
	sort3(s, a, b, c, less)
	if less(s[d], s[c]) {
		s[c], s[d] = s[d], s[c]
		if less(s[c], s[b]) {
			s[b], s[c] = s[c], s[b]
			if less(s[b], s[a]) {
				s[a], s[b] = s[b], s[a]
			}
		}
	}
}

func sort5[T any](s []T, a, b, c, d, e int, less func(x, y T) bool) {
	sort4(s, a, b, c, d, less)
	if less(s[e], s[d]) {
		s[d], s[e] = s[e], s[d]
		if less(s[d], s[c]) {
			s[c], s[d] = s[d], s[c]
			if less(s[c], s[b]) {
				s[b], s[c] = s[c], s[b]
				if less(s[b], s[a]) {
					s[a], s[b] = s[b], s[a]
				}
			}
		}
	}
}

func insertionSort[T any](s []T, first, last int, less func(a, b T) bool) {
	for i := first + 1; i < last; i++ {
		if !less(s[i], s[i-1]) {
			continue
		}
		t := s[i]
		j := i
		for {
			s[j] = s[j-1]
			j--
			if j == first || !less(t, s[j-1]) {
				break
			}
		}
		s[j] = t
	}
}

// insertionSortUnguarded relies on an element before first that is not
// above any in the range.
func insertionSortUnguarded[T any](s []T, first, last int, less func(a, b T) bool) {
	for i := first + 1; i < last; i++ {
		if !less(s[i], s[i-1]) {
			continue
		}
		t := s[i]
		j := i
		for {
			s[j] = s[j-1]
			j--
			if !less(t, s[j-1]) {
				break
			}
		}
		s[j] = t
	}
}

// insertionSortIncomplete sorts s[first:last] unless that takes more than
// eight moves; it reports a finished sort.
func insertionSortIncomplete[T any](s []T, first, last int, less func(a, b T) bool) bool {
	switch last - first {
	case 0, 1:
		return true
	case 2:
		if less(s[last-1], s[first]) {
			s[first], s[last-1] = s[last-1], s[first]
		}
		return true
	case 3:
		sort3(s, first, first+1, last-1, less)
		return true
	case 4:
		sort4(s, first, first+1, first+2, last-1, less)
		return true
	case 5:
		sort5(s, first, first+1, first+2, first+3, last-1, less)
		return true
	}
	sort3(s, first, first+1, first+2, less)
	count := 0
	for i := first + 3; i < last; i++ {
		if !less(s[i], s[i-1]) {
			continue
		}
		t := s[i]
		j := i
		for {
			s[j] = s[j-1]
			j--
			if j == first || !less(t, s[j-1]) {
				break
			}
		}
		s[j] = t
		count++
		if count == sortIncompleteMax {
			return i+1 == last
		}
	}
	return true
}

// heapSort is libc++'s make_heap then sort_heap with Floyd's pop.
func heapSort[T any](s []T, less func(a, b T) bool) {
	n := len(s)
	for start := (n - 2) / 2; start >= 0; start-- {
		siftDown(s, n, start, less)
	}
	for ; n > 1; n-- {
		popHeap(s, n, less)
	}
}

func siftDown[T any](s []T, n, start int, less func(a, b T) bool) {
	child := start
	if n < 2 || (n-2)/2 < child {
		return
	}
	child = 2*child + 1
	if child+1 < n && less(s[child], s[child+1]) {
		child++
	}
	if less(s[child], s[start]) {
		return
	}
	top := s[start]
	for {
		s[start] = s[child]
		start = child
		if (n-2)/2 < child {
			break
		}
		child = 2*child + 1
		if child+1 < n && less(s[child], s[child+1]) {
			child++
		}
		if less(s[child], top) {
			break
		}
	}
	s[start] = top
}

func popHeap[T any](s []T, n int, less func(a, b T) bool) {
	top := s[0]
	hole, child := 0, 0
	for {
		c := 2*child + 1
		if c+1 < n && less(s[c], s[c+1]) {
			c++
		}
		s[hole] = s[c]
		hole, child = c, c
		if child > (n-2)/2 {
			break
		}
	}
	last := n - 1
	if hole == last {
		s[hole] = top
		return
	}
	s[hole] = s[last]
	s[last] = top
	siftUp(s, hole+1, less)
}

// siftUp moves s[n-1] up the heap s[:n].
func siftUp[T any](s []T, n int, less func(a, b T) bool) {
	if n < 2 {
		return
	}
	p := (n - 2) / 2
	last := n - 1
	if !less(s[p], s[last]) {
		return
	}
	t := s[last]
	for {
		s[last] = s[p]
		last = p
		if p == 0 {
			break
		}
		p = (p - 1) / 2
		if !less(s[p], t) {
			break
		}
	}
	s[last] = t
}
