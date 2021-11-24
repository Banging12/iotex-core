// Copyright (c) 2019 IoTeX Foundation
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package block

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/iotexproject/go-pkgs/hash"
	"github.com/iotexproject/iotex-core/action"
	"github.com/iotexproject/iotex-core/pkg/compress"
	"github.com/iotexproject/iotex-core/pkg/log"
	"github.com/iotexproject/iotex-core/pkg/unit"
	"github.com/iotexproject/iotex-core/pkg/version"
	"github.com/iotexproject/iotex-core/test/identityset"
	"github.com/iotexproject/iotex-core/testutil"
	"github.com/iotexproject/iotex-proto/golang/iotextypes"
)

func newTestLog() *action.Log {
	return &action.Log{
		Address:     "1",
		Data:        []byte("cd07d8a74179e032f030d9244"),
		BlockHeight: 1,
		ActionHash:  hash.ZeroHash256,
		Index:       1,
	}
}

func TestMerkle(t *testing.T) {
	require := require.New(t)

	producerAddr := identityset.Address(27).String()
	producerPubKey := identityset.PrivateKey(27).PublicKey()
	producerPriKey := identityset.PrivateKey(27)
	amount := uint64(50 << 22)
	// create testing transactions
	selp0, err := action.SignedTransfer(producerAddr, producerPriKey, 1, big.NewInt(int64(amount)), nil, 100, big.NewInt(0))
	require.NoError(err)

	selp1, err := action.SignedTransfer(identityset.Address(28).String(), producerPriKey, 1, big.NewInt(int64(amount)), nil, 100, big.NewInt(0))
	require.NoError(err)

	selp2, err := action.SignedTransfer(identityset.Address(29).String(), producerPriKey, 1, big.NewInt(int64(amount)), nil, 100, big.NewInt(0))
	require.NoError(err)

	selp3, err := action.SignedTransfer(identityset.Address(30).String(), producerPriKey, 1, big.NewInt(int64(amount)), nil, 100, big.NewInt(0))
	require.NoError(err)

	selp4, err := action.SignedTransfer(identityset.Address(32).String(), producerPriKey, 1, big.NewInt(int64(amount)), nil, 100, big.NewInt(0))
	require.NoError(err)

	// create block using above 5 tx and verify merkle
	actions := []action.SealedEnvelope{selp0, selp1, selp2, selp3, selp4}
	block := NewBlockDeprecated(
		0,
		0,
		hash.ZeroHash256,
		testutil.TimestampNow(),
		producerPubKey,
		actions,
	)
	hash, err := block.CalculateTxRoot()
	require.NoError(err)
	require.Equal("eb5cb75ae199d96de7c1cd726d5e1a3dff15022ed7bdc914a3d8b346f1ef89c9", hex.EncodeToString(hash[:]))

	hashes := block.ActionHashs()
	for i := range hashes {
		h, err := actions[i].Hash()
		require.NoError(err)
		require.Equal(hex.EncodeToString(h[:]), hashes[i])
	}

	// test index map
	txMap, logMap, err := block.TxLogIndexMap()
	require.NoError(err)
	require.Equal(5, len(txMap))
	for _, v := range logMap {
		require.Zero(v)
	}
	h0 := selp0.Hash()
	h1 := selp1.Hash()
	h2 := selp2.Hash()
	h3 := selp3.Hash()
	h4 := selp4.Hash()
	l0, l1, l2 := newTestLog(), newTestLog(), newTestLog()
	block.Receipts = []*action.Receipt{
		(&action.Receipt{ActionHash: h0}).AddLogs(l0, l1, l2),
		(&action.Receipt{ActionHash: h1}).AddLogs(l0, l1, l2),
		(&action.Receipt{ActionHash: h3}).AddLogs(l0, l1),
		(&action.Receipt{ActionHash: h0}).AddLogs(l0, l1),
		{ActionHash: h1},
	}
	txMap, logMap, err = block.TxLogIndexMap()
	require.NoError(err)
	require.Equal(5, len(txMap))
	require.Equal(5, len(logMap))

	for _, v := range []struct {
		h                 hash.Hash256
		txIndex, logIndex uint32
	}{
		{h0, 0, 0},
		{h1, 1, 5},
		{h2, 2, 8},
		{h3, 3, 8},
		{h4, 4, 10},
	} {
		require.Equal(v.txIndex, txMap[v.h])
		require.Equal(v.logIndex, logMap[v.h])
	}
}

