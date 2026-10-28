package sqlserver

import (
	"testing"

	"github.com/gradii/framework/contracts/database/driver"

	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"github.com/gradii/framework/foundation/json"
	mocksdriver "github.com/gradii/framework/mocks/database/driver"
	mocksfoundation "github.com/gradii/framework/mocks/foundation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type GrammarSuite struct {
	suite.Suite
	grammar *Grammar
}

func TestGrammarSuite(t *testing.T) {
	suite.Run(t, &GrammarSuite{})
}

func (s *GrammarSuite) SetupTest() {
	s.grammar = NewGrammar("goravel_")
}

func (s *GrammarSuite) TestCompileAdd() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	mockColumn.EXPECT().GetName().Return("name").Once()
	mockColumn.EXPECT().GetType().Return("string").Twice()
	mockColumn.EXPECT().GetDefault().Return("goravel").Twice()
	mockColumn.EXPECT().GetNullable().Return(false).Once()
	mockColumn.EXPECT().GetLength().Return(1).Once()
	mockColumn.EXPECT().IsChange().Return(false).Twice()

	sql := s.grammar.CompileAdd(mockBlueprint, &driver.Command{
		Column: mockColumn,
	})

	s.Equal(`alter table "goravel_users" add "name" nvarchar(1) default 'goravel' not null`, sql)
}

func (s *GrammarSuite) TestCompileChange() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	mockBlueprint.EXPECT().GetTableName().Return("users").Twice()
	mockColumn.EXPECT().GetName().Return("name").Twice()
	mockColumn.EXPECT().GetType().Return("string").Once()
	mockColumn.EXPECT().GetNullable().Return(false).Once()
	mockColumn.EXPECT().GetLength().Return(1).Once()
	mockColumn.EXPECT().IsChange().Return(true).Times(3)

	sql := s.grammar.CompileChange(mockBlueprint, &driver.Command{
		Column: mockColumn,
	})

	s.Equal([]string{
		`DECLARE @sql NVARCHAR(MAX) = '';SELECT @sql += 'ALTER TABLE "goravel_users" DROP CONSTRAINT ' + OBJECT_NAME([default_object_id]) + ';' FROM sys.columns WHERE [object_id] = OBJECT_ID('"goravel_users"') AND [name] in ('name') AND [default_object_id] <> 0;EXEC(@sql);`,
		`alter table "goravel_users" alter column "name" nvarchar(1) not null`,
	}, sql)
}

func (s *GrammarSuite) TestCompileColumns() {
	tests := []struct {
		name          string
		table         string
		expectedSQL   string
		expectedError error
	}{
		{
			name:  "with schema",
			table: "users",
			expectedSQL: `select col.name, type.name as type_name, ` +
				`col.max_length as length, col.precision as precision, col.scale as places, ` +
				`col.is_nullable as nullable, def.definition as [default], ` +
				`col.is_identity as autoincrement, col.collation_name as collation, ` +
				`com.definition as [expression], is_persisted as [persisted], ` +
				`cast(prop.value as nvarchar(max)) as comment ` +
				`from sys.columns as col ` +
				`join sys.types as type on col.user_type_id = type.user_type_id ` +
				`join sys.objects as obj on col.object_id = obj.object_id ` +
				`join sys.schemas as scm on obj.schema_id = scm.schema_id ` +
				`left join sys.default_constraints def on col.default_object_id = def.object_id and col.object_id = def.parent_object_id ` +
				`left join sys.extended_properties as prop on obj.object_id = prop.major_id and col.column_id = prop.minor_id and prop.name = 'MS_Description' ` +
				`left join sys.computed_columns as com on col.column_id = com.column_id and col.object_id = com.object_id ` +
				`where obj.type in ('U', 'V') and obj.name = 'goravel_users' and scm.name = schema_name() ` +
				`order by col.column_id`,
			expectedError: nil,
		},
		{
			name:  "without schema",
			table: "users",
			expectedSQL: `select col.name, type.name as type_name, ` +
				`col.max_length as length, col.precision as precision, col.scale as places, ` +
				`col.is_nullable as nullable, def.definition as [default], ` +
				`col.is_identity as autoincrement, col.collation_name as collation, ` +
				`com.definition as [expression], is_persisted as [persisted], ` +
				`cast(prop.value as nvarchar(max)) as comment ` +
				`from sys.columns as col ` +
				`join sys.types as type on col.user_type_id = type.user_type_id ` +
				`join sys.objects as obj on col.object_id = obj.object_id ` +
				`join sys.schemas as scm on obj.schema_id = scm.schema_id ` +
				`left join sys.default_constraints def on col.default_object_id = def.object_id and col.object_id = def.parent_object_id ` +
				`left join sys.extended_properties as prop on obj.object_id = prop.major_id and col.column_id = prop.minor_id and prop.name = 'MS_Description' ` +
				`left join sys.computed_columns as com on col.column_id = com.column_id and col.object_id = com.object_id ` +
				`where obj.type in ('U', 'V') and obj.name = 'goravel_users' and scm.name = schema_name() ` +
				`order by col.column_id`,
			expectedError: nil,
		},
		{
			name:          "empty table name",
			table:         "",
			expectedSQL:   "",
			expectedError: errors.SchemaEmptyReferenceString,
		},
		{
			name:          "invalid table format",
			table:         "schema.table.extra",
			expectedSQL:   "",
			expectedError: errors.SchemaErrorReferenceFormat,
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			sql, err := s.grammar.CompileColumns("", test.table)
			s.Equal(test.expectedError, err)
			s.Equal(test.expectedSQL, sql)
		})
	}
}

