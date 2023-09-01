package node

import (
	"github.com/SipengXie/pangu/node"
	"github.com/SipengXie/pangu/node/nodecfg"
	"github.com/SipengXie/pangu/pg"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ledgerwatch/log/v3"
)

type PanguNode struct {
	stack   *node.Node
	backend *pg.PanGu
}

func (pgn *PanguNode) Serve() error {
	defer pgn.Close()
	pgn.run()
	pgn.stack.Wait()
	return nil
}

func (pgn *PanguNode) Close() {
	pgn.stack.Close()
}

func (pgn *PanguNode) run() {
	node.StartNode(pgn.stack)
}

func New(nodeConfig *nodecfg.Config, logger log.Logger) (*PanguNode, error) {
	node, err := node.New(nodeConfig, logger)
	if err != nil {
		utils.Fatalf("Failed to create Erigon node: %v", err)
	}

	// TODO : backend
	// pangu, err :=
	pangu := &pg.PanGu{}
	return &PanguNode{stack: node, backend: pangu}, nil
}
