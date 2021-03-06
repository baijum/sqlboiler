package main

import (
	"fmt"
	"strings"

	"github.com/vattle/sqlboiler/bdb"
	"github.com/vattle/sqlboiler/strmangle"
)

// RelationshipToOneTexts contains text that will be used by templates.
type RelationshipToOneTexts struct {
	ForeignKey bdb.ForeignKey

	LocalTable struct {
		NameGo       string
		ColumnNameGo string
	}

	ForeignTable struct {
		NameGo       string
		NamePluralGo string
		Name         string
		ColumnNameGo string
		ColumnName   string
	}

	Function struct {
		PackageName string
		Name        string
		ForeignName string

		Varname  string
		Receiver string
		OneToOne bool

		LocalAssignment   string
		ForeignAssignment string
	}
}

func textsFromForeignKey(packageName string, tables []bdb.Table, table bdb.Table, fkey bdb.ForeignKey) RelationshipToOneTexts {
	r := RelationshipToOneTexts{}

	r.ForeignKey = fkey

	r.LocalTable.NameGo = strmangle.TitleCase(strmangle.Singular(table.Name))
	r.LocalTable.ColumnNameGo = strmangle.TitleCase(strmangle.Singular(fkey.Column))

	r.ForeignTable.Name = fkey.ForeignTable
	r.ForeignTable.NameGo = strmangle.TitleCase(strmangle.Singular(fkey.ForeignTable))
	r.ForeignTable.NamePluralGo = strmangle.TitleCase(strmangle.Plural(fkey.ForeignTable))
	r.ForeignTable.ColumnName = fkey.ForeignColumn
	r.ForeignTable.ColumnNameGo = strmangle.TitleCase(strmangle.Singular(fkey.ForeignColumn))

	r.Function.PackageName = packageName
	r.Function.Name = strmangle.TitleCase(strmangle.Singular(strings.TrimSuffix(fkey.Column, "_id")))
	plurality := strmangle.Plural
	if fkey.Unique {
		plurality = strmangle.Singular
	}
	r.Function.ForeignName = mkFunctionName(strmangle.Singular(fkey.ForeignTable), strmangle.TitleCase(plurality(fkey.Table)), fkey.Column, false)
	r.Function.Varname = strmangle.CamelCase(strmangle.Singular(fkey.ForeignTable))
	r.Function.Receiver = strings.ToLower(table.Name[:1])

	if fkey.Nullable {
		col := table.GetColumn(fkey.Column)
		r.Function.LocalAssignment = fmt.Sprintf("%s.%s", strmangle.TitleCase(fkey.Column), strings.TrimPrefix(col.Type, "null."))
	} else {
		r.Function.LocalAssignment = strmangle.TitleCase(fkey.Column)
	}

	if fkey.ForeignColumnNullable {
		foreignTable := bdb.GetTable(tables, fkey.ForeignTable)
		col := foreignTable.GetColumn(fkey.ForeignColumn)
		r.Function.ForeignAssignment = fmt.Sprintf("%s.%s", strmangle.TitleCase(fkey.ForeignColumn), strings.TrimPrefix(col.Type, "null."))
	} else {
		r.Function.ForeignAssignment = strmangle.TitleCase(fkey.ForeignColumn)
	}

	return r
}

func textsFromOneToOneRelationship(packageName string, tables []bdb.Table, table bdb.Table, toMany bdb.ToManyRelationship) RelationshipToOneTexts {
	fkey := bdb.ForeignKey{
		Table:    toMany.Table,
		Name:     "none",
		Column:   toMany.Column,
		Nullable: toMany.Nullable,
		Unique:   toMany.Unique,

		ForeignTable:          toMany.ForeignTable,
		ForeignColumn:         toMany.ForeignColumn,
		ForeignColumnNullable: toMany.ForeignColumnNullable,
		ForeignColumnUnique:   toMany.ForeignColumnUnique,
	}

	rel := textsFromForeignKey(packageName, tables, table, fkey)
	rel.Function.Name = strmangle.TitleCase(strmangle.Singular(toMany.ForeignTable))
	rel.Function.ForeignName = mkFunctionName(strmangle.Singular(toMany.Table), strmangle.TitleCase(strmangle.Singular(toMany.Table)), toMany.ForeignColumn, false)
	rel.Function.OneToOne = true
	return rel
}