func (s *GrammarSuite) TestCompileCreate() {
	mockColumn1 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn2 := mocksdriver.NewColumnDefinition(s.T())
	mockBlueprint := mocksdriver.NewBlueprint(s.T())

	// grammar.go::CompileCreate
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	// utils.go::getColumns
	mockBlueprint.EXPECT().GetAddedColumns().Return([]driver.ColumnDefinition{
		mockColumn1, mockColumn2,
	}).Once()
	// utils.go::getColumns
	mockColumn1.EXPECT().GetName().Return("id").Once()
	// utils.go::getType
	mockColumn1.EXPECT().GetType().Return("integer").Once()
	// grammar.go::TypeInteger
	mockColumn1.EXPECT().GetAutoIncrement().Return(true).Once()
	// grammar.go::ModifyDefault
	mockColumn1.EXPECT().GetDefault().Return(nil).Once()
	// grammar.go::ModifyIncrement
	mockBlueprint.EXPECT().HasCommand("primary").Return(false).Once()
	mockColumn1.EXPECT().GetType().Return("integer").Once()
	// grammar.go::ModifyNullable
	mockColumn1.EXPECT().GetNullable().Return(false).Once()
	mockColumn1.EXPECT().IsChange().Return(false).Twice()

	// utils.go::getColumns
	mockColumn2.EXPECT().GetName().Return("name").Once()
	// utils.go::getType
	mockColumn2.EXPECT().GetType().Return("string").Once()
	// grammar.go::TypeString
	mockColumn2.EXPECT().GetLength().Return(100).Once()
	// grammar.go::ModifyDefault
	mockColumn2.EXPECT().GetDefault().Return(nil).Once()
	// grammar.go::ModifyIncrement
	mockColumn2.EXPECT().GetType().Return("string").Once()
	// grammar.go::ModifyNullable
	mockColumn2.EXPECT().GetNullable().Return(true).Once()
	mockColumn2.EXPECT().IsChange().Return(false).Twice()

	s.Equal(`create table "goravel_users" ("id" int identity primary key not null, "name" nvarchar(100) null)`,
		s.grammar.CompileCreate(mockBlueprint))
}

func (s *GrammarSuite) TestCompileDefault() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumnDefinition := mocksdriver.NewColumnDefinition(s.T())

	mockColumnDefinition.EXPECT().IsChange().Return(true).Once()
	mockColumnDefinition.EXPECT().GetDefault().Return("default").Twice()
	mockColumnDefinition.EXPECT().GetName().Return("id").Once()
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	sql := s.grammar.CompileDefault(mockBlueprint, &driver.Command{
		Column: mockColumnDefinition,
	})

	s.Equal(`alter table "goravel_users" add default 'default' for "id"`, sql)
}

func (s *GrammarSuite) TestCompileDropColumn() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Twice()

	s.Equal([]string{`DECLARE @sql NVARCHAR(MAX) = '';SELECT @sql += 'ALTER TABLE "goravel_users" DROP CONSTRAINT ' + OBJECT_NAME([default_object_id]) + ';' FROM sys.columns WHERE [object_id] = OBJECT_ID('"goravel_users"') AND [name] in ('id','name') AND [default_object_id] <> 0;EXEC(@sql); alter table "goravel_users" drop column "id", "name"`}, s.grammar.CompileDropColumn(mockBlueprint, &driver.Command{
		Columns: []string{"id", "name"},
	}))
}

