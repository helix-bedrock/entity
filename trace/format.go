package trace

import (
	"fmt"
	"io"
	"strings"
)

// String writes the record as one line of a timeline. It allocates: use it for dumps and
// views, not on a tick.
func (r *Record) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %-5s %s", r.Tick, r.Kind, r.What)
	if r.Name != "" {
		b.WriteString(" ")
		b.WriteString(r.Name)
	}
	switch r.Kind {
	case KindTick:
		fmt.Fprintf(&b, " pos %s health %.2f", vec(r.V), r.F)
	case KindVelocity:
		fmt.Fprintf(&b, " %s", vec(r.V))
	case KindTarget:
		fmt.Fprintf(&b, " id %d", r.N)
		if r.V != [3]float32{} {
			fmt.Fprintf(&b, " at %s dist %.2f", vec(r.V), r.F)
		}
	case KindBlock:
		fmt.Fprintf(&b, " at (%d %d %d)", int(r.V[0]), int(r.V[1]), int(r.V[2]))
	case KindPhysics:
		fmt.Fprintf(&b, " vel %s%s", vec(r.V), physicsBits(r.N))
	case KindFlag:
		fmt.Fprintf(&b, " %d", r.N)
	case KindGoal:
		if r.N != 0 {
			fmt.Fprintf(&b, " (priority %d)", r.N)
		}
	case KindHurt:
		fmt.Fprintf(&b, " %.2f cause %d", r.F, r.N)
	default:
		if r.N != 0 {
			fmt.Fprintf(&b, " %d", r.N)
		}
		if r.V != [3]float32{} {
			fmt.Fprintf(&b, " %s", vec(r.V))
		}
		if r.F != 0 {
			fmt.Fprintf(&b, " %.3f", r.F)
		}
	}
	if r.Why != "" {
		b.WriteString(" because ")
		b.WriteString(r.Why)
	}
	if r.Who != "" {
		b.WriteString(" [by ")
		b.WriteString(r.Who)
		b.WriteString("]")
	}
	return b.String()
}

func vec(v [3]float32) string { return fmt.Sprintf("(%.3f %.3f %.3f)", v[0], v[1], v[2]) }

var bitNames = [...]string{"wall", "floor/ceiling", "ground", "climbing", "climbed", "liquid"}

func physicsBits(n int64) string {
	var b strings.Builder
	for i, name := range bitNames {
		if n&(1<<i) != 0 {
			b.WriteString(" ")
			b.WriteString(name)
		}
	}
	return b.String()
}

// WriteTo writes every record kept, oldest first, one per line.
func (l *Log) WriteTo(w io.Writer) (int64, error) {
	var n int64
	var err error
	l.Each(func(r *Record) bool {
		var k int
		k, err = io.WriteString(w, r.String()+"\n")
		n += int64(k)
		return err == nil
	})
	return n, err
}
