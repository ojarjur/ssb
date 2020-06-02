package ssb

import (
	"math/rand"
	"sort"
	"testing"
)

func TestBranchCausalitySimple(t *testing.T) {
	var msgs = []fakeMessage{
		{key: "p1", text: "1fii", branches: nil},
		{key: "p2", text: "2faa", branches: []string{"b1"}},
		{key: "p3", text: "3foo", branches: []string{"b2"}},

		{key: "b1", text: "4fum", branches: []string{"p1"}},
		{key: "b2", text: "5fum", branches: []string{"p2", "s1"}},
		{key: "b3", text: "6fum", branches: []string{"p3", "s2"}},

		{key: "s1", text: "7fum", branches: []string{"p1"}},
		{key: "s2", text: "8fum", branches: []string{"b2"}},
		// {key: "s3", text: "9fum", branches: []string{"p3", "s2"}},
	}
	rand.Shuffle(len(msgs), func(i, j int) {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	})

	// stupid interface wrapping
	tp := make([]TangledPost, len(msgs))
	for i, m := range msgs {
		t.Log(i, string(m.Key().Hash))
		tp[i] = TangledPost(m)
	}

	sorter := ByBranches{Items: tp}
	sorter.FillLookup()
	sort.Sort(sorter)

	for i, m := range tp {
		t.Log(i, string(m.Key().Hash), m.(fakeMessage).text)
	}

}

type fakeMessage struct {
	key string

	root     string // same for all
	branches []string

	text string // like post.text
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
