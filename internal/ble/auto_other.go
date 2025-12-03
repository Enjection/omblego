//go:build !darwin

package ble

import (
	"context"
	"fmt"
	"time"
)

// AutoClient is not available on non-macOS platforms
type AutoClient struct{}

func NewAutoClient(debug bool) (*AutoClient, error) {
	return nil, fmt.Errorf("auto-reconnect is only available on macOS")
}

func (c *AutoClient) WaitReady(ctx context.Context) error {
	return fmt.Errorf("auto-reconnect is only available on macOS")
}

func (c *AutoClient) ConnectByUUID(ctx context.Context, uuidStr string, timeout time.Duration) error {
	return fmt.Errorf("auto-reconnect is only available on macOS")
}

func (c *AutoClient) DiscoverOmronServices(ctx context.Context) error {
	return fmt.Errorf("auto-reconnect is only available on macOS")
}

func (c *AutoClient) EnableNotifications(shortUUID string) error {
	return fmt.Errorf("auto-reconnect is only available on macOS")
}

func (c *AutoClient) WaitNotification(shortUUID string, timeout time.Duration) ([]byte, error) {
	return nil, fmt.Errorf("auto-reconnect is only available on macOS")
}

func (c *AutoClient) WriteCharacteristic(shortUUID string, data []byte, withResponse bool) error {
	return fmt.Errorf("auto-reconnect is only available on macOS")
}

func (c *AutoClient) Disconnect() {}
