// Copyright (C) 2019-2024, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"fmt"
	"strings"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

// Tracks the state of a snowman block
type snowmanBlock struct {
	// parameters to initialize the snowball instance with
	params snowball.Parameters

	// block that this node contains. For the genesis, this value will be nil
	blk Block

	// shouldFalter is set to true if this node, and all its descendants received
	// less than Alpha votes
	shouldFalter bool

	// sb is the snowball instance used to decide which child is the canonical
	// child of this block. If this node has not had a child issued under it,
	// this value will be nil
	sb snowball.Consensus

	// children is the set of blocks that have been issued that name this block
	// as their parent. If this node has not had a child issued under it, this value
	// will be nil
	children map[ids.ID]Block
}

func (n snowmanBlock) String() string {
	sb := strings.Builder{}
	fmt.Fprintf(&sb, "params: %v", n.params)
	if n.blk != nil {
		fmt.Fprintf(&sb, " blk: ID: %v Status: %v Parent: %v Height: %v Timestamp: %v", n.blk.ID(), n.blk.Status(), n.blk.Parent(), n.blk.Height(), n.blk.Timestamp())
	}
	if n.sb != nil {
		fmt.Fprintf(&sb, " sb: %v", n.sb)
	}
	if n.children != nil {
		fmt.Fprint(&sb, " children: {")
		kvs := make([]string, 0, len(n.children))
		for k, v := range n.children {
			kvs = append(kvs, fmt.Sprintf("%v: %v", k, v))
		}
		fmt.Fprint(&sb, strings.Join(kvs, ", "))
		fmt.Fprint(&sb, "}")
	}
	return sb.String()
}

func (n *snowmanBlock) AddChild(child Block) {
	childID := child.ID()

	// if the snowball instance is nil, this is the first child. So the instance
	// should be initialized.
	if n.sb == nil {
		n.sb = snowball.NewTree(snowball.SnowballFactory, n.params, childID)
		n.children = make(map[ids.ID]Block)
	} else {
		n.sb.Add(childID)
	}

	n.children[childID] = child
}

func (n *snowmanBlock) Accepted() bool {
	// if the block is nil, then this is the genesis which is defined as
	// accepted
	if n.blk == nil {
		return true
	}
	return n.blk.Status() == choices.Accepted
}
