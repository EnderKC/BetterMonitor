package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPrivacyMigrationsPreserveExistingValuesAndDefaultNewRowsPrivate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:privacy_defaults?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE servers (
		id integer primary key autoincrement,
		name text not null,
		allow_public_view numeric default true
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE life_probes (
		id integer primary key autoincrement,
		name text not null,
		device_id text not null,
		allow_public_view numeric default true
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE system_settings (
		id integer primary key autoincrement,
		allow_public_life_probe_access numeric default true
	)`).Error)

	require.NoError(t, db.Exec(`INSERT INTO servers(id,name,allow_public_view) VALUES
		(1,'public-server',true),(2,'private-server',false)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO life_probes(id,name,device_id,allow_public_view) VALUES
		(1,'public-probe','public-device',true),(2,'private-probe','private-device',false)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO system_settings(id,allow_public_life_probe_access) VALUES
		(1,true),(2,false)`).Error)

	require.NoError(t, db.AutoMigrate(&Server{}, &LifeProbe{}, &SystemSettings{}))

	var servers []Server
	require.NoError(t, db.Order("id").Find(&servers).Error)
	require.Len(t, servers, 2)
	assert.True(t, servers[0].AllowPublicView)
	assert.False(t, servers[1].AllowPublicView)

	var probes []LifeProbe
	require.NoError(t, db.Order("id").Find(&probes).Error)
	require.Len(t, probes, 2)
	assert.True(t, probes[0].AllowPublicView)
	assert.False(t, probes[1].AllowPublicView)

	var settings []SystemSettings
	require.NoError(t, db.Order("id").Find(&settings).Error)
	require.Len(t, settings, 2)
	assert.True(t, settings[0].AllowPublicLifeProbeAccess)
	assert.False(t, settings[1].AllowPublicLifeProbeAccess)

	newServer := Server{Name: "new-server"}
	require.NoError(t, db.Create(&newServer).Error)
	assert.False(t, newServer.AllowPublicView)

	newProbe := LifeProbe{Name: "new-probe", DeviceID: "new-device"}
	require.NoError(t, db.Create(&newProbe).Error)
	assert.False(t, newProbe.AllowPublicView)

	newSettings := SystemSettings{}
	require.NoError(t, db.Create(&newSettings).Error)
	assert.False(t, newSettings.AllowPublicLifeProbeAccess)
}