// RelationshipToManyTexts contains text that will be used by templates.
type RelationshipToManyTexts struct {
	LocalTable struct {
		NameGo       string
		NameSingular string
		ColumnNameGo string
	}

	ForeignTable struct {
		NameGo            string
		NameSingular      string
		NamePluralGo      string
		NameHumanReadable string
		ColumnNameGo      string
		Slice             string
	}

	Function struct {
		Name        string
		ForeignName string
		Receiver    string

		LocalAssignment   string
		ForeignAssignment string
	}
}

// textsFromRelationship creates a struct that does a lot of the text
// transformation in advance for a given relationship.
func textsFromRelationship(tables []bdb.Table, table bdb.Table, rel bdb.ToManyRelationship) RelationshipToManyTexts {
	r := RelationshipToManyTexts{}
	r.LocalTable.NameSingular = strmangle.Singular(table.Name)
	r.LocalTable.NameGo = strmangle.TitleCase(r.LocalTable.NameSingular)
	r.LocalTable.ColumnNameGo = strmangle.TitleCase(rel.Column)

	r.ForeignTable.NameSingular = strmangle.Singular(rel.ForeignTable)
	r.ForeignTable.NamePluralGo = strmangle.TitleCase(strmangle.Plural(rel.ForeignTable))
	r.ForeignTable.NameGo = strmangle.TitleCase(r.ForeignTable.NameSingular)
	r.ForeignTable.ColumnNameGo = strmangle.TitleCase(rel.ForeignColumn)
	r.ForeignTable.Slice = fmt.Sprintf("%sSlice", strmangle.TitleCase(r.ForeignTable.NameSingular))
	r.ForeignTable.NameHumanReadable = strings.Replace(rel.ForeignTable, "_", " ", -1)

	r.Function.Receiver = strings.ToLower(table.Name[:1])
	r.Function.Name = mkFunctionName(r.LocalTable.NameSingular, r.ForeignTable.NamePluralGo, rel.ForeignColumn, rel.ToJoinTable)
	plurality := strmangle.Singular
	foreignNamingColumn := rel.ForeignColumn
	if rel.ToJoinTable {
		plurality = strmangle.Plural
		foreignNamingColumn = rel.JoinLocalColumn
	}
	r.Function.ForeignName = strmangle.TitleCase(plurality(strings.TrimSuffix(foreignNamingColumn, "_id")))

	if rel.Nullable {
		col := table.GetColumn(rel.Column)
		r.Function.LocalAssignment = fmt.Sprintf("%s.%s", strmangle.TitleCase(rel.Column), strings.TrimPrefix(col.Type, "null."))
	} else {
		r.Function.LocalAssignment = strmangle.TitleCase(rel.Column)
	}

	if rel.ForeignColumnNullable {
		foreignTable := bdb.GetTable(tables, rel.ForeignTable)
		col := foreignTable.GetColumn(rel.ForeignColumn)
		r.Function.ForeignAssignment = fmt.Sprintf("%s.%s", strmangle.TitleCase(rel.ForeignColumn), strings.TrimPrefix(col.Type, "null."))
	} else {
		r.Function.ForeignAssignment = strmangle.TitleCase(rel.ForeignColumn)
	}

	return r
}

// mkFunctionName checks to see if the foreign key name is the same as the local table name (minus _id suffix)
// Simple case: yes - we can name the function the same as the plural table name
// Not simple case: We have to name the function based off the foreign key and the foreign table name
func mkFunctionName(fkeyTableSingular, foreignTablePluralGo, fkeyColumn string, toJoinTable bool) string {
	colName := strings.TrimSuffix(fkeyColumn, "_id")
	if toJoinTable || fkeyTableSingular == colName {
		return foreignTablePluralGo
	}

	return strmangle.TitleCase(colName) + foreignTablePluralGo
}

// PreserveDot allows us to pass in templateData to relationship templates
// called with the template function.
type PreserveDot struct {
	Dot templateData
	Rel RelationshipToOneTexts
}

func preserveDot(data templateData, obj RelationshipToOneTexts) PreserveDot {
	return PreserveDot{
		Dot: data,
		Rel: obj,
	}
}
