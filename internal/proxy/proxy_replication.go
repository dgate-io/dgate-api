package proxy

import (
	"github.com/dgate-io/dgate-api/pkg/raftadmin"
	"github.com/hashicorp/raft"
)

type ProxyReplication struct {
	raft   *raft.Raft
	client *raftadmin.Client
}

func NewProxyReplication(raft *raft.Raft, client *raftadmin.Client) *ProxyReplication {
	return &ProxyReplication{
		raft:   raft,
		client: client,
	}
}
