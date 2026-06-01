// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction framework.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package ops

import "sort"

// Member is a node in the cluster.
type Member struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
	Self bool   `json:"self"`
}

// Cluster reports cluster membership. A distributed implementation (gossip,
// raft, or one built on the space NATS adapter) is a drop-in; [StaticCluster] is
// the single-node / static-list default.
type Cluster interface {
	// Self returns this node.
	Self() Member
	// Members returns all known members, including self.
	Members() []Member
}

var _ Cluster = (*StaticCluster)(nil)

// StaticCluster is a fixed-membership Cluster: this node plus an optional static
// list of peers. It is the default when no dynamic membership backend is wired
// in (e.g. a single-node deployment).
type StaticCluster struct {
	self    Member
	members []Member
}

// NewStaticCluster builds a cluster of self plus peers. self.Self is forced true
// and peers' Self is forced false.
func NewStaticCluster(self Member, peers ...Member) *StaticCluster {
	self.Self = true
	members := []Member{self}
	for _, p := range peers {
		p.Self = false
		members = append(members, p)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return &StaticCluster{self: self, members: members}
}

// Self returns this node.
func (c *StaticCluster) Self() Member { return c.self }

// Members returns all members, sorted by ID.
func (c *StaticCluster) Members() []Member {
	return append([]Member(nil), c.members...)
}
