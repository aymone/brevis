package iceberg

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"math/big"
	"strconv"
	"time"

	"brevis.sh/brevis/sdk"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/decimal128"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// BatchRows is how many rows one Arrow batch carries on its way to Parquet.
//
// 65,536: enough that the Parquet writer sees a row group's worth of values
// at a time, small enough that a batch of ten columns is a few megabytes.
// The spec's memory bound -- a million rows under 512 MB -- follows from one
// batch being alive at a time, not from this number being tuned.
const BatchRows = 65_536

// precision and scale are decimal(38, 9), the one decimal this destination
// writes: BigQuery's NUMERIC, which is what the SDK's `numeric` has meant.
const precision, scale = 38, 9

// Records turns a Write's envelopes into Arrow record batches in the shape
// ArrowSchema gives the declaration, BatchRows rows at a time, converting
// every value by the column's DECLARED type and refusing what does not fit.
//
// It reads only the declared columns out of each record. A field the record
// carries and the declaration does not is Reconcile's refusal, not this
// function's; a declared column the record does not carry is null here,
// unless it is Required, in which case the record is refused naming the
// column.
//
// A batch is valid for the body of the loop and released when the body
// returns; a consumer that keeps one calls Retain. The first value that
// cannot be converted ends the iteration with an error naming the record,
// the column, the declared type and what arrived.
func Records(schema sdk.Schema, envelopes []sdk.Envelope) iter.Seq2[arrow.RecordBatch, error] {
	return func(yield func(arrow.RecordBatch, error) bool) {
		as, err := ArrowSchema(schema)
		if err != nil {
			yield(nil, err)
			return
		}
		bld := array.NewRecordBuilder(memory.DefaultAllocator, as)
		defer bld.Release()
		for start := 0; start < len(envelopes); start += BatchRows {
			end := min(start+BatchRows, len(envelopes))
			bld.Reserve(end - start)
			for i, e := range envelopes[start:end] {
				if err := appendRecord(bld, schema, start+i, e); err != nil {
					yield(nil, err)
					return
				}
			}
			rec := bld.NewRecordBatch()
			more := yield(rec, nil)
			rec.Release()
			if !more {
				return
			}
		}
	}
}

// appendRecord appends one envelope as one row, column by column in the
// declaration's order.
func appendRecord(bld *array.RecordBuilder, schema sdk.Schema, i int, e sdk.Envelope) error {
	row, err := object(e.Payload)
	if err != nil {
		return fmt.Errorf("iceberg: %s: %w", recordName(i, e), err)
	}
	for j, c := range schema {
		v, present := row[c.Name]
		if !present || v == nil {
			if c.Required {
				return fmt.Errorf("iceberg: %s: column %q is required and the record has no value for it",
					recordName(i, e), c.Name)
			}
			bld.Field(j).AppendNull()
			continue
		}
		if err := appendValue(bld.Field(j), c.Type, v); err != nil {
			return fmt.Errorf("iceberg: %s, column %q declared %s: %w", recordName(i, e), c.Name, c.Type, err)
		}
	}
	return nil
}

// recordName says which record, the way a producer finds it: by position in
// the batch, counted from one, and by the source's key when it gave one.
func recordName(i int, e sdk.Envelope) string {
	if e.SourceKey == "" {
		return fmt.Sprintf("record %d", i+1)
	}
	return fmt.Sprintf("record %d (source key %q)", i+1, elide(e.SourceKey))
}

