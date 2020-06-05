package box2

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/keys"
)

func TestSpec(t *testing.T) {
	dir, err := os.Open(filepath.Join("spec", "vectors"))
	require.NoError(t, err, "open vectors dir. get https://github.com/ssbc/private-group-spec and run 'ln -s ~/ssb/private-group-spec spec'")

	ls, err := dir.Readdir(0)
	require.NoError(t, err, "list vectors dir")

	for _, fi := range ls {
		if !strings.HasSuffix(fi.Name(), ".json") {
			continue
		}

		f, err := os.Open(filepath.Join("spec", "vectors", fi.Name()))
		require.NoError(t, err, "open vector json file")

		var spec genericSpecTest
		err = json.NewDecoder(f).Decode(&spec)
		require.NoError(t, err, "json parse error")

		t.Log(f.Name(), spec.Description)

		f.Seek(0, 0)

		switch spec.Type {
		case "box":
			var spec boxSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(spec.Description, spec.Test)
		case "unbox":
			var spec unboxSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(spec.Description, spec.Test)
		case "derive_secret":
			var spec deriveSecretSpecTest
			err = json.NewDecoder(f).Decode(&spec)
			require.NoError(t, err, "json parse error")
			t.Run(spec.Description, spec.Test)
		default:
			t.Logf("no test code for type %q, skipping: %s", spec.Type, spec.Description)
		}
	}
}

type specTestHeader struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type genericSpecTest struct {
	specTestHeader

	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	ErrorCode interface{}     `json:"error_code"`
}

type boxSpecTest struct {
	genericSpecTest

	Input  boxSpecTestInput  `json:"input"`
	Output boxSpecTestOutput `json:"output"`
}

type boxSpecTestInput struct {
	PlainText []byte          `json:"plain_text"`
	FeedID    *ssb.FeedRef    `json:"feed_id"`
	PrevMsgID *ssb.MessageRef `json:"prev_msg_id"`
	MsgKey    []byte          `json:"msg_key"`
	RecpKeys  [][]byte        `json:"recp_keys"`
}

type boxSpecTestOutput struct {
	Ciphertext []byte `json:"ciphertext"`
}

func (bt boxSpecTest) Test(t *testing.T) {
	//spew.Dump(bt)
	rand := bytes.NewBuffer([]byte(bt.Input.MsgKey))
	bxr := NewBoxer(rand)

	out, _ := bxr.Encrypt(
		nil,
		bt.Input.PlainText,
		bt.Input.FeedID,
		bt.Input.PrevMsgID,
		func() []keys.Key {
			out := make([]keys.Key, len(bt.Input.RecpKeys))
			for i := range out {
				out[i] = keys.Key(bt.Input.RecpKeys[i])
			}
			return out
		}(),
	)

	require.Equal(t, bt.Output.Ciphertext, out)
}

type unboxSpecTest struct {
	genericSpecTest

	Input  unboxSpecTestInput  `json:"input"`
	Output unboxSpecTestOutput `json:"output"`
}

type unboxSpecTestInput struct {
	Messages  []ssb.KeyValueRaw   `json:"msgs"`
	TrialKeys []unboxSpecTestKeys `json:"trial_keys"`
}

type unboxSpecTestKeys struct {
	Key    b64str `json:"key"` // todo:base64
	Scheme string `json:"string"`
}

type b64str keys.Key

func (s *b64str) UnmarshalJSON(data []byte) error {
	var strdata string
	err := json.Unmarshal(data, &strdata)
	if err != nil {
		return err
	}
	decoded := make([]byte, len(strdata)) // will be shorter
	n, err := base64.URLEncoding.Decode(decoded, []byte(strdata))
	if err != nil {
		spew.Dump(strdata)
		return fmt.Errorf("invalid base64 key: %w", err)
	}

	*s = decoded[:n]
	return nil
}

type unboxSpecTestOutput struct {
	MessageContents []struct {
		Type       string   `json:"type"`
		Text       string   `json:"text"`
		Recipients []string `json:"recps"`
	} `json:"msgsContent"`
}

func (ut unboxSpecTest) Test(t *testing.T) {
	spew.Dump(ut)

	bxr := NewBoxer(nil)

	for keyIdx, trialKey := range ut.Input.TrialKeys {

		for msgIdx, testMsg := range ut.Input.Messages {
			t.Log(testMsg.Value.Author.ShortRef())

			t.Log(testMsg.Value.Content)
			ctext, err := base64.RawStdEncoding.DecodeString(strings.TrimSuffix(string(testMsg.Value.Content), ".box2"))
			out, err := bxr.Decrypt(
				nil,
				ctext,
				&testMsg.Value.Author, //testMsg.Value.Author,
				testMsg.Value.Previous,
				keys.Keys{keys.Key(trialKey.Key)},
			)
			if assert.NoError(t, err, "failed to decrypt: %d %d", keyIdx, msgIdx) {
				continue
			}
			t.Log(string(out))
			//require.Equal(t, ut.Output.Plaintext, out)

		}

	}

}

type deriveSecretSpecTest struct {
	genericSpecTest

	Input  deriveSecretSpecTestInput  `json:"input"`
	Output deriveSecretSpecTestOutput `json:"output"`
}

type deriveSecretSpecTestInput struct {
	FeedID    *ssb.FeedRef    `json:"feed_id"`
	PrevMsgID *ssb.MessageRef `json:"prev_msg_id"`
	MsgKey    []byte          `json:"msg_key"`
}

type deriveSecretSpecTestOutput struct {
	ReadKey   []byte `json:"read_key"`
	HeaderKey []byte `json:"header_key"`
	BodyKey   []byte `json:"body_key"`
}

func (dt deriveSecretSpecTest) Test(t *testing.T) {
	info := makeInfo(dt.Input.FeedID, dt.Input.PrevMsgID)

	var readKey, headerKey, bodyKey [32]byte

	deriveTo(readKey[:], dt.Input.MsgKey, info([]byte("read_key"))...)
	deriveTo(headerKey[:], readKey[:], info([]byte("read_key"))...)
	deriveTo(bodyKey[:], readKey[:], info([]byte("read_key"))...)

	require.Equal(t, dt.Output.ReadKey, readKey)
	require.Equal(t, dt.Output.HeaderKey, headerKey)
	require.Equal(t, dt.Output.BodyKey, bodyKey)
}
