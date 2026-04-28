package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type SyncClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewSyncClient(baseURL string) *SyncClient {
	return &SyncClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

type NotifyRequest struct {
	FileID    string `json:"fileId"`
	Version   int    `json:"version"`
	ChangedBy string `json:"changedBy"`
}

func (c *SyncClient) Notify(ctx context.Context, req NotifyRequest) error {
	body, _ := json.Marshal(req)
	resp, err := c.httpClient.Post(
		c.baseURL+"/internal/notify",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sync notify: status %d", resp.StatusCode)
	}
	return nil
}
