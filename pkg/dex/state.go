package dex

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

// MarketSymbol is the symbol of a trading pair.
//
type MarketSymbol struct {
	Base  TokenID // the unit of the order's quantity
	Quote TokenID // the unit of the order's price
}

// Encode returns the bytes representation of the market symbol.
//
// The bytes is used as a prefix of a path of a patricia tree, It will
// be concatinated with the account addr path postfix to get the tree
// path. The path lead to the pending orders of an account in the
// market.
func (m *MarketSymbol) Encode() []byte {
	bufA := make([]byte, 64)
	bufB := make([]byte, 64)
	binary.LittleEndian.PutUint64(bufA, uint64(m.Quote))
	binary.LittleEndian.PutUint64(bufB, uint64(m.Base))
	return append(bufA, bufB...)
}

func (m *MarketSymbol) Decode(b []byte) error {
	if len(b) != 128 {
		return fmt.Errorf("bytes len not correct, expected 128, received %d", len(b))
	}

	m.Quote = TokenID(binary.LittleEndian.Uint64(b[:64]))
	m.Base = TokenID(binary.LittleEndian.Uint64(b[64:]))
	return nil
}

// State is the state of the DEX.
type State struct {
	db     *trie.Database
	diskDB ethdb.Database

	mu           sync.Mutex
	state        *trie.Trie
	accountCache map[consensus.Addr]*Account
	accountDirty map[consensus.Addr]bool
}

// TODO: add receipt for create, send, freeze, burn token.

var BNBInfo = TokenInfo{
	Symbol:     "BNB",
	Decimals:   8,
	TotalUnits: 200000000 * 100000000,
}

func CreateGenesisState(recipients []consensus.PK, additionalTokens []TokenInfo) *State {
	memDB := ethdb.NewMemDatabase()
	s := NewState(memDB)
	tokens := make([]Token, len(additionalTokens)+1)

	var tokenID TokenID
	tokens[0] = Token{ID: tokenID, TokenInfo: BNBInfo}
	tokenID++

	for i, t := range additionalTokens {
		token := Token{ID: tokenID, TokenInfo: t}
		tokenID++
		tokens[i+1] = token
	}

	for _, t := range tokens {
		s.UpdateToken(t)
	}

	for _, pk := range recipients {
		account := &Account{
			PK:       pk,
			Balances: make(map[TokenID]*Balance),
		}

		for _, t := range tokens {
			avg := t.TotalUnits / uint64(len(recipients))
			account.Balances[t.ID] = &Balance{Available: avg}
		}
		s.UpdateAccount(account)
	}

	return s
}

func newState(state *trie.Trie, db *trie.Database, diskDB ethdb.Database) *State {
	return &State{
		diskDB:       diskDB,
		db:           db,
		state:        state,
		accountCache: make(map[consensus.Addr]*Account),
		accountDirty: make(map[consensus.Addr]bool),
	}
}

func NewState(diskDB ethdb.Database) *State {
	db := trie.NewDatabase(diskDB)
	t, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	return newState(t, db, diskDB)
}

var (
	accountPrefix         = []byte{0}
	marketPrefix          = []byte{1}
	tokenPrefix           = []byte{2}
	orderExpirationPrefix = []byte{3}
	freezeAtRoundPrefix   = []byte{4}
)

func freezeAtRoundToPath(round uint64) []byte {
	b := make([]byte, 64)
	binary.LittleEndian.PutUint64(b, round)
	return append(freezeAtRoundPrefix, b...)
}

func accountAddrToPath(addr consensus.Addr) []byte {
	return append(accountPrefix, addr[:]...)
}

func expirationToPath(round uint64) []byte {
	b := make([]byte, 64)
	binary.LittleEndian.PutUint64(b, round)
	path := append(orderExpirationPrefix, b...)
	return path
}

func tokenPath(tokenID TokenID) []byte {
	path := make([]byte, 64)
	binary.LittleEndian.PutUint64(path, uint64(tokenID))
	return append(tokenPrefix, path...)
}

func marketPath(path []byte) []byte {
	return append(marketPrefix, path...)
}

func encodePath(str []byte) []byte {
	l := len(str) * 2
	var nibbles = make([]byte, l)
	for i, b := range str {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}
	return nibbles
}

func (s *State) CommitCache() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for addr, dirty := range s.accountDirty {
		if !dirty {
			continue
		}

		s.updateAccount(s.accountCache[addr])
	}
}

func (s *State) UpdateToken(token Token) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := tokenPath(token.ID)

	b, err := rlp.EncodeToBytes(token)
	if err != nil {
		// should never happen
		panic(err)
	}

	s.state.Update(path, b)
}

func (s *State) UpdateAccount(acc *Account) {
	addr := acc.PK.Addr()
	s.mu.Lock()
	s.accountDirty[addr] = true
	s.accountCache[addr] = acc
	s.mu.Unlock()
}

func (s *State) updateAccount(acc *Account) {
	addr := acc.PK.Addr()
	b, err := rlp.EncodeToBytes(acc)
	if err != nil {
		panic(err)
	}

	s.state.Update(accountAddrToPath(addr), b)
}

