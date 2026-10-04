package http

import (
	"context"
	"database/sql"
)

type readStateDTO struct {
	CustomerLastSeenSeq uint64 `json:"customer_last_seen_seq"`
	CustomerLastSeenAt  any    `json:"customer_last_seen_at"`
	AgentLastSeenSeq    uint64 `json:"agent_last_seen_seq"`
	AgentLastSeenAt     any    `json:"agent_last_seen_at"`
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Server) conversationReadState(ctx context.Context, q rowQuerier, conversationID uint64) (readStateDTO, error) {
	var state readStateDTO
	var customerSeenAt sql.NullTime
	var agentSeenAt sql.NullTime
	err := q.QueryRowContext(
		ctx,
		`SELECT customer_last_seen_seq, customer_last_seen_at, agent_last_seen_seq, agent_last_seen_at
		 FROM conversations
		 WHERE id = ?
		 LIMIT 1`,
		conversationID,
	).Scan(&state.CustomerLastSeenSeq, &customerSeenAt, &state.AgentLastSeenSeq, &agentSeenAt)
	if err != nil {
		return readStateDTO{}, err
	}
	state.CustomerLastSeenAt = nullableTimeValue(customerSeenAt)
	state.AgentLastSeenAt = nullableTimeValue(agentSeenAt)
	return state, nil
}
