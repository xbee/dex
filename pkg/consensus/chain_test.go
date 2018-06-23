package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type myUpdater struct {
}

func (m *myUpdater) Update(State) {
}

func TestGraphviz(t *testing.T) {
	chain := NewChain(&Block{}, nil, Rand{}, Config{}, nil, &myUpdater{})
	chain.finalized = append(chain.finalized, Hash{1})
	chain.finalized = append(chain.finalized, Hash{2})
	chain.finalized = append(chain.finalized, Hash{3})
	chain.finalized = append(chain.finalized, Hash{4})
	chain.bpNotOnFork = append(chain.bpNotOnFork, &bpNode{BP: Hash{5}})
	chain.bpNotOnFork = append(chain.bpNotOnFork, &bpNode{BP: Hash{6}})
	fork0 := &blockNode{Block: Hash{7}}
	fork01 := &blockNode{Block: Hash{8}}
	fork02 := &blockNode{Block: Hash{9}}
	fork02.bpChildren = []*bpNode{&bpNode{BP: Hash{10}}, &bpNode{BP: Hash{11}}}
	fork0.blockChildren = []*blockNode{fork01, fork02}
	fork1 := &blockNode{Block: Hash{12}}
	fork11 := &blockNode{Block: Hash{13}}
	fork1.blockChildren = []*blockNode{fork11}

	chain.fork = append(chain.fork, fork0)
	chain.fork = append(chain.fork, fork1)
	assert.Equal(t, `digraph chain {
rankdir=LR;
size="12,8"
node [shape = rect, style=filled, color = chartreuse2]; block_3ba7 block_0100 block_0200 block_0300 block_0400
node [shape = rect, style=filled, color = aquamarine]; block_0700 block_0800 block_0900 block_0c00 block_0d00
node [shape = octagon, style=filled, color = aliceblue]; proposal_0500 proposal_0600 proposal_0a00 proposal_0b00
block_3ba7 -> block_0100 -> block_0200 -> block_0300 -> block_0400
block_0400 -> proposal_0500
block_0400 -> proposal_0600
block_0400 -> block_0700
block_0700 -> block_0800
block_0700 -> block_0900
block_0900 -> proposal_0a00
block_0900 -> proposal_0b00
block_0400 -> block_0c00
block_0c00 -> block_0d00

}
`, chain.Graphviz(0))
}
