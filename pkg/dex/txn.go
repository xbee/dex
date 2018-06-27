package dex

import (
	"bytes"
	"encoding/gob"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

const (
	OrderPriceDecimals = 8
)

type TxnType uint8

const (
	PlaceOrder TxnType = iota
	CancelOrder
	IssueToken
	SendToken
	FreezeToken
	BurnToken
)

type Txn struct {
	T          TxnType
	Data       []byte
	NonceIdx   uint8
	NonceValue uint64
	Owner      consensus.Addr
	Sig        Sig
}

func validateNonce(state *State, txn *consensus.Txn) (acc *Account, ready, valid bool) {
	acc = state.Account(txn.Owner)
	if acc == nil {
		log.Warn("txn owner not found")
		return
	}

	// TODO: validate nonce

	// if !txn.Sig.Verify(acc.PK, txn.Encode(false)) {
	// 	log.Warn("invalid txn signature")
	// 	return
	// }

	// if int(txn.NonceIdx) >= len(acc.NonceVec) {
	// 	if txn.NonceValue > 0 {
	// 		ready = false
	// 		valid = true
	// 		return
	// 	}

	// 	ready = true
	// 	valid = true
	// 	return
	// }

	// if acc.NonceVec[txn.NonceIdx] < txn.NonceValue {
	// 	ready = false
	// 	valid = true
	// 	return
	// } else if acc.NonceVec[txn.NonceIdx] > txn.NonceValue {
	// 	valid = false
	// 	return
	// }

	ready = true
	valid = true
	return
}

func (b *Txn) Encode(withSig bool) []byte {
	en := *b
	if !withSig {
		en.Sig = nil
	}

	d, err := rlp.EncodeToBytes(en)
	if err != nil {
		panic(err)
	}

	return d
}

func (b *Txn) Bytes() []byte {
	return b.Encode(true)
}

type PlaceOrderTxn struct {
	SellSide bool
	// quant step size is the decimals of the token, specific when
	// the token is issued, e.g., quant = Quant * 10^-(decimals)
	Quant uint64
	// price tick size is 10^-8, e.g,. price = Price * 10^-8
	Price uint64
	// the order is expired when ExpireRound >= block height
	ExpireRound uint64
	Market      MarketSymbol
}

type CancelOrderTxn struct {
	ID OrderID
}

func MakeCancelOrderTxn(sk SK, owner consensus.Addr, id OrderID, nonceIdx uint8, nonce uint64) []byte {
	t := CancelOrderTxn{
		ID: id,
	}

	txn := &Txn{
		T:          CancelOrder,
		Owner:      owner,
		NonceIdx:   nonceIdx,
		NonceValue: nonce,
		Data:       gobEncode(t),
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakeSendTokenTxn(from SK, owner consensus.Addr, to PK, tokenID TokenID, quant uint64, nonceIdx uint8, nonce uint64) []byte {
	send := SendTokenTxn{
		TokenID: tokenID,
		To:      to,
		Quant:   quant,
	}

	txn := &Txn{
		T:          SendToken,
		Owner:      owner,
		NonceIdx:   nonceIdx,
		NonceValue: nonce,
		Data:       gobEncode(send),
	}

	txn.Sig = from.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakePlaceOrderTxn(sk SK, owner consensus.Addr, t PlaceOrderTxn, nonceIdx uint8, nonceValue uint64) []byte {
	txn := &Txn{
		T:          PlaceOrder,
		Owner:      owner,
		NonceIdx:   nonceIdx,
		NonceValue: nonceValue,
		Data:       gobEncode(t),
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakeIssueTokenTxn(sk SK, owner consensus.Addr, info TokenInfo, nonceIdx uint8, nonceValue uint64) []byte {
	t := IssueTokenTxn{Info: info}
	txn := &Txn{
		T:          IssueToken,
		Data:       gobEncode(t),
		NonceIdx:   nonceIdx,
		NonceValue: nonceValue,
		Owner:      owner,
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakeFreezeTokenTxn(sk SK, owner consensus.Addr, t FreezeTokenTxn, nonceIdx uint8, nonceValue uint64) []byte {
	txn := &Txn{
		T:          FreezeToken,
		Data:       gobEncode(t),
		NonceIdx:   nonceIdx,
		NonceValue: nonceValue,
		Owner:      owner,
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

type IssueTokenTxn struct {
	Info TokenInfo
}

type SendTokenTxn struct {
	TokenID TokenID
	To      PK
	Quant   uint64
}

type FreezeTokenTxn struct {
	TokenID        TokenID
	AvailableRound uint64
	Quant          uint64
}

func gobEncode(v interface{}) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		// should not happen
		panic(err)
	}
	return buf.Bytes()
}