func (s *GrammarSuite) TestCompileDropIfExists() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	s.Equal(`if object_id('"goravel_users"', 'U') is not null drop table "goravel_users"`, s.grammar.CompileDropIfExists(mockBlueprint))
}

func (s *GrammarSuite) TestCompileForeign() {
	var mockBlueprint *mocksdriver.Blueprint

	beforeEach := func() {
		mockBlueprint = mocksdriver.NewBlueprint(s.T())
		mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	}

	tests := []struct {
		name      string
		command   *driver.Command
		expectSql string
	}{
		{
			name: "with on delete and on update",
			command: &driver.Command{
				Index:      "fk_users_role_id",
				Columns:    []string{"role_id", "user_id"},
				On:         "roles",
				References: []string{"id", "user_id"},
				OnDelete:   "cascade",
				OnUpdate:   "restrict",
			},
			expectSql: `alter table "goravel_users" add constraint "fk_users_role_id" foreign key ("role_id", "user_id") references "goravel_roles" ("id", "user_id") on delete cascade on update restrict`,
		},
		{
			name: "without on delete and on update",
			command: &driver.Command{
				Index:      "fk_users_role_id",
				Columns:    []string{"role_id", "user_id"},
				On:         "roles",
				References: []string{"id", "user_id"},
			},
			expectSql: `alter table "goravel_users" add constraint "fk_users_role_id" foreign key ("role_id", "user_id") references "goravel_roles" ("id", "user_id")`,
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			beforeEach()

			sql := s.grammar.CompileForeign(mockBlueprint, test.command)
			s.Equal(test.expectSql, sql)
		})
	}
}

func (s *GrammarSuite) TestCompileIndex() {
	var mockBlueprint *mocksdriver.Blueprint

	beforeEach := func() {
		mockBlueprint = mocksdriver.NewBlueprint(s.T())
		mockBlueprint.EXPECT().GetTableName().Return("users").Once()
	}

	tests := []struct {
		name      string
		command   *driver.Command
		expectSql string
	}{
		{
			name: "with Algorithm",
			command: &driver.Command{
				Index:     "fk_users_role_id",
				Columns:   []string{"role_id", "user_id"},
				Algorithm: "btree",
			},
			expectSql: `create index "fk_users_role_id" on "goravel_users" ("role_id", "user_id")`,
		},
		{
			name: "without Algorithm",
			command: &driver.Command{
				Index:   "fk_users_role_id",
				Columns: []string{"role_id", "user_id"},
			},
			expectSql: `create index "fk_users_role_id" on "goravel_users" ("role_id", "user_id")`,
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			beforeEach()

			sql := s.grammar.CompileIndex(mockBlueprint, test.command)
			s.Equal(test.expectSql, sql)
		})
	}
}

func (s *GrammarSuite) TestCompileJsonColumnsUpdate() {
	tests := []struct {
		name           string
		values         map[string]any
		expectedValues []map[string]any
		hasError       bool
	}{
		{
			name: "invalid values",
			values: map[string]any{"data->invalid": map[string]any{
				"value": func() {},
			}},
			hasError: true,
		},
		{
			name:   "update boolean value",
			values: map[string]any{"data->bool": true},
			expectedValues: []map[string]any{
				{
					"data": legacyRaw(
						"json_modify(?,?,?)",
						legacyRaw(`"data"`),
						`$."bool"`, legacyRaw("cast(1 as bit)"),
					),
				},
			},
		},
		{
			name:   "update float value",
			values: map[string]any{"data->float": 456.789},
			expectedValues: []map[string]any{
				{
					"data": legacyRaw(
						"json_modify(?,?,?)",
						legacyRaw(`"data"`),
						`$."float"`, legacyRaw("cast(? as decimal(6,3))", "456.789"),
					),
				},
			},
		},
		{
			name:   "update single json column",
			values: map[string]any{"data->details": "details value"},
			expectedValues: []map[string]any{
				{"data": legacyRaw("json_modify(?,?,?)", legacyRaw(`"data"`), `$."details"`, "details value")},
			},
		},
		{
			name:   "update single json column(with nested path)",
			values: map[string]any{"data->details->subdetails[0]": "subdetails value"},
			expectedValues: []map[string]any{
				{"data": legacyRaw("json_modify(?,?,?)", legacyRaw(`"data"`), `$."details"."subdetails"[0]`, "subdetails value")},
			},
		},
		{
			name:   "update multiple json columns",
			values: map[string]any{"data->details": "details value", "data->info": "info value"},
			expectedValues: []map[string]any{
				{
					"data": legacyRaw(
						"json_modify(?,?,?)",
						legacyRaw(
							"json_modify(?,?,?)",
							legacyRaw(`"data"`),
							`$."details"`, "details value",
						),
						`$."info"`, "info value",
					),
				},
				{
					"data": legacyRaw(
						"json_modify(?,?,?)",
						legacyRaw(
							"json_modify(?,?,?)",
							legacyRaw(`"data"`),
							`$."info"`, "info value",
						),
						`$."details"`, "details value",
					),
				},
			},
		},
	}

	mockApp := mocksfoundation.NewApplication(s.T())

	originApp := App
	App = mockApp
	s.T().Cleanup(func() {
		App = originApp
	})

	for _, tt := range tests {
		s.Run(tt.name, func() {
			mockApp.EXPECT().GetJson().Return(json.New()).Once()
			actualValues, err := s.grammar.CompileJsonColumnsUpdate(tt.values)
			if tt.hasError {
				s.Error(err)
			} else {
				s.Subset(tt.expectedValues, []any{actualValues})
				s.NoError(err)
			}
		})
	}
}

