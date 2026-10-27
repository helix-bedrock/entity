package path

// ActorFlags is a mob's set of actor flags, by BDS's actor flag number. It
// lives here with the other shared types so physics needs no AI package.
type ActorFlags [3]uint64

// Has reports whether flag n is set.
func (f *ActorFlags) Has(n int) bool { return n >= 0 && n < 192 && f[n>>6]&(1<<(n&63)) != 0 }

// Set sets or clears flag n.
func (f *ActorFlags) Set(n int, on bool) {
	if n < 0 || n >= 192 {
		return
	}
	if on {
		f[n>>6] |= 1 << (n & 63)
	} else {
		f[n>>6] &^= 1 << (n & 63)
	}
}
