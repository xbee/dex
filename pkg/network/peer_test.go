package network

import (
	"bytes"
	"context"
	"encoding/gob"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/network/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSequentialEncDec(t *testing.T) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	var pac packet
	pac.T = txnArg
	pac.Data = []byte{3}
	err := enc.Encode(pac)
	if err != nil {
		panic(err)
	}

	var pac1 packet
	pac1.T = sysTxnArg
	pac1.Data = []byte{4}
	err = enc.Encode(pac1)
	if err != nil {
		panic(err)
	}

	dec := gob.NewDecoder(bytes.NewReader(buf.Bytes()))
	var c packet
	err = dec.Decode(&c)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, pac, c)

	var d packet
	err = dec.Decode(&d)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, pac1, d)
}

func TestPeer(t *testing.T) {
	connCh := make(chan net.Conn, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		l, err := net.Listen("tcp", ":8081")
		if err != nil {
			panic(err)
		}

		wg.Done()

		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		connCh <- conn
	}()

	wg.Wait()
	conn, err := net.Dial("tcp", "127.0.0.1:8081")
	if err != nil {
		panic(err)
	}

	dst := &mocks.Peer{}
	_ = NewPeer(<-connCh, dst)

	r0 := []string{"peer0", "peer1"}
	r1 := []*consensus.RandBeaconSig{&consensus.RandBeaconSig{Round: 1}}
	r2 := []*consensus.Block{&consensus.Block{Round: 1}}
	myself := &mocks.Peer{}
	p := NewPeer(conn, myself)
	dst.On("Txn", mock.Anything).Return(nil)
	dst.On("SysTxn", mock.Anything).Return(nil)
	dst.On("RandBeaconSigShare", mock.Anything).Return(nil)
	dst.On("RandBeaconSig", mock.Anything).Return(nil)
	dst.On("Block", mock.Anything).Return(nil)
	dst.On("BlockProposal", mock.Anything).Return(nil)
	dst.On("NotarizationShare", mock.Anything).Return(nil)
	dst.On("Inventory", mock.Anything, mock.Anything).Return(nil)
	dst.On("GetData", mock.Anything, mock.Anything).Return(nil)
	dst.On("Peers", mock.Anything).Return(r0, nil)
	dst.On("UpdatePeers", mock.Anything).Return(nil)
	dst.On("Ping", mock.Anything).Return(nil)
	dst.On("Sync", mock.Anything).Return(r1, r2, nil)

	for i := 0; i < 3; i++ {
		a0 := []byte{1}
		p.Txn(a0)
		a1 := &consensus.SysTxn{Data: []byte{2}}
		p.SysTxn(a1)
		a2 := &consensus.RandBeaconSigShare{Round: 1}
		p.RandBeaconSigShare(a2)
		a3 := &consensus.RandBeaconSig{Round: 1}
		p.RandBeaconSig(a3)
		a4 := &consensus.Block{Round: 1}
		p.Block(a4)
		a5 := &consensus.BlockProposal{Round: 1}
		p.BlockProposal(a5)
		a6 := &consensus.NtShare{Round: 1}
		p.NotarizationShare(a6)
		a70 := "r"
		a71 := []consensus.ItemID{consensus.ItemID{ItemRound: 1}}
		p.Inventory(a70, a71)
		a80 := "r1"
		a81 := []consensus.ItemID{consensus.ItemID{ItemRound: 2}}
		p.GetData(a80, a81)
		ret0, _ := p.Peers()
		a9 := []string{"p0"}
		p.UpdatePeers(a9)
		p.Ping(context.Background())
		a10 := 1
		ret1, ret2, _ := p.Sync(a10)

		time.Sleep(20 * time.Millisecond)
		dst.AssertCalled(t, "Txn", a0)
		dst.AssertCalled(t, "SysTxn", a1)
		dst.AssertCalled(t, "RandBeaconSigShare", a2)
		dst.AssertCalled(t, "RandBeaconSig", a3)
		dst.AssertCalled(t, "Block", a4)
		dst.AssertCalled(t, "BlockProposal", a5)
		dst.AssertCalled(t, "NotarizationShare", a6)
		dst.AssertCalled(t, "Inventory", a70, a71)
		dst.AssertCalled(t, "GetData", a80, a81)
		dst.AssertCalled(t, "Peers")
		dst.AssertCalled(t, "UpdatePeers", a9)
		dst.AssertCalled(t, "Ping", mock.Anything)
		dst.AssertCalled(t, "Sync", a10)

		assert.Equal(t, r0, ret0)
		assert.Equal(t, r1, ret1)
		assert.Equal(t, r2, ret2)
	}
}