func (s *GrammarSuite) TestCompileJsonContains() {
	tests := []struct {
		name          string
		column        string
		value         any
		isNot         bool
		expectedSql   string
		expectedValue []any
	}{
		{
			name:          "single path with single value",
			column:        "data->details",
			value:         "value1",
			expectedSql:   `? in (select "value" from openjson("data", '$."details"'))`,
			expectedValue: []any{"value1"},
		},
		{
			name:          "single path with multiple values",
			column:        "data->details",
			value:         []string{"value1", "value2"},
			expectedSql:   `? in (select "value" from openjson("data", '$."details"')) AND ? in (select "value" from openjson("data", '$."details"'))`,
			expectedValue: []any{"value1", "value2"},
		},
		{
			name:          "nested path with single value",
			column:        "data->details->subdetails[0]",
			value:         "value1",
			expectedSql:   `? in (select "value" from openjson("data", '$."details"."subdetails"[0]'))`,
			expectedValue: []any{"value1"},
		},
		{
			name:          "nested path with multiple values",
			column:        "data->details[0]->subdetails",
			value:         []string{"value1", "value2"},
			expectedSql:   `? in (select "value" from openjson("data", '$."details"[0]."subdetails"')) AND ? in (select "value" from openjson("data", '$."details"[0]."subdetails"'))`,
			expectedValue: []any{"value1", "value2"},
		},
		{
			name:          "with is not condition",
			column:        "data->details",
			value:         "value1",
			isNot:         true,
			expectedSql:   `not ? in (select "value" from openjson("data", '$."details"'))`,
			expectedValue: []any{"value1"},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			actualSql, actualValue, err := s.grammar.CompileJsonContains(tt.column, tt.value, tt.isNot)
			s.Equal(tt.expectedSql, actualSql)
			s.Equal(tt.expectedValue, actualValue)
			s.NoError(err)
		})
	}
}

func (s *GrammarSuite) TestCompileJsonContainKey() {
	tests := []struct {
		name        string
		column      string
		isNot       bool
		expectedSql string
	}{
		{
			name:        "single path",
			column:      "data->details",
			expectedSql: `'details' in (select "key" from openjson("data"))`,
		},
		{
			name:        "single path with is not",
			column:      "data->details",
			isNot:       true,
			expectedSql: `not 'details' in (select "key" from openjson("data"))`,
		},
		{
			name:        "nested path",
			column:      "data->details->subdetails",
			expectedSql: `'subdetails' in (select "key" from openjson("data", '$."details"'))`,
		},
		{
			name:        "nested path with array index",
			column:      "data->details[0]->subdetails",
			expectedSql: `'subdetails' in (select "key" from openjson("data", '$."details"[0]'))`,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expectedSql, s.grammar.CompileJsonContainsKey(tt.column, tt.isNot))
		})
	}
}

