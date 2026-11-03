package ledger

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/franco-roura/ledger-01/backend/internal/db/dbq"
)

// RatePoint is one cross-currency transfer seen as an exchange rate: units of To per 1 From.
type RatePoint struct {
	On   time.Time
	From string
	To   string
	Rate *big.Rat
}

type pairKey struct{ a, b string } // a < b

type pairPoint struct {
	on   time.Time
	rate *big.Rat // units of b per 1 a
}

// Converter turns amounts between currencies using the household's own transfer rates
// (ADR-0013): the latest transfer between the pair on or before the date, else the earliest
// after it. There is no chaining through a third currency.
type Converter struct {
	pairs      map[pairKey][]pairPoint
	minorUnits map[string]int
}

// NewConverter indexes rate points (any order) and each currency's decimal places.
func NewConverter(points []RatePoint, minorUnits map[string]int) *Converter {
	c := &Converter{pairs: map[pairKey][]pairPoint{}, minorUnits: minorUnits}
	for _, p := range points {
		if p.From == p.To || p.Rate == nil || p.Rate.Sign() <= 0 {
			continue
		}
		k, r := pairKey{p.From, p.To}, p.Rate
		if p.To < p.From {
			k, r = pairKey{p.To, p.From}, new(big.Rat).Inv(p.Rate)
		}
		c.pairs[k] = append(c.pairs[k], pairPoint{on: p.On, rate: r})
	}
	for k := range c.pairs {
		pts := c.pairs[k]
		sort.SliceStable(pts, func(i, j int) bool { return pts[i].on.Before(pts[j].on) })
	}
	return c
}

// converter builds a Converter from the household's cross-currency transfers up to today
// (upcoming transfers are plans, not rates).
func (s *Service) converter(ctx context.Context, a Actor, today time.Time) (*Converter, error) {
	rateRows, err := s.q.TransferRates(ctx, dbq.TransferRatesParams{HouseholdID: a.HouseholdID, Today: today})
	if err != nil {
		return nil, err
	}
	points := make([]RatePoint, 0, len(rateRows))
	for _, rr := range rateRows {
		rate, ok := new(big.Rat).SetString(rr.Rate)
		if !ok {
			continue
		}
		points = append(points, RatePoint{On: rr.OccurredOn, From: strings.TrimSpace(rr.FromCurrency), To: strings.TrimSpace(rr.ToCurrency), Rate: rate})
	}
	currencies, err := s.Currencies(ctx)
	if err != nil {
		return nil, err
	}
	units := make(map[string]int, len(currencies))
	for _, c := range currencies {
		units[c.Code] = c.MinorUnits
	}
	return NewConverter(points, units), nil
}

// Rate returns units of `to` per 1 `from` as of the date, and whether one is known.
func (c *Converter) Rate(from, to string, on time.Time) (*big.Rat, bool) {
	if from == to {
		return big.NewRat(1, 1), true
	}
	k, invert := pairKey{from, to}, false
	if to < from {
		k, invert = pairKey{to, from}, true
	}
	pts := c.pairs[k]
	if len(pts) == 0 {
		return nil, false
	}
	// Latest point on or before the date; else the earliest one.
	i := sort.Search(len(pts), func(i int) bool { return pts[i].on.After(on) })
	p := pts[0]
	if i > 0 {
		p = pts[i-1]
	}
	if invert {
		return new(big.Rat).Inv(p.rate), true
	}
	return p.rate, true
}

// Convert turns minor units of `from` into minor units of `to` (half-even rounding).
// A missing rate returns false; an unrepresentable result returns an explicit error.
func (c *Converter) Convert(amount int64, from, to string, on time.Time) (int64, bool, error) {
	if from == to {
		return amount, true, nil
	}
	rate, ok := c.Rate(from, to, on)
	if !ok {
		return 0, false, nil
	}
	v := new(big.Rat).Mul(new(big.Rat).SetInt64(amount), rate)
	v.Mul(v, pow10(c.minorUnits[to]-c.minorUnits[from]))
	rounded := roundedHalfEven(v)
	if !rounded.IsInt64() {
		return 0, false, conflict("converted amount from %s to %s is outside the supported range", from, to)
	}
	return rounded.Int64(), true, nil
}

func pow10(n int) *big.Rat {
	r := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(int64(n)))), nil))
	if n < 0 {
		r.Inv(r)
	}
	return r
}

func roundHalfEven(r *big.Rat) int64 {
	return roundedHalfEven(r).Int64()
}

func roundedHalfEven(r *big.Rat) *big.Int {
	num, den := new(big.Int).Set(r.Num()), r.Denom()
	neg := num.Sign() < 0
	num.Abs(num)
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	twice := new(big.Int).Mul(rem, big.NewInt(2))
	if c := twice.Cmp(den); c > 0 || (c == 0 && q.Bit(0) == 1) {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return q
}
