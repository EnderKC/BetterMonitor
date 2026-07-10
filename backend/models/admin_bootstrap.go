package models

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/user/server-ops-backend/config"
)

const minimumAdminPasswordLength = 12

var adminUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$`)

func ValidateAdminUsername(username string) error {
	if !adminUsernamePattern.MatchString(strings.TrimSpace(username)) {
		return errors.New("invalid administrator username")
	}
	return nil
}

func ResolveAdminBootstrap(cfg *config.Config) (AdminBootstrap, error) {
	if cfg == nil {
		return AdminBootstrap{}, errors.New("administrator bootstrap config is required")
	}

	username := strings.TrimSpace(cfg.AdminUsername)
	if err := ValidateAdminUsername(username); err != nil {
		return AdminBootstrap{}, err
	}

	passwordFile := strings.TrimSpace(cfg.AdminPasswordFile)
	if cfg.AdminPassword != "" {
		if err := validateAdminBootstrapPassword(cfg.AdminPassword); err != nil {
			return AdminBootstrap{}, err
		}
		return AdminBootstrap{
			Username:     username,
			Password:     cfg.AdminPassword,
			PasswordFile: passwordFile,
		}, nil
	}

	if passwordFile == "" {
		return AdminBootstrap{}, errors.New("administrator password file is required")
	}

	password, err := readAdminPasswordFile(passwordFile)
	if err == nil {
		if err := validateAdminBootstrapPassword(password); err != nil {
			return AdminBootstrap{}, fmt.Errorf("invalid administrator password file: %w", err)
		}
		return AdminBootstrap{Username: username, Password: password, PasswordFile: passwordFile}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return AdminBootstrap{}, fmt.Errorf("read administrator password file: %w", err)
	}

	password, err = generateAdminBootstrapPassword()
	if err != nil {
		return AdminBootstrap{}, err
	}
	if err := config.WritePrivateFileAtomic(passwordFile, []byte(password+"\n")); err != nil {
		return AdminBootstrap{}, fmt.Errorf("write administrator password file: %w", err)
	}
	log.Printf("已生成初始管理员密码文件: %s", passwordFile)

	return AdminBootstrap{Username: username, Password: password, PasswordFile: passwordFile}, nil
}

func validateAdminBootstrapPassword(password string) error {
	if len(password) < minimumAdminPasswordLength {
		return fmt.Errorf("administrator password must be at least %d characters", minimumAdminPasswordLength)
	}
	return nil
}

func readAdminPasswordFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func generateAdminBootstrapPassword() (string, error) {
	randomBytes := make([]byte, 24)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate administrator password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}
