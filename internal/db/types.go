package db

import (
	"math/big"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type pgconnCommandTag = pgconn.CommandTag

const (
	oidDate = 1082
	oidTime = 1083
)

// NormalizeOID is Normalize with knowledge of the column type: dates stay
// "YYYY-MM-DD" and times "HH:MM:SS".
func NormalizeOID(v any, oid uint32) any {
	if t, ok := v.(time.Time); ok {
		switch oid {
		case oidDate:
			return t.Format("2006-01-02")
		case oidTime:
			return t.Format("15:04:05")
		}
	}
	return Normalize(v)
}

// Normalize converts pgx values into plain Go values that serialize cleanly
// to JSON and to JS: numerics become float64, dates become strings.
func Normalize(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case pgtype.Numeric:
		if !x.Valid {
			return nil
		}
		f, _ := x.Float64Value()
		return f.Float64
	case *big.Int:
		f, _ := new(big.Float).SetInt(x).Float64()
		return f
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case pgtype.Date:
		if !x.Valid {
			return nil
		}
		return x.Time.Format("2006-01-02")
	case pgtype.Time:
		if !x.Valid {
			return nil
		}
		return time.Unix(0, x.Microseconds*1000).UTC().Format("15:04:05")
	case int32:
		return int64(x)
	case int16:
		return int64(x)
	case int8:
		return int64(x)
	case int:
		return int64(x)
	case uint32:
		return int64(x)
	case uint16:
		return int64(x)
	case float32:
		return float64(x)
	case []byte:
		return string(x)
	case [16]byte:
		return string(x[:])
	}
	return v
}

// Str formats any value as a string for naming/formatting.
func Str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		if x {
			return "1"
		}
		return "0"
	}
	return fmtAny(v)
}

func fmtAny(v any) string { return stringify(v) }
