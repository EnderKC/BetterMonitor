package models

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAdminMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	return db
}

func createLegacyUsersTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`CREATE TABLE users (
		id integer primary key autoincrement,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime,
		username text not null,
		password text not null,
		email text,
		phone text,
		role text,
		last_login_at datetime
	)`).Error)
}

func failIfBootstrapCalled(t *testing.T) func() (AdminBootstrap, error) {
	t.Helper()
	return func() (AdminBootstrap, error) {
		t.Fatal("bootstrap provider must not be called when legacy account data exists")
		return AdminBootstrap{}, nil
	}
}

func TestMigrateSingleAdminPrefersEarliestAdmin(t *testing.T) {
	db := newAdminMigrationDB(t)
	createLegacyUsersTable(t, db)
	require.NoError(t, db.Exec(`INSERT INTO users(id, username, password, email, phone, role) VALUES
		(1, 'first-user', 'hash-user', 'user@example.com', '100', 'user'),
		(2, 'first-admin', 'hash-admin', 'admin@example.com', '200', 'admin'),
		(3, 'second-admin', 'hash-admin-2', 'admin2@example.com', '300', 'admin')`).Error)

	require.NoError(t, MigrateSingleAdmin(db, failIfBootstrapCalled(t)))

	var admin AdminAccount
	require.NoError(t, db.First(&admin, SingletonAdminID).Error)
	assert.Equal(t, SingletonAdminID, admin.ID)
	assert.Equal(t, "first-admin", admin.Username)
	assert.Equal(t, "hash-admin", admin.Password)
	assert.Equal(t, "admin@example.com", admin.Email)
	assert.Equal(t, "200", admin.Phone)
	assert.Equal(t, uint64(1), admin.SessionVersion)
	assert.False(t, db.Migrator().HasTable("users"))
	assert.True(t, db.Migrator().HasTable(LegacyUsersBackupTable))

	var backupCount int64
	require.NoError(t, db.Table(LegacyUsersBackupTable).Count(&backupCount).Error)
	assert.Equal(t, int64(3), backupCount)
}

func TestMigrateSingleAdminFallsBackToEarliestUser(t *testing.T) {
	db := newAdminMigrationDB(t)
	createLegacyUsersTable(t, db)
	require.NoError(t, db.Exec(`INSERT INTO users(id, username, password, role) VALUES
		(4, 'later-user', 'hash-later', 'user'),
		(2, 'earliest-user', 'hash-earliest', 'user')`).Error)

	require.NoError(t, MigrateSingleAdmin(db, failIfBootstrapCalled(t)))

	var admin AdminAccount
	require.NoError(t, db.First(&admin, SingletonAdminID).Error)
	assert.Equal(t, "earliest-user", admin.Username)
	assert.Equal(t, "hash-earliest", admin.Password)
}

func TestMigrateSingleAdminBootstrapsEmptyDatabase(t *testing.T) {
	db := newAdminMigrationDB(t)
	providerCalls := 0

	err := MigrateSingleAdmin(db, func() (AdminBootstrap, error) {
		providerCalls++
		return AdminBootstrap{
			Username:     "fresh-admin",
			Password:     "generated-password",
			PasswordFile: "/tmp/admin-password",
		}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, providerCalls)

	var admin AdminAccount
	require.NoError(t, db.First(&admin, SingletonAdminID).Error)
	assert.Equal(t, "fresh-admin", admin.Username)
	assert.NotEqual(t, "generated-password", admin.Password)
	assert.True(t, admin.CheckPassword("generated-password"))
	assert.Equal(t, uint64(1), admin.SessionVersion)
	assert.False(t, db.Migrator().HasTable("users"))
}

func TestMigrateSingleAdminRollsBackWhenLegacyBackupConflicts(t *testing.T) {
	db := newAdminMigrationDB(t)
	createLegacyUsersTable(t, db)
	require.NoError(t, db.Exec(`INSERT INTO users(id, username, password, role) VALUES
		(1, 'admin', 'hash-admin', 'admin')`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE `+LegacyUsersBackupTable+` (id integer primary key)`).Error)

	err := MigrateSingleAdmin(db, failIfBootstrapCalled(t))
	require.Error(t, err)

	assert.True(t, db.Migrator().HasTable("users"))
	var legacyCount int64
	require.NoError(t, db.Table("users").Count(&legacyCount).Error)
	assert.Equal(t, int64(1), legacyCount)

	if db.Migrator().HasTable(&AdminAccount{}) {
		var adminCount int64
		require.NoError(t, db.Model(&AdminAccount{}).Count(&adminCount).Error)
		assert.Zero(t, adminCount)
	}
}

func TestMigrateSingleAdminIsIdempotent(t *testing.T) {
	db := newAdminMigrationDB(t)
	createLegacyUsersTable(t, db)
	require.NoError(t, db.Exec(`INSERT INTO users(id, username, password, role) VALUES
		(1, 'admin', 'hash-admin', 'admin')`).Error)

	require.NoError(t, MigrateSingleAdmin(db, failIfBootstrapCalled(t)))
	require.NoError(t, MigrateSingleAdmin(db, failIfBootstrapCalled(t)))

	var count int64
	require.NoError(t, db.Model(&AdminAccount{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	var backupCount int64
	require.NoError(t, db.Table(LegacyUsersBackupTable).Count(&backupCount).Error)
	assert.Equal(t, int64(1), backupCount)
}
