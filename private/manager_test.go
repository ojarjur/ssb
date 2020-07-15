package private

import (
	"context"
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/keks/testops"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	libmkv "go.cryptoscope.co/librarian/mkv"
	"golang.org/x/crypto/nacl/box"
	"modernc.org/kv"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/extra25519"
	"go.cryptoscope.co/ssb/keys"
)

func TestManager(t *testing.T) {
	ks := &keys.Store{
		Index: newMemIndex(keys.Keys{}),
	}

	type testcase struct {
		name    string
		msg     []byte
		sender  *ssb.FeedRef
		rcpts   []ssb.Ref
		encOpts []EncryptOption
	}

	var (
		alice = newIdentity(t, "Alice", ks)
		bob   = newIdentity(t, "Bob", ks)
	)

	populateKeyStore(t, ks, alice, bob)

	type testStruct struct {
		Hello bool `json:"hello"`
	}

	var (
		ctxt []byte
		msg  interface{}

		// TODO: for JS compat all messages need to have a type:string field...
		msgs = []interface{}{
			"plainStringLikeABlob",
			[]int{1, 2, 3, 4, 5},
			map[string]interface{}{"some": 1, "msg": "here"},
			testStruct{true},
			testStruct{false},
			map[string]interface{}{"hello": false},
			map[string]interface{}{"hello": true},
			json.RawMessage("omg this isn't even valid json"),
		}
	)

	tcs2 := []testops.TestCase{
		testops.TestCase{
			Name: "alice->alice, string",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[0],
					Recipients: []ssb.Ref{alice.Id},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,

					Message: &msg,

					ExpMessage: msgs[0],
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice, struct",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[3],
					Recipients: []ssb.Ref{alice.Id},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,

					Message: &testStruct{},

					ExpMessage: msgs[3],
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice+bob, slice",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[1],
					Recipients: []ssb.Ref{alice.Id, bob.Id},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,

					Message: &[]int{},

					ExpMessage: msgs[1],
				},
				OpManagerDecrypt{
					Manager:    bob.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,

					Message: &[]int{},

					ExpMessage: msgs[1],
				},
			},
		},
		testops.TestCase{
			Name: "alice->alice+bob, slice, box2",
			Ops: []testops.Op{
				OpManagerEncrypt{
					Manager:    alice.manager,
					Message:    &msgs[1],
					Recipients: []ssb.Ref{alice.Id, bob.Id},
					Options:    []EncryptOption{WithBox2()},

					Ciphertext: &ctxt,
				},
				OpManagerDecrypt{
					Manager:    alice.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,
					Options:    []EncryptOption{WithBox2()},

					Message: &[]int{},

					ExpMessage: msgs[1],
				},
				OpManagerDecrypt{
					Manager:    bob.manager,
					Sender:     alice.Id,
					Ciphertext: &ctxt,
					Options:    []EncryptOption{WithBox2()},

					Message: &[]int{},

					ExpMessage: msgs[1],
				},
			},
		},
	}

	testops.Run(t, []testops.Env{testops.Env{
		Name: "private.Manager",
		Func: func(tc testops.TestCase) (func(*testing.T), error) {
			return tc.Runner(nil), nil
		},
	}}, tcs2)
}

func newMemIndex(tipe interface{}) librarian.SeqSetterIndex {
	db, err := kv.CreateMem(&kv.Options{})
	if err != nil {
		// this is for testing only and unlikely to fail
		panic(err)
	}

	return libmkv.NewIndex(db, tipe)
}

type testIdentity struct {
	*ssb.KeyPair

	name    string
	manager *Manager
}

var idCount int64

func newIdentity(t *testing.T, name string, km *keys.Store) testIdentity {
	var (
		id  = testIdentity{name: name}
		err error
	)

	rand := rand.New(rand.NewSource(idCount))

	id.KeyPair, err = ssb.NewKeyPair(rand)
	require.NoError(t, err)

	t.Logf("%s is %s", name, id.Id.Ref())

	id.manager = &Manager{
		author: id.Id,
		keymgr: km,
		rand:   rand,
	}

	idCount++

	return id
}

func populateKeyStore(t *testing.T, km *keys.Store, ids ...testIdentity) {
	type keySpec struct {
		Scheme keys.KeyScheme
		ID     keys.ID
		Key    keys.Key
	}

	// TODO make these type strings constants
	specs := make([]keySpec, 0, (len(ids)+2)*len(ids))

	var (
		cvSecs = make([][32]byte, len(ids))
		cvPubs = make([][32]byte, len(ids))
		shared = make([][][32]byte, len(ids))
	)

	for i := range ids {
		specs = append(specs, keySpec{
			keys.SchemeDiffieStyleConvertedED25519,
			keys.ID(ids[i].Id.ID),
			keys.Key(ids[i].Pair.Secret[:]),
		})

		extra25519.PrivateKeyToCurve25519(&cvSecs[i], ids[i].Pair.Secret)
		extra25519.PublicKeyToCurve25519(&cvPubs[i], ids[i].Pair.Public)

		specs = append(specs, keySpec{
			keys.SchemeDiffieStyleConvertedED25519,
			keys.ID(ids[i].Id.ID),
			keys.Key(cvSecs[i][:]),
		})
	}

	for i := range ids {
		shared[i] = make([][32]byte, len(ids))
		for j := range ids {
			var (
				shrd = &shared[i][j]
				pub  = &cvPubs[i]
				sec  = &cvSecs[j]
			)
			box.Precompute(shrd, pub, sec)

			specs = append(specs, keySpec{
				keys.SchemeDiffieStyleConvertedED25519,
				sortAndConcat(keys.ID(ids[i].Id.ID), keys.ID(ids[j].Id.ID)),
				keys.Key(shared[i][j][:]),
			})
		}

	}

	var err error

	ctx := context.TODO()

	for _, spec := range specs {
		t.Logf("adding key %s - %x", spec.Scheme, spec.ID)
		err = km.AddKey(ctx, spec.Scheme, spec.ID, spec.Key)
		require.NoError(t, err)
	}

}