func (s *State) Account(addr consensus.Addr) *Account {
	s.mu.Lock()
	defer s.mu.Unlock()

	cache := s.accountCache[addr]
	if cache != nil {
		return cache
	}

	acc := s.state.Get(accountAddrToPath(addr))
	if acc == nil {
		return nil
	}

	var account Account
	err := rlp.DecodeBytes(acc, &account)
	if err != nil {
		log.Error("decode account error", "err", err)
		return nil
	}

	s.accountCache[addr] = &account
	return &account
}

// loadOrderBook deserializes the order from the state trie.
func (s *State) loadOrderBook(m MarketSymbol) *orderBook {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := marketPath(m.Encode())
	b := s.state.Get(path)
	if b == nil {
		return nil
	}

	var book orderBook
	err := rlp.DecodeBytes(b, &book)
	if err != nil {
		panic(err)
	}

	return &book
}

func (s *State) saveOrderBook(m MarketSymbol, book *orderBook) {
	b, err := rlp.EncodeToBytes(book)
	if err != nil {
		panic(err)
	}

	path := marketPath(m.Encode())
	s.state.Update(path, b)
}

// Tokens returns all issued tokens
func (s *State) Tokens() []Token {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := encodePath(tokenPrefix)
	iter := s.state.NodeIterator(prefix)

	var r []Token
	hasNext := true
	foundPrefix := false

	for ; hasNext; hasNext = iter.Next(true) {
		if err := iter.Error(); err != nil {
			log.Error("error iterating state trie's tokens", "err", err)
			break
		}

		if !iter.Leaf() {
			continue
		}

		path := iter.Path()
		if !bytes.HasPrefix(path, prefix) {
			if foundPrefix {
				break
			}

			continue
		}
		foundPrefix = true

		var token Token
		err := rlp.DecodeBytes(iter.LeafBlob(), &token)
		if err != nil {
			panic(err)
		}

		r = append(r, token)
	}
	return r
}

func (s *State) Serialize() (consensus.TrieBlob, error) {
	s.CommitCache()
	return serializeTrie(s.state, s.db, s.db.DiskDB())
}

func (s *State) Deserialize(b consensus.TrieBlob) error {
	err := b.Fill(s.diskDB)
	if err != nil {
		return err
	}

	db := trie.NewDatabase(s.diskDB)
	t, err := trie.New(common.Hash(b.Root), db)
	if err != nil {
		return err
	}

	s.state = t
	s.db = db
	return nil
}

// Hash returns the state root hash of the state trie.
func (s *State) Hash() consensus.Hash {
	s.mu.Lock()
	defer s.mu.Unlock()

	return consensus.Hash(s.state.Hash())
}

// Transition returns the state change transition.
func (s *State) Transition(round uint64) consensus.Transition {
	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.state.Commit(nil)
	if err != nil {
		panic(err)
	}

	trie, err := trie.New(root, s.db)
	if err != nil {
		panic(err)
	}

	state := newState(trie, s.db, s.diskDB)
	return newTransition(state, round)
}

type orderExpiration struct {
	ID    OrderID
	Owner consensus.Addr
}

func (s *State) GetOrderExpirations(round uint64) []orderExpiration {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getOrderExpirations(round)
}

func (s *State) getOrderExpirations(round uint64) []orderExpiration {
	var all []orderExpiration
	path := expirationToPath(round)
	exisiting := s.state.Get(path)
	if len(exisiting) > 0 {
		err := rlp.DecodeBytes(exisiting, &all)
		if err != nil {
			panic(err)
		}
	}
	return all
}

func (s *State) AddOrderExpirations(round uint64, ids []orderExpiration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := s.getOrderExpirations(round)
	all = append(all, ids...)
	b, err := rlp.EncodeToBytes(all)
	if err != nil {
		panic(err)
	}

	path := expirationToPath(round)
	s.state.Update(path, b)
}

func (s *State) RemoveOrderExpirations(round uint64, ids map[OrderID]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := s.getOrderExpirations(round)
	newExps := make([]orderExpiration, 0, len(all))
	for _, exp := range all {
		if !ids[exp.ID] {
			newExps = append(newExps, exp)
		}
	}

	b, err := rlp.EncodeToBytes(newExps)
	if err != nil {
		panic(err)
	}
	path := expirationToPath(round)
	s.state.Update(path, b)
}

type freezeToken struct {
	Addr    consensus.Addr
	TokenID TokenID
	Quant   uint64
}

func (s *State) GetFreezeTokens(round uint64) []freezeToken {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getFreezeTokens(round)
}

func (s *State) getFreezeTokens(round uint64) []freezeToken {
	path := freezeAtRoundToPath(round)
	b := s.state.Get(path)
	if len(b) == 0 {
		return nil
	}

	var all []freezeToken
	err := rlp.DecodeBytes(b, &all)
	if err != nil {
		panic(err)
	}

	return all
}

func (s *State) FreezeToken(round uint64, f freezeToken) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := s.getFreezeTokens(round)
	all = append(all, f)
	b, err := rlp.EncodeToBytes(all)
	if err != nil {
		panic(err)
	}

	path := freezeAtRoundToPath(round)
	s.state.Update(path, b)
}
