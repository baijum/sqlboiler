package drivers

import (
	"database/sql"
	"fmt"
	"strings"

	// Side-effect import sql driver
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/vattle/sqlboiler/bdb"
	"github.com/vattle/sqlboiler/strmangle"
)

// PostgresDriver holds the database connection string and a handle
// to the database connection.
type PostgresDriver struct {
	connStr string
	dbConn  *sql.DB
}

// validatedTypes are types that cannot be zero values in the database.
var validatedTypes = []string{"uuid"}

// NewPostgresDriver takes the database connection details as parameters and
// returns a pointer to a PostgresDriver object. Note that it is required to
// call PostgresDriver.Open() and PostgresDriver.Close() to open and close
// the database connection once an object has been obtained.
func NewPostgresDriver(user, pass, dbname, host string, port int, sslmode string) *PostgresDriver {
	driver := PostgresDriver{
		connStr: BuildQueryString(user, pass, dbname, host, port, sslmode),
	}

	return &driver
}

// BuildQueryString for Postgres
func BuildQueryString(user, pass, dbname, host string, port int, sslmode string) string {
	parts := []string{}
	if len(user) != 0 {
		parts = append(parts, fmt.Sprintf("user=%s", user))
	}
	if len(pass) != 0 {
		parts = append(parts, fmt.Sprintf("password=%s", pass))
	}
	if len(dbname) != 0 {
		parts = append(parts, fmt.Sprintf("dbname=%s", dbname))
	}
	if len(host) != 0 {
		parts = append(parts, fmt.Sprintf("host=%s", host))
	}
	if port != 0 {
		parts = append(parts, fmt.Sprintf("port=%d", port))
	}
	if len(sslmode) != 0 {
		parts = append(parts, fmt.Sprintf("sslmode=%s", sslmode))
	}

	return strings.Join(parts, " ")
}

// Open opens the database connection using the connection string
func (p *PostgresDriver) Open() error {
	var err error
	p.dbConn, err = sql.Open("postgres", p.connStr)
	if err != nil {
		return err
	}

	return nil
}

// Close closes the database connection
func (p *PostgresDriver) Close() {
	p.dbConn.Close()
}

// UseLastInsertID returns false for postgres
func (p *PostgresDriver) UseLastInsertID() bool {
	return false
}

// TableNames connects to the postgres database and
// retrieves all table names from the information_schema where the
// table schema is public. It excludes common migration tool tables
// such as gorp_migrations
func (p *PostgresDriver) TableNames(exclude []string) ([]string, error) {
	var names []string

	query := `select table_name from information_schema.tables where table_schema = 'public'`
	if len(exclude) > 0 {
		quoteStr := func(x string) string {
			return `'` + x + `'`
		}
		exclude = strmangle.StringMap(quoteStr, exclude)
		query = query + fmt.Sprintf("and table_name not in (%s);", strings.Join(exclude, ","))
	}

	rows, err := p.dbConn.Query(query)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	return names, nil
}

// Columns takes a table name and attempts to retrieve the table information
// from the database information_schema.columns. It retrieves the column names
// and column types and returns those as a []Column after TranslateColumnType()
// converts the SQL types to Go types, for example: "varchar" to "string"
func (p *PostgresDriver) Columns(tableName string) ([]bdb.Column, error) {
	var columns []bdb.Column

	rows, err := p.dbConn.Query(`
		select column_name, data_type, column_default, is_nullable,
			(select exists(
		    select 1
				from information_schema.constraint_column_usage as ccu
		    inner join information_schema.table_constraints tc on ccu.constraint_name = tc.constraint_name
		    where ccu.table_name = c.table_name and ccu.column_name = c.column_name and tc.constraint_type = 'UNIQUE'
			)) OR (select exists(
		    select 1
		    from
		      pg_indexes pgix
		      inner join pg_class pgc on pgix.indexname = pgc.relname and pgc.relkind = 'i'
		      inner join pg_index pgi on pgi.indexrelid = pgc.oid
		      inner join pg_attribute pga on pga.attrelid = pgi.indrelid and pga.attnum = ANY(pgi.indkey)
		    where
		      pgix.schemaname = 'public' and pgix.tablename = c.table_name and pga.attname = c.column_name and pgi.indisunique = true
		)) as is_unique
		from information_schema.columns as c
		where table_name=$1 and table_schema = 'public';
	`, tableName)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var colName, colType, colDefault, nullable string
		var unique bool
		var defaultPtr *string
		if err := rows.Scan(&colName, &colType, &defaultPtr, &nullable, &unique); err != nil {
			return nil, errors.Wrapf(err, "unable to scan for table %s", tableName)
		}

		if defaultPtr == nil {
			colDefault = ""
		} else {
			colDefault = *defaultPtr
		}

		column := bdb.Column{
			Name:      colName,
			DBType:    colType,
			Default:   colDefault,
			Nullable:  nullable == "YES",
			Unique:    unique,
			Validated: isValidated(colType),
		}
		columns = append(columns, column)
	}

	return columns, nil
}

