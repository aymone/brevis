package components

import (
	"unicode"
)

// logoPaint is how a mark is coloured.
type logoPaint int

const (
	paintMono    logoPaint = iota // one colour, the brand's, written in the file
	paintMulti                    // the mark's own colours
	paintDark                     // a brand too dark for a dark ground: drawn in ink
	paintLight                    // a brand too light for a light ground: darkened there
	paintBrand                    // Brevis's own mark, in the accent
	paintOnWhite                  // its own colours, too dark for any dark tile: on white in both themes
)

// logo is one registered key: whose mark it is, how it is painted, and, for
// a key with no mark, the monogram drawn instead.
type logo struct {
	title    string
	paint    logoPaint
	monogram string
}

var logos = map[string]logo{
	"airflow":    {title: "Apache Airflow", paint: paintMono},
	"bigquery":   {title: "Google BigQuery", paint: paintMono},
	"brevissql":  {title: "brevis-sql", paint: paintBrand},
	"clickhouse": {title: "ClickHouse", paint: paintLight},
	"dbt":        {title: "dbt", paint: paintMono},
	"docker":     {title: "Docker", paint: paintMono},
	"duckdb":     {title: "DuckDB", paint: paintLight},
	"gcs":        {title: "Google Cloud Storage", paint: paintLight},
	"geocoding":  {title: "Google Maps", paint: paintMono},
	"go":         {title: "Go", paint: paintMulti},
	"kafka":      {title: "Apache Kafka", paint: paintDark},
	"kubernetes": {title: "Kubernetes", paint: paintMono},
	"mysql":      {title: "MySQL", paint: paintMono},
	"node":       {title: "Node.js", paint: paintMulti},
	"pandas":     {title: "pandas", paint: paintOnWhite},
	"polars":     {title: "Polars", paint: paintMono},
	"postgres":   {title: "PostgreSQL", paint: paintMulti},
	"pubsub":     {title: "Google Pub/Sub", paint: paintLight},
	"python":     {title: "Python", paint: paintMulti},
	"redis":      {title: "Redis", paint: paintMulti},
	"redshift":   {title: "Amazon Redshift", paint: paintMono},
	"s3":         {title: "Amazon S3", paint: paintMono},
	"shell":      {title: "GNU Bash", paint: paintMono},
	"snowflake":  {title: "Snowflake", paint: paintMono},
	"spark":      {title: "Apache Spark", paint: paintMono},
	"sqs":        {title: "Amazon SQS", paint: paintMono},
	"terraform":  {title: "Terraform", paint: paintMono},
	"vision":     {title: "Google Cloud", paint: paintMono},

	// No mark: the handoff's monograms.
	"file":    {title: "Local files", monogram: "FS"},
	"http":    {title: "HTTP", monogram: "{}"},
	"gateway": {title: "Brevis gateway", monogram: "→"},
	"kinesis": {title: "Amazon Kinesis", monogram: "KS"},
	"sftp":    {title: "SFTP", monogram: "SF"},
}

// lookLogo is a key's entry; one nobody registered is its own monogram, the
// first two letters of its name, so an unknown connector is still told apart
// from its neighbours and never drawn as somebody else's mark.
func lookLogo(key string) logo {
	if l, ok := logos[key]; ok {
		return l
	}
	letters := []rune{}
	for _, r := range key {
		if unicode.IsLetter(r) {
			letters = append(letters, unicode.ToUpper(r))
		}
		if len(letters) == 2 {
			break
		}
	}
	return logo{title: key, monogram: string(letters)}
}

// LogoTitle is the name a key's brand is read by: a brand is its own name in
// every language. The two keys that are words ("file", "gateway") are
// sentences the app says (logo.file, logo.gateway), never this.
func LogoTitle(key string) string {
	return lookLogo(key).title
}
