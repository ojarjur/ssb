package box2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type deriveSecretSpecTest struct {
	genericSpecTest

	Input  deriveSecretSpecTestInput  `json:"input"`
	Output deriveSecretSpecTestOutput `json:"output"`
}

type deriveSecretSpecTestInput struct {
	FeedID    b64str `json:"feed_id"`
	PrevMsgID b64str `json:"prev_msg_id"`
	MsgKey    []byte `json:"msg_key"`
}

type deriveSecretSpecTestOutput struct {
	ReadKey   []byte `json:"read_key"`
	HeaderKey []byte `json:"header_key"`
	BodyKey   []byte `json:"body_key"`
}

func (dt deriveSecretSpecTest) Test(t *testing.T) {
	fref := feedRefFromTFK(dt.Input.FeedID)
	mref := messageRefFromTFK(dt.Input.PrevMsgID)

	info := makeInfo(fref, mref)

	var readKey, headerKey, bodyKey [32]byte

	deriveTo(readKey[:], dt.Input.MsgKey, info([]byte("read_key"))...)
	deriveTo(headerKey[:], readKey[:], info([]byte("header_key"))...)
	deriveTo(bodyKey[:], readKey[:], info([]byte("body_key"))...)

	require.EqualValues(t, dt.Output.ReadKey, readKey[:], "read key wrong")
	require.EqualValues(t, dt.Output.HeaderKey, headerKey[:], "header wrong")
	require.EqualValues(t, dt.Output.BodyKey, bodyKey[:], "body wrong")
}
