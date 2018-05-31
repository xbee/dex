package dex

import (
	"bytes"
	"encoding/gob"

	"github.com/helinwang/dex/pkg/matching"
	log "github.com/helinwang/log15"
)

type Transition struct {
	state
	owner *State
	txns  [][]byte
}

func newTransition(s *State, state state) *Transition {
	return &Transition{
		state: state,
		owner: s,
	}
}

// Record records a transition to the state transition.
func (t *Transition) Record(b []byte) (valid, success bool) {
	txn, acc, ready, valid := validateSigAndNonce(&t.state, b)
	if !valid {
		return
	}

	if !ready {
		return true, false
	}

	dec := gob.NewDecoder(bytes.NewBuffer(txn.Data))
	switch txn.T {
	case PlaceOrder:
		var txn PlaceOrderTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("PlaceOrderTxn decode failed", "err", err)
			return
		}
		if !t.placeOrder(acc, txn) {
			log.Warn("PlaceOrderTxn failed")
			return
		}
	case CancelOrder:
		panic("not implemented")
	case CreateToken:
		panic("not implemented")
	case SendToken:
		var txn SendTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("SendTokenTxn decode failed", "err", err)
			return
		}
		if !t.sendToken(acc, txn) {
			log.Warn("SendTokenTxn failed")
			return
		}
	default:
		panic("unknown txn type")
	}

	t.txns = append(t.txns, b)
	return true, true
}

func (t *Transition) placeOrder(owner *Account, txn PlaceOrderTxn) bool {
	// TODO: check if fee is sufficient

	baseInfo := t.tokenCache.Info(txn.Market.Base)
	if baseInfo == nil {
		log.Error("trying to place order on nonexistent token", "token", txn.Market.Base)
		return false
	}

	quoteInfo := t.tokenCache.Info(txn.Market.Quote)
	if quoteInfo == nil {
		log.Error("trying to place order on nonexistent token", "token", txn.Market.Quote)
		return false
	}

	var sellQuant uint64
	var sell TokenID
	if txn.SellSide {
		sellQuant = txn.Quant
		sell = txn.Market.Base
	} else {
		sellQuant = uint64(float64(txn.Quant) * txn.Price)
		sell = txn.Market.Quote
	}

	sb, ok := owner.Balances[sell]
	if !ok {
		log.Warn("does not have balance for the given token", "token", sell)
		return false
	}

	if sb.Available <= sellQuant {
		log.Warn("insufficient quant to sell", "token", sell, "quant", sellQuant)
		return false
	}

	owner.Balances[sell].Available -= sellQuant
	owner.Balances[sell].Pending += sellQuant
	t.UpdateAccount(owner)
	add := PendingOrder{
		Owner: owner.PK.Addr(),
		Order: matching.Order{},
	}
	t.UpdatePendingOrder(txn.Market, &add, nil)
	return true
}

func (t *Transition) sendToken(owner *Account, txn SendTokenTxn) bool {
	if txn.Quant == 0 {
		return false
	}

	b, ok := owner.Balances[txn.TokenID]
	if !ok {
		log.Warn("trying to send token that the owner does not have", "tokenID", txn.TokenID)
		return false
	}

	if b.Available < txn.Quant {
		log.Warn("in sufficient available token balance", "tokenID", txn.TokenID, "quant", txn.Quant, "available", b.Available)
		return false
	}

	toAddr := txn.To.Addr()
	to, err := t.accounts.TryGet(toAddr[:])
	var toAcc *Account
	if err != nil || to == nil {
		toAcc = &Account{PK: txn.To, Balances: make(map[TokenID]*Balance)}
	} else {
		dec := gob.NewDecoder(bytes.NewBuffer(to))
		err := dec.Decode(toAcc)
		if err != nil {
			log.Error("error decode recv account", "account", toAddr)
			return false
		}
	}

	owner.Balances[txn.TokenID].Available -= txn.Quant
	if toAcc.Balances[txn.TokenID] == nil {
		toAcc.Balances[txn.TokenID] = &Balance{}
	}
	toAcc.Balances[txn.TokenID].Available += txn.Quant
	t.UpdateAccount(toAcc)
	t.UpdateAccount(owner)
	return true
}

func (t *Transition) Txns() [][]byte {
	return t.txns
}

// Commit commits the transition to the state root.
func (t *Transition) Commit() {
	t.owner.Commit(t)
}