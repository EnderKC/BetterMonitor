package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const LegacyUsersBackupTable = "users_backup_single_admin"

type AdminBootstrap struct {
	Username     string
	Password     string
	PasswordFile string
}

type legacyUserRow struct {
	ID          uint
	Username    string
	Password    string
	Email       string
	Phone       string
	Role        string
	LastLoginAt time.Time
	DeletedAt   *time.Time
}

func MigrateSingleAdmin(db *gorm.DB, provider func() (AdminBootstrap, error)) error {
	if db == nil {
		return errors.New("migrate single administrator: database is nil")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		hasLegacyUsers := tx.Migrator().HasTable("users")

		if err := tx.AutoMigrate(&AdminAccount{}); err != nil {
			return fmt.Errorf("create admin_accounts table: %w", err)
		}

		var accountCount int64
		if err := tx.Model(&AdminAccount{}).Count(&accountCount).Error; err != nil {
			return fmt.Errorf("count administrator accounts: %w", err)
		}
		if accountCount > 1 {
			return fmt.Errorf("expected one administrator account, found %d", accountCount)
		}

		var admin AdminAccount
		if accountCount == 1 {
			if err := tx.First(&admin, SingletonAdminID).Error; err != nil {
				return fmt.Errorf("load singleton administrator: %w", err)
			}
		} else {
			candidate, found, err := selectLegacyAdministrator(tx, hasLegacyUsers)
			if err != nil {
				return err
			}

			admin = AdminAccount{
				ID:             SingletonAdminID,
				SessionVersion: 1,
			}
			if found {
				admin.Username = strings.TrimSpace(candidate.Username)
				admin.Password = candidate.Password
				admin.Email = candidate.Email
				admin.Phone = candidate.Phone
				admin.LastLoginAt = candidate.LastLoginAt
			} else {
				if provider == nil {
					return errors.New("bootstrap administrator provider is required")
				}
				bootstrap, err := provider()
				if err != nil {
					return fmt.Errorf("resolve bootstrap administrator: %w", err)
				}
				admin.Username = strings.TrimSpace(bootstrap.Username)
				admin.Password, err = HashPassword(bootstrap.Password)
				if err != nil {
					return fmt.Errorf("hash bootstrap administrator password: %w", err)
				}
			}

			if err := validateAdminAccount(&admin); err != nil {
				return err
			}
			if err := tx.Create(&admin).Error; err != nil {
				return fmt.Errorf("create singleton administrator: %w", err)
			}
		}

		if err := validateAdminAccount(&admin); err != nil {
			return err
		}

		if hasLegacyUsers {
			if err := tx.Migrator().RenameTable("users", LegacyUsersBackupTable); err != nil {
				return fmt.Errorf("backup legacy users table: %w", err)
			}
		}

		return nil
	})
}

func selectLegacyAdministrator(tx *gorm.DB, hasLegacyUsers bool) (legacyUserRow, bool, error) {
	if !hasLegacyUsers {
		return legacyUserRow{}, false, nil
	}

	var candidate legacyUserRow
	err := tx.Table("users").
		Where("deleted_at IS NULL").
		Order("CASE WHEN role = 'admin' THEN 0 ELSE 1 END").
		Order("id ASC").
		Limit(1).
		Take(&candidate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return legacyUserRow{}, false, nil
	}
	if err != nil {
		return legacyUserRow{}, false, fmt.Errorf("select legacy administrator: %w", err)
	}
	return candidate, true, nil
}

func validateAdminAccount(admin *AdminAccount) error {
	if admin == nil || admin.ID != SingletonAdminID {
		return errors.New("administrator must use singleton id 1")
	}
	if strings.TrimSpace(admin.Username) == "" {
		return errors.New("administrator username is required")
	}
	if strings.TrimSpace(admin.Password) == "" {
		return errors.New("administrator password hash is required")
	}
	if admin.SessionVersion == 0 {
		return errors.New("administrator session version must be positive")
	}
	return nil
}
