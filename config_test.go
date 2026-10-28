package sqlserver

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	mocksconfig "github.com/gradii/framework/mocks/config"

	"github.com/gradii/sqlserver/contracts"
)

type ConfigTestSuite struct {
	suite.Suite
	config     *Config
	connection string
	mockConfig *mocksconfig.Config
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, &ConfigTestSuite{
		connection: "sqlserver",
	})
}

func (s *ConfigTestSuite) SetupTest() {
	s.mockConfig = mocksconfig.NewConfig(s.T())
	s.config = NewConfig(s.mockConfig, s.connection)
}

func (s *ConfigTestSuite) TestReads() {
	// Test when configs is empty
	s.mockConfig.EXPECT().Get(fmt.Sprintf("database.connections.%s.read", s.connection)).Return(nil).Once()
	s.Nil(s.config.Readers())

	// Test when configs is not empty
	s.mockConfig.EXPECT().Get(fmt.Sprintf("database.connections.%s.read", s.connection)).Return([]contracts.Config{
		{
			Dsn:      "dsn",
			Database: "forge",
			Host:     "localhost",
			Port:     3306,
			Username: "root",
			Password: "123123",
		},
	}).Once()
	s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.prefix", s.connection)).Return("goravel_").Once()
	s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.charset", s.connection)).Return("utf8mb4").Once()
	s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.timezone", s.connection)).Return("UTC").Once()
	s.Equal([]contracts.FullConfig{
		{
			Connection: s.connection,
			Driver:     Name,
			Prefix:     "goravel_",
			Charset:    "utf8mb4",
			Config: contracts.Config{
				Dsn:      "dsn",
				Database: "forge",
				Host:     "localhost",
				Port:     3306,
				Username: "root",
				Password: "123123",
			},
			Timezone: "UTC",
		},
	}, s.config.Readers())
}

func (s *ConfigTestSuite) TestWrites() {
	s.Run("success when configs is empty", func() {
		s.mockConfig.EXPECT().Get(fmt.Sprintf("database.connections.%s.write", s.connection)).Return(nil).Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.prefix", s.connection)).Return("goravel_").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.charset", s.connection)).Return("utf8mb4").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.dsn", s.connection)).Return("dsn").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.host", s.connection)).Return("localhost").Once()
		s.mockConfig.EXPECT().GetInt(fmt.Sprintf("database.connections.%s.port", s.connection)).Return(3306).Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.username", s.connection)).Return("root").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.password", s.connection)).Return("123123").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.database", s.connection)).Return("forge").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.timezone", s.connection)).Return("UTC").Once()

		s.Equal([]contracts.FullConfig{
			{
				Connection: s.connection,
				Driver:     Name,
				Prefix:     "goravel_",
				Charset:    "utf8mb4",
				Config: contracts.Config{
					Dsn:      "dsn",
					Database: "forge",
					Host:     "localhost",
					Port:     3306,
					Username: "root",
					Password: "123123",
				},
				Timezone: "UTC",
			},
		}, s.config.Writers())
	})

	s.Run("success when configs is not empty", func() {
		s.mockConfig.EXPECT().Get(fmt.Sprintf("database.connections.%s.write", s.connection)).Return([]contracts.Config{
			{
				Database: "forge",
			},
		}).Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.prefix", s.connection)).Return("goravel_").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.charset", s.connection)).Return("utf8mb4").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.dsn", s.connection)).Return("dsn").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.host", s.connection)).Return("localhost").Once()
		s.mockConfig.EXPECT().GetInt(fmt.Sprintf("database.connections.%s.port", s.connection)).Return(3306).Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.username", s.connection)).Return("root").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.password", s.connection)).Return("123123").Once()
		s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.timezone", s.connection)).Return("").Once()
		s.mockConfig.EXPECT().GetString("app.timezone", "UTC").Return("Asia/Shanghai").Once()

		s.Equal([]contracts.FullConfig{
			{
				Connection: s.connection,
				Driver:     Name,
				Prefix:     "goravel_",
				Charset:    "utf8mb4",
				Config: contracts.Config{
					Dsn:      "dsn",
					Database: "forge",
					Host:     "localhost",
					Port:     3306,
					Username: "root",
					Password: "123123",
				},
				Timezone: "Asia/Shanghai",
			},
		}, s.config.Writers())
	})
}

func (s *ConfigTestSuite) TestFillDefault() {
	dsn := "dsn"
	host := "localhost"
	port := 3306
	database := "forge"
	username := "root"
	password := "123123"
	prefix := "goravel_"
	charset := "utf8mb4"
	timezone := "UTC"

	tests := []struct {
		name          string
		configs       []contracts.Config
		setup         func()
		expectConfigs []contracts.FullConfig
	}{
		{
			name:    "success when configs is empty",
			setup:   func() {},
			configs: []contracts.Config{},
		},
		{
			name:    "success when configs have item but key is empty",
			configs: []contracts.Config{{}},
			setup: func() {
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.prefix", s.connection)).Return(prefix).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.dsn", s.connection)).Return(dsn).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.host", s.connection)).Return(host).Once()
				s.mockConfig.EXPECT().GetInt(fmt.Sprintf("database.connections.%s.port", s.connection)).Return(port).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.database", s.connection)).Return(database).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.username", s.connection)).Return(username).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.password", s.connection)).Return(password).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.charset", s.connection)).Return(charset).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.timezone", s.connection)).Return(timezone).Once()
			},
			expectConfigs: []contracts.FullConfig{
				{
					Connection: s.connection,
					Driver:     Name,
					Prefix:     prefix,
					Charset:    charset,
					Config: contracts.Config{
						Dsn:      dsn,
						Host:     host,
						Port:     port,
						Database: database,
						Username: username,
						Password: password,
					},
					Timezone: timezone,
				},
			},
		},
		{
			name: "success when configs have item",
			configs: []contracts.Config{
				{
					Dsn:      dsn,
					Host:     host,
					Port:     port,
					Database: database,
					Username: username,
					Password: password,
				},
			},
			setup: func() {
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.prefix", s.connection)).Return(prefix).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.charset", s.connection)).Return(charset).Once()
				s.mockConfig.EXPECT().GetString(fmt.Sprintf("database.connections.%s.timezone", s.connection)).Return(timezone).Once()
			},
			expectConfigs: []contracts.FullConfig{
				{
					Connection: s.connection,
					Driver:     Name,
					Prefix:     prefix,
					Charset:    charset,
					Config: contracts.Config{
						Dsn:      dsn,
						Database: database,
						Host:     host,
						Port:     port,
						Username: username,
						Password: password,
					},
					Timezone: timezone,
				},
			},
		},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			test.setup()
			configs := s.config.fillDefault(test.configs)

			s.Equal(test.expectConfigs, configs)
		})
	}
}
