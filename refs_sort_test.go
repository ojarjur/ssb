package ssb

import (
	"math/rand"
	"sort"
	"testing"
)

func TestBranchCausalitySimple(t *testing.T) {
	var msgs = []fakeMessage{
		{key: "p1", order: 1, branches: nil},
		{key: "p2", order: 2, branches: []string{"b1"}},
		{key: "p3", order: 3, branches: []string{"b2"}},

		{key: "b1", order: 4, branches: []string{"p1"}},
		{key: "b2", order: 5, branches: []string{"p2", "s1"}},
		{key: "b3", order: 6, branches: []string{"p3", "s2"}},

		{key: "s1", order: 7, branches: []string{"p1"}},
		{key: "s2", order: 8, branches: []string{"b2"}},
		// {key: "s3", order: 9, branches: []string{"p3", "s2"}},
	}
	rand.Shuffle(len(msgs), func(i, j int) {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	})

	// stupid interface wrapping
	tp := make([]TangledPost, len(msgs))
	for i, m := range msgs {
		tp[i] = TangledPost(m)
	}

	sorter := ByBranches{Items: tp}
	sorter.FillLookup()
	sort.Sort(sorter)

	for i, m := range tp {
		if m.(fakeMessage).order != i+1 {
			t.Error(i, "not sorted")
		}
	}

}

type fakeMessage struct {
	key string

	root     string // same for all
	branches []string

	order int // test index
}

func (fm fakeMessage) Key() *MessageRef {
	return &MessageRef{
		Hash: []byte(fm.key),
		Algo: "fake",
	}
}

func (fm fakeMessage) Root() *MessageRef {
	return &MessageRef{
		Hash: []byte(fm.root),
		Algo: "fake",
	}
}

func (fm fakeMessage) Branches() []*MessageRef {
	n := len(fm.branches)
	if n == 0 {
		return nil
	}

	brs := make([]*MessageRef, n)
	for i, b := range fm.branches {
		brs[i] = &MessageRef{
			Hash: []byte(b),
			Algo: "fake",
		}
	}
	return brs
}