func (s *GrammarSuite) TestCompileJsonLength() {
	tests := []struct {
		name        string
		column      string
		expectedSql string
	}{
		{
			name:        "single path",
			column:      "data->details",
			expectedSql: `(select count(*) from openjson("data", '$."details"'))`,
		},
		{
			name:        "nested path",
			column:      "data->details->subdetails",
			expectedSql: `(select count(*) from openjson("data", '$."details"."subdetails"'))`,
		},
		{
			name:        "nested path with array index",
			column:      "data->details[0]->subdetails",
			expectedSql: `(select count(*) from openjson("data", '$."details"[0]."subdetails"'))`,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expectedSql, s.grammar.CompileJsonLength(tt.column))
		})
	}
}

func (s *GrammarSuite) TestCompilePrimary() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	s.Equal(`alter table "goravel_users" add constraint "role" primary key ("role_id", "user_id")`, s.grammar.CompilePrimary(mockBlueprint, &driver.Command{
		Columns: []string{"role_id", "user_id"},
		Index:   "role",
	}))
}

func (s *GrammarSuite) TestCompileRenameColumn() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	mockBlueprint.EXPECT().GetTableName().Return("users").Once()

	sql, err := s.grammar.CompileRenameColumn(mockBlueprint, &driver.Command{
		Column: mockColumn,
		From:   "before",
		To:     "after",
	}, nil)

	s.NoError(err)
	s.Equal(`sp_rename '"goravel_users"."before"', "after", N'COLUMN'`, sql)
}

func (s *GrammarSuite) TestGetColumns() {
	mockColumn1 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn2 := mocksdriver.NewColumnDefinition(s.T())
	mockBlueprint := mocksdriver.NewBlueprint(s.T())

	mockBlueprint.EXPECT().GetAddedColumns().Return([]driver.ColumnDefinition{
		mockColumn1, mockColumn2,
	}).Once()
	mockBlueprint.EXPECT().HasCommand("primary").Return(false).Once()

	mockColumn1.EXPECT().GetName().Return("id").Once()
	mockColumn1.EXPECT().GetType().Return("integer").Twice()
	mockColumn1.EXPECT().GetDefault().Return(nil).Once()
	mockColumn1.EXPECT().GetNullable().Return(false).Once()
	mockColumn1.EXPECT().GetAutoIncrement().Return(true).Once()
	mockColumn1.EXPECT().IsChange().Return(false).Twice()

	mockColumn2.EXPECT().GetName().Return("name").Once()
	mockColumn2.EXPECT().GetType().Return("string").Twice()
	mockColumn2.EXPECT().GetDefault().Return("goravel").Twice()
	mockColumn2.EXPECT().GetNullable().Return(true).Once()
	mockColumn2.EXPECT().GetLength().Return(10).Once()
	mockColumn2.EXPECT().IsChange().Return(false).Twice()

	s.Equal([]string{`"id" int identity primary key not null`, `"name" nvarchar(10) default 'goravel' null`}, s.grammar.getColumns(mockBlueprint))
}

func (s *GrammarSuite) TestModifyDefault() {
	var (
		mockBlueprint *mocksdriver.Blueprint
		mockColumn    *mocksdriver.ColumnDefinition
	)

	tests := []struct {
		name      string
		setup     func()
		expectSql string
	}{
		{
			name: "without change and default is nil",
			setup: func() {
				mockColumn.EXPECT().IsChange().Return(false).Once()
				mockColumn.EXPECT().GetDefault().Return(nil).Once()
			},
		},
		{
			name: "without change and default is not nil",
			setup: func() {
				mockColumn.EXPECT().IsChange().Return(false).Once()
				mockColumn.EXPECT().GetDefault().Return("goravel").Twice()
			},
			expectSql: " default 'goravel'",
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			mockBlueprint = mocksdriver.NewBlueprint(s.T())
			mockColumn = mocksdriver.NewColumnDefinition(s.T())

			test.setup()

			sql := s.grammar.ModifyDefault(mockBlueprint, mockColumn)

			s.Equal(test.expectSql, sql)
		})
	}
}

func (s *GrammarSuite) TestModifyNullable() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetNullable().Return(true).Once()

	s.Equal(" null", s.grammar.ModifyNullable(mockBlueprint, mockColumn))

	mockColumn.EXPECT().GetNullable().Return(false).Once()

	s.Equal(" not null", s.grammar.ModifyNullable(mockBlueprint, mockColumn))
}

