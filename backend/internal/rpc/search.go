package rpc

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	v1 "github.com/franco-roura/ledger-01/backend/gen/ledger/v1"
	"github.com/franco-roura/ledger-01/backend/internal/ledger"
)

func searchFromProto(m *v1.SearchTransactionsRequest) (ledger.Search, error) {
	var out ledger.Search
	if m.Period == nil {
		return out, &ledger.InvalidError{Field: "period", Msg: "is required"}
	}
	var err error
	if out.From, err = parseDate("period.from", m.Period.From); err != nil {
		return out, err
	}
	if out.To, err = parseDate("period.to", m.Period.To); err != nil {
		return out, err
	}
	if out.Today, err = parseToday(m.Today); err != nil {
		return out, err
	}
	if out.WalletIDs, err = parseIDs("wallet_ids", m.WalletIds); err != nil {
		return out, err
	}
	if out.CategoryIDs, err = parseIDs("category_ids", m.CategoryIds); err != nil {
		return out, err
	}
	if out.LabelIDs, err = parseIDs("label_ids", m.LabelIds); err != nil {
		return out, err
	}
	for _, kind := range m.Kinds {
		switch kind {
		case v1.TransactionKind_TRANSACTION_KIND_EXPENSE:
			out.Kinds = append(out.Kinds, ledger.KindExpense)
		case v1.TransactionKind_TRANSACTION_KIND_INCOME:
			out.Kinds = append(out.Kinds, ledger.KindIncome)
		case v1.TransactionKind_TRANSACTION_KIND_TRANSFER_OUT:
			out.Kinds = append(out.Kinds, ledger.KindTransferOut)
		case v1.TransactionKind_TRANSACTION_KIND_TRANSFER_IN:
			out.Kinds = append(out.Kinds, ledger.KindTransferIn)
		default:
			return out, &ledger.InvalidError{Field: "kinds", Msg: "contains an unknown transaction kind"}
		}
	}
	out.NoteQuery = strings.TrimSpace(m.NoteQuery)
	return out, nil
}

func (s *Server) SearchTransactions(ctx context.Context, req *connect.Request[v1.SearchTransactionsRequest]) (*connect.Response[v1.SearchTransactionsResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	search, err := searchFromProto(req.Msg)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	rows, next, err := s.svc.SearchTransactions(ctx, a, search, int(req.Msg.PageSize), req.Msg.PageToken)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	txs := make([]*v1.Transaction, len(rows))
	for i, row := range rows {
		txs[i] = transactionToProto(row)
	}
	return connect.NewResponse(&v1.SearchTransactionsResponse{Transactions: txs, NextPageToken: next}), nil
}
