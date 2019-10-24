// SPDX-License-Identifier: MIT

package graph

import (
	"context"
	"encoding/json"
	"math"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/traverse"

	"go.cryptoscope.co/ssb"
)

type logBuilder struct {
	//  KILL ME
	//  KILL ME
	// this is just a left-over from the badger-based builder
	// it's only here to fulfil the Builder interface
	// badger _should_ split it's indexing out of it and then we can remove this here as well
	librarian.SinkIndex
	//  KILL ME
	// dont! call these methods
	//  KILL ME
	//  KILL ME

	logger kitlog.Logger

	current *Graph
}

// NewLogBuilder is a much nicer abstraction than the direct k:v implementation.
// most likely terribly slow though. Additionally, we have to unmarshal from stored.Raw again...
// TODO: actually compare the two with benchmarks if only to compare the 3rd!
func NewLogBuilder(logger kitlog.Logger, contacts margaret.Log) (Builder, error) {
	lb := logBuilder{
		logger: logger,
		current: &Graph{
			WeightedDirectedGraph: simple.NewWeightedDirectedGraph(0, math.Inf(1)),
			lookup:                make(key2node),
		},
	}

	go func() { // TODO: add io.Closer to Builder to kill this query
		src, err := contacts.Query(margaret.Live(true))
		if err != nil {
			err = errors.Wrap(err, "failed to make live query for contacts")
			level.Error(logger).Log("err", err, "event", "query build failed")
			return
		}
		err = luigi.Pump(context.TODO(), luigi.FuncSink(lb.buildGraph), src)
		if err != nil {
			level.Error(logger).Log("err", err, "event", "graph build failed")
		}
	}()

	return &lb, nil
}

func (b *logBuilder) Authorizer(from *ssb.FeedRef, maxHops int) ssb.Authorizer {
	return &authorizer{
		b:       b,
		from:    from,
		maxHops: maxHops,
		log:     b.logger,
	}
}

func (b *logBuilder) Build() (*Graph, error) {
	b.current.Lock()
	defer b.current.Unlock()

	if b.current == nil {
		return nil, errors.Errorf("TODO:wait?!")
	}

	return b.current, nil
}

func (b *logBuilder) buildGraph(ctx context.Context, v interface{}, err error) error {
	if err != nil {
		if luigi.IsEOS(err) {
			return nil
		}
		return err
	}

	b.current.Lock()
	defer b.current.Unlock()
	dg := b.current.WeightedDirectedGraph

	abs, ok := v.(ssb.Message)
	if !ok {
		err := errors.Errorf("graph/idx: invalid msg value %T", v)
		return err
	}
	// fmt.Println("processing", abs.Key())
	var c ssb.Contact
	err = json.Unmarshal(abs.ContentBytes(), &c)
	if err != nil {
		err = errors.Wrapf(err, "db/idx contacts: first json unmarshal failed (msg: %s)", abs.Key().Ref())
		return nil
	}

	author := abs.Author()
	contact := c.Contact

	if author.Equal(contact) {
		// contact self?!
		return nil
	}

	bfrom := author.StoredAddr()
	nFrom, has := b.current.lookup[bfrom]
	if !has {

		sr, err := ssb.NewStorageRef(author)
		if err != nil {
			return errors.Wrap(err, "failed to create graph node for author")
		}
		nFrom = &contactNode{dg.NewNode(), sr, ""}
		dg.AddNode(nFrom)
		b.current.lookup[bfrom] = nFrom
	}

	bto := contact.StoredAddr()
	nTo, has := b.current.lookup[bto]
	if !has {
		sr, err := ssb.NewStorageRef(contact)
		if err != nil {
			return errors.Wrap(err, "failed to create graph node for contact")
		}
		nTo = &contactNode{dg.NewNode(), sr, ""}
		dg.AddNode(nTo)
		b.current.lookup[bto] = nTo
	}

	w := math.Inf(-1)
	if c.Following {
		w = 1
	} else if c.Blocking {
		w = math.Inf(1)
	} else {
		if dg.HasEdgeFromTo(nFrom.ID(), nTo.ID()) {
			dg.RemoveEdge(nFrom.ID(), nTo.ID())
		}
		return nil
	}

	edg := simple.WeightedEdge{F: nFrom, T: nTo, W: w}
	dg.SetWeightedEdge(contactEdge{
		WeightedEdge: edg,
		isBlock:      c.Blocking,
	})

	return nil
}

func (b *logBuilder) Follows(from *ssb.FeedRef) (*StrFeedSet, error) {
	g, err := b.Build()
	if err != nil {
		return nil, errors.Wrap(err, "follows: couldn't build graph")
	}
	fb := from.StoredAddr()
	nFrom, has := g.lookup[fb]
	if !has {
		return nil, ErrNoSuchFrom{from}
	}

	nodes := g.From(nFrom.ID())

	refs := NewFeedSet(nodes.Len())

	for nodes.Next() {
		cnv := nodes.Node().(*contactNode)
		// warning - ignores edge type!
		edg := g.Edge(nFrom.ID(), cnv.ID())
		if edg.(contactEdge).Weight() == 1 {
			if err := refs.AddStored(cnv.feed); err != nil {
				return nil, err
			}
		}
	}
	return refs, nil
}

func (b *logBuilder) Hops(from *ssb.FeedRef, max int) *StrFeedSet {
	g, err := b.Build()
	if err != nil {
		panic(err)
	}
	b.current.Lock()
	defer b.current.Unlock()
	fb := from.StoredAddr()
	nFrom, has := g.lookup[fb]
	if !has {
		fs := NewFeedSet(1)
		fs.AddRef(from)
		return fs
	}
	// fmt.Println(from.Ref(), max)
	w := traverse.BreadthFirst{
		// only traverse friend edges
		Traverse: func(e graph.Edge) bool {
			ce := e.(contactEdge)
			rev := g.Edge(ce.To().ID(), ce.From().ID())
			if rev == nil {
				return true
			}
			return ce.Weight() == 1 && rev.(contactEdge).Weight() == 1
		},
	}
	fs := NewFeedSet(10)
	w.Walk(g, nFrom, func(n graph.Node, d int) bool {
		if d > max+1 {
			return true
		}

		// if d >= len(got) {
		// 	got = append(got, []int64(nil))
		// }
		// got[d] = append(got[d], n.ID())

		cn := n.(*contactNode)
		fs.AddStored(cn.feed)
		return false
	})

	// goon.Dump(got)
	// goon.Dump(final)
	return fs
}
