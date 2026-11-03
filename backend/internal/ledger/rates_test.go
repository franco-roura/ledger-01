package ledger

import (
	"errors"
	"math"
	"math/big"
	"testing"
	"time"
)

func TestConverterInt64Range(t *testing.T) {
	for _, tc := range []struct {
		name     string
		amount   int64
		rate     string
		want     int64
		overflow bool
	}{
		{"maximum", math.MaxInt64, "1", math.MaxInt64, false},
		{"minimum", math.MinInt64, "1", math.MinInt64, false},
		{"positive overflow", MaxAmount, "10000", 0, true},
		{"negative overflow", -MaxAmount, "10000", 0, true},
		{"rounding above maximum", 6148914691236517205, "1.5", 0, true},
		{"rounding to minimum", -6148914691236517205, "1.5", math.MinInt64, false},
		{"below minimum", -6148914691236517206, "1.5", 0, true},
		{"half even within range", math.MaxInt64, "0.5", 4611686018427387904, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewConverter([]RatePoint{{On: d("2026-10-01"), From: "USD", To: "UYU", Rate: rat(tc.rate)}},
				map[string]int{"USD": 2, "UYU": 2})
			got, ok, err := c.Convert(tc.amount, "USD", "UYU", d("2026-10-01"))
			if tc.overflow {
				var conflict *ConflictError
				if !errors.As(err, &conflict) || ok || got != 0 {
					t.Fatalf("overflow returned %d, %v, %v", got, ok, err)
				}
			} else if err != nil || !ok || got != tc.want {
				t.Fatalf("Convert = %d, %v, %v; want %d", got, ok, err, tc.want)
			}
		})
	}
}

func d(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func rat(s string) *big.Rat { r, _ := new(big.Rat).SetString(s); return r }

func TestConverter(t *testing.T) {
	c := NewConverter([]RatePoint{
		{On: d("2026-09-10"), From: "USD", To: "UYU", Rate: rat("40")},
		{On: d("2026-10-27"), From: "USD", To: "UYU", Rate: rat("39.05")},
		{On: d("2026-10-05"), From: "UYU", To: "USD", Rate: rat("0.025")}, // reverse direction = 40
		{On: d("2026-10-01"), From: "EUR", To: "JPY", Rate: rat("160")},
	}, map[string]int{"USD": 2, "UYU": 2, "EUR": 2, "JPY": 0})

	tests := []struct {
		name     string
		amount   int64
		from, to string
		on       string
		want     int64
		ok       bool
	}{
		{"same currency", 1234, "USD", "USD", "2026-10-01", 1234, true},
		{"latest on or before", 390500, "UYU", "USD", "2026-10-28", 10000, true}, // 3905 UYU / 39.05
		{"reverse-direction point", 400000, "UYU", "USD", "2026-10-06", 10000, true},
		{"before any point uses the earliest", 10000, "USD", "UYU", "2026-01-01", 400000, true},
		{"different decimals", 1000, "EUR", "JPY", "2026-10-02", 1600, true},
		{"inverse with decimals", 1600, "JPY", "EUR", "2026-10-02", 1000, true},
		{"no rate", 100, "USD", "EUR", "2026-10-02", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := c.Convert(tt.amount, tt.from, tt.to, d(tt.on))
			if err != nil {
				t.Fatal(err)
			}
			if ok != tt.ok || got != tt.want {
				t.Fatalf("Convert = %d, %v; want %d, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBuckets(t *testing.T) {
	weeks := bucketStarts(d("2026-10-01"), d("2026-10-31"), BucketWeek)
	if weeks[0] != d("2026-09-28") || len(weeks) != 5 {
		t.Fatalf("weeks = %v", weeks)
	}
	months := bucketStarts(d("2026-08-15"), d("2026-10-31"), BucketMonth)
	if len(months) != 3 || months[0] != d("2026-08-01") {
		t.Fatalf("months = %v", months)
	}
	if i := bucketIndex(weeks, d("2026-10-06")); i != 1 {
		t.Fatalf("bucketIndex = %d", i)
	}
}