func (s *GrammarSuite) TestModifyIncrement() {
	mockBlueprint := mocksdriver.NewBlueprint(s.T())

	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockBlueprint.EXPECT().HasCommand("primary").Return(false).Once()
	mockColumn.EXPECT().GetType().Return("bigInteger").Once()
	mockColumn.EXPECT().GetAutoIncrement().Return(true).Once()
	mockColumn.EXPECT().IsChange().Return(false).Once()

	s.Equal(" identity primary key", s.grammar.ModifyIncrement(mockBlueprint, mockColumn))
}

func (s *GrammarSuite) TestTypeBoolean() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	s.Equal("bit", s.grammar.TypeBoolean(mockColumn))
}

func (s *GrammarSuite) TestTypeDecimal() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetTotal().Return(4).Once()
	mockColumn.EXPECT().GetPlaces().Return(2).Once()

	s.Equal("decimal(4, 2)", s.grammar.TypeDecimal(mockColumn))
}

func (s *GrammarSuite) TestTypeEnum() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetName().Return("a").Once()
	mockColumn.EXPECT().GetAllowed().Return([]any{"a", "b"}).Once()

	s.Equal(`nvarchar(255) check ("a" in (N'a', N'b'))`, s.grammar.TypeEnum(mockColumn))
}

func (s *GrammarSuite) TestTypeFloat() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetPrecision().Return(0).Once()

	s.Equal("float", s.grammar.TypeFloat(mockColumn))

	mockColumn.EXPECT().GetPrecision().Return(2).Once()

	s.Equal("float(2)", s.grammar.TypeFloat(mockColumn))
}

func (s *GrammarSuite) TestTypeString() {
	mockColumn1 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn1.EXPECT().GetLength().Return(100).Once()

	s.Equal("nvarchar(100)", s.grammar.TypeString(mockColumn1))

	mockColumn2 := mocksdriver.NewColumnDefinition(s.T())
	mockColumn2.EXPECT().GetLength().Return(0).Once()

	s.Equal("nvarchar(255)", s.grammar.TypeString(mockColumn2))
}

func (s *GrammarSuite) TestTypeTimestamp() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetUseCurrent().Return(true).Once()
	mockColumn.EXPECT().Default(schema.Expression("CURRENT_TIMESTAMP")).Return(mockColumn).Once()
	mockColumn.EXPECT().GetPrecision().Return(3).Twice()
	s.Equal("datetime2(3)", s.grammar.TypeTimestamp(mockColumn))

	mockColumn = mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetUseCurrent().Return(true).Once()
	mockColumn.EXPECT().Default(schema.Expression("CURRENT_TIMESTAMP")).Return(mockColumn).Once()
	mockColumn.EXPECT().GetPrecision().Return(0).Once()
	s.Equal("datetime", s.grammar.TypeTimestamp(mockColumn))
}

func (s *GrammarSuite) TestTypeTimestampTz() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetUseCurrent().Return(true).Once()
	mockColumn.EXPECT().Default(schema.Expression("CURRENT_TIMESTAMP")).Return(mockColumn).Once()
	mockColumn.EXPECT().GetPrecision().Return(3).Twice()
	s.Equal("datetimeoffset(3)", s.grammar.TypeTimestampTz(mockColumn))

	mockColumn = mocksdriver.NewColumnDefinition(s.T())
	mockColumn.EXPECT().GetUseCurrent().Return(true).Once()
	mockColumn.EXPECT().Default(schema.Expression("CURRENT_TIMESTAMP")).Return(mockColumn).Once()
	mockColumn.EXPECT().GetPrecision().Return(0).Once()
	s.Equal("datetimeoffset", s.grammar.TypeTimestampTz(mockColumn))
}

func (s *GrammarSuite) TestTypeUuid() {
	mockColumn := mocksdriver.NewColumnDefinition(s.T())

	s.Equal("uniqueidentifier", s.grammar.TypeUuid(mockColumn))
}

func TestParseSchemaAndTable(t *testing.T) {
	tests := []struct {
		reference      string
		defaultSchema  string
		expectedSchema string
		expectedTable  string
		expectedError  error
	}{
		{"public.users", "public", "public", "users", nil},
		{"users", "goravel", "goravel", "users", nil},
		{"", "", "", "", errors.SchemaEmptyReferenceString},
		{"public.users.extra", "", "", "", errors.SchemaErrorReferenceFormat},
	}

	for _, test := range tests {
		schema, table, err := parseSchemaAndTable(test.reference, test.defaultSchema)
		assert.Equal(t, test.expectedSchema, schema)
		assert.Equal(t, test.expectedTable, table)
		assert.Equal(t, test.expectedError, err)
	}
}
