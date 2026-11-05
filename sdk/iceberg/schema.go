package iceberg

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"brevis.sh/brevis/sdk"
	"github.com/apache/arrow-go/v18/arrow"
	ice "github.com/apache/iceberg-go"
)

// mapping is what one SDK column type becomes, on each side of the write:
// the Iceberg type the table declares and the Arrow type the rows are built
// as before they are written to Parquet.
type mapping struct {
	iceberg ice.Type
	arrow   arrow.DataType
}

// types is the whole map, and it is written rather than inferred: a reader
// in two years meets the type chosen here. One SDK type, one Iceberg type,
// one Arrow type.
//
//	string     string          utf8
//	int64      long            int64
//	float64    double          float64
//	numeric    decimal(38, 9)  decimal128(38, 9)   BigQuery's NUMERIC, which is what the SDK's numeric has meant
//	bool       boolean         bool
//	timestamp  timestamptz     timestamp[us, UTC]  an instant with a zone; Iceberg v2 keeps microseconds
//	date       date            date32
//	json       string          utf8                the serialized document; v3's variant waits for readers
//	bytes      binary          binary
var types = map[sdk.ColumnType]mapping{
	sdk.TypeString:    {ice.PrimitiveTypes.String, arrow.BinaryTypes.String},
	sdk.TypeInt64:     {ice.PrimitiveTypes.Int64, arrow.PrimitiveTypes.Int64},
	sdk.TypeFloat64:   {ice.PrimitiveTypes.Float64, arrow.PrimitiveTypes.Float64},
	sdk.TypeNumeric:   {ice.DecimalTypeOf(precision, scale), &arrow.Decimal128Type{Precision: precision, Scale: scale}},
	sdk.TypeBool:      {ice.PrimitiveTypes.Bool, arrow.FixedWidthTypes.Boolean},
	sdk.TypeTimestamp: {ice.PrimitiveTypes.TimestampTz, &arrow.TimestampType{Unit: arrow.Microsecond, TimeZone: "UTC"}},
	sdk.TypeDate:      {ice.PrimitiveTypes.Date, arrow.FixedWidthTypes.Date32},
	sdk.TypeJSON:      {ice.PrimitiveTypes.String, arrow.BinaryTypes.String},
	sdk.TypeBytes:     {ice.PrimitiveTypes.Binary, arrow.BinaryTypes.Binary},
}

// IcebergSchema is the table's schema for a declared sdk.Schema: the same
// columns, in the same order, with field ids 1..n in that order. A pure
// function -- it opens nothing and takes no context -- because a partition
// spec names its source column by id and a reader matches Parquet columns
// to the table by id, so the numbering has to be reproducible from the
// declaration alone.
//
// Required columns are required; everything else takes null, which is what a
// landing table legitimately does.
func IcebergSchema(s sdk.Schema) (*ice.Schema, error) {
	if err := checkColumns(s); err != nil {
		return nil, err
	}
	fields := make([]ice.NestedField, len(s))
	for i, c := range s {
		fields[i] = ice.NestedField{ID: i + 1, Name: c.Name, Type: types[c.Type].iceberg, Required: c.Required}
	}
	return ice.NewSchema(0, fields...), nil
}

// ArrowSchema is the shape the rows take in memory on their way to Parquet,
// for the same declaration: the same columns and order, nullable unless
// required. iceberg-go matches it to the table by name.
func ArrowSchema(s sdk.Schema) (*arrow.Schema, error) {
	if err := checkColumns(s); err != nil {
		return nil, err
	}
	fields := make([]arrow.Field, len(s))
	for i, c := range s {
		fields[i] = arrow.Field{Name: c.Name, Type: types[c.Type].arrow, Nullable: !c.Required}
	}
	return arrow.NewSchema(fields, nil), nil
}

// checkColumns refuses what cannot become a table, naming it. The type check
// comes first and names the column and the type, because that is the message
// a consumer acts on; the SDK's own Check then covers duplicates, blank names
// and the rest.
func checkColumns(s sdk.Schema) error {
	if len(s) == 0 {
		return errors.New("iceberg: a schema with no columns cannot become a table; declare Schema")
	}
	for _, c := range s {
		if _, ok := types[c.Type]; !ok {
			return fmt.Errorf("iceberg: column %q has type %q, which this destination has no Iceberg type for; it takes %s",
				c.Name, c.Type, knownTypes())
		}
	}
	return s.Check()
}

// knownTypes lists the map's keys, sorted, for a refusal.
func knownTypes() string {
	names := make([]string, 0, len(types))
	for t := range types {
		names = append(names, string(t))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
