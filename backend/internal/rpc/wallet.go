package rpc

import (
	"context"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/franco-roura/ledger-01/backend/gen/ledger/v1"
	"github.com/franco-roura/ledger-01/backend/internal/ledger"
	"github.com/franco-roura/ledger-01/backend/internal/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func walletToProto(w ledger.Wallet) *v1.Wallet {
	return &v1.Wallet{
		Id:                  w.ID.String(),
		Name:                w.Name,
		Currency:            w.Currency,
		InitialBalanceMinor: w.InitialBalance,
		BalanceMinor:        w.Balance,
		Hidden:              w.Hidden,
		Position:            w.Position,
		Archived:            w.Archived,
		TransactionCount:    w.TransactionCount,
		CreatedAt:           timestamppb.New(w.CreatedAt),
		Type:                walletTypeToProto(w.Type),
		ValueMinor:          w.Value,
		ValuedOn:            optDateString(w.ValuedOn),
	}
}

var walletTypes = map[v1.WalletType]ledger.WalletType{
	v1.WalletType_WALLET_TYPE_CASH:        ledger.WalletCash,
	v1.WalletType_WALLET_TYPE_BANK:        ledger.WalletBank,
	v1.WalletType_WALLET_TYPE_INVESTMENT:  ledger.WalletInvestment,
	v1.WalletType_WALLET_TYPE_PROPERTY:    ledger.WalletProperty,
	v1.WalletType_WALLET_TYPE_CREDIT_CARD: ledger.WalletCreditCard,
	v1.WalletType_WALLET_TYPE_LOAN:        ledger.WalletLoan,
}

func walletTypeToProto(t ledger.WalletType) v1.WalletType {
	for p, l := range walletTypes {
		if l == t {
			return p
		}
	}
	return v1.WalletType_WALLET_TYPE_BANK
}

// walletTypeFromProto maps unspecified to "" (the default, bank) and rejects unknown values.
func walletTypeFromProto(field string, t v1.WalletType) (ledger.WalletType, error) {
	if t == v1.WalletType_WALLET_TYPE_UNSPECIFIED {
		return "", nil
	}
	l, ok := walletTypes[t]
	if !ok {
		return "", &ledger.InvalidError{Field: field, Msg: "is not a known wallet type"}
	}
	return l, nil
}

func optDateString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return money.FormatDate(*t)
}

func valuationToProto(v ledger.Valuation) *v1.WalletValuation {
	return &v1.WalletValuation{
		Id: v.ID.String(), WalletId: v.WalletID.String(), ValuedOn: money.FormatDate(v.ValuedOn), ValueMinor: v.Value,
	}
}

func (s *Server) ListWallets(ctx context.Context, req *connect.Request[v1.ListWalletsRequest]) (*connect.Response[v1.ListWalletsResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	today, err := parseToday(req.Msg.Today)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	ws, err := s.svc.ListWallets(ctx, a, req.Msg.IncludeHidden, req.Msg.IncludeArchived, today)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	out := make([]*v1.Wallet, len(ws))
	for i, w := range ws {
		out[i] = walletToProto(w)
	}
	return connect.NewResponse(&v1.ListWalletsResponse{Wallets: out}), nil
}

func (s *Server) GetWallet(ctx context.Context, req *connect.Request[v1.GetWalletRequest]) (*connect.Response[v1.GetWalletResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	today, err := parseToday(req.Msg.Today)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	w, err := s.svc.GetWalletOn(ctx, a, id, today)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return connect.NewResponse(&v1.GetWalletResponse{Wallet: walletToProto(w)}), nil
}

func (s *Server) CreateWallet(ctx context.Context, req *connect.Request[v1.CreateWalletRequest]) (*connect.Response[v1.CreateWalletResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	typ, err := walletTypeFromProto("type", req.Msg.Type)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	w, err := s.svc.CreateWallet(ctx, a, req.Msg.Name, req.Msg.Currency, typ, req.Msg.InitialBalanceMinor)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return connect.NewResponse(&v1.CreateWalletResponse{Wallet: walletToProto(w)}), nil
}

func (s *Server) UpdateWallet(ctx context.Context, req *connect.Request[v1.UpdateWalletRequest]) (*connect.Response[v1.UpdateWalletResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	p := ledger.WalletPatch{
		Name: req.Msg.Name, Hidden: req.Msg.Hidden, Position: req.Msg.Position, InitialBalance: req.Msg.InitialBalanceMinor,
	}
	if req.Msg.Type != nil {
		typ, err := walletTypeFromProto("type", *req.Msg.Type)
		if err != nil {
			return nil, s.fail(ctx, err)
		}
		if typ == "" {
			typ = ledger.WalletBank
		}
		p.Type = &typ
	}
	w, err := s.svc.UpdateWallet(ctx, a, id, p)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return connect.NewResponse(&v1.UpdateWalletResponse{Wallet: walletToProto(w)}), nil
}

func (s *Server) ArchiveWallet(ctx context.Context, req *connect.Request[v1.ArchiveWalletRequest]) (*connect.Response[v1.ArchiveWalletResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	if err := s.svc.ArchiveWallet(ctx, a, id); err != nil {
		return nil, s.fail(ctx, err)
	}
	return connect.NewResponse(&v1.ArchiveWalletResponse{}), nil
}

func (s *Server) DeleteWallet(ctx context.Context, req *connect.Request[v1.DeleteWalletRequest]) (*connect.Response[v1.DeleteWalletResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	if err := s.svc.DeleteWallet(ctx, a, id); err != nil {
		return nil, s.fail(ctx, err)
	}
	return connect.NewResponse(&v1.DeleteWalletResponse{}), nil
}

func (s *Server) ListValuations(ctx context.Context, req *connect.Request[v1.ListValuationsRequest]) (*connect.Response[v1.ListValuationsResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID("wallet_id", req.Msg.WalletId)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	vs, err := s.svc.ListValuations(ctx, a, id)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	out := make([]*v1.WalletValuation, len(vs))
	for i, v := range vs {
		out[i] = valuationToProto(v)
	}
	return connect.NewResponse(&v1.ListValuationsResponse{Valuations: out}), nil
}

func (s *Server) SaveValuation(ctx context.Context, req *connect.Request[v1.SaveValuationRequest]) (*connect.Response[v1.SaveValuationResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID("wallet_id", req.Msg.WalletId)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	on, err := parseDate("valued_on", req.Msg.ValuedOn)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	v, err := s.svc.SaveValuation(ctx, a, id, on, req.Msg.ValueMinor)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return connect.NewResponse(&v1.SaveValuationResponse{Valuation: valuationToProto(v)}), nil
}

func (s *Server) DeleteValuation(ctx context.Context, req *connect.Request[v1.DeleteValuationRequest]) (*connect.Response[v1.DeleteValuationResponse], error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	if err := s.svc.DeleteValuation(ctx, a, id); err != nil {
		return nil, s.fail(ctx, err)
	}
	return connect.NewResponse(&v1.DeleteValuationResponse{}), nil
}