func TestConvertFromBlockPb(t *testing.T) {
	blk := Block{}
	senderPubKey := identityset.PrivateKey(27).PublicKey()
	require.NoError(t, blk.ConvertFromBlockPb(&iotextypes.Block{
		Header: &iotextypes.BlockHeader{
			Core: &iotextypes.BlockHeaderCore{
				Version:   version.ProtocolVersion,
				Height:    123456789,
				Timestamp: ptypes.TimestampNow(),
			},
			ProducerPubkey: senderPubKey.Bytes(),
		},
		Body: &iotextypes.BlockBody{
			Actions: []*iotextypes.Action{
				{
					Core: &iotextypes.ActionCore{
						Action: &iotextypes.ActionCore_Transfer{
							Transfer: &iotextypes.Transfer{},
						},
						Version: version.ProtocolVersion,
						Nonce:   101,
					},
					SenderPubKey: senderPubKey.Bytes(),
					Signature:    action.ValidSig,
				},
				{
					Core: &iotextypes.ActionCore{
						Action: &iotextypes.ActionCore_Transfer{
							Transfer: &iotextypes.Transfer{},
						},
						Version: version.ProtocolVersion,
						Nonce:   102,
					},
					SenderPubKey: senderPubKey.Bytes(),
					Signature:    action.ValidSig,
				},
			},
		},
	}))

	txHash, err := blk.CalculateTxRoot()
	require.NoError(t, err)

	blk.Header.txRoot = txHash
	blk.Header.receiptRoot = hash.Hash256b(([]byte)("test"))

	raw, err := blk.Serialize()
	require.NoError(t, err)

	var newblk Block
	err = newblk.Deserialize(raw)
	require.NoError(t, err)
	require.Equal(t, blk, newblk)
}

func TestBlockCompressionSize(t *testing.T) {
	for _, n := range []int{1, 10, 100, 1000, 10000} {
		blk := makeBlock(t, n)
		blkBytes, err := blk.Serialize()
		require.NoError(t, err)
		compressedBlkBytes, err := compress.CompGzip(blkBytes)
		require.NoError(t, err)
		log.L().Info(
			"Compression result",
			zap.Int("numActions", n),
			zap.Int("before", len(blkBytes)),
			zap.Int("after", len(compressedBlkBytes)),
		)
	}
}

func BenchmarkBlockCompression(b *testing.B) {
	for _, i := range []int{1, 10, 100, 1000, 2000} {
		b.Run(fmt.Sprintf("numActions: %d", i), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				blk := makeBlock(b, i)
				blkBytes, err := blk.Serialize()
				require.NoError(b, err)
				b.StartTimer()
				_, err = compress.CompGzip(blkBytes)
				b.StopTimer()
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkBlockDecompression(b *testing.B) {
	for _, i := range []int{1, 10, 100, 1000, 2000} {
		b.Run(fmt.Sprintf("numActions: %d", i), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				blk := makeBlock(b, i)
				blkBytes, err := blk.Serialize()
				require.NoError(b, err)
				blkBytes, err = compress.CompGzip(blkBytes)
				require.NoError(b, err)
				b.StartTimer()
				_, err = compress.DecompGzip(blkBytes)
				b.StopTimer()
				require.NoError(b, err)
			}
		})
	}
}

func makeBlock(tb testing.TB, n int) *Block {
	rand.Seed(time.Now().Unix())
	sevlps := make([]action.SealedEnvelope, 0)
	for j := 1; j <= n; j++ {
		i := rand.Int()
		tsf, err := action.NewTransfer(
			uint64(i),
			unit.ConvertIotxToRau(1000+int64(i)),
			identityset.Address(i%identityset.Size()).String(),
			nil,
			20000+uint64(i),
			unit.ConvertIotxToRau(1+int64(i)),
		)
		require.NoError(tb, err)
		eb := action.EnvelopeBuilder{}
		evlp := eb.
			SetAction(tsf).
			SetGasLimit(tsf.GasLimit()).
			SetGasPrice(tsf.GasPrice()).
			SetNonce(tsf.Nonce()).
			SetVersion(1).
			Build()
		sevlp, err := action.Sign(evlp, identityset.PrivateKey((i+1)%identityset.Size()))
		require.NoError(tb, err)
		sevlps = append(sevlps, sevlp)
	}
	rap := RunnableActionsBuilder{}
	ra := rap.AddActions(sevlps...).
		Build()
	blk, err := NewBuilder(ra).
		SetHeight(1).
		SetTimestamp(time.Now()).
		SetVersion(1).
		SetReceiptRoot(hash.Hash256b([]byte("hello, world!"))).
		SetDeltaStateDigest(hash.Hash256b([]byte("world, hello!"))).
		SetPrevBlockHash(hash.Hash256b([]byte("hello, block!"))).
		SignAndBuild(identityset.PrivateKey(0))
	require.NoError(tb, err)
	return &blk
}