// object reads the record as a map. A map[string]any -- what every source in
// the SDK produces -- is taken as it is; anything else goes through JSON and
// back, with numbers kept as json.Number so an int64 above 2^53 survives the
// round trip.
func object(payload any) (map[string]any, error) {
	if m, ok := payload.(map[string]any); ok {
		return m, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("the record does not encode to JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("the record is not an object: %s", elide(string(data)))
	}
	return m, nil
}

// appendValue converts one value by the declared type and appends it. The
// builder's concrete type is the one ArrowSchema chose for that column, from
// the same table, so the assertions below hold by construction.
//
//	column     accepts from the record
//	string     a string
//	int64      an integer of any width, an integral float, a json.Number
//	float64    any number
//	numeric    plain decimal text with at most 9 fractional digits, or a number; a float is rounded to the scale
//	bool       a bool
//	timestamp  RFC 3339 text, or a time.Time; stored in UTC at microseconds
//	date       YYYY-MM-DD text, or a time.Time's calendar day
//	json       a map or a slice, serialized; text or bytes that already are a document
//	bytes      []byte, or standard base64 text, which is how encoding/json writes bytes
//
// Everything else is refused naming what arrived. A number into a timestamp
// in particular: seconds or milliseconds would be a guess.
func appendValue(b array.Builder, t sdk.ColumnType, v any) error {
	switch t {
	case sdk.TypeString:
		s, ok := v.(string)
		if !ok {
			return cannot(v, "is not text; a string column takes only a string")
		}
		b.(*array.StringBuilder).Append(s)
	case sdk.TypeInt64:
		n, err := toInt64(v)
		if err != nil {
			return err
		}
		b.(*array.Int64Builder).Append(n)
	case sdk.TypeFloat64:
		f, err := toFloat64(v)
		if err != nil {
			return err
		}
		b.(*array.Float64Builder).Append(f)
	case sdk.TypeNumeric:
		d, err := toDecimal(v)
		if err != nil {
			return err
		}
		b.(*array.Decimal128Builder).Append(d)
	case sdk.TypeBool:
		x, ok := v.(bool)
		if !ok {
			return cannot(v, "is not a boolean")
		}
		b.(*array.BooleanBuilder).Append(x)
	case sdk.TypeTimestamp:
		at, err := toInstant(v)
		if err != nil {
			return err
		}
		b.(*array.TimestampBuilder).Append(arrow.Timestamp(at.UnixMicro()))
	case sdk.TypeDate:
		d, err := toDate(v)
		if err != nil {
			return err
		}
		b.(*array.Date32Builder).Append(arrow.Date32FromTime(d))
	case sdk.TypeJSON:
		doc, err := toDocument(v)
		if err != nil {
			return err
		}
		b.(*array.StringBuilder).Append(doc)
	case sdk.TypeBytes:
		raw, err := toBytes(v)
		if err != nil {
			return err
		}
		b.(*array.BinaryBuilder).Append(raw)
	default:
		// Unreachable: ArrowSchema refused every type outside the map before
		// a builder existed.
		return fmt.Errorf("no Iceberg type for %q", t)
	}
	return nil
}

func toInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int8:
		return int64(n), nil
	case int16:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case uint8:
		return int64(n), nil
	case uint16:
		return int64(n), nil
	case uint32:
		return int64(n), nil
	case uint:
		if uint64(n) > math.MaxInt64 {
			return 0, cannot(v, "does not fit in int64")
		}
		return int64(n), nil
	case uint64:
		if n > math.MaxInt64 {
			return 0, cannot(v, "does not fit in int64")
		}
		return int64(n), nil
	case float32:
		return integral(float64(n), v)
	case float64:
		return integral(n, v)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		f, err := n.Float64()
		if err != nil {
			return 0, cannot(v, "is not a number")
		}
		return integral(f, v)
	}
	return 0, cannot(v, "is not a number; an int64 column takes an integer or an integral float")
}

// integral accepts the float a JSON number arrives as, when it is a whole
// number in range. 3.0 is 3; 3.5 is refused rather than truncated.
func integral(f float64, v any) (int64, error) {
	if f != math.Trunc(f) {
		return 0, cannot(v, "is not integral; an int64 column takes a whole number")
	}
	if f < -0x1p63 || f >= 0x1p63 {
		return 0, cannot(v, "does not fit in int64")
	}
	return int64(f), nil
}

func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, cannot(v, "is not a number")
		}
		return f, nil
	case int, int8, int16, int32, int64, uint8, uint16, uint32:
		i, _ := toInt64(v)
		return float64(i), nil
	case uint:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	}
	return 0, cannot(v, "is not a number")
}

func toDecimal(v any) (decimal128.Num, error) {
	switch n := v.(type) {
	case string:
		return decimalText(n, v)
	case json.Number:
		return decimalText(string(n), v)
	case float64:
		return decimalFloat(n, v)
	case float32:
		return decimalFloat(float64(n), v)
	case uint:
		return decimal128.FromBigInt(new(big.Int).SetUint64(uint64(n))).Rescale(0, scale)
	case uint64:
		return decimal128.FromBigInt(new(big.Int).SetUint64(n)).Rescale(0, scale)
	case int, int8, int16, int32, int64, uint8, uint16, uint32:
		i, _ := toInt64(v)
		return decimal128.FromI64(i).Rescale(0, scale)
	}
	return decimal128.Num{}, cannot(v, "is not a number; a numeric column takes decimal text or a number")
}

