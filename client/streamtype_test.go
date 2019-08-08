package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
)

func TestReadStreamAsInterfaceMessage(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	// srvLog := log.NewJSONLogger(os.Stderr)
	srvLog, _ := logtest.KitLogger("srv", t)
	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"))
	r.NoError(err, "sbot srv init failed")

	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(context.TODO())
		if err != nil {
			srvErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(srvErrc)
	}()

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(context.TODO(), kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	// no messages yet
	seqv, err := srv.RootLog.Seq().Value()
	r.NoError(err, "failed to get root log sequence")
	r.Equal(margaret.SeqEmpty, seqv)

	type testMsg struct {
		Foo string
		Bar int
	}
	var refs []string
	for i := 0; i < 10; i++ {

		msg := testMsg{"hello", 23}
		ref, err := c.Publish(msg)
		r.NoError(err, "failed to call publish")
		r.NotNil(ref)

		// get stored message from the log
		seqv, err = srv.RootLog.Seq().Value()
		r.NoError(err, "failed to get root log sequence")
		wantSeq := margaret.BaseSeq(i)
		a.Equal(wantSeq, seqv)
		msgv, err := srv.RootLog.Get(wantSeq)
		r.NoError(err)
		newMsg, ok := msgv.(ssb.Message)
		r.True(ok)
		r.Equal(newMsg.Key(), ref)
		refs = append(refs, ref.Ref())

		src, err := c.CreateLogStream(message.CreateHistArgs{Limit: 1, Seq: int64(i)})
		r.NoError(err)

		streamV, err := src.Next(context.TODO())
		r.NoError(err, "failed to next msg:%d", i)
		streamMsg, ok := streamV.(ssb.Message)
		r.True(ok, "acutal type: %T", streamV)
		a.Equal(newMsg.Author().Ref(), streamMsg.Author().Ref())

		a.EqualValues(newMsg.Seq(), streamMsg.Seq())

		v, err := src.Next(context.TODO())
		a.Nil(v)
		a.Equal(luigi.EOS{}, errors.Cause(err))
	}

	src, err := c.CreateLogStream(message.CreateHistArgs{Limit: 10})
	r.NoError(err)

	for i := 0; i < 10; i++ {
		streamV, err := src.Next(context.TODO())
		r.NoError(err, "failed to next msg:%d", i)
		msg, ok := streamV.(ssb.Message)
		r.True(ok, "acutal type: %T", streamV)
		a.Equal(refs[i], msg.Key().Ref())
	}

	v, err := src.Next(context.TODO())
	a.Nil(v)
	a.Equal(luigi.EOS{}, errors.Cause(err))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}