// PrimaryKeyInfo looks up the primary key for a table.
func (p *PostgresDriver) PrimaryKeyInfo(tableName string) (*bdb.PrimaryKey, error) {
	pkey := &bdb.PrimaryKey{}
	var err error

	query := `
	select tc.constraint_name
	from information_schema.table_constraints as tc
	where tc.table_name = $1 and tc.constraint_type = 'PRIMARY KEY' and tc.table_schema = 'public';`

	row := p.dbConn.QueryRow(query, tableName)
	if err = row.Scan(&pkey.Name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	queryColumns := `
	select kcu.column_name
	from   information_schema.key_column_usage as kcu
	where  constraint_name = $1 and table_schema = 'public';`

	var rows *sql.Rows
	if rows, err = p.dbConn.Query(queryColumns, pkey.Name); err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var column string

		err = rows.Scan(&column)
		if err != nil {
			return nil, err
		}

		columns = append(columns, column)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	pkey.Columns = columns

	return pkey, nil
}

// ForeignKeyInfo retrieves the foreign keys for a given table name.
func (p *PostgresDriver) ForeignKeyInfo(tableName string) ([]bdb.ForeignKey, error) {
	var fkeys []bdb.ForeignKey

	query := `
	select
		tc.constraint_name,
		kcu.table_name as source_table,
		kcu.column_name as source_column,
		ccu.table_name as dest_table,
		ccu.column_name as dest_column
	from information_schema.table_constraints as tc
		inner join information_schema.key_column_usage as kcu ON tc.constraint_name = kcu.constraint_name
		inner join information_schema.constraint_column_usage as ccu ON tc.constraint_name = ccu.constraint_name
	where tc.table_name = $1 and tc.constraint_type = 'FOREIGN KEY' and tc.table_schema = 'public';`

	var rows *sql.Rows
	var err error
	if rows, err = p.dbConn.Query(query, tableName); err != nil {
		return nil, err
	}

	for rows.Next() {
		var fkey bdb.ForeignKey
		var sourceTable string

		fkey.Table = tableName
		err = rows.Scan(&fkey.Name, &sourceTable, &fkey.Column, &fkey.ForeignTable, &fkey.ForeignColumn)
		if err != nil {
			return nil, err
		}

		fkeys = append(fkeys, fkey)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return fkeys, nil
}

// TranslateColumnType converts postgres database types to Go types, for example
// "varchar" to "string" and "bigint" to "int64". It returns this parsed data
// as a Column object.
func (p *PostgresDriver) TranslateColumnType(c bdb.Column) bdb.Column {
	if c.Nullable {
		switch c.DBType {
		case "bigint", "bigserial":
			c.Type = "null.Int64"
		case "integer", "serial":
			c.Type = "null.Int"
		case "smallint", "smallserial":
			c.Type = "null.Int16"
		case "decimal", "numeric", "double precision", "money":
			c.Type = "null.Float64"
		case "real":
			c.Type = "null.Float32"
		case "bit", "interval", "bit varying", "character", "character varying", "cidr", "inet", "json", "macaddr", "text", "uuid", "xml":
			c.Type = "null.String"
		case "bytea":
			c.Type = "[]byte"
		case "boolean":
			c.Type = "null.Bool"
		case "date", "time", "timestamp without time zone", "timestamp with time zone":
			c.Type = "null.Time"
		default:
			c.Type = "null.String"
		}
	} else {
		switch c.DBType {
		case "bigint", "bigserial":
			c.Type = "int64"
		case "integer", "serial":
			c.Type = "int"
		case "smallint", "smallserial":
			c.Type = "int16"
		case "decimal", "numeric", "double precision", "money":
			c.Type = "float64"
		case "real":
			c.Type = "float32"
		case "bit", "interval", "uuint", "bit varying", "character", "character varying", "cidr", "inet", "json", "macaddr", "text", "uuid", "xml":
			c.Type = "string"
		case "bytea":
			c.Type = "[]byte"
		case "boolean":
			c.Type = "bool"
		case "date", "time", "timestamp without time zone", "timestamp with time zone":
			c.Type = "time.Time"
		default:
			c.Type = "string"
		}
	}

	return c
}

// isValidated checks if the database type is in the validatedTypes list.
func isValidated(typ string) bool {
	for _, v := range validatedTypes {
		if v == typ {
			return true
		}
	}

	return false
}
