package ssb

import (
	"math/rand"
	"sort"
	"testing"
)

func TestBranchCausalitySimple(t *testing.T) {

	var msgs = []fakeMessage{
		{key: "1fii", root: "baz"},
		{key: "2faa", root: "baz"},
		{key: "3foo", root: "baz"},
		{key: "4fum", root: "baz"},
		{key: "5fum", root: "baz"},
		{key: "6fum", root: "baz"},
		{key: "7fum", root: "baz"},
	}
	rand.Shuffle(len(msgs), func(i, j int) {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	})

	tp := make([]TangledPost, len(msgs))
	for i, m := range msgs {
		t.Log(i, string(m.Key().Hash))
		tp[i] = TangledPost(m)
	}

	sorter := ByBranches{Items: tp}
	sorter.FillLookup()
	sort.Sort(sorter)

	for i, m := range tp {
		t.Log(i, string(m.Key().Hash))
	}

}

type fakeMessage struct {
	key string

	root     string
	branches []string
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
