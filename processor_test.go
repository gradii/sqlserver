package sqlserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/gradii/framework/contracts/database/driver"
)

type ProcessorTestSuite struct {
	suite.Suite
	processor *Processor
}

func TestProcessorTestSuite(t *testing.T) {
	suite.Run(t, new(ProcessorTestSuite))
}

func (s *ProcessorTestSuite) SetupTest() {
	s.processor = NewProcessor()
}

func (s *ProcessorTestSuite) TestProcessColumns() {
	tests := []struct {
		name      string
		dbColumns []driver.DBColumn
		expected  []driver.Column
	}{
		{
			name: "ValidInput",
			dbColumns: []driver.DBColumn{
				{Name: "id", TypeName: "int", Nullable: "false", Autoincrement: true, Collation: "utf8_general_ci", Comment: "primary key", Default: "1"},
				{Name: "name", TypeName: "varchar", Nullable: "true", Collation: "utf8_general_ci", Comment: "user name", Default: "default_name", Length: 10},
			},
			expected: []driver.Column{
				{Autoincrement: true, Collation: "utf8_general_ci", Comment: "primary key", Default: "1", Name: "id", Nullable: false, Type: "int", TypeName: "int"},
				{Autoincrement: false, Collation: "utf8_general_ci", Comment: "user name", Default: "default_name", Name: "name", Nullable: true, Type: "varchar(10)", TypeName: "varchar"},
			},
		},
		{
			name:      "EmptyInput",
			dbColumns: []driver.DBColumn{},
		},
		{
			name: "NullableColumn",
			dbColumns: []driver.DBColumn{
				{Name: "description", TypeName: "text", Nullable: "true", Collation: "utf8_general_ci", Comment: "description", Default: "default_description"},
			},
			expected: []driver.Column{
				{Autoincrement: false, Collation: "utf8_general_ci", Comment: "description", Default: "default_description", Name: "description", Nullable: true, Type: "text", TypeName: "text"},
			},
		},
		{
			name: "NonNullableColumn",
			dbColumns: []driver.DBColumn{
				{Name: "created_at", TypeName: "timestamp", Nullable: "false", Collation: "", Comment: "creation time", Default: "CURRENT_TIMESTAMP"},
			},
			expected: []driver.Column{
				{Autoincrement: false, Collation: "", Comment: "creation time", Default: "CURRENT_TIMESTAMP", Name: "created_at", Nullable: false, Type: "timestamp", TypeName: "timestamp"},
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := s.processor.ProcessColumns(tt.dbColumns)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ProcessorTestSuite) TestProcessForeignKeys() {
	tests := []struct {
		name          string
		dbForeignKeys []driver.DBForeignKey
		expected      []driver.ForeignKey
	}{
		{
			name: "ValidInput",
			dbForeignKeys: []driver.DBForeignKey{
				{Name: "fk_user_id", Columns: "user_id", ForeignSchema: "dbo", ForeignTable: "users", ForeignColumns: "id", OnUpdate: "CASCADE", OnDelete: "SET_NULL"},
			},
			expected: []driver.ForeignKey{
				{Name: "fk_user_id", Columns: []string{"user_id"}, ForeignSchema: "dbo", ForeignTable: "users", ForeignColumns: []string{"id"}, OnUpdate: "cascade", OnDelete: "set null"},
			},
		},
		{
			name:          "EmptyInput",
			dbForeignKeys: []driver.DBForeignKey{},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := s.processor.ProcessForeignKeys(tt.dbForeignKeys)
			s.Equal(tt.expected, result)
		})
	}
}

func TestGetType(t *testing.T) {
	tests := []struct {
		name     string
		dbColumn driver.DBColumn
		expected string
	}{
		{
			name:     "BinaryWithMaxLength",
			dbColumn: driver.DBColumn{TypeName: "binary", Length: -1},
			expected: "binary(max)",
		},
		{
			name:     "VarbinaryWithSpecificLength",
			dbColumn: driver.DBColumn{TypeName: "varbinary", Length: 255},
			expected: "varbinary(255)",
		},
		{
			name:     "CharWithSpecificLength",
			dbColumn: driver.DBColumn{TypeName: "char", Length: 10},
			expected: "char(10)",
		},
		{
			name:     "DecimalWithPrecisionAndScale",
			dbColumn: driver.DBColumn{TypeName: "decimal", Precision: 10, Places: 2},
			expected: "decimal(10,2)",
		},
		{
			name:     "FloatWithPrecision",
			dbColumn: driver.DBColumn{TypeName: "float", Precision: 5},
			expected: "float(5)",
		},
		{
			name:     "DefaultTypeName",
			dbColumn: driver.DBColumn{TypeName: "int"},
			expected: "int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getType(tt.dbColumn)
			assert.Equal(t, tt.expected, result)
		})
	}
}