// decimalText parses plain decimal text -- an optional sign, digits, an
// optional point and digits -- at the column's scale. Text with more
// fractional digits than the scale holds is refused rather than rounded:
// the source wrote those digits on purpose. Exponent notation is refused
// too, because 1.5e-10 rounds to zero without a word.
func decimalText(s string, v any) (decimal128.Num, error) {
	digits, ok := decimalShape(s)
	if !ok {
		return decimal128.Num{}, cannot(v, "is not decimal text, as in -1234.50")
	}
	if digits > scale {
		return decimal128.Num{}, cannot(v, fmt.Sprintf("has %d fractional digits; decimal(38, 9) holds 9", digits))
	}
	n, err := decimal128.FromString(s, precision, scale)
	if err != nil {
		return decimal128.Num{}, cannot(v, "does not fit decimal(38, 9)")
	}
	return n, nil
}

// decimalShape reports whether s is plain decimal text, and how many digits
// follow the point.
func decimalShape(s string) (fraction int, ok bool) {
	if s == "" {
		return 0, false
	}
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
	}
	seenDigit, seenPoint := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			seenDigit = true
			if seenPoint {
				fraction++
			}
		case c == '.' && !seenPoint:
			seenPoint = true
		default:
			return 0, false
		}
	}
	return fraction, seenDigit
}

// decimalFloat rounds a float to the scale, which is the right reading of a
// float: its binary noise is not digits anybody wrote.
func decimalFloat(f float64, v any) (decimal128.Num, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return decimal128.Num{}, cannot(v, "is not a finite number")
	}
	n, err := decimal128.FromFloat64(f, precision, scale)
	if err != nil {
		return decimal128.Num{}, cannot(v, "does not fit decimal(38, 9)")
	}
	return n, nil
}

func toInstant(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case string:
		at, err := time.Parse(time.RFC3339Nano, t)
		if err != nil {
			return time.Time{}, cannot(v, "is not an RFC 3339 instant, as in 2026-11-02T10:00:00Z")
		}
		return at, nil
	}
	return time.Time{}, cannot(v, "cannot be an instant; a timestamp column takes RFC 3339 text or a time.Time, and a number would be a guess between seconds and milliseconds")
}

// toDate gives the calendar day: of the text, or of the time.Time in its own
// zone, which is the day the source meant.
func toDate(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	case string:
		d, err := time.Parse("2006-01-02", t)
		if err != nil {
			return time.Time{}, cannot(v, "is not a date, as in 2026-11-02")
		}
		return d, nil
	}
	return time.Time{}, cannot(v, "is not a date; a date column takes YYYY-MM-DD text or a time.Time")
}

// toDocument gives the serialized document. Text or bytes that already are
// one go as they came; a producer with plain text to keep declares `string`.
func toDocument(v any) (string, error) {
	var raw []byte
	switch d := v.(type) {
	case string:
		raw = []byte(d)
	case []byte:
		raw = d
	case json.RawMessage:
		raw = d
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", cannot(v, "does not encode to JSON")
		}
		return string(b), nil
	}
	if !json.Valid(raw) {
		return "", cannot(v, "is not a JSON document")
	}
	return string(raw), nil
}

func toBytes(v any) ([]byte, error) {
	switch b := v.(type) {
	case []byte:
		return b, nil
	case string:
		raw, err := base64.StdEncoding.DecodeString(b)
		if err != nil {
			return nil, cannot(v, "is not base64; a bytes column takes []byte or standard base64 text")
		}
		return raw, nil
	}
	return nil, cannot(v, "is not bytes; a bytes column takes []byte or standard base64 text")
}

// cannot says what arrived -- the Go type and the value, elided -- and why
// it was refused. The value is elided because a 4 KB field has no place in
// a log line, and quoted when it is text so an empty string is visible.
func cannot(v any, why string) error {
	if s, ok := v.(string); ok {
		return fmt.Errorf("%T %s %s", v, strconv.Quote(elide(s)), why)
	}
	return fmt.Errorf("%T %s %s", v, elide(fmt.Sprint(v)), why)
}

func elide(s string) string {
	if len(s) <= 40 {
		return s
	}
	r := []rune(s)
	if len(r) <= 40 {
		return s
	}
	return string(r[:37]) + "..."
}
